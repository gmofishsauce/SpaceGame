package game

// SystemStatus is the player-visible status of a star system.
type SystemStatus string

const (
	StatusHuman      SystemStatus = "human"
	StatusAlien      SystemStatus = "alien"
	StatusContested  SystemStatus = "contested"   // combat reported but no clear victor yet
	StatusUninhabited SystemStatus = "uninhabited"
	StatusUnknown    SystemStatus = "unknown"     // no information has reached Earth
)

// Owner identifies which faction controls a unit or system.
type Owner string

const (
	HumanOwner Owner = "human"
	AlienOwner Owner = "alien"
)

// WeaponType is a canonical weapon/device type identifier.
type WeaponType string

const (
	WeaponOrbitalDefense WeaponType = "orbital_defense"
	WeaponInterceptor    WeaponType = "interceptor"
	WeaponReporter       WeaponType = "reporter"
	WeaponEscort         WeaponType = "escort"
	WeaponBattleship     WeaponType = "battleship"
	WeaponCommLaser      WeaponType = "comm_laser"
)

// WeaponTypeOrder is the canonical display/cost order (ascending cost). (FR-040)
var WeaponTypeOrder = []WeaponType{
	WeaponOrbitalDefense, WeaponInterceptor, WeaponReporter,
	WeaponEscort, WeaponBattleship, WeaponCommLaser,
}

// WeaponDef holds the static properties of one weapon/device type.
type WeaponDef struct {
	Cost          float64 // wealth units required to construct
	MinLevel      int     // minimum system economic level to construct
	AttackPower   int     // 0=none, 1=low, 3=medium, 10=high
	Vulnerability int     // 1=low, 3=medium, 10=high
	CanMove       bool    // interstellar capable
	Reports       bool    // Reporter: flees at combat start and carries result at 0.8c
	CommLaser     bool    // Comm laser: all events in system reported to Sol at c
}

// EventType identifies the kind of game event.
type EventType string

const (
	EventFleetArrival    EventType = "fleet_arrival"
	EventCombatOccurred  EventType = "combat_occurred"  // reporter or comm laser present: full details
	EventCombatSilent    EventType = "combat_silent"    // no reporter: internal only, never broadcast
	EventSystemCaptured  EventType = "system_captured"
	EventSystemRetaken   EventType = "system_retaken"
	EventConstructionDone EventType = "construction_done"
	EventCommandArrived  EventType = "command_arrived"  // command reached target system (FR-015)
	EventCommandExecuted EventType = "command_executed" // command successfully executed
	EventCommandFailed   EventType = "command_failed"   // command impossible or ignored
	EventReporterReturn  EventType = "reporter_return"  // reporter arrived at Earth
	EventEconGrowth      EventType = "econ_growth"      // system econ level increased
	EventAlienSpawn      EventType = "alien_spawn"      // internal only
	EventAlienExhausted  EventType = "alien_exhausted"
	EventGameOver        EventType = "game_over"
)

// CommandType is the kind of player or bot command.
type CommandType string

const (
	CmdConstruct CommandType = "construct"
	CmdMove      CommandType = "move"
	CmdPause     CommandType = "pause"
)

// CombatDetails is the type-specific payload for EventCombatOccurred.
type CombatDetails struct {
	HumanLosses map[WeaponType]int `json:"humanLosses"`
	AlienLosses map[WeaponType]int `json:"alienLosses"`
	HumanWon    bool               `json:"humanWon"`
	AlienWon    bool               `json:"alienWon"`
	Draw        bool               `json:"draw"`
}

// ConstructionDetails is the payload for EventConstructionDone.
type ConstructionDetails struct {
	WeaponType WeaponType `json:"weaponType"`
	Quantity   int        `json:"quantity"`
}

// CommandFailedDetails is the payload for EventCommandFailed.
type CommandFailedDetails struct {
	CommandType CommandType `json:"commandType"`
	Reason      string      `json:"reason"`
}

// EconGrowthDetails is the payload for EventEconGrowth.
type EconGrowthDetails struct {
	NewLevel int `json:"newLevel"`
}

// FleetArrivalDetails is the payload for EventFleetArrival.
type FleetArrivalDetails struct {
	FleetID   string             `json:"fleetId"`
	FleetName string             `json:"fleetName"`
	Owner     Owner              `json:"owner"`
	Units     map[WeaponType]int `json:"units"`
}
