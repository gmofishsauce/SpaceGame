package game

import (
	"reflect"
	"sort"
	"testing"
)

// TestProperty_ReplayMatchesEngineSolView is the architectural-review
// property test (review item J / design §11.2). It establishes that
// every piece of state in SolView at end-of-game is derivable purely
// from (a) the initial SolView and (b) the sequence of broadcast Events.
//
// Procedure:
//   1. Build a small multi-event scenario with deterministic seeds.
//   2. Snapshot the initial SolView.
//   3. Run the engine for ~50 game-years.
//   4. Replay the broadcast events from the snapshotted initial SolView
//      using applyEventToView (the same code the engine used the first
//      time around).
//   5. Assert reflect.DeepEqual between the engine's final SolView and
//      the replay's SolView.
//
// A failure of this assertion indicates either:
//   - a leak: state in SolView that was set outside of applyEventToView
//     (e.g. a mutation path the engine still exercises directly), or
//   - a gap: events not applied symmetrically between the engine's
//     propagation and a clean replay.
func TestProperty_ReplayMatchesEngineSolView(t *testing.T) {
	st, e, _ := twoSystemSetup(t, 5.0)

	// Issue a varied command sequence so multiple EventType cases fire
	// during the run: a non-mobile build, a mobile build, and a fleet
	// move. All target the remote system so the events propagate at
	// finite speed.
	cmds := []*PendingCommand{
		{TargetID: "remote", Type: CmdConstruct, WeaponType: WeaponOrbitalDefense, Quantity: 2, Issuer: HumanOwner},
		{TargetID: "remote", Type: CmdConstruct, WeaponType: WeaponBattleship, Quantity: 1, Issuer: HumanOwner},
	}
	for _, c := range cmds {
		if _, _, err := e.EnqueueCommand(c); err != nil {
			t.Fatalf("EnqueueCommand(%+v): %v", c, err)
		}
	}

	// Snapshot the initial SolView before any events propagate.
	initialView := cloneSolView(st.Views[HumanOwner])

	// Run for ~50 game-years. With CommandSpeedC=1 and dist=5, commands
	// arrive at year 5 and reports return to Sol by year 10; well within
	// the run window so we exercise full round-trips.
	tickUntil(t, st, e, 50.0)

	// Issue a move-back command after some construction has accumulated,
	// so an EventFleetDeparted is also exercised. Find the truth-side
	// fleet that holds the battleship at remote.
	var primaryFleetID string
	for fid, f := range st.truth.Fleets {
		if f.Owner == HumanOwner && f.LocationID == "remote" && f.Units[WeaponBattleship] > 0 {
			primaryFleetID = fid
			break
		}
	}
	if primaryFleetID == "" {
		t.Fatal("expected a human fleet with a battleship at remote after run-in window")
	}
	moveCmd := &PendingCommand{
		TargetID: "remote",
		Type:     CmdMove,
		FleetID:  primaryFleetID,
		DestID:   "sol",
		Issuer:   HumanOwner,
	}
	if _, _, err := e.EnqueueCommand(moveCmd); err != nil {
		t.Fatalf("EnqueueCommand(move): %v", err)
	}

	// Run another 30 years so the move command arrives, the fleet
	// departs, and the EventFleetDeparted matures back at Sol.
	tickUntil(t, st, e, 80.0)

	// Engine final SolView.
	engineView := st.Views[HumanOwner]

	// Replay broadcast events into a clean SolView starting from the
	// snapshotted initial state. Sort by ArrivalYear with a stable
	// tiebreak by record-order; that mirrors the propagator's heap-pop
	// ordering for the only thing the heap uses (ArrivalYear), and
	// preserves the recording order for ties (which heap.Pop does not
	// guarantee but which corresponds to the engine's deterministic
	// record sequence under a fixed RNG).
	events := append([]*Event(nil), st.Events.All...)
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Arrival[HumanOwner] < events[j].Arrival[HumanOwner]
	})

	replayView := cloneSolView(initialView)
	prop := NewPropagator(NewEventManager())
	applied := 0
	for _, evt := range events {
		if !evt.Broadcast[HumanOwner] {
			continue
		}
		prop.applyEventToView(replayView, st.Catalog, evt, HumanOwner)
		applied++
	}
	if applied == 0 {
		t.Fatal("no broadcast events were available to replay; test scenario is degenerate")
	}

	if !reflect.DeepEqual(engineView, replayView) {
		// Provide a concise diagnostic.
		t.Errorf("replay SolView != engine SolView after %d broadcast events", applied)
		diffSolView(t, "engine", engineView, "replay", replayView)
	} else {
		t.Logf("replayed %d broadcast events; SolView matches", applied)
	}
}

// cloneSolView returns a deep copy of v. All pointed-to substructure is
// fresh; mutating the clone does not affect the original.
func cloneSolView(v *PlayerView) *PlayerView {
	if v == nil {
		return nil
	}
	out := &PlayerView{
		Systems:   make(map[string]*KnownSystem, len(v.Systems)),
		Fleets:    make(map[string]*KnownFleet, len(v.Fleets)),
		InTransit: make(map[string]*KnownTransit, len(v.InTransit)),
	}
	for id, ks := range v.Systems {
		out.Systems[id] = &KnownSystem{
			ID:         ks.ID,
			Status:     ks.Status,
			AsOfYear:   ks.AsOfYear,
			EconLevel:  ks.EconLevel,
			Wealth:     ks.Wealth,
			LocalUnits: copyUnits(ks.LocalUnits),
			FleetIDs:   append([]string(nil), ks.FleetIDs...),
		}
	}
	for id, kf := range v.Fleets {
		out.Fleets[id] = &KnownFleet{
			ID:         kf.ID,
			Name:       kf.Name,
			Owner:      kf.Owner,
			Units:      copyUnits(kf.Units),
			LocationID: kf.LocationID,
			AsOfYear:   kf.AsOfYear,
		}
	}
	for id, kt := range v.InTransit {
		out.InTransit[id] = &KnownTransit{
			FleetID:     kt.FleetID,
			Name:        kt.Name,
			Owner:       kt.Owner,
			Units:       copyUnits(kt.Units),
			SourceID:    kt.SourceID,
			DestID:      kt.DestID,
			DepartYear:  kt.DepartYear,
			ArrivalYear: kt.ArrivalYear,
			AsOfYear:    kt.AsOfYear,
		}
	}
	return out
}

// diffSolView prints a summary of differences between two SolViews.
// Used for diagnostics when the property assertion fails.
func diffSolView(t *testing.T, nameA string, a *PlayerView, nameB string, b *PlayerView) {
	t.Helper()
	for id, sa := range a.Systems {
		sb := b.Systems[id]
		if sb == nil {
			t.Logf("system %q only in %s", id, nameA)
			continue
		}
		if !reflect.DeepEqual(sa, sb) {
			t.Logf("system %q differs:", id)
			t.Logf("  %s: %+v", nameA, sa)
			t.Logf("  %s: %+v", nameB, sb)
		}
	}
	for id := range b.Systems {
		if _, ok := a.Systems[id]; !ok {
			t.Logf("system %q only in %s", id, nameB)
		}
	}
	for id, fa := range a.Fleets {
		fb := b.Fleets[id]
		if fb == nil {
			t.Logf("fleet %q only in %s", id, nameA)
			continue
		}
		if !reflect.DeepEqual(fa, fb) {
			t.Logf("fleet %q differs:", id)
			t.Logf("  %s: %+v", nameA, fa)
			t.Logf("  %s: %+v", nameB, fb)
		}
	}
	for id := range b.Fleets {
		if _, ok := a.Fleets[id]; !ok {
			t.Logf("fleet %q only in %s", id, nameB)
		}
	}
	for id, ta := range a.InTransit {
		tb := b.InTransit[id]
		if tb == nil {
			t.Logf("inTransit %q only in %s", id, nameA)
			continue
		}
		if !reflect.DeepEqual(ta, tb) {
			t.Logf("inTransit %q differs:", id)
			t.Logf("  %s: %+v", nameA, ta)
			t.Logf("  %s: %+v", nameB, tb)
		}
	}
	for id := range b.InTransit {
		if _, ok := a.InTransit[id]; !ok {
			t.Logf("inTransit %q only in %s", id, nameB)
		}
	}
}
