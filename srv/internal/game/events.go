package game

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
)

// EventManager manages the SSE client registry and broadcasts events.
// Maturation/apply logic lives in Propagator; EventManager is plumbing.
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

// broadcastEvent emits a single matured event as an SSE "game_event" frame.
// Called by Propagator.Propagate; caller holds state.mu.
func (m *EventManager) broadcastEvent(evt *Event) {
	payload := sseFrame("game_event", eventToMap(evt))
	m.broadcastBytes(payload)
}

// broadcastSystemUpdate emits an SSE "system_update" frame for the given
// system, sourced from SolView (or Sol's ground truth for "sol"). Called
// by Propagator.Propagate; caller holds state.mu.
func (m *EventManager) broadcastSystemUpdate(state *GameState, sysID string) {
	if sysID == "" {
		return
	}
	if _, ok := state.SolView.Systems[sysID]; !ok && sysID != "sol" {
		return
	}
	payload := sseFrame("system_update", systemToMap(state, sysID))
	m.broadcastBytes(payload)
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

// eventToMap converts an Event to a map suitable for JSON encoding.
func eventToMap(evt *Event) map[string]interface{} {
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

// systemToMap returns the player-visible map for one system, sourced from
// SolView (or, for sol, the single ReadSolGroundTruth() escape hatch).
func systemToMap(state *GameState, sysID string) map[string]interface{} {
	cat := state.Catalog.Get(sysID)
	displayName := sysID
	if cat != nil {
		displayName = cat.DisplayName
	}

	if sysID == "sol" {
		gt := state.ReadSolGroundTruth()
		return map[string]interface{}{
			"systemId":        sysID,
			"displayName":     displayName,
			"knownStatus":     string(gt.Status),
			"knownAsOfYear":   state.Clock,
			"knownEconLevel":  gt.EconLevel,
			"knownWealth":     gt.Wealth,
			"knownLocalUnits": unitsToStringMap(gt.LocalUnits),
			"knownFleets":     buildKnownFleetsForSol(state, gt.FleetIDs),
		}
	}

	ks := state.SolView.System(sysID)
	if ks == nil {
		return map[string]interface{}{
			"systemId":        sysID,
			"displayName":     displayName,
			"knownStatus":     string(StatusUnknown),
			"knownLocalUnits": map[string]int{},
			"knownFleets":     []map[string]interface{}{},
		}
	}
	return map[string]interface{}{
		"systemId":        sysID,
		"displayName":     displayName,
		"knownStatus":     string(ks.Status),
		"knownAsOfYear":   ks.AsOfYear,
		"knownEconLevel":  ks.EconLevel,
		"knownWealth":     ks.Wealth,
		"knownLocalUnits": unitsToStringMap(ks.LocalUnits),
		"knownFleets":     buildKnownFleetsForView(state, ks.FleetIDs),
	}
}

// buildKnownFleetsForView returns the player-visible human-fleet map list
// for a non-sol system, reading snapshots from SolView.Fleets.
func buildKnownFleetsForView(state *GameState, fleetIDs []string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(fleetIDs))
	for _, fid := range fleetIDs {
		f := state.SolView.Fleet(fid)
		if f == nil || f.Owner != HumanOwner {
			continue
		}
		result = append(result, knownFleetToMap(f))
	}
	return result
}

// buildKnownFleetsForSol returns Sol's fleet list. Sol's fleets are mirrored
// into SolView.Fleets at construction time (events at sol mature in the same
// tick they are recorded), so we can read from SolView like any other system.
func buildKnownFleetsForSol(state *GameState, fleetIDs []string) []map[string]interface{} {
	return buildKnownFleetsForView(state, fleetIDs)
}

// knownFleetToMap converts a stationed KnownFleet to its SSE/DTO map.
func knownFleetToMap(f *KnownFleet) map[string]interface{} {
	return map[string]interface{}{
		"id":            f.ID,
		"name":          f.Name,
		"owner":         string(f.Owner),
		"units":         unitsToStringMap(f.Units),
		"inTransit":     false,
		"sourceId":      "",
		"destinationId": "",
		"departYear":    0.0,
		"arrivalYear":   0.0,
	}
}

// transitToMap converts a KnownTransit (in-flight fleet known to Sol) to its
// SSE/DTO map.
func transitToMap(t *KnownTransit) map[string]interface{} {
	return map[string]interface{}{
		"id":            t.FleetID,
		"name":          t.Name,
		"owner":         string(t.Owner),
		"units":         unitsToStringMap(t.Units),
		"inTransit":     true,
		"sourceId":      t.SourceID,
		"destinationId": t.DestID,
		"departYear":    t.DepartYear,
		"arrivalYear":   t.ArrivalYear,
	}
}

// unitsToStringMap returns a string-keyed map of positive unit counts.
func unitsToStringMap(m map[WeaponType]int) map[string]int {
	out := map[string]int{}
	for wt, n := range m {
		if n > 0 {
			out[string(wt)] = n
		}
	}
	return out
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
// Reads display names from Catalog and fleet names from SolView.
func describePendingCommandLocal(state *GameState, cmd *PendingCommand) string {
	targetName := cmd.TargetID
	if e := state.Catalog.Get(cmd.TargetID); e != nil {
		targetName = e.DisplayName
	}
	switch cmd.Type {
	case CmdConstruct:
		return fmt.Sprintf("Construct %d %s at %s (executes yr %.1f)",
			cmd.Quantity, cmd.WeaponType, targetName, cmd.ExecuteYear)
	case CmdMove:
		fleetName := cmd.FleetID
		if f := state.SolView.Fleet(cmd.FleetID); f != nil {
			fleetName = f.Name
		}
		destName := cmd.DestID
		if e := state.Catalog.Get(cmd.DestID); e != nil {
			destName = e.DisplayName
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
	systems := make([]map[string]interface{}, 0, len(state.Catalog.Order))
	for _, id := range state.Catalog.Order {
		cat := state.Catalog.Get(id)
		entry := systemToMap(state, id)
		// Augment with static catalog fields used by the connected snapshot.
		entry["displayName"] = cat.DisplayName
		entry["x"] = cat.X
		entry["y"] = cat.Y
		entry["z"] = cat.Z
		entry["distFromSol"] = cat.DistFromSol
		entry["hasPlanets"] = cat.HasPlanets
		entry["isSol"] = cat.IsSol
		entry["id"] = id
		systems = append(systems, entry)
	}

	events := make([]map[string]interface{}, 0)
	for _, evt := range state.Events.All {
		if !evt.Broadcast {
			continue
		}
		if evt.Internal {
			continue
		}
		if evt.ArrivalYear > state.Clock || evt.ArrivalYear >= math.MaxFloat64 {
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
	for _, t := range state.SolView.InTransit {
		if t.Owner == HumanOwner {
			inTransit = append(inTransit, transitToMap(t))
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
