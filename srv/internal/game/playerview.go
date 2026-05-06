package game

// PlayerView is one player's filtered, light-speed-delayed view of the world.
// It is mutated only by the Propagator (the single bridge between EventLog
// and the views) and read by HTTP/SSE handlers. Fleet entries are SNAPSHOTS
// taken at AsOfYear -- they do not alias into Truth. (FR-12)
//
// One PlayerView exists per player and lives on GameState.Views, keyed by
// Owner. (Renamed from SolView in M2.)
type PlayerView struct {
	Systems   map[string]*KnownSystem  // keyed by system ID
	Fleets    map[string]*KnownFleet   // keyed by fleet ID; snapshots, not aliases
	InTransit map[string]*KnownTransit // keyed by fleet ID; transits known to this player
}

// KnownSystem is what the player knows about one star system.
type KnownSystem struct {
	ID         string
	Status     SystemStatus
	AsOfYear   float64 // year of latest news from this system
	EconLevel  int
	Wealth     float64 // last reported wealth (not extrapolated server-side)
	LocalUnits map[WeaponType]int
	FleetIDs   []string // IDs into PlayerView.Fleets, NOT Truth.Fleets
}

// KnownFleet is the player's snapshot view of one fleet, frozen at AsOfYear.
// Subsequent ground-truth changes do not retroactively alter this snapshot.
type KnownFleet struct {
	ID         string
	Name       string
	Owner      Owner
	Units      map[WeaponType]int // SNAPSHOT at AsOfYear
	LocationID string             // "" if in transit (then look in PlayerView.InTransit)
	AsOfYear   float64
}

// KnownTransit is the player's view of a fleet currently in transit.
type KnownTransit struct {
	FleetID     string
	Name        string
	Owner       Owner
	Units       map[WeaponType]int // SNAPSHOT at departure-as-known
	SourceID    string
	DestID      string
	DepartYear  float64
	ArrivalYear float64
	AsOfYear    float64
}

// HomeGroundTruthSnapshot is the single explicit affordance by which the
// HTTP layer reads a player's own home-system state from Truth (the player
// sees their own home with no light-speed delay). Returned by
// GameState.ReadHomeGroundTruth. (Renamed from SolGroundTruthSnapshot in M2.)
type HomeGroundTruthSnapshot struct {
	Status     SystemStatus
	EconLevel  int
	Wealth     float64
	LocalUnits map[WeaponType]int // copy; safe for the caller to retain
	FleetIDs   []string           // copy
}

// System returns the KnownSystem with the given id, or nil if not found.
func (v *PlayerView) System(id string) *KnownSystem {
	if v == nil {
		return nil
	}
	return v.Systems[id]
}

// Fleet returns the KnownFleet with the given id, or nil if not found.
func (v *PlayerView) Fleet(id string) *KnownFleet {
	if v == nil {
		return nil
	}
	return v.Fleets[id]
}
