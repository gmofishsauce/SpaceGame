package game

import (
	"fmt"
	"math"
	"math/rand"
)

// AccumulateWealth adds wealth to each human-held system proportional to
// deltaYears at that system's econ rate. (FR-046)
func AccumulateWealth(state *GameState, deltaYears float64) {
	for _, sys := range state.Systems {
		if sys.Status == StatusHuman && sys.EconLevel >= 0 && sys.EconLevel <= 4 {
			sys.Wealth += EconWealthRate[sys.EconLevel] * deltaYears
		}
	}
}

// AdvanceEconLevels checks and applies economic level growth for each system.
// Called on every engine tick. (FR-048)
func AdvanceEconLevels(state *GameState) {
	for _, sys := range state.Systems {
		if sys.Status != StatusHuman {
			continue
		}
		if sys.EconLevel < 4 && state.Clock >= sys.EconGrowthYear {
			sys.EconLevel++
			sys.EconGrowthYear = state.Clock + EconGrowthIntervalYears

			hasCommLaser := sys.LocalUnits[WeaponCommLaser] > 0
			if hasCommLaser {
				arrYear := arrivalYearFor(state.Clock, sys.DistFromSol, true)
				state.RecordEvent(&GameEvent{
					EventYear:   state.Clock,
					ArrivalYear: arrYear,
					SystemID:    sys.ID,
					Type:        EventEconGrowth,
					Description: fmt.Sprintf("%s economy grew to level %d", sys.DisplayName, sys.EconLevel),
					CanReport:   true,
					Details:     &EconGrowthDetails{NewLevel: sys.EconLevel},
				})
			}
		}
	}
}

// ApplyEconomicCombatPenalty reduces econ level by 1, destroys a random
// fraction of wealth, and resets the growth clock. (FR-048)
func ApplyEconomicCombatPenalty(rng *rand.Rand, state *GameState, sys *StarSystem) {
	if sys.EconLevel > 0 {
		sys.EconLevel--
	}
	// Destroy 0–WealthPenaltyMaxFraction of accumulated wealth
	sys.Wealth *= 1.0 - rng.Float64()*WealthPenaltyMaxFraction
	sys.EconGrowthYear = state.Clock + EconGrowthIntervalYears
}

// ValidateConstruct checks whether a construction command can execute.
// Returns nil if valid, error describing the rejection reason if not. (FR-047)
func ValidateConstruct(sys *StarSystem, wt WeaponType, qty int) error {
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
	if sys.EconLevel < def.MinLevel {
		return fmt.Errorf("economic level %d required, system has %d", def.MinLevel, sys.EconLevel)
	}
	totalCost := def.Cost * float64(qty)
	if sys.Wealth < totalCost {
		return fmt.Errorf("insufficient wealth: need %.1f, have %.1f", totalCost, sys.Wealth)
	}
	return nil
}

// ExecuteConstruct applies an approved construction order to the system.
// Panics if the weapon type is invalid (programming error). (FR-036)
func ExecuteConstruct(state *GameState, sys *StarSystem, wt WeaponType, qty int) {
	def := WeaponDefs[wt] // panics on invalid type by design
	sys.Wealth -= def.Cost * float64(qty)

	if def.CanMove {
		// Add newly built mobile units to the system's primary (1st) fleet.
		// If the primary fleet has been sent away, create a new one.
		primary, ok := state.Fleets[sys.PrimaryFleetID]
		if ok && !primary.InTransit && primary.LocationID == sys.ID {
			primary.Units[wt] += qty
		} else {
			fleetID := state.NewFleetID()
			fleet := &Fleet{
				ID:         fleetID,
				Name:       sys.DisplayName + "-1st Fleet",
				Owner:      HumanOwner,
				Units:      map[WeaponType]int{wt: qty},
				LocationID: sys.ID,
				InTransit:  false,
			}
			state.Fleets[fleetID] = fleet
			sys.FleetIDs = append(sys.FleetIDs, fleetID)
			sys.PrimaryFleetID = fleetID
		}
	} else {
		sys.LocalUnits[wt] += qty
	}

	// Log construction complete event; reportable only if system has a comm laser
	hasCommLaser := sys.LocalUnits[WeaponCommLaser] > 0
	arrYear := math.MaxFloat64
	if hasCommLaser {
		arrYear = state.Clock + sys.DistFromSol
	}
	state.RecordEvent(&GameEvent{
		EventYear:   state.Clock,
		ArrivalYear: arrYear,
		SystemID:    sys.ID,
		Type:        EventConstructionDone,
		Description: fmt.Sprintf("Constructed %d %s at %s", qty, wt, sys.DisplayName),
		CanReport:   hasCommLaser,
		Details:     &ConstructionDetails{WeaponType: wt, Quantity: qty},
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

// ExecuteCreateFleet creates a new empty named fleet at sys.
func ExecuteCreateFleet(state *GameState, sys *StarSystem) error {
	if sys.Status != StatusHuman {
		return fmt.Errorf("system %q is not human-held", sys.ID)
	}
	sys.FleetCount++
	fleetID := state.NewFleetID()
	fleet := &Fleet{
		ID:         fleetID,
		Name:       fmt.Sprintf("%s-%s Fleet", sys.DisplayName, ordinal(sys.FleetCount)),
		Owner:      HumanOwner,
		Units:      map[WeaponType]int{},
		LocationID: sys.ID,
		InTransit:  false,
	}
	state.Fleets[fleetID] = fleet
	sys.FleetIDs = append(sys.FleetIDs, fleetID)
	return nil
}

// ExecuteReassign moves units from SourceFleetID to TargetFleetID at sys.
// Both fleets must be stationed (not in transit) at sys.
// If the source fleet becomes empty after the transfer it is dissolved.
func ExecuteReassign(state *GameState, sys *StarSystem, cmd *PendingCommand) error {
	src, ok := state.Fleets[cmd.SourceFleetID]
	if !ok {
		return fmt.Errorf("source fleet %q not found", cmd.SourceFleetID)
	}
	if src.InTransit || src.LocationID != sys.ID {
		return fmt.Errorf("source fleet %q is not stationed at %q", cmd.SourceFleetID, sys.ID)
	}
	dst, ok := state.Fleets[cmd.TargetFleetID]
	if !ok {
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
		delete(state.Fleets, src.ID)
	}
	return nil
}

// ProjectedWealth returns the estimated accumulated wealth at futureYear,
// given current wealth and the system's econ level. (A-4)
func ProjectedWealth(state *GameState, sys *StarSystem, futureYear float64) float64 {
	deltaYears := futureYear - state.Clock
	if deltaYears < 0 {
		deltaYears = 0
	}
	level := sys.EconLevel
	if level < 0 {
		level = 0
	}
	if level > 4 {
		level = 4
	}
	return sys.Wealth + EconWealthRate[level]*deltaYears
}
