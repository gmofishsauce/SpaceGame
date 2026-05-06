package game

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// Engine runs the authoritative game loop and coordinates all subsystems.
type Engine struct {
	State  *GameState
	Events *EventManager

	tickCount       int
	rng             *rand.Rand
	connectedMu     sync.Mutex
	connectedCounts map[Owner]int // SSE connections per player
}

// NewEngine creates an Engine wired to the given state and event manager.
// rng is injected (DR-11) so tests can seed deterministically. The Propagator
// is constructed here and stored on state. The game starts paused until both
// players connect. (FR-45, FR-46)
func NewEngine(state *GameState, events *EventManager, rng *rand.Rand) *Engine {
	state.Propagator = NewPropagator(events)
	state.Paused = true // wait for both players (FR-45)
	return &Engine{
		State:           state,
		Events:          events,
		rng:             rng,
		connectedCounts: map[Owner]int{HumanOwner: 0, AlienOwner: 0},
	}
}

// Run blocks, running the game loop until ctx is cancelled. Call in a goroutine.
// (FR-011, FR-012, FR-013)
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(TickIntervalMs * time.Millisecond)
	defer ticker.Stop()

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

// OnPlayerConnected is called by the SSE handler when a player's client
// connects. Auto-unpauses when both players have at least one connection.
// (FR-46)
func (e *Engine) OnPlayerConnected(player Owner) {
	e.connectedMu.Lock()
	e.connectedCounts[player]++
	both := e.connectedCounts[HumanOwner] > 0 && e.connectedCounts[AlienOwner] > 0
	e.connectedMu.Unlock()

	if both {
		e.State.Lock()
		if e.State.Paused && !e.State.GameOver {
			e.State.Paused = false
			log.Printf("engine: both players connected; game unpaused")
		}
		e.State.Unlock()
		e.State.RLock()
		e.Events.BroadcastClockSync(e.State)
		e.State.RUnlock()
	}
}

// OnPlayerDisconnected is called by the SSE handler when a player's last
// client disconnects. Pauses the game and logs. (FR-47, NFR-13)
func (e *Engine) OnPlayerDisconnected(player Owner) {
	e.connectedMu.Lock()
	if e.connectedCounts[player] > 0 {
		e.connectedCounts[player]--
	}
	wasLast := e.connectedCounts[player] == 0
	e.connectedMu.Unlock()

	if wasLast {
		e.State.Lock()
		e.State.Paused = true
		e.State.Unlock()
		log.Printf("server: %s disconnected; game paused", player)
		e.State.RLock()
		e.Events.BroadcastClockSync(e.State)
		e.State.RUnlock()
	}
}

// EnqueueCommand validates and enqueues a player command. Returns the command
// ID and estimated arrival year, or an error. Safe to call from HTTP handlers.
// (FR-25, FR-26, FR-27)
func (e *Engine) EnqueueCommand(cmd *PendingCommand) (string, float64, error) {
	e.State.Lock()
	defer e.State.Unlock()

	cat := e.State.Catalog.Get(cmd.TargetID)
	if cat == nil {
		return "", 0, fmt.Errorf("unknown system %q", cmd.TargetID)
	}

	homeID := e.State.Homes[cmd.Issuer]

	// Quick pre-check against the issuer's known view; full validation at execution.
	if cmd.TargetID != homeID {
		if ks := e.State.Views[cmd.Issuer].System(cmd.TargetID); ks != nil && statusToOwner(ks.Status) == OpposingOwner(cmd.Issuer) {
			return "", 0, fmt.Errorf("system %q is known to be opponent-held; cannot issue commands", cmd.TargetID)
		}
	}

	var executeYear float64
	if cmd.TargetID == homeID {
		executeYear = e.State.Clock
	} else {
		executeYear = e.State.Clock + e.State.Catalog.Distance(homeID, cmd.TargetID)/CommandSpeedC
	}

	cmd.ID = e.State.NewCommandID()
	cmd.ExecuteYear = executeYear
	cmd.OriginID = homeID
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
			}
			// Note: fleet-departure broadcasting now happens via EventFleetDeparted
			// recorded inside ApplyCommand; no synchronous broadcast here.
		} else {
			remaining = append(remaining, cmd)
		}
	}
	e.State.PendingCmds = remaining

	// Accumulate wealth and advance economic levels.
	AccumulateWealth(e.State, YearsPerTick)
	AdvanceEconLevels(e.State)

	// Check for and resolve combat in any system where both sides are present.
	for _, sys := range e.State.truth.Systems {
		if humanForcesPresent(e.State.truth, sys) && alienForcesPresent(e.State.truth, sys) {
			Resolve(e.rng, e.State, sys)
		}
	}

	e.tickCount++

	// Propagate matured events: apply to SolView and broadcast SSE.
	// Replaces the old UpdateKnownStates + BroadcastMatured pair.
	e.State.Propagator.Propagate(e.State)

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
	for _, fleet := range e.State.truth.Fleets {
		if !fleet.InTransit || fleet.ArrivalYear > e.State.Clock {
			continue
		}
		dest := e.State.truth.System(fleet.DestID)
		if dest == nil {
			log.Printf("engine: fleet %s destination %q not found, removing", fleet.ID, fleet.DestID)
			delete(e.State.truth.Fleets, fleet.ID)
			continue
		}

		// Reporter fleets arriving at their owner's home are consumed (their
		// event was already logged at combat time). (FR-23)
		homeID := e.State.Homes[fleet.Owner]
		if fleet.DestID == homeID && fleetIsReporterOnly(fleet) {
			delete(e.State.truth.Fleets, fleet.ID)
			e.State.RecordEvent(&Event{
				EventYear:   fleet.ArrivalYear,
				SystemID:    homeID,
				Type:        EventReporterReturn,
				Description: fmt.Sprintf("Reporter fleet %s returned home with intelligence", fleet.Name),
			}, fleet.Owner, true, false)
			continue
		}

		// Normal fleet arrival.
		fleet.InTransit = false
		fleet.LocationID = fleet.DestID
		fleet.DestID = ""
		fleet.SourceID = ""

		dest.FleetIDs = appendIfMissing(dest.FleetIDs, fleet.ID)

		// Determine if this arrival is reportable (comm laser at destination)
		hasCommLaser := systemHasCommLaser(e.State.truth, dest)
		displayName := dest.ID
		if c := e.State.Catalog.Get(dest.ID); c != nil {
			displayName = c.DisplayName
		}
		e.State.RecordEvent(&Event{
			EventYear:   e.State.Clock,
			SystemID:    dest.ID,
			Type:        EventFleetArrival,
			Description: fmt.Sprintf("Fleet %s arrived at %s", fleet.Name, displayName),
			Details: &FleetArrivalDetails{
				FleetID:   fleet.ID,
				FleetName: fleet.Name,
				Owner:     fleet.Owner,
				Units:     copyUnits(fleet.Units),
			},
		}, fleet.Owner, hasCommLaser, false)

		// Conquest: a fleet carrying a comm laser that arrives at an
		// uninhabited system claims it for its owner. Economy starts at
		// level 0, wealth 0.
		if dest.Status == StatusUninhabited && fleet.Units[WeaponCommLaser] > 0 {
			dest.Status = statusOf(fleet.Owner)
			dest.EconLevel = 0
			dest.Wealth = 0
			dest.EconGrowthYear = e.State.Clock + EconGrowthIntervalYears

			e.State.RecordEvent(&Event{
				EventYear:   e.State.Clock,
				SystemID:    dest.ID,
				Type:        EventSystemConquered,
				Description: fmt.Sprintf("Fleet %s established a colony at %s", fleet.Name, displayName),
			}, fleet.Owner, true, false)
		}
	}
}

// logCommandFailed records a command_failed event for a command that could not execute.
func (e *Engine) logCommandFailed(cmd *PendingCommand, err error) {
	displayName := cmd.TargetID
	if c := e.State.Catalog.Get(cmd.TargetID); c != nil {
		displayName = c.DisplayName
	}
	sys := e.State.truth.System(cmd.TargetID)
	hasCommLaser := sys != nil && systemHasCommLaser(e.State.truth, sys)
	e.State.RecordEvent(&Event{
		EventYear:   e.State.Clock,
		SystemID:    cmd.TargetID,
		Type:        EventCommandFailed,
		Description: fmt.Sprintf("Command %s at %s failed: %v", cmd.Type, displayName, err),
		Details:     &CommandFailedDetails{CommandType: cmd.Type, Reason: err.Error()},
	}, cmd.Issuer, hasCommLaser, false)
}

// --- Force presence checks ---

func humanForcesPresent(truth *Truth, sys *TrueSystem) bool {
	for wt, n := range sys.LocalUnits {
		if !WeaponDefs[wt].CommLaser && n > 0 {
			return true
		}
	}
	for _, fid := range sys.FleetIDs {
		f := truth.Fleets[fid]
		if f == nil || f.InTransit || f.Owner != HumanOwner {
			continue
		}
		if totalUnits(f.Units) > 0 {
			return true
		}
	}
	return false
}

func alienForcesPresent(truth *Truth, sys *TrueSystem) bool {
	for _, fid := range sys.FleetIDs {
		f := truth.Fleets[fid]
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
// either as a local unit or in any stationed fleet (any owner).
func systemHasCommLaser(truth *Truth, sys *TrueSystem) bool {
	if sys.LocalUnits[WeaponCommLaser] > 0 {
		return true
	}
	for _, fid := range sys.FleetIDs {
		f := truth.Fleets[fid]
		if f == nil || f.InTransit {
			continue
		}
		if f.Units[WeaponCommLaser] > 0 {
			return true
		}
	}
	return false
}

// totalUnits sums all unit counts in a fleet.
func totalUnits(units map[WeaponType]int) int {
	total := 0
	for _, n := range units {
		total += n
	}
	return total
}

func fleetIsReporterOnly(fleet *TrueFleet) bool {
	for wt, n := range fleet.Units {
		if wt != WeaponReporter && n > 0 {
			return false
		}
	}
	return true
}
