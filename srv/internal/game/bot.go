package game

import (
	"sort"
	"sync"
)

// BotAgent is the interface the engine uses to drive the alien side.
// An alternative bot replaces only this implementation. (FR-062, FR-064)
type BotAgent interface {
	// Initialize is called once before the game loop starts.
	Initialize(state *GameState)

	// Tick is called every BotTickCadence engine ticks with the engine's write
	// lock held. The bot must not call state.mu.Lock/RLock (would deadlock).
	// Returns zero or more commands to execute immediately (no travel delay).
	Tick(state *GameState, currentYear float64) []BotCommand

	// OnEvent is called for every newly-broadcast event.
	OnEvent(evt *GameEvent)
}

// BotCommand is a command returned by BotAgent.Tick for immediate execution.
type BotCommand struct {
	Type       CommandType
	SystemID   string     // source system for CmdMove
	WeaponType WeaponType // for CmdConstruct
	Quantity   int
	FleetID    string // for CmdMove
	DestID     string // for CmdMove
}

// DefaultBot implements the built-in alien bot. (FR-062, FR-063)
// Strategy: move all available alien fleets toward the nearest human-held system.
type DefaultBot struct {
	mu      sync.Mutex
	targets []string // system IDs prioritized for attack (sorted by dist from alien entry)
}

// NewDefaultBot creates an initialized DefaultBot.
func NewDefaultBot() *DefaultBot {
	return &DefaultBot{}
}

func (b *DefaultBot) Initialize(state *GameState) {
	// Initial target list will be computed on first Tick.
}

// Tick computes bot commands for the current game tick. (FR-063)
// Called with state.mu write-locked; must not acquire any state locks.
func (b *DefaultBot) Tick(state *GameState, currentYear float64) []BotCommand {
	defer func() {
		if r := recover(); r != nil {
			// Bot panics are logged by the engine; no commands returned.
		}
	}()

	var cmds []BotCommand

	// Build a priority target list: human-held systems sorted by distance
	// from the nearest alien entry point (closest first).
	targets := humanTargetsByProximity(state)
	if len(targets) == 0 {
		return nil
	}

	// For each alien fleet that is stationed (not in transit), dispatch it
	// toward the highest-priority target that no other alien fleet is heading to.
	inbound := alienInboundTargets(state)

	for _, fleet := range state.Fleets {
		if fleet.Owner != AlienOwner || fleet.InTransit || fleet.LocationID == "" {
			continue
		}
		if totalUnits(fleet.Units) == 0 {
			continue
		}
		// Find the best uncovered target
		for _, targetID := range targets {
			if inbound[targetID] {
				continue
			}
			destSys, ok := state.Systems[targetID]
			if !ok {
				continue
			}
			srcSys, ok := state.Systems[fleet.LocationID]
			if !ok {
				continue
			}
			if srcSys.ID == destSys.ID {
				continue // already there
			}
			cmds = append(cmds, BotCommand{
				Type:    CmdMove,
				SystemID: fleet.LocationID,
				FleetID: fleet.ID,
				DestID:  targetID,
			})
			inbound[targetID] = true
			break
		}
	}

	return cmds
}

func (b *DefaultBot) OnEvent(evt *GameEvent) {
	// DefaultBot does not react to individual events; it re-evaluates on every Tick.
}

// --- Bot helpers ---

// humanTargetsByProximity returns a sorted list of human-held system IDs,
// ordered by distance from the nearest alien entry point (closest first).
func humanTargetsByProximity(state *GameState) []string {
	type scored struct {
		id   string
		dist float64
	}

	var candidates []scored
	for id, sys := range state.Systems {
		if sys.Status != StatusHuman {
			continue
		}
		minDist := 1e18
		for _, epID := range state.Alien.EntryPointIDs {
			ep, ok := state.Systems[epID]
			if !ok {
				continue
			}
			d := distBetween(sys, ep)
			if d < minDist {
				minDist = d
			}
		}
		candidates = append(candidates, scored{id: id, dist: minDist})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].dist < candidates[j].dist
	})
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.id
	}
	return ids
}

// alienInboundTargets returns a set of system IDs that alien fleets are
// already heading toward.
func alienInboundTargets(state *GameState) map[string]bool {
	inbound := map[string]bool{}
	for _, fleet := range state.Fleets {
		if fleet.Owner == AlienOwner && fleet.InTransit && fleet.DestID != "" {
			inbound[fleet.DestID] = true
		}
	}
	return inbound
}

// totalUnits sums all unit counts in a fleet.
func totalUnits(units map[WeaponType]int) int {
	total := 0
	for _, n := range units {
		total += n
	}
	return total
}
