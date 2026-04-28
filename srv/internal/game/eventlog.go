package game

import (
	"container/heap"
	"fmt"
	"math"
)

// Event is the single propagation primitive. Every change to SolView is the
// result of applying a matured Event whose ArrivalYear <= engine clock.
//
// Event replaces the older GameEvent during the dual-state refactor. The
// CanReport field of GameEvent is gone -- it was redundant with
// "ArrivalYear < math.MaxFloat64". Internal events (e.g. EventAlienSpawn,
// EventCombatSilent) set Internal=true; they never enter the heap, are
// never broadcast, and are never applied to SolView.
type Event struct {
	ID            string
	EventYear     float64
	ArrivalYear   float64 // math.MaxFloat64 if never reportable
	SystemID      string
	Type          EventType
	Description   string
	Details       interface{}
	Internal      bool // true: never broadcast, never applied to view
	Broadcast     bool // true once SSE-sent
	AppliedToView bool // true once applied to SolView
}

// EventLog stores events in chronological record order and keeps an
// ArrivalYear-indexed min-heap of unmatured reportable events for efficient
// maturation (DR-10).
type EventLog struct {
	All      []*Event            // chronological by record time
	BySystem map[string][]*Event // for "events at this system" queries

	pending *eventHeap // min-heap by ArrivalYear; only unmatured + non-internal

	nextID int
}

// NewEventLog returns an empty EventLog ready for Record.
func NewEventLog() *EventLog {
	h := &eventHeap{}
	heap.Init(h)
	return &EventLog{
		BySystem: map[string][]*Event{},
		pending:  h,
	}
}

// Record appends e to the log, assigning an ID if not set, and pushes it
// onto the maturation heap if it is reportable (non-internal and has a
// finite ArrivalYear). Caller must hold the engine write lock.
func (l *EventLog) Record(e *Event) {
	if e.ID == "" {
		l.nextID++
		e.ID = fmt.Sprintf("evt-%d", l.nextID)
	}
	l.All = append(l.All, e)
	l.BySystem[e.SystemID] = append(l.BySystem[e.SystemID], e)
	if !e.Internal && e.ArrivalYear < math.MaxFloat64 {
		heap.Push(l.pending, e)
	}
}

// PopMatured pops every event whose ArrivalYear <= clock, in non-decreasing
// ArrivalYear order. Caller is responsible for marking each returned event
// AppliedToView and Broadcast as appropriate.
func (l *EventLog) PopMatured(clock float64) []*Event {
	var out []*Event
	for l.pending.Len() > 0 {
		top := (*l.pending)[0]
		if top.ArrivalYear > clock {
			break
		}
		out = append(out, heap.Pop(l.pending).(*Event))
	}
	return out
}

// --- internal min-heap by ArrivalYear ---

type eventHeap []*Event

func (h eventHeap) Len() int            { return len(h) }
func (h eventHeap) Less(i, j int) bool  { return h[i].ArrivalYear < h[j].ArrivalYear }
func (h eventHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x interface{}) { *h = append(*h, x.(*Event)) }
func (h *eventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
