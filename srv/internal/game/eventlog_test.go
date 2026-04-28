package game

import (
	"math"
	"testing"
)

// TestEventLog_RecordInternalNotInHeap verifies that an event with
// Internal=true is appended to All / BySystem but never enters the
// maturation heap, so it cannot mature and cannot be broadcast.
func TestEventLog_RecordInternalNotInHeap(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{
		EventYear:   0,
		ArrivalYear: 1.0,
		SystemID:    "sol",
		Type:        EventAlienSpawn,
		Internal:    true,
	})
	if got := len(log.All); got != 1 {
		t.Errorf("expected All len=1, got %d", got)
	}
	if got := len(log.BySystem["sol"]); got != 1 {
		t.Errorf("expected BySystem[sol] len=1, got %d", got)
	}
	matured := log.PopMatured(1000.0)
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
		ArrivalYear: math.MaxFloat64,
		SystemID:    "alpha-centauri",
		Type:        EventCombatSilent,
	})
	matured := log.PopMatured(math.MaxFloat64)
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
		ArrivalYear: 50.0,
		SystemID:    "far",
		Type:        EventConstructionDone,
	})
	log.Record(&Event{
		EventYear:   2.0,
		ArrivalYear: 5.0,
		SystemID:    "near",
		Type:        EventConstructionDone,
	})
	log.Record(&Event{
		EventYear:   3.0,
		ArrivalYear: 25.0,
		SystemID:    "mid",
		Type:        EventConstructionDone,
	})

	matured := log.PopMatured(100.0)
	if len(matured) != 3 {
		t.Fatalf("expected 3 matured events, got %d", len(matured))
	}
	wantOrder := []float64{5.0, 25.0, 50.0}
	for i, evt := range matured {
		if evt.ArrivalYear != wantOrder[i] {
			t.Errorf("matured[%d].ArrivalYear=%.1f, want %.1f", i, evt.ArrivalYear, wantOrder[i])
		}
	}
}

// TestEventLog_PopMaturedRespectsClock verifies that events with
// ArrivalYear > clock remain in the heap and are returned by a later
// PopMatured call once the clock has advanced.
func TestEventLog_PopMaturedRespectsClock(t *testing.T) {
	log := NewEventLog()
	log.Record(&Event{ArrivalYear: 5.0, SystemID: "a", Type: EventConstructionDone})
	log.Record(&Event{ArrivalYear: 15.0, SystemID: "b", Type: EventConstructionDone})

	first := log.PopMatured(10.0)
	if len(first) != 1 || first[0].ArrivalYear != 5.0 {
		t.Fatalf("first PopMatured: expected 1 event at ArrivalYear=5, got %v", first)
	}
	// Second event still pending.
	stillPending := log.PopMatured(10.0)
	if len(stillPending) != 0 {
		t.Errorf("expected 0 events at clock=10 after first pop, got %d", len(stillPending))
	}
	// Advance clock; second event matures.
	second := log.PopMatured(20.0)
	if len(second) != 1 || second[0].ArrivalYear != 15.0 {
		t.Errorf("second PopMatured: expected 1 event at ArrivalYear=15, got %v", second)
	}
}

// TestEventLog_RecordAssignsID verifies Record auto-assigns an ID when one
// is not supplied, and leaves an existing ID untouched.
func TestEventLog_RecordAssignsID(t *testing.T) {
	log := NewEventLog()
	e1 := &Event{ArrivalYear: 1, SystemID: "x", Type: EventConstructionDone}
	log.Record(e1)
	if e1.ID == "" {
		t.Error("expected Record to assign an ID, got empty")
	}
	preset := &Event{ID: "preset-id", ArrivalYear: 2, SystemID: "y", Type: EventConstructionDone}
	log.Record(preset)
	if preset.ID != "preset-id" {
		t.Errorf("expected preset ID retained, got %q", preset.ID)
	}
}
