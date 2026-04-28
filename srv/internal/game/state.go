package game

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
)

// GameState is the in-memory authoritative game state. The engine is the
// sole writer; HTTP handlers hold RLock for reads. The truth field is
// unexported so packages outside game cannot reach ground truth (DR-1).
type GameState struct {
	mu sync.RWMutex // protects all fields below; excluded from JSON (json:"-")

	Clock     float64
	Paused    bool
	GameOver  bool
	Winner    Owner // "" if not over
	WinReason string

	Catalog *StarCatalog
	truth   *Truth // unexported: only package game may dereference
	SolView *SolView
	Events  *EventLog

	Propagator *Propagator

	Human       HumanFaction
	Alien       AlienFaction
	PendingCmds []*PendingCommand

	rng *rand.Rand // injected (DR-11)

	nextFleetNum int
	nextCmdID    int
}

// PendingCommand is a player command in flight toward its target system.
type PendingCommand struct {
	ID            string
	ExecuteYear   float64
	OriginID      string // always "sol" for player commands
	TargetID      string
	Type          CommandType
	WeaponType    WeaponType // for CmdConstruct
	Quantity      int        // for CmdConstruct
	FleetID       string     // for CmdMove
	DestID        string     // for CmdMove
	SourceFleetID string     // for CmdReassign
	TargetFleetID string     // for CmdReassign
	ReassignUnits map[WeaponType]int
	IsBot         bool
}

// HumanFaction holds human-side aggregate state.
type HumanFaction struct {
	InitialSystemIDs []string // systems held at game start (for win condition)
}

// AlienFaction holds alien-side aggregate state.
type AlienFaction struct {
	TotalLost     int
	Exhausted     bool
	EntryPointIDs []string
	NextSpawnYear float64
}

// --- Lock helpers (for use by packages that cannot access the unexported mu) ---

func (s *GameState) Lock()    { s.mu.Lock() }
func (s *GameState) Unlock()  { s.mu.Unlock() }
func (s *GameState) RLock()   { s.mu.RLock() }
func (s *GameState) RUnlock() { s.mu.RUnlock() }

// Truth returns the omniscient ground-truth state. It is intended for use
// by package game only (engine, combat, economy, bot). Code outside the
// game package shall not call this method (DR-1 verification: a grep for
// "state.Truth()" under srv/internal/server/ must return nothing).
func (s *GameState) Truth() *Truth { return s.truth }

// Rng returns the injected RNG. Use via state.Rng() so tests can inject a
// deterministic seed (DR-11).
func (s *GameState) Rng() *rand.Rand { return s.rng }

// ReadSolGroundTruth is the single explicit affordance by which the HTTP
// layer reads Sol's own state from Truth. The player sees Sol with no
// light-speed delay; this preserves DR-1 in spirit by limiting access to
// one grep-able accessor with a clear name.
func (s *GameState) ReadSolGroundTruth() SolGroundTruthSnapshot {
	sol := s.truth.Systems["sol"]
	if sol == nil {
		return SolGroundTruthSnapshot{}
	}
	return SolGroundTruthSnapshot{
		Status:     sol.Status,
		EconLevel:  sol.EconLevel,
		Wealth:     sol.Wealth,
		LocalUnits: copyUnits(sol.LocalUnits),
		FleetIDs:   append([]string(nil), sol.FleetIDs...),
	}
}

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

// NewCommandID returns the next unique command ID.
func (s *GameState) NewCommandID() string {
	s.nextCmdID++
	return fmt.Sprintf("cmd-%d", s.nextCmdID)
}

// --- State mutation (caller must hold mu.Lock) ---

// ApplyCommand executes a pending command against ground-truth state.
// Returns an error if the command cannot be executed (insufficient wealth,
// fleet not found, etc.). The caller logs the failure.
func (s *GameState) ApplyCommand(cmd *PendingCommand) error {
	sys := s.truth.System(cmd.TargetID)
	if sys == nil {
		return fmt.Errorf("system %q not found", cmd.TargetID)
	}
	displayName := cmd.TargetID
	if e := s.Catalog.Get(cmd.TargetID); e != nil {
		displayName = e.DisplayName
	}

	// Log command arrival (FR-015)
	hasCommLaser := systemHasCommLaser(s.truth, sys)
	distFromSol := 0.0
	if e := s.Catalog.Get(cmd.TargetID); e != nil {
		distFromSol = e.DistFromSol
	}
	arrivalArrYear := arrivalYearFor(s.Clock, distFromSol, hasCommLaser)
	s.Events.Record(&Event{
		EventYear:   s.Clock,
		ArrivalYear: arrivalArrYear,
		SystemID:    cmd.TargetID,
		Type:        EventCommandArrived,
		Description: fmt.Sprintf("Command %s arrived at %s", cmd.Type, displayName),
	})

	switch cmd.Type {
	case CmdConstruct:
		if err := ValidateConstruct(sys, cmd.WeaponType, cmd.Quantity); err != nil {
			return err
		}
		ExecuteConstruct(s, sys, cmd.WeaponType, cmd.Quantity)

	case CmdMove:
		fleet := s.truth.Fleet(cmd.FleetID)
		if fleet == nil {
			return fmt.Errorf("fleet %q not found", cmd.FleetID)
		}
		if fleet.InTransit {
			return fmt.Errorf("fleet %q is already in transit", cmd.FleetID)
		}
		if fleet.LocationID != cmd.TargetID {
			return fmt.Errorf("fleet %q is not at system %q", cmd.FleetID, cmd.TargetID)
		}
		dest := s.truth.System(cmd.DestID)
		if dest == nil {
			return fmt.Errorf("destination system %q not found", cmd.DestID)
		}
		travelYears := s.Catalog.Distance(cmd.TargetID, cmd.DestID) / FleetSpeedC
		fleet.InTransit = true
		fleet.DepartYear = s.Clock
		fleet.ArrivalYear = s.Clock + travelYears
		fleet.DestID = cmd.DestID
		fleet.SourceID = cmd.TargetID
		// Remove from current system's fleet list
		sys.FleetIDs = removeString(sys.FleetIDs, fleet.ID)
		fleet.LocationID = ""

		// Record EventFleetDeparted; reportable iff source has a comm laser.
		// (Replaces the old synchronous BroadcastFleetDeparted path; DR-3.)
		if fleet.Owner == HumanOwner {
			depArrYear := arrivalYearFor(s.Clock, distFromSol, hasCommLaser)
			s.Events.Record(&Event{
				EventYear:   s.Clock,
				ArrivalYear: depArrYear,
				SystemID:    cmd.TargetID,
				Type:        EventFleetDeparted,
				Description: fmt.Sprintf("Fleet %s departed %s for %s", fleet.Name, displayName, cmd.DestID),
				Details: &FleetDepartureDetails{
					FleetID:     fleet.ID,
					FleetName:   fleet.Name,
					Owner:       fleet.Owner,
					Units:       copyUnits(fleet.Units),
					SourceID:    cmd.TargetID,
					DestID:      cmd.DestID,
					DepartYear:  fleet.DepartYear,
					ArrivalYear: fleet.ArrivalYear,
				},
			})
		}

	case CmdCreateFleet:
		if err := ExecuteCreateFleet(s, sys); err != nil {
			return err
		}

	case CmdReassign:
		if err := ExecuteReassign(s, sys, cmd); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown command type %q", cmd.Type)
	}

	// Log successful execution
	execArrYear := arrivalYearFor(s.Clock, distFromSol, hasCommLaser)
	s.Events.Record(&Event{
		EventYear:   s.Clock,
		ArrivalYear: execArrYear,
		SystemID:    cmd.TargetID,
		Type:        EventCommandExecuted,
		Description: fmt.Sprintf("Command %s executed at %s", cmd.Type, displayName),
	})
	return nil
}

// CheckVictory evaluates win/loss conditions. Returns (true, winner, reason)
// if the game is over, or (false, "", "") otherwise. (FR-056, FR-057)
func (s *GameState) CheckVictory() (over bool, winner Owner, reason string) {
	totalSystems := len(s.truth.Systems)
	if totalSystems == 0 {
		return
	}

	// Count current system statuses
	humanHeld := 0
	alienHeld := 0
	for _, sys := range s.truth.Systems {
		switch sys.Status {
		case StatusHuman:
			humanHeld++
		case StatusAlien:
			alienHeld++
		}
	}

	// FR-057: Alien wins if it captures Earth OR holds ≥ AlienWinCaptureFraction of all systems.
	if sol, ok := s.truth.Systems["sol"]; ok && sol.Status == StatusAlien {
		return true, AlienOwner, "Earth has been captured by alien forces."
	}
	humanInitial := len(s.Human.InitialSystemIDs)
	if humanInitial > 0 && float64(alienHeld)/float64(humanInitial) >= AlienWinCaptureFraction {
		return true, AlienOwner, fmt.Sprintf("Alien forces control %.0f%% of human systems.", float64(alienHeld)/float64(humanInitial)*100)
	}

	// FR-056: Human wins if alien exhausted AND Earth human-held AND
	// fraction of originally human-held systems still human-held ≥ HumanWinRetentionFraction.
	if s.Alien.Exhausted {
		sol, solOK := s.truth.Systems["sol"]
		if solOK && sol.Status == StatusHuman {
			initialCount := len(s.Human.InitialSystemIDs)
			if initialCount > 0 {
				retained := 0
				for _, id := range s.Human.InitialSystemIDs {
					if sys, ok := s.truth.Systems[id]; ok && sys.Status == StatusHuman {
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
