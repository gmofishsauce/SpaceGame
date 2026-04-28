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
// Response is cached for 24 hours since positions never change.
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

// handleState returns a full player-visible game state snapshot. (FR-004a)
// Reads from SolView (and ReadSolGroundTruth for sol). Ground truth is not
// otherwise exposed.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.state.RLock()
	defer s.state.RUnlock()

	cat := s.state.Catalog
	systems := make([]SystemDTO, 0, len(cat.Order))
	for _, id := range cat.Order {
		systems = append(systems, buildSystemDTO(s.state, id))
	}

	events := make([]EventDTO, 0)
	for _, evt := range s.state.Events.All {
		if evt.ArrivalYear > s.state.Clock || evt.ArrivalYear >= math.MaxFloat64 {
			continue
		}
		if evt.Internal {
			continue
		}
		events = append(events, EventDTO{
			ID:          evt.ID,
			ArrivalYear: evt.ArrivalYear,
			SystemID:    evt.SystemID,
			Type:        string(evt.Type),
			Description: evt.Description,
		})
	}

	inTransit := make([]FleetDTO, 0)
	for _, t := range s.state.SolView.InTransit {
		if t.Owner == game.HumanOwner {
			inTransit = append(inTransit, transitToDTO(t))
		}
	}

	resp := StateResponse{
		GameYear:             s.state.Clock,
		Paused:               s.state.Paused,
		GameOver:             s.state.GameOver,
		Winner:               string(s.state.Winner),
		WinReason:            s.state.WinReason,
		Systems:              systems,
		Events:               events,
		PendingCommands:      buildPendingCommandDTOs(s.state),
		HumanFleetsInTransit: inTransit,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleEvents streams SSE events to the client. (FR-017, FR-025)
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	clientID := fmt.Sprintf("client-%d", clientSeq.Add(1))
	ch := s.events.Register(clientID)
	defer s.events.Unregister(clientID)

	// Send the current full state as the initial "connected" event.
	s.state.RLock()
	s.events.BroadcastConnected(clientID, s.state)
	s.state.RUnlock()

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

// handleCommand processes a player command from the client. (FR-029, FR-031)
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

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
		req.Quantity = 1 // default quantity for MVP
	}

	cmd := &game.PendingCommand{
		OriginID:      "sol",
		TargetID:      req.SystemID,
		Type:          req.Type,
		WeaponType:    req.WeaponType,
		Quantity:      req.Quantity,
		FleetID:       req.FleetID,
		DestID:        req.DestID,
		SourceFleetID: req.SourceFleetID,
		TargetFleetID: req.TargetFleetID,
		ReassignUnits: convertUnits(req.Units),
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
		Description: describePendingCommand(s.state, cmd),
	}
	var fleetName string
	if cmd.Type == game.CmdCreateFleet {
		// NextFleetName needs the truth-side TrueSystem to read FleetCount.
		// This is the only command path that returns a server-projected
		// fleet name (echoed back to the client for immediate display).
		fleetName = nextFleetNamePreview(s.state, cmd.TargetID)
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

// handleDebugState returns the full authoritative event log with ground-truth
// EventYear timestamps, including internal-only event types, for debugging.
func (s *Server) handleDebugState(w http.ResponseWriter, r *http.Request) {
	s.state.RLock()
	defer s.state.RUnlock()

	all := s.state.Events.All
	events := make([]DebugEventDTO, 0, len(all))
	for _, evt := range all {
		events = append(events, DebugEventDTO{
			ID:          evt.ID,
			EventYear:   evt.EventYear,
			ArrivalYear: evt.ArrivalYear,
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

// handlePause toggles pause state. (FR-013)
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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

// buildSystemDTO builds the player-visible DTO for a system from the catalog
// (static fields) and SolView (or, for sol, ReadSolGroundTruth).
func buildSystemDTO(state *game.GameState, sysID string) SystemDTO {
	cat := state.Catalog.Get(sysID)
	displayName := sysID
	if cat != nil {
		displayName = cat.DisplayName
	}

	if sysID == "sol" {
		gt := state.ReadSolGroundTruth()
		return SystemDTO{
			ID:              sysID,
			DisplayName:     displayName,
			KnownStatus:     gt.Status,
			KnownAsOfYear:   state.Clock,
			KnownEconLevel:  gt.EconLevel,
			KnownWealth:     gt.Wealth,
			KnownLocalUnits: weaponMapToStringMap(gt.LocalUnits),
			KnownFleets:     buildKnownFleetDTOs(state, gt.FleetIDs),
		}
	}

	ks := state.SolView.System(sysID)
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
		KnownFleets:     buildKnownFleetDTOs(state, ks.FleetIDs),
	}
}

// buildKnownFleetDTOs returns FleetDTOs for the human-owned fleets in the
// given ID list, reading snapshots from SolView.Fleets.
func buildKnownFleetDTOs(state *game.GameState, fleetIDs []string) []FleetDTO {
	out := make([]FleetDTO, 0, len(fleetIDs))
	for _, fid := range fleetIDs {
		f := state.SolView.Fleet(fid)
		if f == nil || f.Owner != game.HumanOwner {
			continue
		}
		out = append(out, knownFleetToDTO(f))
	}
	return out
}

// buildPendingCommandDTOs returns player-visible pending-command DTOs,
// excluding bot commands.
func buildPendingCommandDTOs(state *game.GameState) []PendingCommandDTO {
	out := make([]PendingCommandDTO, 0, len(state.PendingCmds))
	for _, cmd := range state.PendingCmds {
		if cmd.IsBot {
			continue
		}
		out = append(out, PendingCommandDTO{
			ID:          cmd.ID,
			Type:        string(cmd.Type),
			OriginID:    cmd.OriginID,
			TargetID:    cmd.TargetID,
			ExecuteYear: cmd.ExecuteYear,
			Description: describePendingCommand(state, cmd),
		})
	}
	return out
}

// describePendingCommand formats the hover-text description for an in-flight command.
func describePendingCommand(state *game.GameState, cmd *game.PendingCommand) string {
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
		if f := state.SolView.Fleet(cmd.FleetID); f != nil {
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
		if f := state.SolView.Fleet(cmd.SourceFleetID); f != nil {
			srcName = f.Name
		}
		dstName := cmd.TargetFleetID
		if f := state.SolView.Fleet(cmd.TargetFleetID); f != nil {
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
// fleet created at sysID. The server reads truth.FleetCount via SolView's
// FleetIDs as a proxy: SolView reflects sol's known fleets in lockstep, and
// for non-sol systems the player-issued CmdCreateFleet always targets a
// human-known system. This avoids reaching into truth from the server.
func nextFleetNamePreview(state *game.GameState, sysID string) string {
	cat := state.Catalog.Get(sysID)
	displayName := sysID
	if cat != nil {
		displayName = cat.DisplayName
	}
	count := 0
	if sysID == "sol" {
		count = len(state.ReadSolGroundTruth().FleetIDs)
	} else if ks := state.SolView.System(sysID); ks != nil {
		count = len(ks.FleetIDs)
	}
	return fmt.Sprintf("%s-%s Fleet", displayName, ordinalServer(count+1))
}

// ordinalServer mirrors game.ordinal but is package-local so handlers don't
// need to dig into game internals for this one display helper.
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
