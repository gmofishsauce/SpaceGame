package game

import (
	"testing"
)

// TestPropagator_ConstructionDone_NonMobile verifies that a non-mobile
// construction event updates KnownSystem.LocalUnits and decrements
// KnownSystem.Wealth by def.Cost*Quantity (DR-6).
func TestPropagator_ConstructionDone_NonMobile(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.Wealth = 100

	const qty = 3
	st.Events.Record(&Event{
		EventYear:   5.0,
		Arrival: humanArrival(10.0),
		SystemID:    "alpha-centauri",
		Type:        EventConstructionDone,
		Details:     &ConstructionDetails{WeaponType: WeaponOrbitalDefense, Quantity: qty},
	})

	st.Propagator.Propagate(st)

	if got := ks.LocalUnits[WeaponOrbitalDefense]; got != qty {
		t.Errorf("LocalUnits[orbital_defense]=%d, want %d", got, qty)
	}
	wantWealth := 100 - WeaponDefs[WeaponOrbitalDefense].Cost*float64(qty)
	if ks.Wealth != wantWealth {
		t.Errorf("Wealth=%.2f, want %.2f", ks.Wealth, wantWealth)
	}
}

// TestPropagator_ConstructionDone_MobileExistingFleet verifies that a
// mobile construction event referencing an existing KnownFleet adds units
// to that fleet, advances its AsOfYear, and decrements Wealth.
func TestPropagator_ConstructionDone_MobileExistingFleet(t *testing.T) {
	st := newMinimalState()
	st.Clock = 30.0
	remote := st.truth.Systems["alpha-centauri"]
	primary := addFleet(st, remote, HumanOwner, map[WeaponType]int{WeaponEscort: 1})
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.Wealth = 200

	const qty = 2
	const eventYear = 12.0
	st.Events.Record(&Event{
		EventYear:   eventYear,
		Arrival: humanArrival(16.0),
		SystemID:    "alpha-centauri",
		Type:        EventConstructionDone,
		Details: &ConstructionDetails{
			WeaponType: WeaponBattleship,
			Quantity:   qty,
			FleetID:    primary.ID,
			FleetName:  primary.Name,
		},
	})

	st.Propagator.Propagate(st)

	kf := st.Views[HumanOwner].Fleet(primary.ID)
	if kf == nil {
		t.Fatalf("expected KnownFleet %q to exist", primary.ID)
	}
	if got := kf.Units[WeaponBattleship]; got != qty {
		t.Errorf("KnownFleet.Units[battleship]=%d, want %d", got, qty)
	}
	if got := kf.Units[WeaponEscort]; got != 1 {
		t.Errorf("KnownFleet.Units[escort]=%d, want 1 (preserved)", got)
	}
	if kf.AsOfYear != eventYear {
		t.Errorf("KnownFleet.AsOfYear=%.2f, want %.2f", kf.AsOfYear, eventYear)
	}
	wantWealth := 200 - WeaponDefs[WeaponBattleship].Cost*float64(qty)
	if ks.Wealth != wantWealth {
		t.Errorf("Wealth=%.2f, want %.2f", ks.Wealth, wantWealth)
	}
}

// TestPropagator_ConstructionDone_MobileNewFleet verifies that a mobile
// construction whose Details.FleetID does not exist in SolView.Fleets
// causes the propagator to mint a new KnownFleet and append its ID to
// KnownSystem.FleetIDs.
func TestPropagator_ConstructionDone_MobileNewFleet(t *testing.T) {
	st := newMinimalState()
	st.Clock = 40.0
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.Wealth = 200
	beforeIDs := append([]string(nil), ks.FleetIDs...)

	const newFleetID = "fleet-new"
	const newFleetName = "Alpha Centauri-2nd Fleet"

	st.Events.Record(&Event{
		EventYear:   15.0,
		Arrival: humanArrival(20.0),
		SystemID:    "alpha-centauri",
		Type:        EventConstructionDone,
		Details: &ConstructionDetails{
			WeaponType: WeaponEscort,
			Quantity:   1,
			FleetID:    newFleetID,
			FleetName:  newFleetName,
		},
	})

	st.Propagator.Propagate(st)

	kf := st.Views[HumanOwner].Fleet(newFleetID)
	if kf == nil {
		t.Fatalf("expected new KnownFleet %q to be created", newFleetID)
	}
	if kf.Name != newFleetName {
		t.Errorf("KnownFleet.Name=%q, want %q", kf.Name, newFleetName)
	}
	if kf.LocationID != "alpha-centauri" {
		t.Errorf("KnownFleet.LocationID=%q, want alpha-centauri", kf.LocationID)
	}
	if got := kf.Units[WeaponEscort]; got != 1 {
		t.Errorf("KnownFleet.Units[escort]=%d, want 1", got)
	}

	// FleetIDs must include the new fleet (and not double-count any prior IDs).
	wantLen := len(beforeIDs) + 1
	if len(ks.FleetIDs) != wantLen {
		t.Errorf("FleetIDs len=%d, want %d", len(ks.FleetIDs), wantLen)
	}
	found := false
	for _, id := range ks.FleetIDs {
		if id == newFleetID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected FleetIDs to contain %q, got %v", newFleetID, ks.FleetIDs)
	}
}

// TestPropagator_FleetDeparted verifies that an EventFleetDeparted removes
// the fleet from the source KnownSystem's FleetIDs, removes it from
// SolView.Fleets, and inserts a KnownTransit into SolView.InTransit.
func TestPropagator_FleetDeparted(t *testing.T) {
	st := newMinimalState()
	st.Clock = 30.0
	remote := st.truth.Systems["alpha-centauri"]
	tf := addFleet(st, remote, HumanOwner, map[WeaponType]int{WeaponEscort: 2})

	st.Events.Record(&Event{
		EventYear:   10.0,
		Arrival: humanArrival(15.0),
		SystemID:    "alpha-centauri",
		Type:        EventFleetDeparted,
		Details: &FleetDepartureDetails{
			FleetID:     tf.ID,
			FleetName:   tf.Name,
			Owner:       HumanOwner,
			Units:       copyUnits(tf.Units),
			SourceID:    "alpha-centauri",
			DestID:      "sol",
			DepartYear:  10.0,
			ArrivalYear: 15.46, // arbitrary; not the same as Event.ArrivalYear
		},
	})

	st.Propagator.Propagate(st)

	// Source system's FleetIDs should no longer list the fleet.
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	for _, id := range ks.FleetIDs {
		if id == tf.ID {
			t.Errorf("source KnownSystem.FleetIDs still contains %q after departure", tf.ID)
		}
	}
	// SolView.Fleets no longer contains the stationed entry.
	if st.Views[HumanOwner].Fleet(tf.ID) != nil {
		t.Errorf("SolView.Fleets still contains %q after departure", tf.ID)
	}
	// SolView.InTransit lists the fleet.
	tr, ok := st.Views[HumanOwner].InTransit[tf.ID]
	if !ok {
		t.Fatalf("expected SolView.InTransit[%q] after departure", tf.ID)
	}
	if tr.SourceID != "alpha-centauri" || tr.DestID != "sol" {
		t.Errorf("KnownTransit endpoints = %s -> %s, want alpha-centauri -> sol", tr.SourceID, tr.DestID)
	}
	if got := tr.Units[WeaponEscort]; got != 2 {
		t.Errorf("KnownTransit.Units[escort]=%d, want 2", got)
	}
}

// TestPropagator_CombatOccurred_FleetLostAllUnits verifies that an
// EventCombatOccurred whose HumanLosses zero out a fleet's mobile units
// causes that fleet to be removed from SolView.Fleets and from the
// system's FleetIDs.
func TestPropagator_CombatOccurred_FleetLostAllUnits(t *testing.T) {
	st := newMinimalState()
	st.Clock = 30.0
	remote := st.truth.Systems["alpha-centauri"]
	// One fleet with 2 escorts (the only mobile units at this system).
	tf := addFleet(st, remote, HumanOwner, map[WeaponType]int{WeaponEscort: 2})

	st.Events.Record(&Event{
		EventYear:   10.0,
		Arrival: humanArrival(14.0),
		SystemID:    "alpha-centauri",
		Type:        EventCombatOccurred,
		Details: &CombatDetails{
			HumanLosses: map[WeaponType]int{WeaponEscort: 2},
			AlienLosses: map[WeaponType]int{WeaponBattleship: 1},
			AlienWon:    true,
		},
	})

	st.Propagator.Propagate(st)

	if st.Views[HumanOwner].Fleet(tf.ID) != nil {
		t.Errorf("expected fleet %q to be removed from SolView.Fleets after losing all units", tf.ID)
	}
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	for _, id := range ks.FleetIDs {
		if id == tf.ID {
			t.Errorf("KnownSystem.FleetIDs still contains %q after fleet wiped out", tf.ID)
		}
	}
	if ks.Status != StatusAlien {
		t.Errorf("KnownSystem.Status=%q after AlienWon, want alien", ks.Status)
	}
}

// TestPropagator_CombatOccurred_NonMobileLosses verifies that non-mobile
// human losses (e.g. orbital defenses destroyed) reduce KnownSystem.LocalUnits.
func TestPropagator_CombatOccurred_NonMobileLosses(t *testing.T) {
	st := newMinimalState()
	st.Clock = 30.0
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.LocalUnits[WeaponOrbitalDefense] = 5

	st.Events.Record(&Event{
		EventYear:   10.0,
		Arrival: humanArrival(14.0),
		SystemID:    "alpha-centauri",
		Type:        EventCombatOccurred,
		Details: &CombatDetails{
			HumanLosses: map[WeaponType]int{WeaponOrbitalDefense: 3},
			HumanWon:    true,
		},
	})

	st.Propagator.Propagate(st)

	if got := ks.LocalUnits[WeaponOrbitalDefense]; got != 2 {
		t.Errorf("LocalUnits[orbital_defense]=%d after losing 3 of 5, want 2", got)
	}
	if ks.Status != StatusHuman {
		t.Errorf("Status=%q after HumanWon, want human", ks.Status)
	}
}

// TestPropagator_SystemCaptured verifies KnownSystem.Status flips to alien.
func TestPropagator_SystemCaptured(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0

	st.Events.Record(&Event{
		EventYear:   5,
		Arrival: humanArrival(10),
		SystemID:    "alpha-centauri",
		Type:        EventSystemCaptured,
	})
	st.Propagator.Propagate(st)

	if got := st.Views[HumanOwner].Systems["alpha-centauri"].Status; got != StatusAlien {
		t.Errorf("Status=%q, want alien", got)
	}
}

// TestPropagator_SystemRetaken verifies KnownSystem.Status flips to human.
func TestPropagator_SystemRetaken(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0
	st.Views[HumanOwner].Systems["alpha-centauri"].Status = StatusAlien

	st.Events.Record(&Event{
		EventYear:   5,
		Arrival: humanArrival(10),
		SystemID:    "alpha-centauri",
		Type:        EventSystemRetaken,
	})
	st.Propagator.Propagate(st)

	if got := st.Views[HumanOwner].Systems["alpha-centauri"].Status; got != StatusHuman {
		t.Errorf("Status=%q, want human", got)
	}
}

// TestPropagator_SystemConquered verifies that a conquest event sets
// Status to human and resets EconLevel and Wealth to zero.
func TestPropagator_SystemConquered(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.Status = StatusUninhabited
	ks.EconLevel = 4
	ks.Wealth = 99

	st.Events.Record(&Event{
		EventYear:   5,
		Arrival: humanArrival(10),
		SystemID:    "alpha-centauri",
		Type:        EventSystemConquered,
	})
	st.Propagator.Propagate(st)

	if ks.Status != StatusHuman {
		t.Errorf("Status=%q, want human", ks.Status)
	}
	if ks.EconLevel != 0 {
		t.Errorf("EconLevel=%d, want 0", ks.EconLevel)
	}
	if ks.Wealth != 0 {
		t.Errorf("Wealth=%.2f, want 0", ks.Wealth)
	}
}

// TestPropagator_EconGrowth verifies that EventEconGrowth updates EconLevel.
func TestPropagator_EconGrowth(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.EconLevel = 2

	st.Events.Record(&Event{
		EventYear:   5,
		Arrival: humanArrival(10),
		SystemID:    "alpha-centauri",
		Type:        EventEconGrowth,
		Details:     &EconGrowthDetails{NewLevel: 3},
	})
	st.Propagator.Propagate(st)

	if ks.EconLevel != 3 {
		t.Errorf("EconLevel=%d, want 3", ks.EconLevel)
	}
}

// TestPropagator_CommandEventsAdvanceAsOfYearOnly verifies that
// EventCommandArrived / EventCommandExecuted / EventCommandFailed perform
// no SolView mutation other than advancing AsOfYear.
func TestPropagator_CommandEventsAdvanceAsOfYearOnly(t *testing.T) {
	st := newMinimalState()
	st.Clock = 20.0
	ks := st.Views[HumanOwner].Systems["alpha-centauri"]
	ks.Status = StatusHuman
	ks.EconLevel = 3
	ks.Wealth = 50
	ks.AsOfYear = 0

	st.Events.Record(&Event{
		EventYear:   8,
		Arrival: humanArrival(12),
		SystemID:    "alpha-centauri",
		Type:        EventCommandExecuted,
	})
	st.Propagator.Propagate(st)

	if ks.AsOfYear != 8 {
		t.Errorf("AsOfYear=%.2f, want 8", ks.AsOfYear)
	}
	if ks.Status != StatusHuman || ks.EconLevel != 3 || ks.Wealth != 50 {
		t.Errorf("unexpected SolView mutation: status=%q econ=%d wealth=%.1f",
			ks.Status, ks.EconLevel, ks.Wealth)
	}
}
