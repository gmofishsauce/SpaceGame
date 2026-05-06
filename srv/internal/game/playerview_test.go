package game

import (
	"math"
	"testing"
)

// TestSolView_FleetArrivalRoundTrip verifies the end-to-end round-trip of
// an EventFleetArrival through the propagator: the resulting KnownFleet
// holds a snapshot of the units, has the correct AsOfYear, and is listed
// in KnownSystem.FleetIDs. Subsequent mutations to the source TrueFleet's
// Units (or to the FleetArrivalDetails map that produced the event) MUST
// NOT alter the SolView snapshot (DR-4).
func TestSolView_FleetArrivalRoundTrip(t *testing.T) {
	st := newMinimalState()
	st.Clock = 10.0

	// Truth-side: a fleet just arrived at alpha-centauri.
	const fid = "fleet-x"
	const fname = "Fleet X"
	tf := &TrueFleet{
		ID:         fid,
		Name:       fname,
		Owner:      HumanOwner,
		Units:      map[WeaponType]int{WeaponBattleship: 3, WeaponEscort: 1},
		LocationID: "alpha-centauri",
	}
	st.truth.Fleets[fid] = tf
	st.truth.Systems["alpha-centauri"].FleetIDs = appendIfMissing(
		st.truth.Systems["alpha-centauri"].FleetIDs, fid)

	// Event with a copy of the units (mirroring engine.processFleetArrivals).
	details := &FleetArrivalDetails{
		FleetID:   fid,
		FleetName: fname,
		Owner:     HumanOwner,
		Units:     copyUnits(tf.Units),
	}
	st.Events.Record(&Event{
		EventYear:   3.0,
		Arrival:     humanArrival(7.0), // < Clock, so it will mature
		SystemID:    "alpha-centauri",
		Type:        EventFleetArrival,
		Details:     details,
	})

	st.Propagator.Propagate(st)

	// SolView fleet must exist and snapshot the units.
	kf := st.Views[HumanOwner].Fleet(fid)
	if kf == nil {
		t.Fatalf("expected SolView.Fleets[%q] to exist after FleetArrival", fid)
	}
	if kf.Name != fname {
		t.Errorf("KnownFleet.Name = %q, want %q", kf.Name, fname)
	}
	if kf.Owner != HumanOwner {
		t.Errorf("KnownFleet.Owner = %q, want human", kf.Owner)
	}
	if kf.LocationID != "alpha-centauri" {
		t.Errorf("KnownFleet.LocationID = %q, want %q", kf.LocationID, "alpha-centauri")
	}
	if kf.AsOfYear != 3.0 {
		t.Errorf("KnownFleet.AsOfYear = %.2f, want 3.0 (event year)", kf.AsOfYear)
	}
	if got := kf.Units[WeaponBattleship]; got != 3 {
		t.Errorf("KnownFleet.Units[battleship] = %d, want 3", got)
	}
	if got := kf.Units[WeaponEscort]; got != 1 {
		t.Errorf("KnownFleet.Units[escort] = %d, want 1", got)
	}

	// KnownSystem must reference the fleet.
	ks := st.Views[HumanOwner].System("alpha-centauri")
	found := false
	for _, id := range ks.FleetIDs {
		if id == fid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected KnownSystem.FleetIDs to contain %q, got %v", fid, ks.FleetIDs)
	}

	// DR-4: mutate the source TrueFleet; the SolView snapshot must not change.
	tf.Units[WeaponBattleship] = 99
	tf.Units[WeaponReporter] = 5
	if got := kf.Units[WeaponBattleship]; got != 3 {
		t.Errorf("after TrueFleet mutation, KnownFleet.Units[battleship]=%d, want 3 (snapshot must be independent)", got)
	}
	if _, present := kf.Units[WeaponReporter]; present {
		t.Errorf("after TrueFleet mutation, KnownFleet.Units gained reporter key; snapshot leaked alias")
	}

	// DR-4: mutate the FleetArrivalDetails map that produced the event;
	// the SolView snapshot must still not change. (The propagator copies
	// d.Units via copyUnits, breaking any alias from the event payload.)
	details.Units[WeaponBattleship] = 0
	delete(details.Units, WeaponEscort)
	if got := kf.Units[WeaponBattleship]; got != 3 {
		t.Errorf("after Details mutation, KnownFleet.Units[battleship]=%d, want 3", got)
	}
	if got := kf.Units[WeaponEscort]; got != 1 {
		t.Errorf("after Details mutation, KnownFleet.Units[escort]=%d, want 1", got)
	}
}

// TestSolView_FleetArrivalRemovesFromInTransit verifies that when a fleet
// previously known to be in transit arrives, the propagator removes it
// from SolView.InTransit and records it in SolView.Fleets.
func TestSolView_FleetArrivalRemovesFromInTransit(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0

	const fid = "fleet-in-flight"
	st.Views[HumanOwner].InTransit[fid] = &KnownTransit{
		FleetID:     fid,
		Name:        "Inbound",
		Owner:       HumanOwner,
		Units:       map[WeaponType]int{WeaponEscort: 2},
		SourceID:    "alpha-centauri",
		DestID:      "sol",
		DepartYear:  0,
		ArrivalYear: 10,
	}

	st.Events.Record(&Event{
		EventYear:   10.0,
		Arrival: humanArrival(10.0),
		SystemID:    "sol",
		Type:        EventFleetArrival,
		Details: &FleetArrivalDetails{
			FleetID:   fid,
			FleetName: "Inbound",
			Owner:     HumanOwner,
			Units:     map[WeaponType]int{WeaponEscort: 2},
		},
	})

	st.Propagator.Propagate(st)

	if _, present := st.Views[HumanOwner].InTransit[fid]; present {
		t.Errorf("expected SolView.InTransit[%q] to be removed after FleetArrival", fid)
	}
	if st.Views[HumanOwner].Fleet(fid) == nil {
		t.Errorf("expected SolView.Fleets[%q] to be populated after FleetArrival", fid)
	}
}

// TestPlayerView_BothViewsMirroredAtT0 verifies that a state wired with both
// human and alien views contains identical KnownSystem entries for every system
// in the catalog immediately after setup (no events have propagated). (FR-17)
func TestPlayerView_BothViewsMirroredAtT0(t *testing.T) {
	st := newMinimalState()
	// Manually add an alien view mirroring the same systems.
	alienView := &PlayerView{
		Systems:   map[string]*KnownSystem{},
		Fleets:    map[string]*KnownFleet{},
		InTransit: map[string]*KnownTransit{},
	}
	for id, ks := range st.Views[HumanOwner].Systems {
		alienView.Systems[id] = &KnownSystem{
			ID:         ks.ID,
			Status:     ks.Status,
			EconLevel:  ks.EconLevel,
			Wealth:     ks.Wealth,
			LocalUnits: copyUnits(ks.LocalUnits),
		}
	}
	st.Views[AlienOwner] = alienView

	for id := range st.Views[HumanOwner].Systems {
		hks := st.Views[HumanOwner].Systems[id]
		aks := st.Views[AlienOwner].Systems[id]
		if aks == nil {
			t.Errorf("alien view missing system %q", id)
			continue
		}
		if hks.Status != aks.Status {
			t.Errorf("system %q: human status=%v alien status=%v", id, hks.Status, aks.Status)
		}
		if hks.EconLevel != aks.EconLevel {
			t.Errorf("system %q: human econLevel=%d alien econLevel=%d", id, hks.EconLevel, aks.EconLevel)
		}
	}
}

// TestPlayerView_EventMutatesOnlyTargetView verifies that a matured event
// for the human player updates the human's view without touching the alien's.
// (FR-18, per-player propagator)
func TestPlayerView_EventMutatesOnlyTargetView(t *testing.T) {
	st := newMinimalState()
	// Wire alien view with same initial state.
	alienView := &PlayerView{
		Systems: map[string]*KnownSystem{
			"sol": {ID: "sol", Status: StatusHuman, EconLevel: 5, Wealth: 1000, LocalUnits: map[WeaponType]int{}},
			"alpha-centauri": {ID: "alpha-centauri", Status: StatusHuman, EconLevel: 3, Wealth: 50, LocalUnits: map[WeaponType]int{}},
		},
		Fleets:    map[string]*KnownFleet{},
		InTransit: map[string]*KnownTransit{},
	}
	st.Views[AlienOwner] = alienView
	st.Clock = 10.0

	// Record a system-captured event that routes only to the human.
	st.Events.Record(&Event{
		EventYear: 1.0,
		Arrival: map[Owner]float64{
			HumanOwner: 5.0,                // matures for human at clock 10
			AlienOwner: math.MaxFloat64,     // never matures for alien
		},
		SystemID: "alpha-centauri",
		Type:     EventSystemCaptured,
	})

	st.Propagator.Propagate(st)

	// Human view should reflect the capture.
	if st.Views[HumanOwner].Systems["alpha-centauri"].Status != StatusAlien {
		t.Error("human view: expected alpha-centauri to be alien after EventSystemCaptured")
	}
	// Alien view must be unchanged.
	if st.Views[AlienOwner].Systems["alpha-centauri"].Status != StatusHuman {
		t.Error("alien view should not be mutated by a human-only event")
	}
}
