package game

import (
	"fmt"
	"math/rand"
	"sort"
)

// combatUnit is a single unit participating in combat.
type combatUnit struct {
	weaponType WeaponType
	owner      Owner
}

// Resolve resolves all combat in the given system for the current tick.
// It mutates ground-truth forces, logs events, and updates system status.
// (FR-049–FR-054a, FR-20, FR-21, AS-4)
func Resolve(rng *rand.Rand, state *GameState, sys *TrueSystem) {
	humanUnits := collectHumanUnits(state, sys)
	alienUnits := collectAlienUnits(state, sys)

	if len(humanUnits) == 0 || len(alienUnits) == 0 {
		return
	}

	displayName := sys.ID
	if e := state.Catalog.Get(sys.ID); e != nil {
		displayName = e.DisplayName
	}

	// Capture the pre-combat system owner. All reports triggered by this
	// combat route to that side's home, per FR-20/FR-21/AS-4. May be ""
	// (uninhabited/contested), which RecordEvent treats as unreportable.
	precombatStatus := sys.Status
	reportTo := statusToOwner(precombatStatus)

	// Step 1: Comm Laser reports alien arrival at c BEFORE any combat. (FR-053)
	hasCommLaser := systemHasCommLaser(state.truth, sys)
	if hasCommLaser {
		state.RecordEvent(&Event{
			EventYear:   state.Clock,
			SystemID:    sys.ID,
			Type:        EventFleetArrival,
			Description: fmt.Sprintf("Hostile forces detected at %s (comm laser)", displayName),
		}, reportTo, true, false)
	}

	// Step 2: Reporters flee immediately before combat begins. Each side's
	// reporter fleets head to that side's home (FR-20). The returned bool
	// reports whether the precombat-owner side's reporters fled; that
	// drives event-routing for the captured/retaken/combat events below.
	reportersFled := extractAndSendReporters(state, sys, &humanUnits, &alienUnits, reportTo)

	// Step 3: Round-based parallel combat. (FR-054a)
	humanLosses := map[WeaponType]int{}
	alienLosses := map[WeaponType]int{}

	for round := 0; round < MaxCombatRounds && len(humanUnits) > 0 && len(alienUnits) > 0; round++ {
		var toDestroyHuman []int
		var toDestroyAlien []int

		// All units fire simultaneously; collect casualty indices.
		for _, attacker := range humanUnits {
			if WeaponDefs[attacker.weaponType].AttackPower == 0 {
				continue
			}
			targetIdx := rng.Intn(len(alienUnits))
			target := alienUnits[targetIdx]
			if rng.Float64() < hitProbability(attacker.weaponType, target.weaponType) {
				toDestroyAlien = append(toDestroyAlien, targetIdx)
			}
		}
		for _, attacker := range alienUnits {
			if WeaponDefs[attacker.weaponType].AttackPower == 0 {
				continue
			}
			targetIdx := rng.Intn(len(humanUnits))
			target := humanUnits[targetIdx]
			if rng.Float64() < hitProbability(attacker.weaponType, target.weaponType) {
				toDestroyHuman = append(toDestroyHuman, targetIdx)
			}
		}

		// Remove casualties (end of round — parallel resolution).
		for _, idx := range uniqueIndices(toDestroyAlien) {
			alienLosses[alienUnits[idx].weaponType]++
			alienUnits = append(alienUnits[:idx], alienUnits[idx+1:]...)
		}
		for _, idx := range uniqueIndices(toDestroyHuman) {
			humanLosses[humanUnits[idx].weaponType]++
			humanUnits = append(humanUnits[:idx], humanUnits[idx+1:]...)
		}
	}

	// Determine outcome.
	humanWon := len(alienUnits) == 0 && len(humanUnits) > 0
	alienWon := len(humanUnits) == 0 && len(alienUnits) > 0
	draw := len(humanUnits) == 0 && len(alienUnits) == 0

	// Apply economic combat penalty regardless of outcome. (FR-048)
	ApplyEconomicCombatPenalty(rng, state, sys)

	// Update system status and clear forces.
	if alienWon || draw {
		sys.Status = StatusAlien
		sys.EconLevel = 0
		clearHumanForces(state, sys)
		if precombatStatus == StatusHuman {
			state.RecordEvent(&Event{
				EventYear:   state.Clock,
				SystemID:    sys.ID,
				Type:        EventSystemCaptured,
				Description: fmt.Sprintf("%s captured by alien forces", displayName),
			}, reportTo, hasCommLaser, reportersFled)
		}
	}
	if humanWon {
		sys.Status = StatusHuman
		clearAlienForces(state, sys)
		if precombatStatus == StatusAlien {
			state.RecordEvent(&Event{
				EventYear:   state.Clock,
				SystemID:    sys.ID,
				Type:        EventSystemRetaken,
				Description: fmt.Sprintf("%s retaken by human forces", displayName),
			}, reportTo, hasCommLaser, reportersFled)
		}
	}

	// Write surviving unit counts back to system state.
	reconcileForces(state, sys, humanUnits, alienUnits)

	// Log combat event. Reportable iff a comm laser fired or reporters fled.
	canReport := hasCommLaser || reportersFled
	evtType := EventCombatOccurred
	internal := false
	if !canReport {
		evtType = EventCombatSilent
		internal = true
	}

	desc := summarizeCombat(humanWon, alienWon, draw, humanLosses, alienLosses)

	// DEFERRED: reporter survival is not coupled to report delivery.
	//
	// Today, when combat occurs at a system without a comm laser, we spawn
	// reporter fleets toward Sol AND record the combat Event with an
	// ArrivalYear keyed to the reporter's ETA at 0.8c. The two are
	// independent: the Event is delivered to SolView when its ArrivalYear
	// matures, regardless of whether the reporter fleet still exists.
	//
	// This is currently safe ONLY because reporters cannot be destroyed in
	// transit (no in-transit combat exists in the game). If that ever
	// changes -- in-transit interception, alien patrols, escorts that
	// engage passing fleets, or any other mechanism by which a fleet may
	// die en route -- this becomes a real bug: the player will receive
	// reports that should have been lost with their carrier.
	//
	// Fix when needed (review item E in architecturalreview.md):
	//   - give TrueFleet a CarriedEvents []*Event field
	//   - have combat attach the report to the spawned reporter fleet
	//     instead of calling state.Events.Record() directly here
	//   - in processFleetArrivals, Record() each carried event when the
	//     fleet lands; on fleet destruction, drop them
	// The propagation pipeline introduced by the dual-state refactor
	// already supports this without further restructuring.
	state.RecordEvent(&Event{
		EventYear:   state.Clock,
		SystemID:    sys.ID,
		Type:        evtType,
		Description: desc,
		Internal:    internal,
		Details: &CombatDetails{
			HumanLosses: humanLosses,
			AlienLosses: alienLosses,
			HumanWon:    humanWon,
			AlienWon:    alienWon,
			Draw:        draw,
		},
	}, reportTo, hasCommLaser, reportersFled)
}

// hitProbability returns the probability that an attacker of attackerType
// destroys a unit of targetType. (FR-050)
func hitProbability(attackerType, targetType WeaponType) float64 {
	attackPower := WeaponDefs[attackerType].AttackPower
	vulnerability := WeaponDefs[targetType].Vulnerability
	if attackPower == 0 {
		return 0.0
	}
	p := float64(attackPower) / float64(attackPower+vulnerability)
	if p < 0.05 {
		return 0.05
	}
	if p > 0.95 {
		return 0.95
	}
	return p
}

// collectHumanUnits flattens all human forces in a system into a unit slice.
func collectHumanUnits(state *GameState, sys *TrueSystem) []combatUnit {
	var units []combatUnit
	for wt, count := range sys.LocalUnits {
		for i := 0; i < count; i++ {
			units = append(units, combatUnit{weaponType: wt, owner: HumanOwner})
		}
	}
	for _, fid := range sys.FleetIDs {
		fleet := state.truth.Fleets[fid]
		if fleet == nil || fleet.Owner != HumanOwner || fleet.InTransit {
			continue
		}
		for wt, count := range fleet.Units {
			for i := 0; i < count; i++ {
				units = append(units, combatUnit{weaponType: wt, owner: HumanOwner})
			}
		}
	}
	return units
}

// collectAlienUnits flattens all alien forces in a system into a unit slice.
func collectAlienUnits(state *GameState, sys *TrueSystem) []combatUnit {
	var units []combatUnit
	for _, fid := range sys.FleetIDs {
		fleet := state.truth.Fleets[fid]
		if fleet == nil || fleet.Owner != AlienOwner || fleet.InTransit {
			continue
		}
		for wt, count := range fleet.Units {
			for i := 0; i < count; i++ {
				units = append(units, combatUnit{weaponType: wt, owner: AlienOwner})
			}
		}
	}
	return units
}

// extractAndSendReporters removes all Reporter units from all fleets at the
// system (both sides) and creates in-transit reporter fleets toward each
// side's home. Returns true if the reportTo side had any reporters flee.
// (FR-020, FR-021, FR-053)
func extractAndSendReporters(state *GameState, sys *TrueSystem, humanUnits *[]combatUnit, alienUnits *[]combatUnit, reportTo Owner) bool {
	unitSlices := map[Owner]*[]combatUnit{
		HumanOwner: humanUnits,
		AlienOwner: alienUnits,
	}
	reportersFledBySide := map[Owner]bool{}

	for _, fid := range sys.FleetIDs {
		fleet := state.truth.Fleets[fid]
		if fleet == nil || fleet.InTransit {
			continue
		}
		reporterCount := fleet.Units[WeaponReporter]
		if reporterCount == 0 {
			continue
		}

		homeID := state.Homes[fleet.Owner]
		if homeID == "" {
			continue
		}
		dist := state.Catalog.Distance(sys.ID, homeID)

		// Remove reporters from this fleet (they flee before combat).
		delete(fleet.Units, WeaponReporter)

		// Remove reporter combatUnits from this side's slice.
		units := unitSlices[fleet.Owner]
		filtered := (*units)[:0]
		removed := 0
		for _, u := range *units {
			if u.weaponType == WeaponReporter && removed < reporterCount {
				removed++
			} else {
				filtered = append(filtered, u)
			}
		}
		*units = filtered

		// Create a reporter fleet in transit toward this side's home.
		travelYears := dist / FleetSpeedC
		reportFleetID := state.NewFleetID()
		reportFleetName := state.NewFleetName()
		state.truth.Fleets[reportFleetID] = &TrueFleet{
			ID:          reportFleetID,
			Name:        reportFleetName,
			Owner:       fleet.Owner,
			Units:       map[WeaponType]int{WeaponReporter: reporterCount},
			LocationID:  "",
			DestID:      homeID,
			DepartYear:  state.Clock,
			ArrivalYear: state.Clock + travelYears,
			InTransit:   true,
		}
		reportersFledBySide[fleet.Owner] = true
	}

	return reportersFledBySide[reportTo]
}

// clearHumanForces removes all human units and fleets from a system.
func clearHumanForces(state *GameState, sys *TrueSystem) {
	for wt := range sys.LocalUnits {
		sys.LocalUnits[wt] = 0
	}
	for _, fid := range sys.FleetIDs {
		fleet := state.truth.Fleets[fid]
		if fleet != nil && fleet.Owner == HumanOwner {
			delete(state.truth.Fleets, fid)
		}
	}
	// Rebuild FleetIDs keeping only alien fleets
	var remaining []string
	for _, fid := range sys.FleetIDs {
		if f := state.truth.Fleets[fid]; f != nil && f.Owner == AlienOwner {
			remaining = append(remaining, fid)
		}
	}
	sys.FleetIDs = remaining
}

// clearAlienForces removes all alien fleets from a system.
func clearAlienForces(state *GameState, sys *TrueSystem) {
	for _, fid := range sys.FleetIDs {
		fleet := state.truth.Fleets[fid]
		if fleet != nil && fleet.Owner == AlienOwner {
			delete(state.truth.Fleets, fid)
		}
	}
	var remaining []string
	for _, fid := range sys.FleetIDs {
		if f := state.truth.Fleets[fid]; f != nil && f.Owner == HumanOwner {
			remaining = append(remaining, fid)
		}
	}
	sys.FleetIDs = remaining
}

// reconcileForces writes surviving unit counts back to the system's authoritative state.
func reconcileForces(state *GameState, sys *TrueSystem, humanUnits, alienUnits []combatUnit) {
	// Rebuild local units for human side
	newLocal := map[WeaponType]int{}
	for _, u := range humanUnits {
		if !WeaponDefs[u.weaponType].CanMove {
			newLocal[u.weaponType]++
		}
	}
	sys.LocalUnits = newLocal

	// Rebuild fleet units for human side (survivors go back into their fleets).
	// Simple approach: consolidate all surviving mobile human units into the
	// first human fleet still present (or create one).
	survivingMobileHuman := map[WeaponType]int{}
	for _, u := range humanUnits {
		if WeaponDefs[u.weaponType].CanMove {
			survivingMobileHuman[u.weaponType]++
		}
	}
	if len(survivingMobileHuman) > 0 {
		var humanFleetID string
		for _, fid := range sys.FleetIDs {
			if f := state.truth.Fleets[fid]; f != nil && f.Owner == HumanOwner {
				humanFleetID = fid
				break
			}
		}
		if humanFleetID == "" {
			fid := state.NewFleetID()
			fname := state.NewFleetName()
			state.truth.Fleets[fid] = &TrueFleet{
				ID: fid, Name: fname, Owner: HumanOwner,
				Units: survivingMobileHuman, LocationID: sys.ID,
			}
			sys.FleetIDs = append(sys.FleetIDs, fid)
		} else {
			state.truth.Fleets[humanFleetID].Units = survivingMobileHuman
		}
	}

	// Rebuild alien fleet units similarly
	survivingAlien := map[WeaponType]int{}
	for _, u := range alienUnits {
		survivingAlien[u.weaponType]++
	}
	if len(survivingAlien) > 0 {
		var alienFleetID string
		for _, fid := range sys.FleetIDs {
			if f := state.truth.Fleets[fid]; f != nil && f.Owner == AlienOwner {
				alienFleetID = fid
				break
			}
		}
		if alienFleetID == "" {
			fid := state.NewFleetID()
			fname := state.NewFleetName()
			state.truth.Fleets[fid] = &TrueFleet{
				ID: fid, Name: fname, Owner: AlienOwner,
				Units: survivingAlien, LocationID: sys.ID,
			}
			sys.FleetIDs = append(sys.FleetIDs, fid)
		} else {
			state.truth.Fleets[alienFleetID].Units = survivingAlien
		}
	}
}

// summarizeCombat generates a human-readable combat outcome description.
func summarizeCombat(humanWon, alienWon, _ bool, humanLosses, alienLosses map[WeaponType]int) string {
	totalH := 0
	for _, n := range humanLosses {
		totalH += n
	}
	totalA := 0
	for _, n := range alienLosses {
		totalA += n
	}
	switch {
	case humanWon:
		return fmt.Sprintf("Human forces victorious. %d alien units and %d human units lost.", totalA, totalH)
	case alienWon:
		return fmt.Sprintf("Alien forces victorious. %d human units and %d alien units lost.", totalH, totalA)
	default:
		return fmt.Sprintf("Mutual destruction. %d human units and %d alien units lost.", totalH, totalA)
	}
}

// uniqueIndices returns a sorted, deduplicated copy of indices.
func uniqueIndices(indices []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, i := range indices {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	// Sort descending so removal from slice works back-to-front
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out
}
