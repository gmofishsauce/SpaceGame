package game

// Truth is the omniscient world state. The engine is the sole writer; the
// alien bot reads it directly. HTTP handlers must not see it -- the
// GameState.truth field is unexported precisely to enforce that boundary.
//
// Static star geometry (positions, distances, display names) lives in
// StarCatalog, not here. Truth contains only state that can vary.
type Truth struct {
	Systems map[string]*TrueSystem // keyed by system ID
	Fleets  map[string]*TrueFleet  // keyed by fleet ID
}

// TrueSystem is the ground-truth state of one star system.
type TrueSystem struct {
	ID             string
	Status         SystemStatus
	EconLevel      int     // 0 for uninhabited/alien-held; grows over time
	Wealth         float64 // accumulated wealth; deducted by construction
	EconGrowthYear float64 // game year at which next level-up occurs
	LocalUnits     map[WeaponType]int // stationary units (OrbitalDefense, Interceptor, CommLaser)
	FleetIDs       []string           // IDs of fleets currently present
	PrimaryFleetID string             // ID of this system's "1st Fleet" (receives new construction)
	FleetCount     int                // number of fleets ever created here; drives ordinal naming
}

// TrueFleet is the ground-truth state of one fleet.
type TrueFleet struct {
	ID          string
	Name        string
	Owner       Owner
	Units       map[WeaponType]int
	LocationID  string  // system ID if stationed; "" if in transit
	SourceID    string  // origin system when in transit; "" otherwise
	DestID      string  // "" if not in transit
	DepartYear  float64
	ArrivalYear float64
	InTransit   bool
}

// System returns the TrueSystem with the given id, or nil if not found.
func (t *Truth) System(id string) *TrueSystem {
	if t == nil {
		return nil
	}
	return t.Systems[id]
}

// Fleet returns the TrueFleet with the given id, or nil if not found.
func (t *Truth) Fleet(id string) *TrueFleet {
	if t == nil {
		return nil
	}
	return t.Fleets[id]
}
