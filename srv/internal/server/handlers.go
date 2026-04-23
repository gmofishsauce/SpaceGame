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

	stars := make([]StarDTO, 0, len(s.state.Systems))
	for _, id := range s.state.SystemOrder {
		sys := s.state.Systems[id]
		stars = append(stars, StarDTO{
			ID:          sys.ID,
			DisplayName: sys.DisplayName,
			X:           sys.X,
			Y:           sys.Y,
			Z:           sys.Z,
			DistFromSol: sys.DistFromSol,
			HasPlanets:  sys.HasPlanets,
			IsSol:       sys.ID == "sol",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=86400")
	json.NewEncoder(w).Encode(stars)
}

// handleState returns a full player-visible game state snapshot. (FR-004a)
// Only KnownState fields are returned for each system; ground truth is not exposed.
// Sol is special-cased to always show current accurate information. (FR-023)
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.state.RLock()
	defer s.state.RUnlock()

	systems := make([]SystemDTO, 0, len(s.state.Systems))
	for _, id := range s.state.SystemOrder {
		sys := s.state.Systems[id]
		dto := buildSystemDTO(s.state, sys)
		systems = append(systems, dto)
	}

	events := make([]EventDTO, 0)
	for _, evt := range s.state.Events {
		if evt.ArrivalYear > s.state.Clock || evt.ArrivalYear >= math.MaxFloat64 {
			continue
		}
		if evt.Type == game.EventCombatSilent || evt.Type == game.EventAlienSpawn {
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
	for _, f := range s.state.Fleets {
		if f.Owner == game.HumanOwner && f.InTransit {
			inTransit = append(inTransit, fleetToDTO(f))
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
		OriginID:   "sol",
		TargetID:   req.SystemID,
		Type:       req.Type,
		WeaponType: req.WeaponType,
		Quantity:   req.Quantity,
		FleetID:    req.FleetID,
		DestID:     req.DestID,
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
	s.state.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CommandResponse{
		OK:                   true,
		CommandID:            cmdID,
		EstimatedArrivalYear: arrivalYear,
		Pending:              &dto,
	})
}

// handleDebugState returns the full authoritative event log with ground-truth
// EventYear timestamps, including internal-only event types, for debugging.
func (s *Server) handleDebugState(w http.ResponseWriter, r *http.Request) {
	s.state.RLock()
	defer s.state.RUnlock()

	events := make([]DebugEventDTO, 0, len(s.state.Events))
	for _, evt := range s.state.Events {
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

// buildSystemDTO builds the player-visible DTO for a system.
// Sol always shows ground truth (FR-023); all other systems show KnownState.
func buildSystemDTO(state *game.GameState, sys *game.StarSystem) SystemDTO {
	var status game.SystemStatus
	var econLevel int
	var wealth float64
	var asOfYear float64
	var localUnits map[string]int
	var knownFleets []FleetDTO

	if sys.ID == "sol" {
		// Sol: show current ground truth
		status = sys.Status
		econLevel = sys.EconLevel
		wealth = sys.Wealth
		asOfYear = state.Clock
		localUnits = weaponMapToStringMap(sys.LocalUnits)
		for _, fid := range sys.FleetIDs {
			f := state.Fleets[fid]
			if f != nil && f.Owner == game.HumanOwner {
				knownFleets = append(knownFleets, fleetToDTO(f))
			}
		}
	} else {
		status = sys.KnownStatus
		econLevel = sys.KnownEconLevel
		wealth = sys.KnownWealth
		asOfYear = sys.KnownAsOfYear
		localUnits = weaponMapToStringMap(sys.KnownLocalUnits)
		for _, fid := range sys.KnownFleetIDs {
			f := state.Fleets[fid]
			if f != nil && f.Owner == game.HumanOwner {
				knownFleets = append(knownFleets, fleetToDTO(f))
			}
		}
	}

	if knownFleets == nil {
		knownFleets = []FleetDTO{}
	}

	return SystemDTO{
		ID:              sys.ID,
		DisplayName:     sys.DisplayName,
		KnownStatus:     status,
		KnownAsOfYear:   asOfYear,
		KnownEconLevel:  econLevel,
		KnownWealth:     wealth,
		KnownLocalUnits: localUnits,
		KnownFleets:     knownFleets,
	}
}

// buildPendingCommandDTOs returns player-visible pending-command DTOs,
// excluding bot commands. Descriptions are formed server-side using
// readily available system/fleet display names.
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
	if sys, ok := state.Systems[cmd.TargetID]; ok {
		targetName = sys.DisplayName
	}
	switch cmd.Type {
	case game.CmdConstruct:
		return fmt.Sprintf("Construct %d %s at %s (executes yr %.1f)",
			cmd.Quantity, cmd.WeaponType, targetName, cmd.ExecuteYear)
	case game.CmdMove:
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

func fleetToDTO(f *game.Fleet) FleetDTO {
	units := map[string]int{}
	for wt, n := range f.Units {
		if n > 0 {
			units[string(wt)] = n
		}
	}
	return FleetDTO{
		ID:          f.ID,
		Name:        f.Name,
		Owner:       f.Owner,
		Units:       units,
		InTransit:   f.InTransit,
		SourceID:    f.SourceID,
		DestID:      f.DestID,
		DepartYear:  f.DepartYear,
		ArrivalYear: f.ArrivalYear,
	}
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
