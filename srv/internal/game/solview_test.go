package game

import (
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
		ArrivalYear: 7.0, // < Clock, so it will mature
		SystemID:    "alpha-centauri",
		Type:        EventFleetArrival,
		Details:     details,
	})

	st.Propagator.Propagate(st)

	// SolView fleet must exist and snapshot the units.
	kf := st.SolView.Fleet(fid)
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
	ks := st.SolView.System("alpha-centauri")
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
	st.SolView.InTransit[fid] = &KnownTransit{
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
		ArrivalYear: 10.0,
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

	if _, present := st.SolView.InTransit[fid]; present {
		t.Errorf("expected SolView.InTransit[%q] to be removed after FleetArrival", fid)
	}
	if st.SolView.Fleet(fid) == nil {
		t.Errorf("expected SolView.Fleets[%q] to be populated after FleetArrival", fid)
	}
}
