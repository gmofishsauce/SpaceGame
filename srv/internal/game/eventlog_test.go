package game

import (
	"math"
	"testing"
)

// alienArrival returns an Arrival map with the given year for the alien player
// and math.MaxFloat64 (unreportable) for the human player.
func alienArrival(y float64) map[Owner]float64 {
	return map[Owner]float64{HumanOwner: math.MaxFloat64, AlienOwner: y}
}

// TestEventLog_RecordInternalNotInHeap verifies that an event with
// Internal=true is appended to All / BySystem but never enters the
// maturation heap, so it cannot mature and cannot be broadcast.
func TestEventLog_RecordInternalNotInHeap(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{
		EventYear:   0,
		Arrival: humanArrival(1.0),
		SystemID:    "sol",
		Type:        EventCombatSilent,
		Internal:    true,
	})
	if got := len(log.All); got != 1 {
		t.Errorf("expected All len=1, got %d", got)
	}
	if got := len(log.BySystem["sol"]); got != 1 {
		t.Errorf("expected BySystem[sol] len=1, got %d", got)
	}
	matured := log.PopMatured(1000.0, HumanOwner)
	if len(matured) != 0 {
		t.Errorf("internal event should never mature, got %d matured", len(matured))
	}
}

// TestEventLog_RecordUnreportableNotInHeap verifies that an event with
// ArrivalYear=MaxFloat64 (unreportable) is appended to All but never
// matures.
func TestEventLog_RecordUnreportableNotInHeap(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{
		EventYear:   0,
		Arrival: humanArrival(math.MaxFloat64),
		SystemID:    "alpha-centauri",
		Type:        EventCombatSilent,
	})
	matured := log.PopMatured(math.MaxFloat64, HumanOwner)
	if len(matured) != 0 {
		t.Errorf("unreportable event should never mature, got %d matured", len(matured))
	}
}

// TestEventLog_PopMaturedOrderByArrivalYear verifies that PopMatured
// returns events in non-decreasing ArrivalYear order even when they were
// recorded in non-monotonic order. This matters because a near system
// with a comm laser can produce a report whose ArrivalYear is earlier
// than a previously-recorded event from a more distant system.
func TestEventLog_PopMaturedOrderByArrivalYear(t *testing.T) {
	log := NewEventLog()

	// Record in non-monotonic order: distant first, then close.
	log.Record(&Event{
		EventYear:   1.0,
		Arrival: humanArrival(50.0),
		SystemID:    "far",
		Type:        EventConstructionDone,
	})
	log.Record(&Event{
		EventYear:   2.0,
		Arrival: humanArrival(5.0),
		SystemID:    "near",
		Type:        EventConstructionDone,
	})
	log.Record(&Event{
		EventYear:   3.0,
		Arrival: humanArrival(25.0),
		SystemID:    "mid",
		Type:        EventConstructionDone,
	})

	matured := log.PopMatured(100.0, HumanOwner)
	if len(matured) != 3 {
		t.Fatalf("expected 3 matured events, got %d", len(matured))
	}
	wantOrder := []float64{5.0, 25.0, 50.0}
	for i, evt := range matured {
		if evt.Arrival[HumanOwner] != wantOrder[i] {
			t.Errorf("matured[%d].ArrivalYear=%.1f, want %.1f", i, evt.Arrival[HumanOwner], wantOrder[i])
		}
	}
}

// TestEventLog_PopMaturedRespectsClock verifies that events with
// ArrivalYear > clock remain in the heap and are returned by a later
// PopMatured call once the clock has advanced.
func TestEventLog_PopMaturedRespectsClock(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{Arrival: humanArrival(5.0), SystemID: "a", Type: EventConstructionDone})
	log.Record(&Event{Arrival: humanArrival(15.0), SystemID: "b", Type: EventConstructionDone})

	first := log.PopMatured(10.0, HumanOwner)
	if len(first) != 1 || first[0].Arrival[HumanOwner] != 5.0 {
		t.Fatalf("first PopMatured: expected 1 event at Arrival=5, got %v", first)
	}
	// Second event still pending.
	stillPending := log.PopMatured(10.0, HumanOwner)
	if len(stillPending) != 0 {
		t.Errorf("expected 0 events at clock=10 after first pop, got %d", len(stillPending))
	}
	// Advance clock; second event matures.
	second := log.PopMatured(20.0, HumanOwner)
	if len(second) != 1 || second[0].Arrival[HumanOwner] != 15.0 {
		t.Errorf("second PopMatured: expected 1 event at Arrival=15, got %v", second)
	}
}

// TestEventLog_RecordAssignsID verifies Record auto-assigns an ID when one
// is not supplied, and leaves an existing ID untouched.
func TestEventLog_RecordAssignsID(t *testing.T) {
	log := NewEventLog()
	e1 := &Event{Arrival: humanArrival(1), SystemID: "x", Type: EventConstructionDone}
	log.Record(e1)
	if e1.ID == "" {
		t.Error("expected Record to assign an ID, got empty")
	}
	preset := &Event{ID: "preset-id", Arrival: humanArrival(2), SystemID: "y", Type: EventConstructionDone}
	log.Record(preset)
	if preset.ID != "preset-id" {
		t.Errorf("expected preset ID retained, got %q", preset.ID)
	}
}

// TestEventLog_PerPlayerHeaps verifies that when Arrival[Human] < Arrival[Alien],
// popping the human heap at a clock between the two arrival times returns the
// event only for the human player, not the alien player. (M3, FR-15)
func TestEventLog_PerPlayerHeaps(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{
		Arrival: map[Owner]float64{
			HumanOwner: 10.0,
			AlienOwner: 30.0,
		},
		SystemID: "mid",
		Type:     EventConstructionDone,
	})

	// Clock = 20: human arrival has matured; alien arrival has not.
	humanMatured := log.PopMatured(20.0, HumanOwner)
	if len(humanMatured) != 1 {
		t.Fatalf("expected 1 event matured for human at clock=20, got %d", len(humanMatured))
	}
	alienMatured := log.PopMatured(20.0, AlienOwner)
	if len(alienMatured) != 0 {
		t.Errorf("expected 0 events matured for alien at clock=20, got %d", len(alienMatured))
	}

	// Clock = 35: alien arrival now matured.
	alienMatured2 := log.PopMatured(35.0, AlienOwner)
	if len(alienMatured2) != 1 {
		t.Errorf("expected 1 event matured for alien at clock=35, got %d", len(alienMatured2))
	}
}

// TestEventLog_AlienOnlyEvent verifies that an event unreportable to humans
// (Arrival[Human]=MaxFloat64) appears only on the alien's stream, never the human's.
func TestEventLog_AlienOnlyEvent(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{
		Arrival:  alienArrival(5.0),
		SystemID: "remote",
		Type:     EventConstructionDone,
	})

	humanMatured := log.PopMatured(100.0, HumanOwner)
	if len(humanMatured) != 0 {
		t.Errorf("alien-only event must not appear in human heap, got %d", len(humanMatured))
	}
	alienMatured := log.PopMatured(100.0, AlienOwner)
	if len(alienMatured) != 1 {
		t.Errorf("alien-only event must appear in alien heap, got %d", len(alienMatured))
	}
}
