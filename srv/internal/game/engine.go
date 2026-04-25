package game

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"
)

// Engine runs the authoritative game loop and coordinates all subsystems.
type Engine struct {
	State  *GameState
	Bot    BotAgent
	Events *EventManager

	tickCount int
	rng       *rand.Rand
}

// NewEngine creates an Engine wired to the given state, bot, and event manager.
func NewEngine(state *GameState, bot BotAgent, events *EventManager) *Engine {
	return &Engine{
		State:  state,
		Bot:    bot,
		Events: events,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run blocks, running the game loop until ctx is cancelled. Call in a goroutine.
// (FR-011, FR-012, FR-013)
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(TickIntervalMs * time.Millisecond)
	defer ticker.Stop()

	e.Bot.Initialize(e.State)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.State.mu.Lock()
			if !e.State.Paused && !e.State.GameOver {
				e.tick()
			}
			e.State.mu.Unlock()
		}
	}
}

// SetPaused pauses or unpauses the game. Safe to call from HTTP handlers. (FR-013)
func (e *Engine) SetPaused(paused bool) {
	e.State.Lock()
	e.State.Paused = paused
	e.State.Unlock()
	// Broadcast clock sync to all SSE clients immediately.
	e.State.RLock()
	e.Events.BroadcastClockSync(e.State)
	e.State.RUnlock()
}

// EnqueueCommand validates and enqueues a player command. Returns the command
// ID and estimated arrival year, or an error. Safe to call from HTTP handlers.
func (e *Engine) EnqueueCommand(cmd *PendingCommand) (string, float64, error) {
	e.State.Lock()
	defer e.State.Unlock()

	sys, ok := e.State.Systems[cmd.TargetID]
	if !ok {
		return "", 0, fmt.Errorf("unknown system %q", cmd.TargetID)
	}

	// Quick known-state validation (full validation happens at execution time)
	if cmd.TargetID != "sol" {
		if sys.KnownStatus == StatusAlien {
			return "", 0, fmt.Errorf("system %q is known to be alien-held; cannot issue commands", cmd.TargetID)
		}
	}

	var executeYear float64
	if cmd.TargetID == "sol" {
		executeYear = e.State.Clock // immediate for Sol
	} else {
		sol := e.State.Systems["sol"]
		if sol == nil {
			return "", 0, fmt.Errorf("sol system not found")
		}
		executeYear = e.State.Clock + distBetween(sol, sys)/CommandSpeedC
	}

	cmd.ID = e.State.NewCommandID()
	cmd.ExecuteYear = executeYear
	cmd.OriginID = "sol"
	e.State.PendingCmds = append(e.State.PendingCmds, cmd)

	return cmd.ID, executeYear, nil
}

// tick runs one engine tick. Called with state.mu write-locked.
func (e *Engine) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("engine: panic in tick: %v", r)
		}
	}()

	e.State.Clock += YearsPerTick

	// Process fleet arrivals before commands or combat.
	e.processFleetArrivals()

	// Process matured pending commands.
	remaining := e.State.PendingCmds[:0]
	for _, cmd := range e.State.PendingCmds {
		if cmd.ExecuteYear <= e.State.Clock {
			if err := e.State.ApplyCommand(cmd); err != nil {
				e.logCommandFailed(cmd, err)
			} else if cmd.Type == CmdMove {
				// Broadcast fleet departure so the client can render an in-transit arrow.
				if f, ok := e.State.Fleets[cmd.FleetID]; ok && f.InTransit && f.Owner == HumanOwner {
					e.Events.BroadcastFleetDeparted(f)
				}
			}
		} else {
			remaining = append(remaining, cmd)
		}
	}
	e.State.PendingCmds = remaining

	// Accumulate wealth and advance economic levels.
	AccumulateWealth(e.State, YearsPerTick)
	AdvanceEconLevels(e.State)

	// Check for and resolve combat in any system where both sides are present.
	for _, sys := range e.State.Systems {
		if humanForcesPresent(e.State, sys) && alienForcesPresent(e.State, sys) {
			Resolve(e.rng, e.State, sys)
		}
	}

	// Alien spawning.
	if !e.State.Alien.Exhausted && e.State.Clock >= e.State.Alien.NextSpawnYear {
		e.spawnAlienForces()
		e.State.Alien.NextSpawnYear += AlienSpawnIntervalYears
	}

	// Bot tick every BotTickCadence ticks.
	e.tickCount++
	if e.tickCount%BotTickCadence == 0 {
		cmds := func() (result []BotCommand) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("engine: bot panic: %v", r)
				}
			}()
			return e.Bot.Tick(e.State, e.State.Clock)
		}()
		for _, bc := range cmds {
			e.applyBotCommand(bc)
		}
	}

	// Update known states for all systems.
	e.State.UpdateKnownStates(e.State.Clock)

	// Broadcast matured events via SSE.
	e.Events.BroadcastMatured(e.State)

	// Periodic clock sync broadcast (every ClockSyncCadence ticks).
	if e.tickCount%ClockSyncCadence == 0 {
		e.Events.BroadcastClockSync(e.State)
	}

	// Check victory/defeat conditions.
	if over, winner, reason := e.State.CheckVictory(); over {
		e.State.GameOver = true
		e.State.Winner = winner
		e.State.WinReason = reason
		e.State.Paused = true
		e.Events.BroadcastGameOver(winner, reason)
	}
}

// processFleetArrivals moves fleets that have completed their transit.
func (e *Engine) processFleetArrivals() {
	for _, fleet := range e.State.Fleets {
		if !fleet.InTransit || fleet.ArrivalYear > e.State.Clock {
			continue
		}
		dest, ok := e.State.Systems[fleet.DestID]
		if !ok {
			log.Printf("engine: fleet %s destination %q not found, removing", fleet.ID, fleet.DestID)
			delete(e.State.Fleets, fleet.ID)
			continue
		}

		// Reporter fleets arriving at Sol are consumed (their event was already logged).
		if fleet.DestID == "sol" && fleet.Owner == HumanOwner && fleetIsReporterOnly(fleet) {
			// Reporter arrival: the fleet itself is consumed.
			delete(e.State.Fleets, fleet.ID)
			e.State.RecordEvent(&GameEvent{
				EventYear:   fleet.ArrivalYear,
				ArrivalYear: fleet.ArrivalYear,
				SystemID:    "sol",
				Type:        EventReporterReturn,
				Description: fmt.Sprintf("Reporter fleet %s returned to Sol with intelligence", fleet.Name),
				CanReport:   true,
			})
			continue
		}

		// Normal fleet arrival.
		fleet.InTransit = false
		fleet.LocationID = fleet.DestID
		fleet.DestID = ""
		fleet.SourceID = ""

		dest.FleetIDs = appendIfMissing(dest.FleetIDs, fleet.ID)

		// Determine if this arrival is reportable (comm laser at destination)
		hasCommLaser := systemHasCommLaser(e.State, dest)
		arrYear := arrivalYearFor(e.State.Clock, dest.DistFromSol, hasCommLaser)
		e.State.RecordEvent(&GameEvent{
			EventYear:   e.State.Clock,
			ArrivalYear: arrYear,
			SystemID:    dest.ID,
			Type:        EventFleetArrival,
			Description: fmt.Sprintf("Fleet %s arrived at %s", fleet.Name, dest.DisplayName),
			CanReport:   hasCommLaser,
			Details: &FleetArrivalDetails{
				FleetID:   fleet.ID,
				FleetName: fleet.Name,
				Owner:     fleet.Owner,
				Units:     copyUnits(fleet.Units),
			},
		})

		// Conquest: a human fleet carrying a comm laser that arrives at an
		// uninhabited system claims it. Economy starts at level 0, wealth 0.
		if fleet.Owner == HumanOwner &&
			dest.Status == StatusUninhabited &&
			fleet.Units[WeaponCommLaser] > 0 {

			dest.Status = StatusHuman
			dest.EconLevel = 0
			dest.Wealth = 0
			dest.EconGrowthYear = e.State.Clock + EconGrowthIntervalYears

			e.State.RecordEvent(&GameEvent{
				EventYear:   e.State.Clock,
				ArrivalYear: arrivalYearFor(e.State.Clock, dest.DistFromSol, true),
				SystemID:    dest.ID,
				Type:        EventSystemConquered,
				Description: fmt.Sprintf("Fleet %s established a colony at %s", fleet.Name, dest.DisplayName),
				CanReport:   true,
			})
		}
	}
}

// spawnAlienForces adds a wave of alien forces at each entry point.
func (e *Engine) spawnAlienForces() {
	for _, epID := range e.State.Alien.EntryPointIDs {
		ep, ok := e.State.Systems[epID]
		if !ok {
			continue
		}
		fleetID := e.State.NewFleetID()
		fleetName := e.State.NewFleetName()
		fleet := &Fleet{
			ID:         fleetID,
			Name:       fleetName,
			Owner:      AlienOwner,
			Units:      copyUnits(AlienSpawnComposition),
			LocationID: epID,
			InTransit:  false,
		}
		e.State.Fleets[fleetID] = fleet
		ep.FleetIDs = append(ep.FleetIDs, fleetID)
		ep.Status = StatusAlien

		e.State.RecordEvent(&GameEvent{
			EventYear:   e.State.Clock,
			ArrivalYear: e.State.Clock, // internal only
			SystemID:    epID,
			Type:        EventAlienSpawn,
			Description: fmt.Sprintf("Alien forces reinforced at %s", ep.DisplayName),
			CanReport:   false,
		})
	}
}

// applyBotCommand applies a bot command immediately (no travel delay).
// Bot fleet moves are direct state mutations. Called with state.mu held.
func (e *Engine) applyBotCommand(bc BotCommand) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("engine: applyBotCommand panic: %v", r)
		}
	}()

	switch bc.Type {
	case CmdMove:
		fleet, ok := e.State.Fleets[bc.FleetID]
		if !ok || fleet.InTransit || fleet.Owner != AlienOwner {
			return
		}
		src, srcOK := e.State.Systems[fleet.LocationID]
		dest, destOK := e.State.Systems[bc.DestID]
		if !srcOK || !destOK {
			return
		}
		travelYears := distBetween(src, dest) / FleetSpeedC
		fleet.InTransit = true
		fleet.DepartYear = e.State.Clock
		fleet.ArrivalYear = e.State.Clock + travelYears
		fleet.DestID = bc.DestID
		fleet.SourceID = src.ID
		src.FleetIDs = removeString(src.FleetIDs, fleet.ID)
		fleet.LocationID = ""
	}
}

// logCommandFailed records a command_failed event for a command that could not execute.
func (e *Engine) logCommandFailed(cmd *PendingCommand, err error) {
	sys := e.State.Systems[cmd.TargetID]
	var displayName, sysID string
	var distFromSol float64
	if sys != nil {
		displayName = sys.DisplayName
		sysID = sys.ID
		distFromSol = sys.DistFromSol
	} else {
		displayName = cmd.TargetID
		sysID = cmd.TargetID
	}
	hasCommLaser := sys != nil && systemHasCommLaser(e.State, sys)
	arrYear := arrivalYearFor(e.State.Clock, distFromSol, hasCommLaser)
	e.State.RecordEvent(&GameEvent{
		EventYear:   e.State.Clock,
		ArrivalYear: arrYear,
		SystemID:    sysID,
		Type:        EventCommandFailed,
		Description: fmt.Sprintf("Command %s at %s failed: %v", cmd.Type, displayName, err),
		CanReport:   hasCommLaser,
		Details:     &CommandFailedDetails{CommandType: cmd.Type, Reason: err.Error()},
	})
}

// --- Force presence checks ---

func humanForcesPresent(state *GameState, sys *StarSystem) bool {
	for wt, n := range sys.LocalUnits {
		if !WeaponDefs[wt].CommLaser && n > 0 {
			return true
		}
	}
	for _, fid := range sys.FleetIDs {
		f := state.Fleets[fid]
		if f == nil || f.InTransit || f.Owner != HumanOwner {
			continue
		}
		if totalUnits(f.Units) > 0 {
			return true
		}
	}
	return false
}

func alienForcesPresent(state *GameState, sys *StarSystem) bool {
	for _, fid := range sys.FleetIDs {
		f := state.Fleets[fid]
		if f == nil || f.InTransit || f.Owner != AlienOwner {
			continue
		}
		if totalUnits(f.Units) > 0 {
			return true
		}
	}
	return false
}

// systemHasCommLaser reports whether a system has a comm laser available,
// either as a local unit or in any stationed human fleet.
func systemHasCommLaser(state *GameState, sys *StarSystem) bool {
	if sys.LocalUnits[WeaponCommLaser] > 0 {
		return true
	}
	for _, fid := range sys.FleetIDs {
		f := state.Fleets[fid]
		if f == nil || f.InTransit || f.Owner != HumanOwner {
			continue
		}
		if f.Units[WeaponCommLaser] > 0 {
			return true
		}
	}
	return false
}

func fleetIsReporterOnly(fleet *Fleet) bool {
	for wt, n := range fleet.Units {
		if wt != WeaponReporter && n > 0 {
			return false
		}
	}
	return true
}
