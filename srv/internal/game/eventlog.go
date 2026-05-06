package game

import (
	"container/heap"
	"fmt"
	"math"
)

// Event is the single propagation primitive. Every change to a PlayerView
// is the result of applying a matured Event whose Arrival[player] <= engine
// clock. Internal events (e.g. EventCombatSilent) set Internal=true; they
// never enter any heap, are never broadcast, and are never applied to
// any view.
//
// Per-player arrival times (FR-13): math.MaxFloat64 means "never reportable
// to that player." Per-player AppliedToView/Broadcast flags (FR-13) record
// which players have already received the event.
type Event struct {
	ID          string
	EventYear   float64
	SystemID    string
	Type        EventType
	Description string
	Details     interface{}
	Internal    bool

	// Per-player arrival year. Both keys must be present for non-internal
	// events; missing key is treated as math.MaxFloat64. (FR-13)
	Arrival map[Owner]float64

	// Per-player flags; lazily initialised by Record. (FR-13)
	AppliedToView map[Owner]bool
	Broadcast     map[Owner]bool

	// seqNo is the record-order sequence number, used as the secondary
	// heap-ordering key so events with identical Arrival[player] are
	// returned in record order. (NFR-2 determinism.)
	seqNo int
}

// EventLog stores events in chronological record order and keeps one
// per-player min-heap of unmatured reportable events for efficient
// per-player maturation. (FR-15, NFR-2)
type EventLog struct {
	All      []*Event            // chronological by record time
	BySystem map[string][]*Event // for "events at this system" queries

	// Per-player min-heap by Arrival[player]. Only unmatured non-internal
	// events with a finite Arrival[player] are pushed.
	pending map[Owner]*eventHeap

	nextID int
}

// NewEventLog returns an empty EventLog ready for Record.
func NewEventLog() *EventLog {
	pending := map[Owner]*eventHeap{
		HumanOwner: {player: HumanOwner},
		AlienOwner: {player: AlienOwner},
	}
	for _, h := range pending {
		heap.Init(h)
	}
	return &EventLog{
		BySystem: map[string][]*Event{},
		pending:  pending,
	}
}

// Record appends e to the log, assigning an ID and seqNo if not set, and
// pushes it onto each player's maturation heap whose Arrival entry is
// finite. Internal events are recorded but not pushed to any heap.
// Caller must hold the engine write lock.
func (l *EventLog) Record(e *Event) {
	l.nextID++
	if e.ID == "" {
		e.ID = fmt.Sprintf("evt-%d", l.nextID)
	}
	e.seqNo = l.nextID

	if e.AppliedToView == nil {
		e.AppliedToView = map[Owner]bool{HumanOwner: false, AlienOwner: false}
	}
	if e.Broadcast == nil {
		e.Broadcast = map[Owner]bool{HumanOwner: false, AlienOwner: false}
	}

	l.All = append(l.All, e)
	l.BySystem[e.SystemID] = append(l.BySystem[e.SystemID], e)

	if e.Internal || e.Arrival == nil {
		return
	}
	for player, h := range l.pending {
		if e.Arrival[player] < math.MaxFloat64 {
			heap.Push(h, e)
		}
	}
}

// PopMatured pops every event whose Arrival[player] <= clock from the
// player-specific heap, in non-decreasing Arrival[player] order (with
// record order as the tiebreaker for determinism, NFR-2). Caller is
// responsible for marking each returned event AppliedToView[player] and
// Broadcast[player] as appropriate.
func (l *EventLog) PopMatured(clock float64, player Owner) []*Event {
	h, ok := l.pending[player]
	if !ok {
		return nil
	}
	var out []*Event
	for h.Len() > 0 {
		top := h.items[0]
		if top.Arrival[player] > clock {
			break
		}
		out = append(out, heap.Pop(h).(*Event))
	}
	return out
}

// --- per-player min-heap by Arrival[player] (NFR-2) ---

type eventHeap struct {
	items  []*Event
	player Owner
}

func (h *eventHeap) Len() int { return len(h.items) }
func (h *eventHeap) Less(i, j int) bool {
	ai := h.items[i].Arrival[h.player]
	aj := h.items[j].Arrival[h.player]
	if ai != aj {
		return ai < aj
	}
	return h.items[i].seqNo < h.items[j].seqNo
}
func (h *eventHeap) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *eventHeap) Push(x interface{}) { h.items = append(h.items, x.(*Event)) }
func (h *eventHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	x := old[n-1]
	h.items = old[:n-1]
	return x
}
