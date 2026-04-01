package game

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
)

// EventManager manages the SSE client registry and broadcasts matured events.
type EventManager struct {
	mu      sync.Mutex
	clients map[string]chan []byte // key: client ID, value: buffered channel
}

// NewEventManager creates a ready-to-use EventManager.
func NewEventManager() *EventManager {
	return &EventManager{
		clients: make(map[string]chan []byte),
	}
}

// Register adds an SSE client. Returns a receive-only channel that receives
// SSE-formatted frames (already formatted as "event: ...\ndata: ...\n\n").
func (m *EventManager) Register(clientID string) <-chan []byte {
	ch := make(chan []byte, 64)
	m.mu.Lock()
	m.clients[clientID] = ch
	m.mu.Unlock()
	return ch
}

// Unregister removes a disconnected SSE client and closes its channel.
func (m *EventManager) Unregister(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.clients[clientID]; ok {
		close(ch)
		delete(m.clients, clientID)
	}
}

// BroadcastMatured sends all newly-matured events (arrivalYear ≤ state.Clock,
// !Broadcast) and their system state updates to all registered clients.
// Must be called with state.mu held (engine tick holds it). (FR-017)
func (m *EventManager) BroadcastMatured(state *GameState) {
	for _, evt := range state.Events {
		if evt.Broadcast {
			continue
		}
		if evt.ArrivalYear > state.Clock || evt.ArrivalYear >= math.MaxFloat64 {
			continue
		}
		if evt.Type == EventCombatSilent || evt.Type == EventAlienSpawn {
			evt.Broadcast = true // mark internal-only events as handled
			continue
		}

		payload := sseFrame("game_event", eventToMap(evt))
		m.broadcastBytes(payload)
		evt.Broadcast = true

		// Also send updated known state for the event's system
		if sys, ok := state.Systems[evt.SystemID]; ok {
			sysPayload := sseFrame("system_update", systemToMap(sys))
			m.broadcastBytes(sysPayload)
		}
	}
}

// BroadcastClockSync sends a clock synchronisation event to all registered clients.
// Safe to call with state.mu held (reads only clock and paused).
func (m *EventManager) BroadcastClockSync(state *GameState) {
	payload := sseFrame("clock_sync", map[string]interface{}{
		"gameYear": state.Clock,
		"paused":   state.Paused,
	})
	m.broadcastBytes(payload)
}

// BroadcastGameOver sends the game-over event to all registered clients.
func (m *EventManager) BroadcastGameOver(winner Owner, reason string) {
	payload := sseFrame("game_over", map[string]interface{}{
		"winner": string(winner),
		"reason": reason,
	})
	m.broadcastBytes(payload)
}

// BroadcastConnected sends the full current state snapshot to a single client
// (called when the client first connects). state.mu must be held by caller.
func (m *EventManager) BroadcastConnected(clientID string, state *GameState) {
	m.mu.Lock()
	ch, ok := m.clients[clientID]
	m.mu.Unlock()
	if !ok {
		return
	}
	payload := sseFrame("connected", fullStateMap(state))
	safeSend(ch, payload)
}

// broadcastBytes sends payload to all registered client channels.
// If a client channel is full, the payload is dropped for that client.
// If sending to a closed channel panics, the client is unregistered.
func (m *EventManager) broadcastBytes(payload []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ch := range m.clients {
		if !safeSend(ch, payload) {
			log.Printf("events: client %s channel full or closed, dropping event", id)
		}
	}
}

// safeSend sends payload to ch without blocking. Returns false if the channel
// is full or if sending panics (closed channel).
func safeSend(ch chan []byte, payload []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	select {
	case ch <- payload:
		return true
	default:
		return false
	}
}

// --- SSE frame formatting ---

// sseFrame encodes eventType and data as a standard SSE text/event-stream frame.
func sseFrame(eventType string, data interface{}) []byte {
	b, err := json.Marshal(data)
	if err != nil {
		b = []byte(`{}`)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(b)))
}

// eventToMap converts a GameEvent to a map suitable for JSON encoding.
func eventToMap(evt *GameEvent) map[string]interface{} {
	m := map[string]interface{}{
		"id":          evt.ID,
		"arrivalYear": evt.ArrivalYear,
		"systemId":    evt.SystemID,
		"type":        string(evt.Type),
		"description": evt.Description,
	}
	if evt.Details != nil {
		m["details"] = evt.Details
	}
	return m
}

// systemToMap converts a system's known-state fields to a map for JSON encoding.
func systemToMap(sys *StarSystem) map[string]interface{} {
	knownUnits := map[string]int{}
	for wt, n := range sys.KnownLocalUnits {
		if n > 0 {
			knownUnits[string(wt)] = n
		}
	}
	return map[string]interface{}{
		"systemId":        sys.ID,
		"knownStatus":     string(sys.KnownStatus),
		"knownAsOfYear":   sys.KnownAsOfYear,
		"knownEconLevel":  sys.KnownEconLevel,
		"knownLocalUnits": knownUnits,
		"knownFleets":     sys.KnownFleetIDs,
	}
}

// fullStateMap builds the initial full-state snapshot for a newly connected client.
// (handleEvents uses this for the "connected" SSE event.)
func fullStateMap(state *GameState) map[string]interface{} {
	systems := make([]map[string]interface{}, 0, len(state.Systems))
	for _, id := range state.SystemOrder {
		sys := state.Systems[id]
		knownUnits := map[string]int{}
		for wt, n := range sys.KnownLocalUnits {
			if n > 0 {
				knownUnits[string(wt)] = n
			}
		}
		systems = append(systems, map[string]interface{}{
			"id":              sys.ID,
			"displayName":     sys.DisplayName,
			"x":               sys.X,
			"y":               sys.Y,
			"z":               sys.Z,
			"distFromSol":     sys.DistFromSol,
			"hasPlanets":      sys.HasPlanets,
			"isSol":           sys.ID == "sol",
			"knownStatus":     string(sys.KnownStatus),
			"knownAsOfYear":   sys.KnownAsOfYear,
			"knownEconLevel":  sys.KnownEconLevel,
			"knownLocalUnits": knownUnits,
			"knownFleets":     sys.KnownFleetIDs,
		})
	}

	events := make([]map[string]interface{}, 0)
	for _, evt := range state.Events {
		if !evt.Broadcast {
			continue
		}
		if evt.Type == EventCombatSilent || evt.Type == EventAlienSpawn {
			continue
		}
		events = append(events, eventToMap(evt))
	}

	return map[string]interface{}{
		"gameYear":  state.Clock,
		"paused":    state.Paused,
		"gameOver":  state.GameOver,
		"winner":    string(state.Winner),
		"winReason": state.WinReason,
		"systems":   systems,
		"events":    events,
	}
}
