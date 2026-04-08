package game

import (
	"fmt"
	"math"
	"sync"
)

// GameState is the in-memory authoritative game state. The engine is the
// sole writer; HTTP handlers hold RLock for reads.
type GameState struct {
	mu sync.RWMutex // protects all fields below; excluded from JSON (json:"-")

	Clock    float64
	Paused   bool
	GameOver bool
	Winner   Owner  // "" if not over
	WinReason string

	Systems     map[string]*StarSystem // key: system ID (e.g. "gj-551")
	SystemOrder []string               // insertion order: Sol first, then by distance
	Fleets      map[string]*Fleet      // key: fleet ID
	Events      []*GameEvent           // all events, chronological by EventYear
	PendingCmds []*PendingCommand      // commands not yet arrived at target

	Human HumanFaction
	Alien AlienFaction

	nextFleetNum    int
	nextEventID     int
	nextCmdID       int
	knownStateIdx   int // index into Events: all events before this are already applied
}

// StarSystem is one game entity (one or more co-located stars treated as one system).
type StarSystem struct {
	ID          string
	DisplayName string
	X, Y, Z     float64
	DistFromSol float64
	HasPlanets  bool

	// Ground truth — only the engine writes these.
	Status         SystemStatus
	EconLevel      int     // 0 for uninhabited/alien-held; grows over time (FR-048)
	Wealth         float64 // accumulated wealth; deducted by construction
	EconGrowthYear float64 // game year at which next level-up occurs (reset on combat)
	LocalUnits     map[WeaponType]int // stationary units (OrbitalDefense, Interceptor, CommLaser)
	FleetIDs       []string           // IDs of fleets currently present

	// Last known state — derived from reported events with arrivalYear ≤ currentClock.
	KnownStatus     SystemStatus
	KnownAsOfYear   float64
	KnownEconLevel  int
	KnownWealth     float64
	KnownLocalUnits map[WeaponType]int
	KnownFleetIDs   []string
}

// Fleet represents a named group of mobile units.
type Fleet struct {
	ID         string
	Name       string
	Owner      Owner
	Units      map[WeaponType]int
	LocationID string  // system ID if stationed; "" if in transit
	DestID     string  // "" if not in transit
	DepartYear float64
	ArrivalYear float64
	InTransit  bool
}

// GameEvent is an occurrence recorded in the server-side event log.
type GameEvent struct {
	ID          string
	EventYear   float64
	ArrivalYear float64   // math.MaxFloat64 if event never reaches Earth (unreported)
	SystemID    string
	Type        EventType
	Description string
	Broadcast   bool      // true once pushed to SSE clients
	CanReport   bool      // true if a reporting mechanism existed at event time
	AppliedToKnown bool   // true once applied to system KnownState fields
	Details     interface{} // type-specific payload (CombatDetails, etc.)
}

// PendingCommand is a player command in flight toward its target system.
type PendingCommand struct {
	ID          string
	ExecuteYear float64
	OriginID    string      // always "sol" for player commands
	TargetID    string
	Type        CommandType
	WeaponType  WeaponType  // for CmdConstruct
	Quantity    int         // for CmdConstruct
	FleetID     string      // for CmdMove
	DestID      string      // for CmdMove
	IsBot       bool
}

// HumanFaction holds human-side aggregate state.
type HumanFaction struct {
	InitialSystemIDs []string // systems held at game start (for win condition)
}

// AlienFaction holds alien-side aggregate state.
type AlienFaction struct {
	TotalLost      int
	Exhausted      bool
	EntryPointIDs  []string
	NextSpawnYear  float64
}

// --- Lock helpers (for use by packages that cannot access the unexported mu) ---

func (s *GameState) Lock()    { s.mu.Lock() }
func (s *GameState) Unlock()  { s.mu.Unlock() }
func (s *GameState) RLock()   { s.mu.RLock() }
func (s *GameState) RUnlock() { s.mu.RUnlock() }

// --- ID generators (caller must hold mu.Lock) ---

var fleetNames = []string{
	"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot",
	"Golf", "Hotel", "India", "Juliet", "Kilo", "Lima", "Mike",
	"November", "Oscar", "Papa", "Quebec", "Romeo", "Sierra",
	"Tango", "Uniform", "Victor", "Whiskey", "Xray", "Yankee", "Zulu",
}

// NewFleetID returns the next unique fleet ID and advances the counter.
func (s *GameState) NewFleetID() string {
	s.nextFleetNum++
	return fmt.Sprintf("fleet-%d", s.nextFleetNum)
}

// NewFleetName returns the name corresponding to the current fleet counter.
// Must be called immediately after NewFleetID (uses s.nextFleetNum).
func (s *GameState) NewFleetName() string {
	n := s.nextFleetNum
	idx := n - 1
	if idx < len(fleetNames) {
		return "Fleet " + fleetNames[idx]
	}
	gen := idx / len(fleetNames)
	return fmt.Sprintf("Fleet %s %d", fleetNames[idx%len(fleetNames)], gen+1)
}

// NewEventID returns the next unique event ID.
func (s *GameState) NewEventID() string {
	s.nextEventID++
	return fmt.Sprintf("evt-%d", s.nextEventID)
}

// NewCommandID returns the next unique command ID.
func (s *GameState) NewCommandID() string {
	s.nextCmdID++
	return fmt.Sprintf("cmd-%d", s.nextCmdID)
}

// --- State mutation (caller must hold mu.Lock) ---

// RecordEvent appends evt to the event log, assigning an ID if not set.
func (s *GameState) RecordEvent(evt *GameEvent) {
	if evt.ID == "" {
		evt.ID = s.NewEventID()
	}
	s.Events = append(s.Events, evt)
}

// ApplyCommand executes a pending command against ground-truth state.
// Returns an error if the command cannot be executed (insufficient wealth,
// fleet not found, etc.). The caller logs the failure.
func (s *GameState) ApplyCommand(cmd *PendingCommand) error {
	sys, ok := s.Systems[cmd.TargetID]
	if !ok {
		return fmt.Errorf("system %q not found", cmd.TargetID)
	}

	// Log command arrival (FR-015)
	hasCommLaser := sys.LocalUnits[WeaponCommLaser] > 0
	arrivalArrYear := arrivalYearFor(s.Clock, sys.DistFromSol, hasCommLaser)
	s.RecordEvent(&GameEvent{
		EventYear:   s.Clock,
		ArrivalYear: arrivalArrYear,
		SystemID:    cmd.TargetID,
		Type:        EventCommandArrived,
		Description: fmt.Sprintf("Command %s arrived at %s", cmd.Type, sys.DisplayName),
		CanReport:   hasCommLaser,
	})

	switch cmd.Type {
	case CmdConstruct:
		if err := ValidateConstruct(sys, cmd.WeaponType, cmd.Quantity); err != nil {
			return err
		}
		ExecuteConstruct(s, sys, cmd.WeaponType, cmd.Quantity)

	case CmdMove:
		fleet, ok := s.Fleets[cmd.FleetID]
		if !ok {
			return fmt.Errorf("fleet %q not found", cmd.FleetID)
		}
		if fleet.InTransit {
			return fmt.Errorf("fleet %q is already in transit", cmd.FleetID)
		}
		if fleet.LocationID != cmd.TargetID {
			return fmt.Errorf("fleet %q is not at system %q", cmd.FleetID, cmd.TargetID)
		}
		dest, ok := s.Systems[cmd.DestID]
		if !ok {
			return fmt.Errorf("destination system %q not found", cmd.DestID)
		}
		travelYears := distBetween(sys, dest) / FleetSpeedC
		fleet.InTransit = true
		fleet.DepartYear = s.Clock
		fleet.ArrivalYear = s.Clock + travelYears
		fleet.DestID = cmd.DestID
		// Remove from current system's fleet list
		sys.FleetIDs = removeString(sys.FleetIDs, fleet.ID)
		fleet.LocationID = ""

	default:
		return fmt.Errorf("unknown command type %q", cmd.Type)
	}

	// Log successful execution
	execArrYear := arrivalYearFor(s.Clock, sys.DistFromSol, hasCommLaser)
	s.RecordEvent(&GameEvent{
		EventYear:   s.Clock,
		ArrivalYear: execArrYear,
		SystemID:    cmd.TargetID,
		Type:        EventCommandExecuted,
		Description: fmt.Sprintf("Command %s executed at %s", cmd.Type, sys.DisplayName),
		CanReport:   hasCommLaser,
	})
	return nil
}

// UpdateKnownStates applies all newly matured events (arrivalYear ≤ clock,
// !AppliedToKnown) to each system's KnownState fields. (FR-018)
func (s *GameState) UpdateKnownStates(clock float64) {
	for s.knownStateIdx < len(s.Events) {
		evt := s.Events[s.knownStateIdx]
		if evt.ArrivalYear > clock || evt.ArrivalYear >= math.MaxFloat64 {
			break
		}
		sys, ok := s.Systems[evt.SystemID]
		if ok {
			applyEventToKnownState(sys, evt)
		}
		evt.AppliedToKnown = true
		s.knownStateIdx++
	}
}

// applyEventToKnownState updates the system's known fields based on the event.
func applyEventToKnownState(sys *StarSystem, evt *GameEvent) {
	// Always advance the "known as of" year to the event's origin year.
	if evt.EventYear > sys.KnownAsOfYear {
		sys.KnownAsOfYear = evt.EventYear
	}

	switch evt.Type {
	case EventCombatOccurred:
		if d, ok := evt.Details.(*CombatDetails); ok {
			if d.HumanWon {
				sys.KnownStatus = StatusHuman
			} else if d.AlienWon {
				sys.KnownStatus = StatusAlien
			} else if d.Draw {
				sys.KnownStatus = StatusContested
			}
			// Apply known losses
			for wt, n := range d.HumanLosses {
				if sys.KnownLocalUnits != nil {
					if sys.KnownLocalUnits[wt] >= n {
						sys.KnownLocalUnits[wt] -= n
					} else {
						sys.KnownLocalUnits[wt] = 0
					}
				}
			}
		}

	case EventSystemCaptured:
		sys.KnownStatus = StatusAlien

	case EventSystemRetaken:
		sys.KnownStatus = StatusHuman

	case EventConstructionDone:
		if d, ok := evt.Details.(*ConstructionDetails); ok {
			if sys.KnownLocalUnits == nil {
				sys.KnownLocalUnits = map[WeaponType]int{}
			}
			if !WeaponDefs[d.WeaponType].CanMove {
				sys.KnownLocalUnits[d.WeaponType] += d.Quantity
			}
		}

	case EventEconGrowth:
		if d, ok := evt.Details.(*EconGrowthDetails); ok {
			sys.KnownEconLevel = d.NewLevel
		}

	case EventFleetArrival:
		if d, ok := evt.Details.(*FleetArrivalDetails); ok && d.Owner == HumanOwner {
			sys.KnownFleetIDs = appendIfMissing(sys.KnownFleetIDs, d.FleetID)
		}
	}
}

// CheckVictory evaluates win/loss conditions. Returns (true, winner, reason)
// if the game is over, or (false, "", "") otherwise. (FR-056, FR-057)
func (s *GameState) CheckVictory() (over bool, winner Owner, reason string) {
	totalSystems := len(s.Systems)
	if totalSystems == 0 {
		return
	}

	// Count current system statuses
	humanHeld := 0
	alienHeld := 0
	for _, sys := range s.Systems {
		switch sys.Status {
		case StatusHuman:
			humanHeld++
		case StatusAlien:
			alienHeld++
		}
	}

	// FR-057: Alien wins if it captures Earth OR holds ≥ AlienWinCaptureFraction of all systems.
	if sol, ok := s.Systems["sol"]; ok && sol.Status == StatusAlien {
		return true, AlienOwner, "Earth has been captured by alien forces."
	}
	humanInitial := len(s.Human.InitialSystemIDs)
	if humanInitial > 0 && float64(alienHeld)/float64(humanInitial) >= AlienWinCaptureFraction {
		return true, AlienOwner, fmt.Sprintf("Alien forces control %.0f%% of human systems.", float64(alienHeld)/float64(humanInitial)*100)
	}

	// FR-056: Human wins if alien exhausted AND Earth human-held AND
	// fraction of originally human-held systems still human-held ≥ HumanWinRetentionFraction.
	if s.Alien.Exhausted {
		sol, solOK := s.Systems["sol"]
		if solOK && sol.Status == StatusHuman {
			initialCount := len(s.Human.InitialSystemIDs)
			if initialCount > 0 {
				retained := 0
				for _, id := range s.Human.InitialSystemIDs {
					if sys, ok := s.Systems[id]; ok && sys.Status == StatusHuman {
						retained++
					}
				}
				retainedFrac := float64(retained) / float64(initialCount)
				if retainedFrac >= HumanWinRetentionFraction {
					return true, HumanOwner, fmt.Sprintf(
						"Alien forces exhausted. Earth and %.0f%% of systems retained.", retainedFrac*100)
				}
			}
		}
	}

	return false, "", ""
}

// --- Helpers ---

// arrivalYearFor computes the event arrival year at Sol based on whether
// a comm laser is present (speed c) or not (math.MaxFloat64 = unreported).
func arrivalYearFor(clock, distFromSol float64, hasCommLaser bool) float64 {
	if hasCommLaser {
		return clock + distFromSol // at c
	}
	return math.MaxFloat64
}

// distBetween returns the Euclidean distance between two systems in light-years.
func distBetween(a, b *StarSystem) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// removeString removes the first occurrence of s from slice.
func removeString(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// appendIfMissing appends s to slice only if not already present.
func appendIfMissing(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// copyUnits returns a shallow copy of a WeaponType→int map.
func copyUnits(m map[WeaponType]int) map[WeaponType]int {
	out := make(map[WeaponType]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
