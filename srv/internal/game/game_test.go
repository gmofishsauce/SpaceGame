package game

import (
	"math"
	"math/rand"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers: minimal state factories
// ---------------------------------------------------------------------------

// newMinimalState returns a GameState with Sol and one remote system. It is
// NOT a fully-initialised game (no bot, no engine) — just enough structure
// for unit tests.
func newMinimalState() *GameState {
	sol := &StarSystem{
		ID:              "sol",
		DisplayName:     "Sol",
		X:               0, Y: 0, Z: 0,
		DistFromSol:     0,
		Status:          StatusHuman,
		EconLevel:       5,
		Wealth:          1000,
		EconGrowthYear:  EconGrowthIntervalYears,
		LocalUnits:      map[WeaponType]int{},
		KnownStatus:     StatusHuman,
		KnownLocalUnits: map[WeaponType]int{},
		KnownFleetIDs:   []string{},
	}
	remote := &StarSystem{
		ID:              "alpha-centauri",
		DisplayName:     "Alpha Centauri",
		X:               4.37, Y: 0, Z: 0,
		DistFromSol:     4.37,
		Status:          StatusHuman,
		EconLevel:       3,
		Wealth:          50,
		EconGrowthYear:  EconGrowthIntervalYears,
		LocalUnits:      map[WeaponType]int{},
		KnownStatus:     StatusHuman,
		KnownLocalUnits: map[WeaponType]int{},
		KnownFleetIDs:   []string{},
	}

	st := &GameState{
		Systems:     map[string]*StarSystem{"sol": sol, "alpha-centauri": remote},
		SystemOrder: []string{"sol", "alpha-centauri"},
		Fleets:      map[string]*Fleet{},
		Events:      []*GameEvent{},
		PendingCmds: []*PendingCommand{},
	}
	st.Human.InitialSystemIDs = []string{"sol", "alpha-centauri"}
	return st
}

// addFleet adds a fleet to the state and to the given system's FleetIDs.
func addFleet(st *GameState, sys *StarSystem, owner Owner, units map[WeaponType]int) *Fleet {
	fid := st.NewFleetID()
	fname := st.NewFleetName()
	f := &Fleet{
		ID:         fid,
		Name:       fname,
		Owner:      owner,
		Units:      units,
		LocationID: sys.ID,
		InTransit:  false,
	}
	st.Fleets[fid] = f
	sys.FleetIDs = append(sys.FleetIDs, fid)
	return f
}

// addInTransitFleet creates a fleet in transit to destID, arriving at arrivalYear.
func addInTransitFleet(st *GameState, owner Owner, units map[WeaponType]int, destID string, arrivalYear float64) *Fleet {
	fid := st.NewFleetID()
	fname := st.NewFleetName()
	f := &Fleet{
		ID:          fid,
		Name:        fname,
		Owner:       owner,
		Units:       units,
		InTransit:   true,
		DestID:      destID,
		ArrivalYear: arrivalYear,
	}
	st.Fleets[fid] = f
	return f
}

// newUninhabitedSystem returns a minimal uninhabited StarSystem not yet added to any state.
func newUninhabitedSystem(id string, distFromSol float64) *StarSystem {
	return &StarSystem{
		ID:              id,
		DisplayName:     id,
		DistFromSol:     distFromSol,
		Status:          StatusUninhabited,
		LocalUnits:      map[WeaponType]int{},
		KnownLocalUnits: map[WeaponType]int{},
		KnownFleetIDs:   []string{},
	}
}

// ---------------------------------------------------------------------------
// TestCheckVictory
// ---------------------------------------------------------------------------

func TestCheckVictory_NoConditionMet(t *testing.T) {
	st := newMinimalState()
	over, _, _ := st.CheckVictory()
	if over {
		t.Fatal("expected game not over at start, got over=true")
	}
}

func TestCheckVictory_AlienCapturesEarth(t *testing.T) {
	st := newMinimalState()
	st.Systems["sol"].Status = StatusAlien

	over, winner, reason := st.CheckVictory()
	if !over {
		t.Fatal("expected game over when Sol is alien-held")
	}
	if winner != AlienOwner {
		t.Errorf("expected alien winner, got %q", winner)
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestCheckVictory_AlienCapturesFraction(t *testing.T) {
	// 5 initial human systems; 2 alien-held = 40% which equals AlienWinCaptureFraction.
	st := newMinimalState()
	for _, id := range []string{"sys-b", "sys-c", "sys-d"} {
		sys := &StarSystem{
			ID:              id,
			DisplayName:     id,
			Status:          StatusHuman,
			LocalUnits:      map[WeaponType]int{},
			KnownLocalUnits: map[WeaponType]int{},
			KnownFleetIDs:   []string{},
		}
		st.Systems[id] = sys
		st.SystemOrder = append(st.SystemOrder, id)
		st.Human.InitialSystemIDs = append(st.Human.InitialSystemIDs, id)
	}
	st.Systems["sys-b"].Status = StatusAlien
	st.Systems["sys-c"].Status = StatusAlien // 2/5 = 0.40

	over, winner, _ := st.CheckVictory()
	if !over {
		t.Fatal("expected alien win at 40% capture, got over=false")
	}
	if winner != AlienOwner {
		t.Errorf("expected alien winner, got %q", winner)
	}
}

func TestCheckVictory_AlienJustBelowFraction(t *testing.T) {
	// 5 initial human systems; 1 alien-held = 20% < 40%; Sol still human.
	st := newMinimalState()
	for _, id := range []string{"sys-b", "sys-c", "sys-d"} {
		sys := &StarSystem{
			ID:              id,
			Status:          StatusHuman,
			LocalUnits:      map[WeaponType]int{},
			KnownLocalUnits: map[WeaponType]int{},
			KnownFleetIDs:   []string{},
		}
		st.Systems[id] = sys
		st.SystemOrder = append(st.SystemOrder, id)
		st.Human.InitialSystemIDs = append(st.Human.InitialSystemIDs, id)
	}
	st.Systems["sys-b"].Status = StatusAlien // 1/5 = 20%

	over, _, _ := st.CheckVictory()
	if over {
		t.Fatal("expected game not over at 20% alien capture, got over=true")
	}
}

func TestCheckVictory_HumanWin(t *testing.T) {
	st := newMinimalState()
	st.Alien.Exhausted = true
	// Both initial systems still human-held → retainedFrac = 1.0 ≥ 0.60.

	over, winner, _ := st.CheckVictory()
	if !over {
		t.Fatal("expected human win when alien exhausted and all systems retained")
	}
	if winner != HumanOwner {
		t.Errorf("expected human winner, got %q", winner)
	}
}

func TestCheckVictory_HumanExhaustedButTooManySystemsLost(t *testing.T) {
	// 3 initial systems; 2 lost = 33% retained < 60% threshold.
	// Alien exhausted but retention fraction not met → no human win.
	// However note: 2/3 ≈ 67% captured ≥ 40% → alien win via fraction.
	// We just verify the human win condition is NOT triggered.
	st := newMinimalState()
	// Add a third initial system so we can lose 2 of 3.
	extra := &StarSystem{
		ID:              "barnard",
		Status:          StatusAlien,
		LocalUnits:      map[WeaponType]int{},
		KnownLocalUnits: map[WeaponType]int{},
		KnownFleetIDs:   []string{},
	}
	st.Systems["barnard"] = extra
	st.SystemOrder = append(st.SystemOrder, "barnard")
	st.Human.InitialSystemIDs = []string{"sol", "alpha-centauri", "barnard"}
	st.Systems["alpha-centauri"].Status = StatusAlien

	st.Alien.Exhausted = true

	over, winner, _ := st.CheckVictory()
	if over && winner == HumanOwner {
		t.Fatal("expected human NOT to win when retention < 60%, but got human win")
	}
}

// ---------------------------------------------------------------------------
// TestValidateConstruct
// ---------------------------------------------------------------------------

func TestValidateConstruct_ValidCase(t *testing.T) {
	sys := &StarSystem{
		ID:         "sol",
		Status:     StatusHuman,
		EconLevel:  5,
		Wealth:     1000,
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponBattleship, 1); err != nil {
		t.Errorf("expected no error for valid construct, got: %v", err)
	}
}

func TestValidateConstruct_InsufficientWealth(t *testing.T) {
	sys := &StarSystem{
		ID:         "sol",
		Status:     StatusHuman,
		EconLevel:  5,
		Wealth:     10, // Battleship costs 32
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponBattleship, 1); err == nil {
		t.Fatal("expected error for insufficient wealth, got nil")
	}
}


func TestValidateConstruct_AlienHeldSystem(t *testing.T) {
	sys := &StarSystem{
		ID:         "remote",
		Status:     StatusAlien,
		EconLevel:  5,
		Wealth:     1000,
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponOrbitalDefense, 1); err == nil {
		t.Fatal("expected error for alien-held system, got nil")
	}
}

func TestValidateConstruct_ZeroQuantity(t *testing.T) {
	sys := &StarSystem{
		ID:         "sol",
		Status:     StatusHuman,
		EconLevel:  5,
		Wealth:     1000,
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponOrbitalDefense, 0); err == nil {
		t.Fatal("expected error for zero quantity, got nil")
	}
}

func TestValidateConstruct_UnknownWeaponType(t *testing.T) {
	sys := &StarSystem{
		ID:         "sol",
		Status:     StatusHuman,
		EconLevel:  5,
		Wealth:     1000,
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponType("phaser_bank"), 1); err == nil {
		t.Fatal("expected error for unknown weapon type, got nil")
	}
}

func TestValidateConstruct_ExactWealth(t *testing.T) {
	// Exactly enough wealth should succeed.
	def := WeaponDefs[WeaponOrbitalDefense] // cost = 1
	sys := &StarSystem{
		ID:         "sol",
		Status:     StatusHuman,
		EconLevel:  5,
		Wealth:     def.Cost,
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponOrbitalDefense, 1); err != nil {
		t.Errorf("expected no error with exact wealth, got: %v", err)
	}
}

func TestValidateConstruct_MultipleQuantityCost(t *testing.T) {
	// Ordering 3 orbital defenses costs 3; wealth of 2 should fail.
	sys := &StarSystem{
		ID:         "sol",
		Status:     StatusHuman,
		EconLevel:  5,
		Wealth:     2,
		LocalUnits: map[WeaponType]int{},
	}
	if err := ValidateConstruct(sys, WeaponOrbitalDefense, 3); err == nil {
		t.Fatal("expected error when total cost (3) exceeds wealth (2), got nil")
	}
}

// ---------------------------------------------------------------------------
// TestExtractAndSendReporters
// ---------------------------------------------------------------------------

func TestExtractAndSendReporters_ReportersFleeBeforeCombat(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]

	// Fleet with 2 reporters and 1 escort.
	fleet := addFleet(st, remote, HumanOwner, map[WeaponType]int{
		WeaponReporter: 2,
		WeaponEscort:   1,
	})

	humanUnits := []combatUnit{
		{weaponType: WeaponReporter, owner: HumanOwner},
		{weaponType: WeaponReporter, owner: HumanOwner},
		{weaponType: WeaponEscort, owner: HumanOwner},
	}

	fled := extractAndSendReporters(st, remote, &humanUnits)

	if !fled {
		t.Fatal("expected reportersFled=true, got false")
	}
	// Reporters must be gone from the source fleet.
	if n := fleet.Units[WeaponReporter]; n != 0 {
		t.Errorf("expected reporters removed from source fleet, got %d", n)
	}
	// Escort must remain.
	if n := fleet.Units[WeaponEscort]; n != 1 {
		t.Errorf("expected escort to remain in source fleet, got %d", n)
	}

	// A new in-transit reporter fleet must head to Sol.
	var reporterFleet *Fleet
	for _, f := range st.Fleets {
		if f.InTransit && f.DestID == "sol" && f.Units[WeaponReporter] > 0 {
			reporterFleet = f
		}
	}
	if reporterFleet == nil {
		t.Fatal("expected an in-transit reporter fleet heading to Sol, found none")
	}
	if reporterFleet.Units[WeaponReporter] != 2 {
		t.Errorf("reporter fleet has %d reporters, want 2", reporterFleet.Units[WeaponReporter])
	}

	// Reporter combatUnits must have been removed from humanUnits.
	for _, u := range humanUnits {
		if u.weaponType == WeaponReporter {
			t.Error("reporter combatUnit still present in humanUnits after extraction")
		}
	}
	if len(humanUnits) != 1 {
		t.Errorf("expected 1 remaining combatUnit (escort), got %d", len(humanUnits))
	}
}

func TestExtractAndSendReporters_NoReporters(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	addFleet(st, remote, HumanOwner, map[WeaponType]int{WeaponEscort: 2})

	humanUnits := []combatUnit{
		{weaponType: WeaponEscort, owner: HumanOwner},
		{weaponType: WeaponEscort, owner: HumanOwner},
	}

	fled := extractAndSendReporters(st, remote, &humanUnits)

	if fled {
		t.Error("expected reportersFled=false when no reporters present, got true")
	}
	if len(humanUnits) != 2 {
		t.Errorf("expected humanUnits unchanged (len 2), got %d", len(humanUnits))
	}
}

func TestExtractAndSendReporters_ArrivalYearMatchesTravel(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"] // DistFromSol = 4.37 LY
	addFleet(st, remote, HumanOwner, map[WeaponType]int{WeaponReporter: 1})
	humanUnits := []combatUnit{{weaponType: WeaponReporter, owner: HumanOwner}}

	extractAndSendReporters(st, remote, &humanUnits)

	expectedTravel := remote.DistFromSol / FleetSpeedC // 4.37 / 0.8 ≈ 5.4625 years
	for _, f := range st.Fleets {
		if f.InTransit && f.DestID == "sol" {
			if math.Abs(f.ArrivalYear-expectedTravel) > 1e-9 {
				t.Errorf("reporter arrival year = %.6f, want %.6f", f.ArrivalYear, expectedTravel)
			}
		}
	}
}

func TestExtractAndSendReporters_NoSolSystem(t *testing.T) {
	// If Sol is missing from state, reporters should not panic and fled=false.
	st := newMinimalState()
	delete(st.Systems, "sol")
	remote := st.Systems["alpha-centauri"]
	addFleet(st, remote, HumanOwner, map[WeaponType]int{WeaponReporter: 2})
	humanUnits := []combatUnit{
		{weaponType: WeaponReporter, owner: HumanOwner},
		{weaponType: WeaponReporter, owner: HumanOwner},
	}

	fled := extractAndSendReporters(st, remote, &humanUnits)
	if fled {
		t.Error("expected fled=false when Sol system is missing")
	}
}

// ---------------------------------------------------------------------------
// TestHitProbability
// ---------------------------------------------------------------------------

func TestHitProbability_ZeroAttackPower(t *testing.T) {
	// Reporter and CommLaser have AttackPower == 0; must always return 0.
	for _, wt := range []WeaponType{WeaponReporter, WeaponCommLaser} {
		p := hitProbability(wt, WeaponBattleship)
		if p != 0.0 {
			t.Errorf("hitProbability(%s, battleship) = %v, want 0.0", wt, p)
		}
	}
}

func TestHitProbability_KnownValues(t *testing.T) {
	cases := []struct {
		attacker WeaponType
		target   WeaponType
		want     float64
	}{
		// Battleship (10) vs OrbitalDefense (10): 10/20 = 0.5
		{WeaponBattleship, WeaponOrbitalDefense, 10.0 / 20.0},
		// Battleship (10) vs Battleship (1): 10/11 ≈ 0.909
		{WeaponBattleship, WeaponBattleship, 10.0 / 11.0},
		// OrbitalDefense (1) vs CommLaser (10): 1/11 ≈ 0.0909
		{WeaponOrbitalDefense, WeaponCommLaser, 1.0 / 11.0},
		// Interceptor (3) vs Interceptor (3): 3/6 = 0.5
		{WeaponInterceptor, WeaponInterceptor, 3.0 / 6.0},
	}
	for _, tc := range cases {
		got := hitProbability(tc.attacker, tc.target)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("hitProbability(%s, %s) = %.6f, want %.6f",
				tc.attacker, tc.target, got, tc.want)
		}
	}
}

func TestHitProbability_AlwaysInBounds(t *testing.T) {
	// For every weapon pair, result must be in [0.0, 0.95].
	// Zero-attack weapons return 0.0; all others are clamped to [0.05, 0.95].
	for _, attackerType := range WeaponTypeOrder {
		for _, targetType := range WeaponTypeOrder {
			p := hitProbability(attackerType, targetType)
			if p < 0 {
				t.Errorf("hitProbability(%s, %s) = %v < 0", attackerType, targetType, p)
			}
			if p > 0.95 {
				t.Errorf("hitProbability(%s, %s) = %v > 0.95", attackerType, targetType, p)
			}
			if WeaponDefs[attackerType].AttackPower > 0 && p < 0.05 {
				t.Errorf("hitProbability(%s, %s) = %v < min 0.05", attackerType, targetType, p)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// TestUpdateKnownStates
// ---------------------------------------------------------------------------

func TestUpdateKnownStates_CaptureEventApplied(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	remote.KnownStatus = StatusHuman
	st.Clock = 10.0

	evt := &GameEvent{
		ID:          "evt-1",
		EventYear:   3.0,
		ArrivalYear: 5.0, // arrived before clock
		SystemID:    "alpha-centauri",
		Type:        EventSystemCaptured,
		CanReport:   true,
	}
	st.Events = append(st.Events, evt)

	st.UpdateKnownStates(st.Clock)

	if remote.KnownStatus != StatusAlien {
		t.Errorf("expected KnownStatus=alien after capture event, got %q", remote.KnownStatus)
	}
	if !evt.AppliedToKnown {
		t.Error("expected evt.AppliedToKnown=true")
	}
}

func TestUpdateKnownStates_FutureEventNotApplied(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	remote.KnownStatus = StatusHuman
	st.Clock = 10.0

	evt := &GameEvent{
		ID:          "evt-2",
		EventYear:   5.0,
		ArrivalYear: 20.0, // arrives after current clock
		SystemID:    "alpha-centauri",
		Type:        EventSystemCaptured,
		CanReport:   true,
	}
	st.Events = append(st.Events, evt)

	st.UpdateKnownStates(st.Clock)

	if remote.KnownStatus != StatusHuman {
		t.Errorf("expected KnownStatus=human (event not yet arrived), got %q", remote.KnownStatus)
	}
	if evt.AppliedToKnown {
		t.Error("expected evt.AppliedToKnown=false for future event")
	}
}

func TestUpdateKnownStates_UnreportedEventNeverApplied(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	remote.KnownStatus = StatusHuman
	st.Clock = 1000.0

	evt := &GameEvent{
		ID:          "evt-3",
		EventYear:   1.0,
		ArrivalYear: math.MaxFloat64, // unreported
		SystemID:    "alpha-centauri",
		Type:        EventSystemCaptured,
		CanReport:   false,
	}
	st.Events = append(st.Events, evt)

	st.UpdateKnownStates(st.Clock)

	if remote.KnownStatus != StatusHuman {
		t.Errorf("expected KnownStatus=human (unreported), got %q", remote.KnownStatus)
	}
	if evt.AppliedToKnown {
		t.Error("expected evt.AppliedToKnown=false for unreported event")
	}
}

func TestUpdateKnownStates_Idempotent(t *testing.T) {
	// Calling UpdateKnownStates twice must not apply the event twice.
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	remote.KnownStatus = StatusHuman
	st.Clock = 10.0

	evt := &GameEvent{
		ID:          "evt-4",
		EventYear:   1.0,
		ArrivalYear: 5.0,
		SystemID:    "alpha-centauri",
		Type:        EventSystemCaptured,
		CanReport:   true,
	}
	st.Events = append(st.Events, evt)

	st.UpdateKnownStates(st.Clock)
	st.UpdateKnownStates(st.Clock)

	if remote.KnownStatus != StatusAlien {
		t.Errorf("expected KnownStatus=alien after double update, got %q", remote.KnownStatus)
	}
}

func TestUpdateKnownStates_ConstructionAddsKnownUnits(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	st.Clock = 10.0

	evt := &GameEvent{
		ID:          "evt-5",
		EventYear:   1.0,
		ArrivalYear: 5.0,
		SystemID:    "alpha-centauri",
		Type:        EventConstructionDone,
		CanReport:   true,
		Details:     &ConstructionDetails{WeaponType: WeaponOrbitalDefense, Quantity: 3},
	}
	st.Events = append(st.Events, evt)

	st.UpdateKnownStates(st.Clock)

	if got := remote.KnownLocalUnits[WeaponOrbitalDefense]; got != 3 {
		t.Errorf("expected KnownLocalUnits[orbital_defense]=3, got %d", got)
	}
}

func TestUpdateKnownStates_RetakenEventUpdatesStatus(t *testing.T) {
	st := newMinimalState()
	remote := st.Systems["alpha-centauri"]
	remote.KnownStatus = StatusAlien // start as alien-held (known)
	st.Clock = 10.0

	evt := &GameEvent{
		ID:          "evt-6",
		EventYear:   2.0,
		ArrivalYear: 6.0,
		SystemID:    "alpha-centauri",
		Type:        EventSystemRetaken,
		CanReport:   true,
	}
	st.Events = append(st.Events, evt)

	st.UpdateKnownStates(st.Clock)

	if remote.KnownStatus != StatusHuman {
		t.Errorf("expected KnownStatus=human after retaken event, got %q", remote.KnownStatus)
	}
}

// ---------------------------------------------------------------------------
// TestGaussianEconLevel
// ---------------------------------------------------------------------------

func TestGaussianEconLevel_AlwaysInRange(t *testing.T) {
	// Run 10,000 samples with a fixed seed; every result must be in [1, 5].
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10_000; i++ {
		level := gaussianEconLevel(rng)
		if level < 1 || level > 5 {
			t.Errorf("iteration %d: gaussianEconLevel returned %d, want [1, 5]", i, level)
		}
	}
}

func TestGaussianEconLevel_ProducesVariety(t *testing.T) {
	// The distribution should not be degenerate — we expect at least 3 distinct
	// values in 1000 samples.
	rng := rand.New(rand.NewSource(99))
	seen := map[int]bool{}
	for i := 0; i < 1000; i++ {
		seen[gaussianEconLevel(rng)] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected ≥3 distinct econ levels in 1000 samples, got %d: %v", len(seen), seen)
	}
}

// ---------------------------------------------------------------------------
// TestApplyEconomicCombatPenalty
// ---------------------------------------------------------------------------

func TestApplyEconomicCombatPenalty_ReducesEconLevel(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	sys.EconLevel = 3
	sys.Wealth = 100
	sys.EconGrowthYear = 500

	ApplyEconomicCombatPenalty(rand.New(rand.NewSource(1)), st, sys)

	if sys.EconLevel != 2 {
		t.Errorf("expected EconLevel=2 after penalty (was 3), got %d", sys.EconLevel)
	}
	if sys.Wealth >= 100 {
		t.Errorf("expected Wealth < 100 after penalty, got %.2f", sys.Wealth)
	}
	if sys.Wealth < 0 {
		t.Errorf("expected Wealth ≥ 0 after penalty, got %.2f", sys.Wealth)
	}
	expected := st.Clock + EconGrowthIntervalYears
	if sys.EconGrowthYear != expected {
		t.Errorf("expected EconGrowthYear=%.1f (clock+interval), got %.1f",
			expected, sys.EconGrowthYear)
	}
}

func TestApplyEconomicCombatPenalty_EconLevelFloorIsZero(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	sys.EconLevel = 0

	ApplyEconomicCombatPenalty(rand.New(rand.NewSource(1)), st, sys)

	if sys.EconLevel < 0 {
		t.Errorf("EconLevel went below 0: got %d", sys.EconLevel)
	}
}

func TestApplyEconomicCombatPenalty_WealthNeverNegative(t *testing.T) {
	// Run many calls; wealth must stay in [0, initialWealth].
	st := newMinimalState()
	for i := 0; i < 1000; i++ {
		sys := &StarSystem{
			ID:             "test",
			Status:         StatusHuman,
			EconLevel:      3,
			Wealth:         50,
			EconGrowthYear: 0,
			LocalUnits:     map[WeaponType]int{},
		}
		st.Systems["test"] = sys
		ApplyEconomicCombatPenalty(rand.New(rand.NewSource(1)), st, sys)
		if sys.Wealth < 0 {
			t.Fatalf("iteration %d: wealth went negative: %.4f", i, sys.Wealth)
		}
		if sys.Wealth > 50 {
			t.Fatalf("iteration %d: wealth increased: %.4f", i, sys.Wealth)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAccumulateWealth
// ---------------------------------------------------------------------------

func TestAccumulateWealth_HumanSystemAccumulates(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	sys.EconLevel = 3 // rate = 2^3 = 8 per year
	sys.Wealth = 0

	AccumulateWealth(st, 1.0)

	want := EconWealthRate[3] * 1.0
	if math.Abs(sys.Wealth-want) > 1e-9 {
		t.Errorf("expected Wealth=%.4f after 1 year at level 3, got %.4f", want, sys.Wealth)
	}
}

func TestAccumulateWealth_AlienHeldDoesNotAccumulate(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	sys.Status = StatusAlien
	sys.Wealth = 0

	AccumulateWealth(st, 1.0)

	if sys.Wealth != 0 {
		t.Errorf("alien-held system accumulated wealth: %.4f", sys.Wealth)
	}
}

func TestAccumulateWealth_UninithabitedDoesNotAccumulate(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	sys.Status = StatusUninhabited
	sys.EconLevel = 0
	sys.Wealth = 0

	AccumulateWealth(st, 1.0)

	if sys.Wealth != 0 {
		t.Errorf("uninhabited system accumulated wealth: %.4f", sys.Wealth)
	}
}

func TestAccumulateWealth_RateMatchesTable(t *testing.T) {
	// Verify each economic level accumulates at 2^level per year.
	for level := 0; level <= 5; level++ {
		st := newMinimalState()
		sys := st.Systems["alpha-centauri"]
		sys.EconLevel = level
		sys.Wealth = 0

		AccumulateWealth(st, 1.0)

		want := EconWealthRate[level]
		if math.Abs(sys.Wealth-want) > 1e-9 {
			t.Errorf("level %d: expected Wealth=%.4f, got %.4f", level, want, sys.Wealth)
		}
	}
}

// ---------------------------------------------------------------------------
// TestArrivalYearFor
// ---------------------------------------------------------------------------

func TestArrivalYearFor_WithCommLaser(t *testing.T) {
	clock, dist := 10.0, 4.37
	got := arrivalYearFor(clock, dist, true)
	want := clock + dist
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("arrivalYearFor with comm laser = %.6f, want %.6f", got, want)
	}
}

func TestArrivalYearFor_WithoutCommLaser(t *testing.T) {
	got := arrivalYearFor(10.0, 4.37, false)
	if got != math.MaxFloat64 {
		t.Errorf("arrivalYearFor without comm laser = %v, want MaxFloat64", got)
	}
}

func TestArrivalYearFor_SolIsImmediate(t *testing.T) {
	// Sol has DistFromSol=0; with comm laser, arrival = clock + 0 = clock.
	clock := 42.5
	got := arrivalYearFor(clock, 0, true)
	if math.Abs(got-clock) > 1e-9 {
		t.Errorf("Sol arrival year = %.6f, want %.6f (same as clock)", got, clock)
	}
}

// ---------------------------------------------------------------------------
// TestConquest
// ---------------------------------------------------------------------------

func newConquestEngine(st *GameState) *Engine {
	return &Engine{State: st, rng: rand.New(rand.NewSource(42))}
}

func TestConquest_HumanFleetWithCommLaser_ClaimsUninhabited(t *testing.T) {
	st := newMinimalState()
	st.Clock = 10.0
	uninhabited := newUninhabitedSystem("proxima", 4.24)
	st.Systems["proxima"] = uninhabited

	addInTransitFleet(st, HumanOwner, map[WeaponType]int{
		WeaponCommLaser:  1,
		WeaponBattleship: 1,
	}, "proxima", 10.0) // arrives exactly at clock

	newConquestEngine(st).processFleetArrivals()

	if uninhabited.Status != StatusHuman {
		t.Errorf("expected Status=human after conquest, got %q", uninhabited.Status)
	}
	if uninhabited.EconLevel != 0 {
		t.Errorf("expected EconLevel=0, got %d", uninhabited.EconLevel)
	}
	if uninhabited.Wealth != 0 {
		t.Errorf("expected Wealth=0, got %.2f", uninhabited.Wealth)
	}
	wantGrowthYear := st.Clock + EconGrowthIntervalYears
	if uninhabited.EconGrowthYear != wantGrowthYear {
		t.Errorf("expected EconGrowthYear=%.1f, got %.1f", wantGrowthYear, uninhabited.EconGrowthYear)
	}

	var conquestEvt *GameEvent
	for _, e := range st.Events {
		if e.Type == EventSystemConquered && e.SystemID == "proxima" {
			conquestEvt = e
			break
		}
	}
	if conquestEvt == nil {
		t.Fatal("expected system_conquered event, found none")
	}
	if !conquestEvt.CanReport {
		t.Error("expected conquest event CanReport=true")
	}
	wantArrYear := st.Clock + uninhabited.DistFromSol // at c, since comm laser present
	if math.Abs(conquestEvt.ArrivalYear-wantArrYear) > 1e-9 {
		t.Errorf("conquest event ArrivalYear=%.6f, want %.6f", conquestEvt.ArrivalYear, wantArrYear)
	}
}

func TestConquest_NoCommLaser_NoConquest(t *testing.T) {
	st := newMinimalState()
	st.Clock = 10.0
	uninhabited := newUninhabitedSystem("proxima", 4.24)
	st.Systems["proxima"] = uninhabited

	addInTransitFleet(st, HumanOwner, map[WeaponType]int{
		WeaponBattleship: 2,
	}, "proxima", 10.0)

	newConquestEngine(st).processFleetArrivals()

	if uninhabited.Status != StatusUninhabited {
		t.Errorf("expected Status=uninhabited (no comm laser), got %q", uninhabited.Status)
	}
	for _, e := range st.Events {
		if e.Type == EventSystemConquered {
			t.Error("expected no system_conquered event when fleet has no comm laser")
		}
	}
}

func TestConquest_AlienFleet_NoConquest(t *testing.T) {
	st := newMinimalState()
	st.Clock = 10.0
	uninhabited := newUninhabitedSystem("proxima", 4.24)
	st.Systems["proxima"] = uninhabited

	addInTransitFleet(st, AlienOwner, map[WeaponType]int{
		WeaponCommLaser: 1,
	}, "proxima", 10.0)

	newConquestEngine(st).processFleetArrivals()

	if uninhabited.Status != StatusUninhabited {
		t.Errorf("expected Status=uninhabited (alien fleet), got %q", uninhabited.Status)
	}
}

func TestConquest_AlreadyHumanSystem_NoConquest(t *testing.T) {
	st := newMinimalState()
	st.Clock = 10.0
	// alpha-centauri is StatusHuman in newMinimalState
	sys := st.Systems["alpha-centauri"]

	addInTransitFleet(st, HumanOwner, map[WeaponType]int{
		WeaponCommLaser: 1,
	}, "alpha-centauri", 10.0)

	newConquestEngine(st).processFleetArrivals()

	// Status must remain human, and no conquest event should appear.
	if sys.Status != StatusHuman {
		t.Errorf("expected Status=human (unchanged), got %q", sys.Status)
	}
	for _, e := range st.Events {
		if e.Type == EventSystemConquered {
			t.Error("expected no system_conquered event for already-human system")
		}
	}
}

func TestConquest_KnownStateUpdatedAfterConquest(t *testing.T) {
	// Verify UpdateKnownStates applies EventSystemConquered correctly.
	st := newMinimalState()
	st.Clock = 10.0
	uninhabited := newUninhabitedSystem("proxima", 4.24)
	uninhabited.KnownStatus = StatusUninhabited
	st.Systems["proxima"] = uninhabited

	addInTransitFleet(st, HumanOwner, map[WeaponType]int{
		WeaponCommLaser: 1,
	}, "proxima", 10.0)

	newConquestEngine(st).processFleetArrivals()

	// The conquest event arrives at clock + dist = 14.24; advance clock past that.
	st.Clock = 20.0
	st.UpdateKnownStates(st.Clock)

	if uninhabited.KnownStatus != StatusHuman {
		t.Errorf("expected KnownStatus=human after conquest event applied, got %q", uninhabited.KnownStatus)
	}
	if uninhabited.KnownEconLevel != 0 {
		t.Errorf("expected KnownEconLevel=0, got %d", uninhabited.KnownEconLevel)
	}
}

// ---------------------------------------------------------------------------
// TestAlienCompositionSumMatchesConstants
//

// ---------------------------------------------------------------------------
// TestSystemHasCommLaser
// ---------------------------------------------------------------------------

func TestSystemHasCommLaser_LocalUnit(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	sys.LocalUnits[WeaponCommLaser] = 1

	if !systemHasCommLaser(st, sys) {
		t.Error("expected true when comm laser in LocalUnits, got false")
	}
}

func TestSystemHasCommLaser_StationedFleet(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	addFleet(st, sys, HumanOwner, map[WeaponType]int{WeaponCommLaser: 1})

	if !systemHasCommLaser(st, sys) {
		t.Error("expected true when stationed human fleet carries comm laser, got false")
	}
}

func TestSystemHasCommLaser_InTransitNotCounted(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	f := addFleet(st, sys, HumanOwner, map[WeaponType]int{WeaponCommLaser: 1})
	f.InTransit = true
	f.LocationID = ""

	if systemHasCommLaser(st, sys) {
		t.Error("expected false when comm laser fleet is in transit, got true")
	}
}

func TestSystemHasCommLaser_AlienFleetNotCounted(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	addFleet(st, sys, AlienOwner, map[WeaponType]int{WeaponCommLaser: 1})

	if systemHasCommLaser(st, sys) {
		t.Error("expected false when comm laser is in alien fleet, got true")
	}
}

func TestSystemHasCommLaser_NonePresent(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["alpha-centauri"]
	addFleet(st, sys, HumanOwner, map[WeaponType]int{WeaponBattleship: 2})

	if systemHasCommLaser(st, sys) {
		t.Error("expected false when no comm laser present, got true")
	}
}

// ---------------------------------------------------------------------------
// TestExecuteConstruct_CommLaserGoesIntoFleet
// ---------------------------------------------------------------------------

func TestExecuteConstruct_CommLaserGoesIntoFleet(t *testing.T) {
	st := newMinimalState()
	sys := st.Systems["sol"] // level 5, wealth 1000 — meets MinLevel 4 and cost 64

	ExecuteConstruct(st, sys, WeaponCommLaser, 1)

	// Must NOT appear in LocalUnits (comm laser is now mobile).
	if sys.LocalUnits[WeaponCommLaser] != 0 {
		t.Errorf("expected LocalUnits[comm_laser]=0, got %d", sys.LocalUnits[WeaponCommLaser])
	}

	// Must appear in the primary fleet at Sol.
	fleet := st.Fleets[sys.PrimaryFleetID]
	if fleet == nil {
		t.Fatal("expected a primary fleet to be created at Sol, got nil")
	}
	if fleet.Units[WeaponCommLaser] != 1 {
		t.Errorf("expected fleet Units[comm_laser]=1, got %d", fleet.Units[WeaponCommLaser])
	}
}
