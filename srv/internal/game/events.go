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
			sysPayload := sseFrame("system_update", systemToMap(state, sys))
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
func systemToMap(state *GameState, sys *StarSystem) map[string]interface{} {
	knownUnits := map[string]int{}
	for wt, n := range sys.KnownLocalUnits {
		if n > 0 {
			knownUnits[string(wt)] = n
		}
	}
	wealth := sys.KnownWealth
	if sys.ID == "sol" {
		wealth = sys.Wealth // Sol: always ground truth (FR-023)
	}
	return map[string]interface{}{
		"systemId":        sys.ID,
		"knownStatus":     string(sys.KnownStatus),
		"knownAsOfYear":   sys.KnownAsOfYear,
		"knownEconLevel":  sys.KnownEconLevel,
		"knownWealth":     wealth,
		"knownLocalUnits": knownUnits,
		"knownFleets":     buildKnownFleets(state, sys),
	}
}

// buildKnownFleets returns player-visible fleet objects for a system.
// Sol uses ground-truth FleetIDs; all other systems use KnownFleetIDs.
func buildKnownFleets(state *GameState, sys *StarSystem) []map[string]interface{} {
	fleetIDs := sys.KnownFleetIDs
	if sys.ID == "sol" {
		fleetIDs = sys.FleetIDs
	}
	result := []map[string]interface{}{}
	for _, fid := range fleetIDs {
		f := state.Fleets[fid]
		if f != nil && f.Owner == HumanOwner {
			result = append(result, fleetToMap(f))
		}
	}
	return result
}

// fleetToMap converts a Fleet to a map suitable for JSON encoding.
func fleetToMap(f *Fleet) map[string]interface{} {
	units := map[string]int{}
	for wt, n := range f.Units {
		if n > 0 {
			units[string(wt)] = n
		}
	}
	return map[string]interface{}{
		"id":            f.ID,
		"name":          f.Name,
		"owner":         string(f.Owner),
		"units":         units,
		"inTransit":     f.InTransit,
		"sourceId":      f.SourceID,
		"destinationId": f.DestID,
		"departYear":    f.DepartYear,
		"arrivalYear":   f.ArrivalYear,
	}
}

// pendingCommandToMap converts a PendingCommand to a map for JSON encoding,
// including a server-formed hover description.
func pendingCommandToMap(state *GameState, cmd *PendingCommand) map[string]interface{} {
	return map[string]interface{}{
		"id":          cmd.ID,
		"type":        string(cmd.Type),
		"originId":    cmd.OriginID,
		"targetId":    cmd.TargetID,
		"executeYear": cmd.ExecuteYear,
		"description": describePendingCommandLocal(state, cmd),
	}
}

// describePendingCommandLocal formats hover text for an in-flight command.
// Kept in the game package for the SSE path; mirrors the REST describePendingCommand.
func describePendingCommandLocal(state *GameState, cmd *PendingCommand) string {
	targetName := cmd.TargetID
	if sys, ok := state.Systems[cmd.TargetID]; ok {
		targetName = sys.DisplayName
	}
	switch cmd.Type {
	case CmdConstruct:
		return fmt.Sprintf("Construct %d %s at %s (executes yr %.1f)",
			cmd.Quantity, cmd.WeaponType, targetName, cmd.ExecuteYear)
	case CmdMove:
		fleetName := cmd.FleetID
		if f, ok := state.Fleets[cmd.FleetID]; ok {
			fleetName = f.Name
		}
		destName := cmd.DestID
		if sys, ok := state.Systems[cmd.DestID]; ok {
			destName = sys.DisplayName
		}
		return fmt.Sprintf("Order: Move %s to %s (arrives yr %.1f)",
			fleetName, destName, cmd.ExecuteYear)
	default:
		return fmt.Sprintf("Command %s to %s (arrives yr %.1f)",
			cmd.Type, targetName, cmd.ExecuteYear)
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
		wealth := sys.KnownWealth
		if sys.ID == "sol" {
			wealth = sys.Wealth // Sol: always ground truth (FR-023)
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
			"knownWealth":     wealth,
			"knownLocalUnits": knownUnits,
			"knownFleets":     buildKnownFleets(state, sys),
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

	pendingCommands := make([]map[string]interface{}, 0, len(state.PendingCmds))
	for _, cmd := range state.PendingCmds {
		if cmd.IsBot {
			continue
		}
		pendingCommands = append(pendingCommands, pendingCommandToMap(state, cmd))
	}

	inTransit := make([]map[string]interface{}, 0)
	for _, f := range state.Fleets {
		if f.Owner == HumanOwner && f.InTransit {
			inTransit = append(inTransit, fleetToMap(f))
		}
	}

	return map[string]interface{}{
		"gameYear":             state.Clock,
		"paused":               state.Paused,
		"gameOver":             state.GameOver,
		"winner":               string(state.Winner),
		"winReason":            state.WinReason,
		"systems":              systems,
		"events":               events,
		"pendingCommands":      pendingCommands,
		"humanFleetsInTransit": inTransit,
	}
}

// BroadcastFleetDeparted sends a fleet_departed SSE event. Called by the engine
// immediately after a human-owned move command begins transit.
// Caller must hold state.mu.
func (m *EventManager) BroadcastFleetDeparted(f *Fleet) {
	payload := sseFrame("fleet_departed", fleetToMap(f))
	m.broadcastBytes(payload)
}
