package game

import (
	"math/rand"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers for integration tests that drive Engine.tick() directly.
// ---------------------------------------------------------------------------

// twoSystemSetup constructs a deterministic 2-system game (Sol + remote at
// distLY light-years) wired through Truth, SolView, EventLog, Propagator,
// and Engine. Both systems are human-held at EconLevel 5 (so no econ-growth
// events fire) with comm lasers (so all events are reportable). No alien
// entry points are configured, so AlienSpawn is a no-op.
//
// Returns the GameState, the Engine, and the remote system's TrueSystem
// for direct manipulation in tests.
func twoSystemSetup(t *testing.T, distLY float64) (*GameState, *Engine, *TrueSystem) {
	t.Helper()

	cat := NewStarCatalog([]*CatalogEntry{
		{ID: "sol", DisplayName: "Sol", X: 0, Y: 0, Z: 0, DistFromSol: 0, IsSol: true},
		{ID: "remote", DisplayName: "Remote", X: distLY, Y: 0, Z: 0, DistFromSol: distLY},
	})

	sol := &TrueSystem{
		ID:             "sol",
		Status:         StatusHuman,
		EconLevel:      5,
		Wealth:         10000,
		EconGrowthYear: EconGrowthIntervalYears,
		LocalUnits:     map[WeaponType]int{WeaponCommLaser: 1},
	}
	remote := &TrueSystem{
		ID:             "remote",
		Status:         StatusHuman,
		EconLevel:      5,
		Wealth:         10000,
		EconGrowthYear: EconGrowthIntervalYears,
		LocalUnits:     map[WeaponType]int{WeaponCommLaser: 1},
	}
	truth := &Truth{
		Systems: map[string]*TrueSystem{"sol": sol, "remote": remote},
		Fleets:  map[string]*TrueFleet{},
	}

	view := &PlayerView{
		Systems: map[string]*KnownSystem{
			"sol": {
				ID:         "sol",
				Status:     StatusHuman,
				EconLevel:  5,
				Wealth:     sol.Wealth,
				LocalUnits: copyUnits(sol.LocalUnits),
			},
			"remote": {
				ID:         "remote",
				Status:     StatusHuman,
				EconLevel:  5,
				Wealth:     remote.Wealth,
				LocalUnits: copyUnits(remote.LocalUnits),
			},
		},
		Fleets:    map[string]*KnownFleet{},
		InTransit: map[string]*KnownTransit{},
	}

	st := &GameState{
		Catalog:     cat,
		truth:       truth,
		Views:       map[Owner]*PlayerView{HumanOwner: view},
		Events:      NewEventLog(),
		PendingCmds: []*PendingCommand{},
		rng:         rand.New(rand.NewSource(1)),
	}
	st.Factions = map[Owner]*Faction{
		HumanOwner: {InitialSystemIDs: []string{"sol", "remote"}},
		AlienOwner: {},
	}
	st.Homes = map[Owner]string{HumanOwner: "sol"}

	events := NewEventManager()
	rng := rand.New(rand.NewSource(2))
	engine := NewEngine(st, events, rng)
	return st, engine, remote
}

// tickUntil advances the engine by repeatedly calling tick() until the
// game clock reaches at least target. The helper has a safety budget to
// avoid runaway loops; if it is exceeded, the test fails.
func tickUntil(t *testing.T, st *GameState, e *Engine, target float64) {
	t.Helper()
	safety := 10000
	for st.Clock < target {
		if safety <= 0 {
			t.Fatalf("tickUntil(%.2f) exceeded safety budget; current Clock=%.2f", target, st.Clock)
		}
		st.Lock()
		e.tick()
		st.Unlock()
		safety--
	}
}

// ---------------------------------------------------------------------------
// DR-3: light-speed gating of construction
// ---------------------------------------------------------------------------

// TestIntegration_LightSpeedGating_Construct verifies that a construction
// command issued at Sol targeting a remote system at 10 LY is not visible
// in SolView until 20 game-years after it is issued (10y outbound command
// + 10y inbound report).
func TestIntegration_LightSpeedGating_Construct(t *testing.T) {
	st, e, _ := twoSystemSetup(t, 10.0)

	// Issue a non-mobile construction (orbital_defense, cost=1) targeting remote.
	cmd := &PendingCommand{
		TargetID:   "remote",
		Type:       CmdConstruct,
		WeaponType: WeaponOrbitalDefense,
		Quantity:   1,
		Issuer:     HumanOwner,
	}
	if _, _, err := e.EnqueueCommand(cmd); err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	// Advance to Clock=9 (< 10): command has not yet arrived at remote.
	tickUntil(t, st, e, 9.0)
	if got := st.Views[HumanOwner].Systems["remote"].LocalUnits[WeaponOrbitalDefense]; got != 0 {
		t.Errorf("at Clock=%.2f: SolView.LocalUnits[orbital_defense]=%d, want 0 "+
			"(command has not yet arrived)", st.Clock, got)
	}
	// Truth-side must also still be 0 -- the engine has not yet executed
	// the command.
	if got := st.truth.Systems["remote"].LocalUnits[WeaponOrbitalDefense]; got != 0 {
		t.Errorf("at Clock=%.2f: truth LocalUnits[orbital_defense]=%d, want 0", st.Clock, got)
	}

	// Advance to Clock=21 (> 20): command has arrived (Clock=10), executed,
	// and the EventConstructionDone has matured at Sol (Clock=20).
	tickUntil(t, st, e, 21.0)
	if got := st.Views[HumanOwner].Systems["remote"].LocalUnits[WeaponOrbitalDefense]; got != 1 {
		t.Errorf("at Clock=%.2f: SolView.LocalUnits[orbital_defense]=%d, want 1 "+
			"(report should have arrived)", st.Clock, got)
	}
	if got := st.truth.Systems["remote"].LocalUnits[WeaponOrbitalDefense]; got != 1 {
		t.Errorf("at Clock=%.2f: truth LocalUnits[orbital_defense]=%d, want 1", st.Clock, got)
	}
}

// ---------------------------------------------------------------------------
// DR-4: snapshot independence (no live aliasing) -- end-to-end
// ---------------------------------------------------------------------------

// TestIntegration_NoLiveAliasing verifies that after a mobile-unit
// construction has been reported to Sol via the engine pipeline, mutating
// the truth-side fleet's Units does not affect the SolView snapshot.
func TestIntegration_NoLiveAliasing(t *testing.T) {
	st, e, _ := twoSystemSetup(t, 10.0)

	// Issue a battleship construction (mobile, cost=32, MinLevel=3).
	cmd := &PendingCommand{
		TargetID:   "remote",
		Type:       CmdConstruct,
		WeaponType: WeaponBattleship,
		Quantity:   1,
		Issuer:     HumanOwner,
	}
	if _, _, err := e.EnqueueCommand(cmd); err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	// Run past the report-arrival year (20.0).
	tickUntil(t, st, e, 21.0)

	// Locate the truth-side fleet that received the battleship.
	var primaryID string
	for fid, f := range st.truth.Fleets {
		if f.Owner == HumanOwner && f.LocationID == "remote" && f.Units[WeaponBattleship] > 0 {
			primaryID = fid
			break
		}
	}
	if primaryID == "" {
		t.Fatal("expected a truth-side human fleet at remote with a battleship; found none")
	}
	kf := st.Views[HumanOwner].Fleet(primaryID)
	if kf == nil {
		t.Fatalf("expected SolView.Fleets[%q] populated by report; got nil", primaryID)
	}
	if got := kf.Units[WeaponBattleship]; got != 1 {
		t.Errorf("KnownFleet.Units[battleship]=%d, want 1", got)
	}

	// Mutate truth: add 99 reporters and bump battleships to 50.
	st.Truth().Fleets[primaryID].Units[WeaponBattleship] = 50
	st.Truth().Fleets[primaryID].Units[WeaponReporter] = 99

	// SolView snapshot must be unchanged.
	if got := kf.Units[WeaponBattleship]; got != 1 {
		t.Errorf("after truth mutation, KnownFleet.Units[battleship]=%d, want 1 (snapshot must be independent)", got)
	}
	if _, present := kf.Units[WeaponReporter]; present {
		t.Errorf("after truth mutation, KnownFleet.Units gained reporter key; snapshot leaked alias")
	}
}

// ---------------------------------------------------------------------------
// DR-3: light-speed gating of fleet departure
// ---------------------------------------------------------------------------

// TestIntegration_DepartureGating verifies that issuing a CmdMove against
// a remote fleet does not populate SolView.InTransit until the
// EventFleetDeparted matures at Sol -- i.e. only after another dist/c
// years following the order's execution at remote.
func TestIntegration_DepartureGating(t *testing.T) {
	st, e, remote := twoSystemSetup(t, 10.0)

	// Pre-position a stationed fleet at remote, mirrored into SolView.
	tf := &TrueFleet{
		ID:         "remote-fleet",
		Name:       "Remote Fleet",
		Owner:      HumanOwner,
		Units:      map[WeaponType]int{WeaponEscort: 2},
		LocationID: "remote",
	}
	st.truth.Fleets[tf.ID] = tf
	remote.FleetIDs = append(remote.FleetIDs, tf.ID)
	st.Views[HumanOwner].Fleets[tf.ID] = &KnownFleet{
		ID:         tf.ID,
		Name:       tf.Name,
		Owner:      HumanOwner,
		Units:      copyUnits(tf.Units),
		LocationID: "remote",
	}
	st.Views[HumanOwner].Systems["remote"].FleetIDs = append(
		st.Views[HumanOwner].Systems["remote"].FleetIDs, tf.ID)

	// Issue a move command from remote to sol.
	cmd := &PendingCommand{
		TargetID: "remote",
		Type:     CmdMove,
		FleetID:  tf.ID,
		DestID:   "sol",
		Issuer:   HumanOwner,
	}
	if _, _, err := e.EnqueueCommand(cmd); err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	// Command travels 10y (CommandSpeedC=1) to remote, then the
	// EventFleetDeparted travels 10y back to Sol. Total ~20y.
	//
	// At Clock=15: command has executed (truth shows fleet in transit), but
	// the departure report has not yet matured at Sol. SolView.InTransit
	// must be empty.
	tickUntil(t, st, e, 15.0)
	if !tf.InTransit {
		t.Errorf("at Clock=%.2f: truth fleet should be in transit", st.Clock)
	}
	if len(st.Views[HumanOwner].InTransit) != 0 {
		t.Errorf("at Clock=%.2f: SolView.InTransit=%v, want empty (report not yet matured)",
			st.Clock, st.Views[HumanOwner].InTransit)
	}
	// SolView still shows the fleet stationed at remote.
	if st.Views[HumanOwner].Fleet(tf.ID) == nil {
		t.Errorf("at Clock=%.2f: SolView.Fleets[%q] missing; should still appear stationed",
			st.Clock, tf.ID)
	}

	// At Clock=21: EventFleetDeparted (ArrivalYear=20) has matured.
	// Note: the truth-side fleet arrives at Sol at DepartYear + dist/FleetSpeedC
	// = 10 + 10/0.8 = 22.5; we stop at 21 to observe the in-transit state.
	tickUntil(t, st, e, 21.0)
	if _, ok := st.Views[HumanOwner].InTransit[tf.ID]; !ok {
		t.Errorf("at Clock=%.2f: expected SolView.InTransit[%q] after departure event matured",
			st.Clock, tf.ID)
	}
	if st.Views[HumanOwner].Fleet(tf.ID) != nil {
		t.Errorf("at Clock=%.2f: SolView.Fleets[%q] should be removed once the fleet is in transit",
			st.Clock, tf.ID)
	}
	// Source system's KnownSystem.FleetIDs should no longer list it.
	for _, id := range st.Views[HumanOwner].Systems["remote"].FleetIDs {
		if id == tf.ID {
			t.Errorf("at Clock=%.2f: KnownSystem.FleetIDs still contains %q", st.Clock, tf.ID)
		}
	}
}
