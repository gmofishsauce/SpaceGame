package game

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
)

// EventManager manages the per-player SSE client registry and broadcasts events.
// Maturation/apply logic lives in Propagator; EventManager is plumbing.
// (FR-43, FR-42, FR-47)
type EventManager struct {
	mu       sync.Mutex
	byPlayer map[Owner]map[string]chan []byte // player → clientID → channel
}

// NewEventManager creates a ready-to-use EventManager.
func NewEventManager() *EventManager {
	return &EventManager{
		byPlayer: map[Owner]map[string]chan []byte{
			HumanOwner: {},
			AlienOwner: {},
		},
	}
}

// Register adds an SSE client for the given player. Returns a receive-only
// channel carrying SSE-formatted frames.
func (m *EventManager) Register(player Owner, clientID string) <-chan []byte {
	ch := make(chan []byte, 64)
	m.mu.Lock()
	if m.byPlayer[player] == nil {
		m.byPlayer[player] = map[string]chan []byte{}
	}
	m.byPlayer[player][clientID] = ch
	m.mu.Unlock()
	return ch
}

// Unregister removes a disconnected SSE client and closes its channel.
// Returns the player the client belonged to, and whether that player now has
// zero remaining clients (so the caller can drive OnPlayerDisconnected).
func (m *EventManager) Unregister(clientID string) (player Owner, wasLast bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for p, clients := range m.byPlayer {
		if ch, ok := clients[clientID]; ok {
			close(ch)
			delete(clients, clientID)
			return p, len(clients) == 0
		}
	}
	return "", false
}

// broadcastEvent emits a single matured event as an SSE "game_event" frame
// to all clients registered under player.
// Called by Propagator.Propagate; caller holds state.mu.
func (m *EventManager) broadcastEvent(player Owner, evt *Event) {
	payload := sseFrame("game_event", eventToMap(player, evt))
	m.broadcastToPlayer(player, payload)
}

// broadcastSystemUpdate emits an SSE "system_update" frame for the given
// system, sourced from the player's view.
// Called by Propagator.Propagate; caller holds state.mu.
func (m *EventManager) broadcastSystemUpdate(player Owner, state *GameState, sysID string) {
	if sysID == "" {
		return
	}
	if _, ok := state.Views[player].Systems[sysID]; !ok && sysID != state.Homes[player] {
		return
	}
	payload := sseFrame("system_update", systemToMap(player, state, sysID))
	m.broadcastToPlayer(player, payload)
}

// BroadcastClockSync sends a clock sync event to all registered clients (both players).
func (m *EventManager) BroadcastClockSync(state *GameState) {
	payload := sseFrame("clock_sync", map[string]interface{}{
		"gameYear": state.Clock,
		"paused":   state.Paused,
	})
	m.broadcastAll(payload)
}

// BroadcastGameOver sends the game-over event to all registered clients.
func (m *EventManager) BroadcastGameOver(winner Owner, reason string) {
	payload := sseFrame("game_over", map[string]interface{}{
		"winner": string(winner),
		"reason": reason,
	})
	m.broadcastAll(payload)
}

// BroadcastConnected sends the full current state snapshot to a single client.
// player is the owner of clientID. state.mu must be held by caller.
func (m *EventManager) BroadcastConnected(player Owner, clientID string, state *GameState) {
	m.mu.Lock()
	clients := m.byPlayer[player]
	ch, ok := clients[clientID]
	m.mu.Unlock()
	if !ok {
		return
	}
	payload := sseFrame("connected", fullStateMap(player, state))
	safeSend(ch, payload)
}

// broadcastToPlayer sends payload to all clients registered under player.
func (m *EventManager) broadcastToPlayer(player Owner, payload []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ch := range m.byPlayer[player] {
		if !safeSend(ch, payload) {
			log.Printf("events: client %s channel full or closed, dropping event", id)
		}
	}
}

// broadcastAll sends payload to all registered clients regardless of player.
func (m *EventManager) broadcastAll(payload []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, clients := range m.byPlayer {
		for id, ch := range clients {
			if !safeSend(ch, payload) {
				log.Printf("events: client %s channel full or closed, dropping event", id)
			}
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

// eventToMap converts an Event to a map suitable for JSON encoding for player.
func eventToMap(player Owner, evt *Event) map[string]interface{} {
	m := map[string]interface{}{
		"id":          evt.ID,
		"arrivalYear": evt.Arrival[player],
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
// the player's view (or the home ground-truth for the home system).
func systemToMap(player Owner, state *GameState, sysID string) map[string]interface{} {
	cat := state.Catalog.Get(sysID)
	displayName := sysID
	if cat != nil {
		displayName = cat.DisplayName
	}

	if sysID == state.Homes[player] {
		gt := state.ReadHomeGroundTruth(player)
		return map[string]interface{}{
			"systemId":        sysID,
			"displayName":     displayName,
			"knownStatus":     string(gt.Status),
			"knownAsOfYear":   state.Clock,
			"knownEconLevel":  gt.EconLevel,
			"knownWealth":     gt.Wealth,
			"knownLocalUnits": unitsToStringMap(gt.LocalUnits),
			"knownFleets":     buildKnownFleetsForView(player, state, gt.FleetIDs),
		}
	}

	ks := state.Views[player].System(sysID)
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
		"knownFleets":     buildKnownFleetsForView(player, state, ks.FleetIDs),
	}
}

// buildKnownFleetsForView returns the player-visible fleet map list for a
// system, reading snapshots from the player's view.
func buildKnownFleetsForView(player Owner, state *GameState, fleetIDs []string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(fleetIDs))
	for _, fid := range fleetIDs {
		f := state.Views[player].Fleet(fid)
		if f == nil {
			continue
		}
		result = append(result, knownFleetToMap(f))
	}
	return result
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

// transitToMap converts a KnownTransit (in-flight fleet) to its SSE/DTO map.
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

// pendingCommandToMap converts a PendingCommand to a map for JSON encoding.
func pendingCommandToMap(player Owner, state *GameState, cmd *PendingCommand) map[string]interface{} {
	return map[string]interface{}{
		"id":          cmd.ID,
		"type":        string(cmd.Type),
		"originId":    cmd.OriginID,
		"targetId":    cmd.TargetID,
		"executeYear": cmd.ExecuteYear,
		"description": describePendingCommandLocal(player, state, cmd),
	}
}

// describePendingCommandLocal formats hover text for an in-flight command.
func describePendingCommandLocal(player Owner, state *GameState, cmd *PendingCommand) string {
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
		if f := state.Views[player].Fleet(cmd.FleetID); f != nil {
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
func fullStateMap(player Owner, state *GameState) map[string]interface{} {
	systems := make([]map[string]interface{}, 0, len(state.Catalog.Order))
	for _, id := range state.Catalog.Order {
		cat := state.Catalog.Get(id)
		entry := systemToMap(player, state, id)
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
		if !evt.Broadcast[player] {
			continue
		}
		if evt.Internal {
			continue
		}
		arrival := evt.Arrival[player]
		if arrival > state.Clock || arrival >= math.MaxFloat64 {
			continue
		}
		events = append(events, eventToMap(player, evt))
	}

	pendingCommands := make([]map[string]interface{}, 0, len(state.PendingCmds))
	for _, cmd := range state.PendingCmds {
		if cmd.Issuer != player {
			continue
		}
		pendingCommands = append(pendingCommands, pendingCommandToMap(player, state, cmd))
	}

	inTransit := make([]map[string]interface{}, 0)
	for _, t := range state.Views[player].InTransit {
		inTransit = append(inTransit, transitToMap(t))
	}

	return map[string]interface{}{
		"gameYear":        state.Clock,
		"paused":          state.Paused,
		"gameOver":        state.GameOver,
		"winner":          string(state.Winner),
		"winReason":       state.WinReason,
		"systems":         systems,
		"events":          events,
		"pendingCommands": pendingCommands,
		"fleetsInTransit": inTransit,
	}
}
