package game

import (
	"fmt"
	"math"
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
// (FR-049–FR-054a)
func Resolve(rng *rand.Rand, state *GameState, sys *TrueSystem) {
	humanUnits := collectHumanUnits(state, sys)
	alienUnits := collectAlienUnits(state, sys)

	if len(humanUnits) == 0 || len(alienUnits) == 0 {
		return
	}

	distFromSol := 0.0
	displayName := sys.ID
	if e := state.Catalog.Get(sys.ID); e != nil {
		distFromSol = e.DistFromSol
		displayName = e.DisplayName
	}

	// Step 1: Comm Laser reports alien arrival at c BEFORE any combat. (FR-053)
	hasCommLaser := systemHasCommLaser(state.truth, sys)
	if hasCommLaser {
		state.Events.Record(&Event{
			EventYear:   state.Clock,
			ArrivalYear: state.Clock + distFromSol,
			SystemID:    sys.ID,
			Type:        EventFleetArrival,
			Description: fmt.Sprintf("Alien forces detected at %s (comm laser)", displayName),
		})
	}

	// Step 2: Reporters flee immediately before combat begins. (FR-053)
	reportersFled := extractAndSendReporters(state, sys, &humanUnits)

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
			state.Alien.TotalLost++
			alienUnits = append(alienUnits[:idx], alienUnits[idx+1:]...)
		}
		for _, idx := range uniqueIndices(toDestroyHuman) {
			humanLosses[humanUnits[idx].weaponType]++
			humanUnits = append(humanUnits[:idx], humanUnits[idx+1:]...)
		}
	}

	// Check alien exhaustion after losses.
	if !state.Alien.Exhausted && state.Alien.TotalLost >= AlienExhaustionThreshold {
		state.Alien.Exhausted = true
		state.Events.Record(&Event{
			EventYear:   state.Clock,
			ArrivalYear: state.Clock, // Sol knows immediately (global event)
			SystemID:    "sol",
			Type:        EventAlienExhausted,
			Description: "Alien forces have been exhausted by cumulative losses.",
		})
	}

	// Determine outcome.
	humanWon := len(alienUnits) == 0 && len(humanUnits) > 0
	alienWon := len(humanUnits) == 0 && len(alienUnits) > 0
	draw := len(humanUnits) == 0 && len(alienUnits) == 0

	// Apply economic combat penalty regardless of outcome. (FR-048)
	ApplyEconomicCombatPenalty(rng, state, sys)

	// Update system status and clear forces.
	if alienWon || draw {
		oldStatus := sys.Status
		sys.Status = StatusAlien
		sys.EconLevel = 0
		clearHumanForces(state, sys)
		if oldStatus == StatusHuman {
			state.Events.Record(&Event{
				EventYear:   state.Clock,
				ArrivalYear: reportArrivalYear(state.Clock, distFromSol, hasCommLaser, reportersFled),
				SystemID:    sys.ID,
				Type:        EventSystemCaptured,
				Description: fmt.Sprintf("%s captured by alien forces", displayName),
			})
		}
	}
	if humanWon {
		oldStatus := sys.Status
		sys.Status = StatusHuman
		clearAlienForces(state, sys)
		if oldStatus == StatusAlien {
			state.Events.Record(&Event{
				EventYear:   state.Clock,
				ArrivalYear: reportArrivalYear(state.Clock, distFromSol, hasCommLaser, reportersFled),
				SystemID:    sys.ID,
				Type:        EventSystemRetaken,
				Description: fmt.Sprintf("%s retaken by human forces", displayName),
			})
		}
	}

	// Write surviving unit counts back to system state.
	reconcileForces(state, sys, humanUnits, alienUnits)

	// Log internal combat event (always). (FR-052)
	canReport := hasCommLaser || reportersFled
	arrYear := reportArrivalYear(state.Clock, distFromSol, hasCommLaser, reportersFled)

	evtType := EventCombatOccurred
	internal := false
	if !canReport {
		evtType = EventCombatSilent
		arrYear = math.MaxFloat64
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
	state.Events.Record(&Event{
		EventYear:   state.Clock,
		ArrivalYear: arrYear,
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
	})
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

// extractAndSendReporters removes all Reporter units from the system's human
// fleets and creates in-transit reporter fleets toward Sol. Returns true if
// any reporters fled. (FR-053)
//
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
func extractAndSendReporters(state *GameState, sys *TrueSystem, humanUnits *[]combatUnit) bool {
	reportersFled := false
	if state.truth.System("sol") == nil {
		return false
	}
	distToSol := 0.0
	if e := state.Catalog.Get(sys.ID); e != nil {
		distToSol = e.DistFromSol
	}

	for _, fid := range sys.FleetIDs {
		fleet := state.truth.Fleets[fid]
		if fleet == nil || fleet.Owner != HumanOwner || fleet.InTransit {
			continue
		}
		reporterCount := fleet.Units[WeaponReporter]
		if reporterCount == 0 {
			continue
		}
		// Remove reporters from this fleet (they flee before combat)
		delete(fleet.Units, WeaponReporter)

		// Remove reporter combatUnits from the human side
		filtered := (*humanUnits)[:0]
		removed := 0
		for _, u := range *humanUnits {
			if u.weaponType == WeaponReporter && removed < reporterCount {
				removed++
			} else {
				filtered = append(filtered, u)
			}
		}
		*humanUnits = filtered

		// Create a reporter fleet in transit toward Sol
		reportFleetID := state.NewFleetID()
		reportFleetName := state.NewFleetName()
		travelYears := distToSol / FleetSpeedC
		reportFleet := &TrueFleet{
			ID:          reportFleetID,
			Name:        reportFleetName,
			Owner:       HumanOwner,
			Units:       map[WeaponType]int{WeaponReporter: reporterCount},
			LocationID:  "",
			DestID:      "sol",
			DepartYear:  state.Clock,
			ArrivalYear: state.Clock + travelYears,
			InTransit:   true,
		}
		state.truth.Fleets[reportFleetID] = reportFleet
		reportersFled = true
	}
	return reportersFled
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

// reportArrivalYear returns the arrival year at Sol for a combat report,
// choosing the fastest available mechanism (comm laser > reporter > unreported).
func reportArrivalYear(clock, distFromSol float64, hasCommLaser, reportersFled bool) float64 {
	if hasCommLaser {
		return clock + distFromSol
	}
	if reportersFled {
		return clock + distFromSol/FleetSpeedC
	}
	return math.MaxFloat64
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
