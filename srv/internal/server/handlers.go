package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync/atomic"

	"github.com/gmofishsauce/SpaceGame/srv/internal/game"
)

var clientSeq atomic.Int64

// handleStars returns the static star positions for Three.js rendering. (FR-019)
// No ?player required — star positions are public.
func (s *Server) handleStars(w http.ResponseWriter, r *http.Request) {
	s.state.RLock()
	defer s.state.RUnlock()

	cat := s.state.Catalog
	stars := make([]StarDTO, 0, len(cat.Order))
	for _, id := range cat.Order {
		e := cat.Get(id)
		if e == nil {
			continue
		}
		stars = append(stars, StarDTO{
			ID:          e.ID,
			DisplayName: e.DisplayName,
			X:           e.X,
			Y:           e.Y,
			Z:           e.Z,
			DistFromSol: e.DistFromSol,
			HasPlanets:  e.HasPlanets,
			IsSol:       e.IsSol,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=86400")
	json.NewEncoder(w).Encode(stars)
}

// handleState returns a full player-visible game state snapshot. (FR-041)
// Requires ?player. Returns the requesting player's view.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	player := playerOf(r)

	s.state.RLock()
	defer s.state.RUnlock()

	cat := s.state.Catalog
	systems := make([]SystemDTO, 0, len(cat.Order))
	for _, id := range cat.Order {
		systems = append(systems, buildSystemDTO(player, s.state, id))
	}

	events := make([]EventDTO, 0)
	for _, evt := range s.state.Events.All {
		arrival := evt.Arrival[player]
		if arrival > s.state.Clock || arrival >= math.MaxFloat64 {
			continue
		}
		if evt.Internal {
			continue
		}
		events = append(events, EventDTO{
			ID:          evt.ID,
			ArrivalYear: arrival,
			SystemID:    evt.SystemID,
			Type:        string(evt.Type),
			Description: evt.Description,
		})
	}

	inTransit := make([]FleetDTO, 0)
	for _, t := range s.state.Views[player].InTransit {
		inTransit = append(inTransit, transitToDTO(t))
	}

	resp := StateResponse{
		GameYear:        s.state.Clock,
		Paused:          s.state.Paused,
		GameOver:        s.state.GameOver,
		Winner:          string(s.state.Winner),
		WinReason:       s.state.WinReason,
		Systems:         systems,
		Events:          events,
		PendingCommands: buildPendingCommandDTOs(player, s.state),
		FleetsInTransit: inTransit,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleEvents streams SSE events to the client. (FR-042, FR-46, FR-47)
// Requires ?player. Registers the client under that player's channel set.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	player := playerOf(r)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	clientID := fmt.Sprintf("%s-client-%d", player, clientSeq.Add(1))
	ch := s.events.Register(player, clientID)
	defer func() {
		_, wasLast := s.events.Unregister(clientID)
		if wasLast {
			s.engine.OnPlayerDisconnected(player)
		}
	}()

	// Send the current full state as the initial "connected" event.
	s.state.RLock()
	s.events.BroadcastConnected(player, clientID, s.state)
	s.state.RUnlock()

	// Notify engine that this player is now connected.
	s.engine.OnPlayerConnected(player)

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, open := <-ch:
			if !open {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// handleCommand processes a player command. (FR-024, FR-025, FR-27)
// Requires ?player. Sets Issuer from the resolved player.
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	player := playerOf(r)

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.SystemID == "" {
		writeError(w, http.StatusBadRequest, "systemId is required")
		return
	}
	if req.Quantity == 0 && req.Type == game.CmdConstruct {
		req.Quantity = 1
	}

	cmd := &game.PendingCommand{
		TargetID:      req.SystemID,
		Type:          req.Type,
		WeaponType:    req.WeaponType,
		Quantity:      req.Quantity,
		FleetID:       req.FleetID,
		DestID:        req.DestID,
		SourceFleetID: req.SourceFleetID,
		TargetFleetID: req.TargetFleetID,
		ReassignUnits: convertUnits(req.Units),
		Issuer:        player,
	}

	cmdID, arrivalYear, err := s.engine.EnqueueCommand(cmd)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CommandResponse{OK: false, Error: err.Error()})
		return
	}

	s.state.RLock()
	dto := PendingCommandDTO{
		ID:          cmd.ID,
		Type:        string(cmd.Type),
		OriginID:    cmd.OriginID,
		TargetID:    cmd.TargetID,
		ExecuteYear: cmd.ExecuteYear,
		Description: describePendingCommand(player, s.state, cmd),
	}
	var fleetName string
	if cmd.Type == game.CmdCreateFleet {
		fleetName = nextFleetNamePreview(player, s.state, cmd.TargetID)
	}
	s.state.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CommandResponse{
		OK:                   true,
		CommandID:            cmdID,
		EstimatedArrivalYear: arrivalYear,
		Pending:              &dto,
		FleetName:            fleetName,
	})
}

// handleDebugState returns the full authoritative event log for debugging.
// No ?player required — this is a debug endpoint. (FR-44)
func (s *Server) handleDebugState(w http.ResponseWriter, r *http.Request) {
	s.state.RLock()
	defer s.state.RUnlock()

	all := s.state.Events.All
	events := make([]DebugEventDTO, 0, len(all))
	for _, evt := range all {
		events = append(events, DebugEventDTO{
			ID:          evt.ID,
			EventYear:   evt.EventYear,
			ArrivalYear: evt.Arrival[game.HumanOwner],
			SystemID:    evt.SystemID,
			Type:        string(evt.Type),
			Description: evt.Description,
		})
	}

	resp := DebugStateResponse{
		GameYear: s.state.Clock,
		Events:   events,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handlePause toggles pause state. (FR-013, FR-50)
// Requires ?player. Only the human player may pause.
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	player := playerOf(r)
	if player != game.HumanOwner {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(CommandResponse{OK: false, Error: "only the human player may pause"})
		return
	}

	var req PauseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.engine.SetPaused(req.Paused)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// --- DTO builders ---

// buildSystemDTO builds the player-visible DTO for a system. (FR-041)
func buildSystemDTO(player game.Owner, state *game.GameState, sysID string) SystemDTO {
	cat := state.Catalog.Get(sysID)
	displayName := sysID
	if cat != nil {
		displayName = cat.DisplayName
	}

	if sysID == state.Homes[player] {
		gt := state.ReadHomeGroundTruth(player)
		return SystemDTO{
			ID:              sysID,
			DisplayName:     displayName,
			KnownStatus:     gt.Status,
			KnownAsOfYear:   state.Clock,
			KnownEconLevel:  gt.EconLevel,
			KnownWealth:     gt.Wealth,
			KnownLocalUnits: weaponMapToStringMap(gt.LocalUnits),
			KnownFleets:     buildKnownFleetDTOs(player, state, gt.FleetIDs),
		}
	}

	ks := state.Views[player].System(sysID)
	if ks == nil {
		return SystemDTO{
			ID:              sysID,
			DisplayName:     displayName,
			KnownStatus:     game.StatusUnknown,
			KnownLocalUnits: map[string]int{},
			KnownFleets:     []FleetDTO{},
		}
	}
	return SystemDTO{
		ID:              sysID,
		DisplayName:     displayName,
		KnownStatus:     ks.Status,
		KnownAsOfYear:   ks.AsOfYear,
		KnownEconLevel:  ks.EconLevel,
		KnownWealth:     ks.Wealth,
		KnownLocalUnits: weaponMapToStringMap(ks.LocalUnits),
		KnownFleets:     buildKnownFleetDTOs(player, state, ks.FleetIDs),
	}
}

// buildKnownFleetDTOs returns FleetDTOs for fleets in the given ID list,
// reading snapshots from the player's view.
func buildKnownFleetDTOs(player game.Owner, state *game.GameState, fleetIDs []string) []FleetDTO {
	out := make([]FleetDTO, 0, len(fleetIDs))
	for _, fid := range fleetIDs {
		f := state.Views[player].Fleet(fid)
		if f == nil {
			continue
		}
		out = append(out, knownFleetToDTO(f))
	}
	return out
}

// buildPendingCommandDTOs returns player-visible pending-command DTOs.
func buildPendingCommandDTOs(player game.Owner, state *game.GameState) []PendingCommandDTO {
	out := make([]PendingCommandDTO, 0, len(state.PendingCmds))
	for _, cmd := range state.PendingCmds {
		if cmd.Issuer != player {
			continue
		}
		out = append(out, PendingCommandDTO{
			ID:          cmd.ID,
			Type:        string(cmd.Type),
			OriginID:    cmd.OriginID,
			TargetID:    cmd.TargetID,
			ExecuteYear: cmd.ExecuteYear,
			Description: describePendingCommand(player, state, cmd),
		})
	}
	return out
}

// describePendingCommand formats the hover-text description for an in-flight command.
func describePendingCommand(player game.Owner, state *game.GameState, cmd *game.PendingCommand) string {
	targetName := cmd.TargetID
	if e := state.Catalog.Get(cmd.TargetID); e != nil {
		targetName = e.DisplayName
	}
	switch cmd.Type {
	case game.CmdConstruct:
		return fmt.Sprintf("Construct %d %s at %s (executes yr %.1f)",
			cmd.Quantity, cmd.WeaponType, targetName, cmd.ExecuteYear)
	case game.CmdMove:
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
	case game.CmdCreateFleet:
		return fmt.Sprintf("Create fleet at %s (executes yr %.1f)", targetName, cmd.ExecuteYear)
	case game.CmdReassign:
		srcName := cmd.SourceFleetID
		if f := state.Views[player].Fleet(cmd.SourceFleetID); f != nil {
			srcName = f.Name
		}
		dstName := cmd.TargetFleetID
		if f := state.Views[player].Fleet(cmd.TargetFleetID); f != nil {
			dstName = f.Name
		}
		return fmt.Sprintf("Reassign units from %s to %s at %s (executes yr %.1f)",
			srcName, dstName, targetName, cmd.ExecuteYear)
	default:
		return fmt.Sprintf("Command %s to %s (arrives yr %.1f)",
			cmd.Type, targetName, cmd.ExecuteYear)
	}
}

// nextFleetNamePreview returns the name that would be assigned to the next
// fleet created at sysID, using the player's view to read fleet count.
func nextFleetNamePreview(player game.Owner, state *game.GameState, sysID string) string {
	cat := state.Catalog.Get(sysID)
	displayName := sysID
	if cat != nil {
		displayName = cat.DisplayName
	}
	count := 0
	if sysID == state.Homes[player] {
		count = len(state.ReadHomeGroundTruth(player).FleetIDs)
	} else if ks := state.Views[player].System(sysID); ks != nil {
		count = len(ks.FleetIDs)
	}
	return fmt.Sprintf("%s-%s Fleet", displayName, ordinalServer(count+1))
}

// ordinalServer mirrors game.ordinal but is package-local.
func ordinalServer(n int) string {
	if n >= 11 && n <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}

// knownFleetToDTO converts a stationed KnownFleet to its public DTO.
func knownFleetToDTO(f *game.KnownFleet) FleetDTO {
	units := map[string]int{}
	for wt, n := range f.Units {
		if n > 0 {
			units[string(wt)] = n
		}
	}
	return FleetDTO{
		ID:    f.ID,
		Name:  f.Name,
		Owner: f.Owner,
		Units: units,
	}
}

// transitToDTO converts an in-transit KnownTransit to its public DTO.
func transitToDTO(t *game.KnownTransit) FleetDTO {
	units := map[string]int{}
	for wt, n := range t.Units {
		if n > 0 {
			units[string(wt)] = n
		}
	}
	return FleetDTO{
		ID:          t.FleetID,
		Name:        t.Name,
		Owner:       t.Owner,
		Units:       units,
		InTransit:   true,
		SourceID:    t.SourceID,
		DestID:      t.DestID,
		DepartYear:  t.DepartYear,
		ArrivalYear: t.ArrivalYear,
	}
}

// convertUnits converts a JSON string→int map to the game WeaponType→int map.
func convertUnits(m map[string]int) map[game.WeaponType]int {
	if m == nil {
		return nil
	}
	out := map[game.WeaponType]int{}
	for k, v := range m {
		out[game.WeaponType(k)] = v
	}
	return out
}

func weaponMapToStringMap(m map[game.WeaponType]int) map[string]int {
	out := map[string]int{}
	for wt, n := range m {
		if n > 0 {
			out[string(wt)] = n
		}
	}
	return out
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(CommandResponse{OK: false, Error: msg})
}
