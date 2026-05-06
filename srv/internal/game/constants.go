// Package game contains all game-engine logic for SpaceGame.
// This file is the single location for all tunable game parameters (NFR-005).
package game

const (
	// Time (FR-012)
	TimeScaleYearsPerSecond = 10.0 / 180.0 // 10 in-game years per 3 real minutes
	TickIntervalMs          = 100           // real milliseconds per game tick
	TickIntervalSec         = 0.1           // = TickIntervalMs / 1000
	YearsPerTick            = TimeScaleYearsPerSecond * TickIntervalSec // ≈ 0.005556

	// Fleet and command speed (FR-031, FR-041)
	FleetSpeedC   = 0.8 // fraction of c; LY per in-game year
	CommandSpeedC = 1.0 // commands travel at c (Sol has a comm laser)

	// Economic level growth: rises 1 level per N in-game years without combat (FR-048)
	EconGrowthIntervalYears = 100.0

	// Initial economic level distribution (Section 4 Assumptions)
	EconLevelMean   = 2.5
	EconLevelStddev = 1.0

	// WinRetentionFraction is the shared per-side threshold for FR-55:
	// a player wins if it holds at least this fraction of the opponent's
	// initial systems. Single tunable; identical for both sides. (FR-55,
	// FR-58, NFR-15)
	WinRetentionFraction = 0.60

	// DrawYearCap is the in-game year at which an unfinished game ends
	// in a draw. Subject to playtesting. (FR-57, FR-58, NFR-15)
	DrawYearCap = 500.0

	// AlienHomeDisplayName is the common name of the alien home star in the
	// alien CSV files. Used to identify and seed the alien home system. (AS-2)
	AlienHomeDisplayName = "61 Ursae Majoris"

	// Clock sync broadcast cadence: every N engine ticks (≈ every 10 real seconds)
	ClockSyncCadence = 100

	// Combat
	MaxCombatRounds = 20 // safety cap on rounds per engagement

	// Economic combat penalty: destroy 0–WealthPenaltyMaxFraction of wealth on any combat
	WealthPenaltyMaxFraction = 0.5

	// Minimum economic level required to build each type is defined in WeaponDefs below.
)

// EconWealthRate[level] = wealth units accumulated per in-game year = 2^level. (FR-046)
// Indices 0–5.
var EconWealthRate = [6]float64{1.0, 2.0, 4.0, 8.0, 16.0, 32.0}

// WeaponDefs defines all weapon/device type parameters. (FR-040, FR-050)
// AttackPower and Vulnerability use: 0=none, 1=low, 3=medium, 10=high.
// Hit probability = AttackPower / (AttackPower + Vulnerability); units with
// AttackPower==0 always return 0.0 (no clamp applied to zero-attack types).
var WeaponDefs = map[WeaponType]WeaponDef{
	WeaponOrbitalDefense: {Cost: 1, MinLevel: 1, AttackPower: 1, Vulnerability: 10, CanMove: false, Reports: false, CommLaser: false},
	WeaponInterceptor:    {Cost: 2, MinLevel: 1, AttackPower: 3, Vulnerability: 3, CanMove: false, Reports: false, CommLaser: false},
	WeaponReporter:       {Cost: 4, MinLevel: 1, AttackPower: 0, Vulnerability: 3, CanMove: true, Reports: true, CommLaser: false},
	WeaponEscort:         {Cost: 8, MinLevel: 2, AttackPower: 3, Vulnerability: 3, CanMove: true, Reports: false, CommLaser: false},
	WeaponBattleship:     {Cost: 32, MinLevel: 3, AttackPower: 10, Vulnerability: 1, CanMove: true, Reports: false, CommLaser: false},
	WeaponCommLaser:      {Cost: 64, MinLevel: 4, AttackPower: 0, Vulnerability: 10, CanMove: true, Reports: false, CommLaser: true},
}

