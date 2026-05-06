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
	// Views replaces SolView. One PlayerView per Owner. (FR-12)
	Views  map[Owner]*PlayerView
	Events *EventLog

	Propagator *Propagator

	// Factions and Homes replace the asymmetric Human/Alien fields. (FR-8)
	// Both maps are keyed by Owner (HumanOwner / AlienOwner). Homes is
	// populated at load time (AS-2): "sol" for HumanOwner; the alien home
	// ID will be added in M6 when the alien CSVs are loaded.
	Factions    map[Owner]*Faction
	Homes       map[Owner]string
	PendingCmds []*PendingCommand

	rng *rand.Rand // injected (DR-11)

	nextFleetNum int
	nextCmdID    int
}

// PendingCommand is a player command in flight toward its target system.
type PendingCommand struct {
	ID            string
	ExecuteYear   float64
	OriginID      string // = state.Homes[Issuer] for player commands
	Issuer        Owner  // which player issued this command (FR-25, FR-27)
	TargetID      string
	Type          CommandType
	WeaponType    WeaponType // for CmdConstruct
	Quantity      int        // for CmdConstruct
	FleetID       string     // for CmdMove
	DestID        string     // for CmdMove
	SourceFleetID string     // for CmdReassign
	TargetFleetID string     // for CmdReassign
	ReassignUnits map[WeaponType]int
}

// Faction holds per-player state. Used twice — once for human, once for
// alien — keyed by Owner in GameState.Factions. (FR-8)
type Faction struct {
	InitialSystemIDs []string // systems held by this side at t=0 (FR-55)
}

// DrawWinner is the sentinel Owner value used to encode a draw in
// GameState.Winner and on the wire (FR-60). It is not a real owner.
const DrawWinner Owner = "draw"

// OpposingOwner returns the other player. (FR-54, FR-55)
func OpposingOwner(p Owner) Owner {
	if p == HumanOwner {
		return AlienOwner
	}
	return HumanOwner
}

// HomeIDOf returns the home system ID for the given player. The map is
// populated at load time (AS-2); a missing entry yields "" so callers
// dereferencing into Truth get a nil and short-circuit safely.
func (s *GameState) HomeIDOf(p Owner) string { return s.Homes[p] }

// statusToOwner converts a SystemStatus to the corresponding Owner, or
// "" for non-owned statuses (uninhabited / contested / unknown).
func statusToOwner(st SystemStatus) Owner {
	switch st {
	case StatusHuman:
		return HumanOwner
	case StatusAlien:
		return AlienOwner
	default:
		return ""
	}
}

// statusOf returns the SystemStatus that corresponds to owner.
func statusOf(owner Owner) SystemStatus {
	switch owner {
	case HumanOwner:
		return StatusHuman
	case AlienOwner:
		return StatusAlien
	default:
		return StatusUninhabited
	}
}

// RecordEvent computes per-player arrival times for an event originating
// at e.SystemID at time e.EventYear, with the given report path
// (comm laser → 1.0c, reporter-fled → 0.8c, neither → unreportable),
// then records the event. Reports route to the reportTo player only;
// the other player's arrival is set to math.MaxFloat64. (FR-13, FR-20,
// FR-21, design §6.4)
//
// reportTo == "" means the event is unreportable to either side
// (e.g. EventCombatSilent). Internal events should set Internal=true on
// the event before calling — Record() will skip the heap push regardless
// of arrival times.
//
// Caller must hold state.mu (write lock).
func (s *GameState) RecordEvent(e *Event, reportTo Owner, hasCommLaser bool, reporterFled bool) {
	arrivals := map[Owner]float64{
		HumanOwner: math.MaxFloat64,
		AlienOwner: math.MaxFloat64,
	}
	if reportTo != "" {
		homeID := s.Homes[reportTo]
		var d float64
		if homeID != "" {
			d = s.Catalog.Distance(e.SystemID, homeID)
		}
		switch {
		case hasCommLaser:
			arrivals[reportTo] = e.EventYear + d // 1.0c (FR-20)
		case reporterFled:
			arrivals[reportTo] = e.EventYear + d/FleetSpeedC // 0.8c (FR-20)
		}
	}
	e.Arrival = arrivals
	s.Events.Record(e)
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

// NewGameStateForTest constructs a GameState for use in server-level tests
// that live outside the game package. The caller supplies fully-populated
// truth, human view, and alien view; the constructor wires them into the
// unexported truth field and registers both player views. It also attaches a
// Propagator, EventLog, and empty Factions/Homes/PendingCmds.
func NewGameStateForTest(cat *StarCatalog, truth *Truth, humanView, alienView *PlayerView, rng *rand.Rand) *GameState {
	em := NewEventManager()
	st := &GameState{
		Catalog: cat,
		truth:   truth,
		Views:   map[Owner]*PlayerView{HumanOwner: humanView, AlienOwner: alienView},
		Events:  NewEventLog(),
		Factions: map[Owner]*Faction{
			HumanOwner: {},
			AlienOwner: {},
		},
		Homes:       map[Owner]string{HumanOwner: "sol"},
		PendingCmds: []*PendingCommand{},
		rng:         rng,
	}
	st.Propagator = NewPropagator(em)
	return st
}

// ReadHomeGroundTruth is the single explicit affordance by which the HTTP
// layer reads a player's own home-system state from Truth. The player sees
// their own home with no light-speed delay; this preserves DR-1 in spirit
// by limiting access to one grep-able accessor with a clear name.
// (FR-44 partial; renamed from ReadSolGroundTruth in M2.)
func (s *GameState) ReadHomeGroundTruth(p Owner) HomeGroundTruthSnapshot {
	home := s.truth.Systems[s.Homes[p]]
	if home == nil {
		return HomeGroundTruthSnapshot{}
	}
	return HomeGroundTruthSnapshot{
		Status:     home.Status,
		EconLevel:  home.EconLevel,
		Wealth:     home.Wealth,
		LocalUnits: copyUnits(home.LocalUnits),
		FleetIDs:   append([]string(nil), home.FleetIDs...),
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
	s.RecordEvent(&Event{
		EventYear:   s.Clock,
		SystemID:    cmd.TargetID,
		Type:        EventCommandArrived,
		Description: fmt.Sprintf("Command %s arrived at %s", cmd.Type, displayName),
	}, cmd.Issuer, hasCommLaser, false)

	switch cmd.Type {
	case CmdConstruct:
		if err := ValidateConstruct(sys, cmd.WeaponType, cmd.Quantity, cmd.Issuer); err != nil {
			return err
		}
		ExecuteConstruct(s, sys, cmd.WeaponType, cmd.Quantity, cmd.Issuer)

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
		s.RecordEvent(&Event{
			EventYear:   s.Clock,
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
		}, cmd.Issuer, hasCommLaser, false)

	case CmdCreateFleet:
		if err := ExecuteCreateFleet(s, sys, cmd.Issuer); err != nil {
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
	s.RecordEvent(&Event{
		EventYear:   s.Clock,
		SystemID:    cmd.TargetID,
		Type:        EventCommandExecuted,
		Description: fmt.Sprintf("Command %s executed at %s", cmd.Type, displayName),
	}, cmd.Issuer, hasCommLaser, false)
	return nil
}

// CheckVictory evaluates win conditions symmetrically. Returns (true,
// winner, reason) if the game is over, or (false, "", "") otherwise.
// (FR-54, FR-55, FR-56, FR-57, FR-60)
func (s *GameState) CheckVictory() (over bool, winner Owner, reason string) {
	if len(s.truth.Systems) == 0 {
		return
	}

	// 1. Capture-of-home check, both directions.
	for _, p := range []Owner{HumanOwner, AlienOwner} {
		opp := OpposingOwner(p)
		oppHome := s.truth.Systems[s.Homes[opp]]
		if oppHome != nil && statusToOwner(oppHome.Status) == p {
			return true, p, fmt.Sprintf("%s captured the opponent's home (%s).",
				p, oppHome.ID)
		}
	}

	// 2. Initial-systems-fraction check, both directions.
	for _, p := range []Owner{HumanOwner, AlienOwner} {
		opp := OpposingOwner(p)
		fac := s.Factions[opp]
		if fac == nil || len(fac.InitialSystemIDs) == 0 {
			continue
		}
		held := 0
		for _, id := range fac.InitialSystemIDs {
			if sys, ok := s.truth.Systems[id]; ok && statusToOwner(sys.Status) == p {
				held++
			}
		}
		frac := float64(held) / float64(len(fac.InitialSystemIDs))
		if frac >= WinRetentionFraction {
			return true, p, fmt.Sprintf(
				"%s holds %.0f%% of opponent's initial systems.", p, frac*100)
		}
	}

	// 3. Draw on game-length cap.
	if s.Clock >= DrawYearCap {
		return true, DrawWinner, fmt.Sprintf(
			"Game ended at year %.1f without a victor.", s.Clock)
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
