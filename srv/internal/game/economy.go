package game

import (
	"fmt"
	"math/rand"
)

// AccumulateWealth adds wealth to each human-held system proportional to
// deltaYears at that system's econ rate. (FR-046)
func AccumulateWealth(state *GameState, deltaYears float64) {
	for _, sys := range state.truth.Systems {
		if sys.Status == StatusHuman && sys.EconLevel >= 0 && sys.EconLevel <= 5 {
			sys.Wealth += EconWealthRate[sys.EconLevel] * deltaYears
		}
	}
}

// AdvanceEconLevels checks and applies economic level growth for each system.
// Called on every engine tick. (FR-048)
func AdvanceEconLevels(state *GameState) {
	for id, sys := range state.truth.Systems {
		if sys.Status != StatusHuman {
			continue
		}
		if sys.EconLevel < 5 && state.Clock >= sys.EconGrowthYear {
			sys.EconLevel++
			sys.EconGrowthYear = state.Clock + EconGrowthIntervalYears

			hasCommLaser := systemHasCommLaser(state.truth, sys)
			if hasCommLaser {
				distFromSol := 0.0
				displayName := id
				if e := state.Catalog.Get(id); e != nil {
					distFromSol = e.DistFromSol
					displayName = e.DisplayName
				}
				arrYear := arrivalYearFor(state.Clock, distFromSol, true)
				state.Events.Record(&Event{
					EventYear:   state.Clock,
					ArrivalYear: arrYear,
					SystemID:    id,
					Type:        EventEconGrowth,
					Description: fmt.Sprintf("%s economy grew to level %d", displayName, sys.EconLevel),
					Details:     &EconGrowthDetails{NewLevel: sys.EconLevel},
				})
			}
		}
	}
}

// ApplyEconomicCombatPenalty reduces econ level by 1, destroys a random
// fraction of wealth, and resets the growth clock. (FR-048)
func ApplyEconomicCombatPenalty(rng *rand.Rand, state *GameState, sys *TrueSystem) {
	if sys.EconLevel > 0 {
		sys.EconLevel--
	}
	// Destroy 0–WealthPenaltyMaxFraction of accumulated wealth
	sys.Wealth *= 1.0 - rng.Float64()*WealthPenaltyMaxFraction
	sys.EconGrowthYear = state.Clock + EconGrowthIntervalYears
}

// ValidateConstruct checks whether a construction command can execute.
// Returns nil if valid, error describing the rejection reason if not. (FR-047)
func ValidateConstruct(sys *TrueSystem, wt WeaponType, qty int) error {
	def, ok := WeaponDefs[wt]
	if !ok {
		return fmt.Errorf("unknown weapon type %q", wt)
	}
	if qty <= 0 {
		return fmt.Errorf("quantity must be positive, got %d", qty)
	}
	if sys.Status != StatusHuman {
		return fmt.Errorf("system %q is not human-held", sys.ID)
	}
	totalCost := def.Cost * float64(qty)
	if sys.Wealth < totalCost {
		return fmt.Errorf("insufficient wealth: need %.1f, have %.1f", totalCost, sys.Wealth)
	}
	return nil
}

// ExecuteConstruct applies an approved construction order to the system.
// Panics if the weapon type is invalid (programming error). (FR-036)
//
// For mobile weapons, ConstructionDetails.FleetID and .FleetName are
// populated with the truth-side fleet that received the units (whether the
// existing primary or freshly minted) so the propagator can mirror the
// decision into SolView (DR-5).
func ExecuteConstruct(state *GameState, sys *TrueSystem, wt WeaponType, qty int) {
	def := WeaponDefs[wt] // panics on invalid type by design
	sys.Wealth -= def.Cost * float64(qty)

	displayName := sys.ID
	distFromSol := 0.0
	if e := state.Catalog.Get(sys.ID); e != nil {
		displayName = e.DisplayName
		distFromSol = e.DistFromSol
	}

	var receivedByFleetID, receivedByFleetName string

	if def.CanMove {
		// Add newly built mobile units to the system's primary (1st) fleet.
		// If the primary fleet has been sent away, create a new one.
		primary := state.truth.Fleets[sys.PrimaryFleetID]
		if primary != nil && !primary.InTransit && primary.LocationID == sys.ID {
			primary.Units[wt] += qty
			receivedByFleetID = primary.ID
			receivedByFleetName = primary.Name
		} else {
			fleetID := state.NewFleetID()
			fleetName := displayName + "-1st Fleet"
			tf := &TrueFleet{
				ID:         fleetID,
				Name:       fleetName,
				Owner:      HumanOwner,
				Units:      map[WeaponType]int{wt: qty},
				LocationID: sys.ID,
				InTransit:  false,
			}
			state.truth.Fleets[fleetID] = tf
			sys.FleetIDs = append(sys.FleetIDs, fleetID)
			sys.PrimaryFleetID = fleetID
			receivedByFleetID = fleetID
			receivedByFleetName = fleetName
		}
	} else {
		sys.LocalUnits[wt] += qty
	}

	// Log construction complete event; reportable only if system has a comm laser.
	hasCommLaser := systemHasCommLaser(state.truth, sys)
	arrYear := arrivalYearFor(state.Clock, distFromSol, hasCommLaser)
	state.Events.Record(&Event{
		EventYear:   state.Clock,
		ArrivalYear: arrYear,
		SystemID:    sys.ID,
		Type:        EventConstructionDone,
		Description: fmt.Sprintf("Constructed %d %s at %s", qty, wt, displayName),
		Details: &ConstructionDetails{
			WeaponType: wt,
			Quantity:   qty,
			FleetID:    receivedByFleetID,
			FleetName:  receivedByFleetName,
		},
	})
}

// ordinal returns the English ordinal string for n (1→"1st", 2→"2nd", etc.).
func ordinal(n int) string {
	if n >= 11 && n <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}

// NextFleetName returns the name that would be given to the next fleet
// created at sys.
func NextFleetName(state *GameState, sys *TrueSystem) string {
	displayName := sys.ID
	if e := state.Catalog.Get(sys.ID); e != nil {
		displayName = e.DisplayName
	}
	return fmt.Sprintf("%s-%s Fleet", displayName, ordinal(sys.FleetCount+1))
}

// ExecuteCreateFleet creates a new empty named fleet at sys.
func ExecuteCreateFleet(state *GameState, sys *TrueSystem) error {
	if sys.Status != StatusHuman {
		return fmt.Errorf("system %q is not human-held", sys.ID)
	}
	displayName := sys.ID
	if e := state.Catalog.Get(sys.ID); e != nil {
		displayName = e.DisplayName
	}
	sys.FleetCount++
	fleetID := state.NewFleetID()
	tf := &TrueFleet{
		ID:         fleetID,
		Name:       fmt.Sprintf("%s-%s Fleet", displayName, ordinal(sys.FleetCount)),
		Owner:      HumanOwner,
		Units:      map[WeaponType]int{},
		LocationID: sys.ID,
		InTransit:  false,
	}
	state.truth.Fleets[fleetID] = tf
	sys.FleetIDs = append(sys.FleetIDs, fleetID)
	return nil
}

// ExecuteReassign moves units from SourceFleetID to TargetFleetID at sys.
// Both fleets must be stationed (not in transit) at sys.
// If the source fleet becomes empty after the transfer it is dissolved.
func ExecuteReassign(state *GameState, sys *TrueSystem, cmd *PendingCommand) error {
	src := state.truth.Fleets[cmd.SourceFleetID]
	if src == nil {
		return fmt.Errorf("source fleet %q not found", cmd.SourceFleetID)
	}
	if src.InTransit || src.LocationID != sys.ID {
		return fmt.Errorf("source fleet %q is not stationed at %q", cmd.SourceFleetID, sys.ID)
	}
	dst := state.truth.Fleets[cmd.TargetFleetID]
	if dst == nil {
		return fmt.Errorf("target fleet %q not found", cmd.TargetFleetID)
	}
	if dst.InTransit || dst.LocationID != sys.ID {
		return fmt.Errorf("target fleet %q is not stationed at %q", cmd.TargetFleetID, sys.ID)
	}
	for wt, n := range cmd.ReassignUnits {
		if src.Units[wt] < n {
			return fmt.Errorf("source fleet has %d %s, need %d", src.Units[wt], wt, n)
		}
	}
	for wt, n := range cmd.ReassignUnits {
		src.Units[wt] -= n
		dst.Units[wt] += n
	}
	// Dissolve source fleet if now empty
	total := 0
	for _, n := range src.Units {
		total += n
	}
	if total == 0 {
		sys.FleetIDs = removeString(sys.FleetIDs, src.ID)
		if sys.PrimaryFleetID == src.ID {
			sys.PrimaryFleetID = ""
		}
		delete(state.truth.Fleets, src.ID)
	}
	return nil
}

// ProjectedWealth returns the estimated accumulated wealth at futureYear,
// given current wealth and the system's econ level. (A-4)
func ProjectedWealth(state *GameState, sys *TrueSystem, futureYear float64) float64 {
	deltaYears := futureYear - state.Clock
	if deltaYears < 0 {
		deltaYears = 0
	}
	level := sys.EconLevel
	if level < 0 {
		level = 0
	}
	if level > 5 {
		level = 5
	}
	return sys.Wealth + EconWealthRate[level]*deltaYears
}
