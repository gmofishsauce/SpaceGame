# Design: SpaceGame
Version: 0.1

---

## 1. Overview

SpaceGame is a single-player, limited-information interstellar strategy game served as a single-page web application from a Go HTTP server running on localhost:8080. The player commands humanity's defenses against an alien invasion across the ~108 nearest real star systems, rendered in 3D using Three.js. The defining mechanic is that all information propagates at the speed of light: the player always commands based on stale intelligence, and combat results are only knowable if the player has pre-positioned reporter vessels. A built-in bot drives the alien side through a defined interface that permits future replacement. A single session runs 2–4 real hours and covers hundreds of in-game years.

---

## 2. Requirements Summary

Requirements are numbered as in `requirements.md`. This section is the developer's authoritative reference; `requirements.md` need not be consulted separately.

### 2.1 Web Server

| ID | Requirement |
|----|-------------|
| **FR-001** | A Go HTTP server listens on port 8080 on localhost only. |
| **FR-002** | The server responds to `GET /` with a single HTML page; all CSS and JavaScript are bundled into that response (no separate file downloads after initial load). |
| **FR-003** | The server uses only the Go standard library; no third-party Go packages. |
| **FR-004** | The server is the sole source of truth for game state; the client is a display and input device only. |

### 2.2 Game Initialization

| ID | Requirement |
|----|-------------|
| **FR-005** | On startup, star and planet data are loaded from `nearest.csv` and `planets.csv`. |
| **FR-006** | Every star system that has confirmed planets in `planets.csv` is initialized as human-held. |
| **FR-007** | Every star system without planets whose distance from Sol is ≤ half the distance of the farthest star in `nearest.csv` is also initialized as human-held. |
| **FR-008** | Each human-held system is assigned an initial economic level (1–5) drawn from a Gaussian distribution; Earth (Sol) is always level 5. |
| **FR-009** | Alien entry points are randomized from systems at the periphery; re-randomized at every new game. |
| **FR-010** | No alien-held systems appear at game start; alien presence is revealed only through reported combat events. |

### 2.3 Game Time

| ID | Requirement |
|----|-------------|
| **FR-011** | The server maintains a game clock in pulsar-calibrated in-game years, starting at year 0. |
| **FR-012** | Game time advances at 10 in-game years per 3 real minutes (≈ 0.05556 in-game years per real second). This rate is a named, tunable constant. |
| **FR-013** | Pressing Escape pauses or unpauses the game; all simulation, time, and bot activity halt while paused. |
| **FR-014** | The current in-game year is always visible in the UI. |

### 2.4 Information and the Event Log

| ID | Requirement |
|----|-------------|
| **FR-015** | Every game event (combat, construction completion, fleet arrival, system capture, failed command, alien attack) is recorded in a server-side event log with its in-game year and origin system. |
| **FR-016** | For each event the server computes the earliest year it could reach Earth. Standard events propagate at c (1 LY/year): `arrivalYear = eventYear + dist(system, Sol)`. Combat events reported by a Reporter vessel propagate at 0.8c: `arrivalYear = eventYear + dist(system, Sol) / 0.8`. |
| **FR-017** | The player UI only shows events whose `arrivalYear ≤ currentGameYear`. |
| **FR-018** | The displayed status of a star system is derived from the most recent event about that system whose `arrivalYear ≤ currentGameYear`. |

### 2.5 Star Map Display

| ID | Requirement |
|----|-------------|
| **FR-019** | All star systems are rendered in 3D via Three.js at their astronomical Cartesian positions (light-years from Sol at origin), consistent with the existing prototype. |
| **FR-020** | Each star marker is color-coded by its most recently known status: human-held, alien-held, contested, unknown/stale, or uninhabited. |
| **FR-021** | Mouse-hovering a star shows a popup with: system name, most recently known status, the in-game year of that information, known economic level (if any), and known forces present (if any). |
| **FR-022** | The prototype's dotted axis-projection lines on hover are retained unchanged. |
| **FR-023** | Sol always shows current accurate information (no propagation delay applies to Earth). |
| **FR-024** | Camera orbit, zoom, and pan (OrbitControls) are retained from the prototype. |

### 2.6 Event Sidebar

| ID | Requirement |
|----|-------------|
| **FR-025** | A sidebar shows a scrolling event log sorted by `arrivalYear` ascending. |
| **FR-026** | Each sidebar entry shows: arrival year, system name, event description. |
| **FR-027** | Hovering a sidebar entry highlights the corresponding star on the 3D map. |
| **FR-028** | The sidebar auto-scrolls to the most recent event; the player can scroll back manually. |

### 2.7 Player Actions — General

| ID | Requirement |
|----|-------------|
| **FR-029** | All player actions begin by right-clicking a star system to open a context menu. |
| **FR-030** | The context menu only shows actions valid for the system's most recently known state. |
| **FR-031** | Commands to non-Sol systems are delayed by `dist(Sol, system) / 0.8` in-game years before taking effect. Commands to Sol take effect immediately. |
| **FR-032** | When presenting a construction or fleet-command dialog for a remote system, the UI displays projected state estimates at command-arrival time, not the current known state. |
| **FR-033** | If a command arrives at its target system and cannot execute (e.g., system captured, insufficient output), a failed-execution event is logged and propagates to Earth at c. |

### 2.8 Player Actions — Construction

| ID | Requirement |
|----|-------------|
| **FR-034** | Systems capable of construction show a "Construct…" option in the right-click menu. |
| **FR-035** | The construction dialog lists weapon types affordable given the system's projected economic output and level at command-arrival time. |
| **FR-036** | Selecting a weapon type dispatches a construction order; on arrival, the unit count increments and accumulated output is debited. |

### 2.9 Player Actions — Fleet Command

| ID | Requirement |
|----|-------------|
| **FR-037** | Systems with interstellar-capable fleets show a "Command…" option in the right-click menu. |
| **FR-038** | The fleet-command dialog lets the player select a fleet, then click a destination system on the 3D map. |
| **FR-039** | On confirmation, the movement order is dispatched after command-travel delay; the fleet then travels to its destination at 0.8c. |

### 2.10 Forces and Weapons

| ID | Requirement |
|----|-------------|
| **FR-040** | The system supports exactly five weapon types (ascending cost): Orbital Defense, Interceptor, Reporter, Escort, Battleship. See Section 7 for full stats. |
| **FR-041** | All interstellar-capable ships travel at 0.8c. This speed is a named, tunable constant. |
| **FR-042** | Interstellar-capable ships are grouped into named fleets; names are auto-assigned. |
| **FR-043** | Each system tracks counts of each weapon type and which named fleets are present. |
| **FR-044** | Minimum economic level required to build each weapon type is a named, tunable constant. |

### 2.11 Economic System

| ID | Requirement |
|----|-------------|
| **FR-045** | Each human-held system has an economic level 1–5. |
| **FR-046** | Accumulated economic output grows continuously at a rate proportional to economic level; the accumulation rate per level is a named, tunable constant. |
| **FR-047** | Construction deducts cost from accumulated output; orders are rejected if output is insufficient. |
| **FR-048** | Economic levels are fixed in the MVP; they do not change during play. |

### 2.12 Combat

| ID | Requirement |
|----|-------------|
| **FR-049** | Combat occurs automatically whenever alien and human forces occupy the same system at the same game tick. |
| **FR-050** | Combat is resolved stochastically by the game engine; the player has no real-time input. |
| **FR-051** | Combat occurs only within star systems, never during interstellar transit. |
| **FR-052** | Every combat event is recorded in the server's internal event log, whether or not a Reporter is present. |
| **FR-053** | Combat results propagate to Earth (appear in the player's sidebar) only if a Reporter vessel was present in the system at the time of combat. Without a Reporter, neither the occurrence nor outcome of combat reaches Earth directly. |
| **FR-054** | A system captured by alien forces becomes alien-held; it may be retaken to human-held. |

### 2.13 Victory and Defeat

| ID | Requirement |
|----|-------------|
| **FR-055** | Alien attack capacity decreases as a function of cumulative alien losses, representing exhaustion of the alien empire. |
| **FR-056** | Human wins if: alien exhaustion is reached AND Earth is human-held AND the fraction of originally human-held systems still human-held ≥ `HUMAN_WIN_RETENTION`. |
| **FR-057** | Alien wins if: it captures Earth, OR the fraction of all systems it holds ≥ `ALIEN_WIN_THRESHOLD`. |
| **FR-058** | When a victory/defeat condition is detected, the game pauses and displays a game-over screen. |

### 2.14 Save and Load *(Deferred from MVP)*

| ID | Requirement |
|----|-------------|
| **FR-059** | The player can save current game state at any time. |
| **FR-060** | A saved game can be loaded to restore all game state including event log, forces, systems, and clock. |
| **FR-061** | At least one save slot is supported. |

### 2.15 Bot API

| ID | Requirement |
|----|-------------|
| **FR-062** | The alien bot interacts with the game engine exclusively through a defined programmatic interface. No bot logic is embedded in the game engine. |
| **FR-063** | The bot interface exposes: querying ground-truth game state, issuing fleet movement orders for alien forces, and receiving game event notifications. |
| **FR-064** | The bot interface is designed so that substituting an alternative bot requires replacing only a single module, with no changes to the game engine. |

### 2.16 Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| **NFR-001** | The SPA runs in current Chrome, Firefox, and Safari with no plugins. |
| **NFR-002** | The Go server starts and is ready within 2 seconds of launch. |
| **NFR-003** | Game simulation runs without perceptible stuttering on a modern developer laptop. |
| **NFR-004** | No internet connection required at runtime; all assets (including Three.js) are served locally. |
| **NFR-005** | All tunable parameters (time scale, ship speed, economic rates, weapon costs, victory thresholds) are named constants in a single file. |
| **NFR-006** | Single-user local use; no authentication, HTTPS, or multi-user support. |

---

## 3. Requirements Issues

### Ambiguities

**A-1 — FR-007: "half the distance of the farthest star"**
This could mean (a) the median distance, or (b) `max_distance / 2`. This design assumes interpretation (b): systems with `dist ≤ maxDist / 2` (where `maxDist` is the largest distance in `nearest.csv`, approximately 22.7 LY) are initially human-held if they lack planets. Threshold ≈ 11.35 LY.

**A-2 — FR-009: "periphery of human space"**
Undefined numerically. This design assumes: systems with `dist > PERIPHERY_FRACTION * maxDist` (see `constants.go`; default `PERIPHERY_FRACTION = 0.75`, i.e., dist > ~17 LY). At least two such systems are selected randomly as alien entry points per game.

**A-3 — FR-016: Reporter vs. radio propagation**
FR-016 distinguishes "events from a reporter vessel" (travel time = dist / 0.8c) from standard events. This design interprets it as: standard events (fleet arrivals, construction, system captures that *are observed*) propagate at c. Combat events (which require a physical reporter vessel to certify) propagate at 0.8c (reporter's travel speed). Reporter reports arrive *slightly later* than a radio message would, but carry certified combat details.

**A-4 — FR-032: "projected estimates"**
The requirements call for projected state at command-arrival time. For accumulated economic output, projection = `current_accum + econRate × commandTravelTime`. For system forces, projection is identical to current known state (no way to predict other orders in flight). The dialog will display both the projected output and a note that force projections may be outdated.

**A-5 — FR-038: How to select destination**
The requirements say "select a fleet and a destination star system" but do not specify the interaction. This design implements: after the player selects a fleet in the Command dialog, the dialog closes and the map enters "destination-selection mode" (cursor changes to crosshair, Escape cancels). The next left-click on any star opens a confirmation dialog showing transit time. Escape or clicking a non-star area cancels.

**A-6 — FR-053: "other signals"**
FR-053 states that without a Reporter, the occurrence of combat "may eventually be inferred from other signals." This design defers that inference mechanic entirely (MVP does not implement it). A system with no Reporter present during combat generates no event that reaches Earth; the system's known status simply stops updating.

**A-7 — VisionStatement weapon order vs. FR-040 weapon order**
The VisionStatement lists weapons as: Orbital Defense → Reporters → Interceptors → Escorts → Battleships. FR-040 lists: Orbital Defense, Interceptor, Reporter, Escort, Battleship. These differ in the placement of Reporter vs. Interceptor. **This design follows FR-040** (the requirements document is more authoritative): Orbital Defense < Interceptor < Reporter < Escort < Battleship by cost. The VisionStatement ordering appears to be narrative, not a cost hierarchy.

### Contradictions

**C-1 — FR-048 vs. VisionStatement on economic levels changing**
FR-048 says economic levels "do not change during normal play unless altered by game events." The VisionStatement says they "grow over time." For MVP, this design implements FR-048 (levels fixed), consistent with the MVP scope section which does not include economic growth. Economic level change in response to events (e.g., alien capture resets a system's economy) *is* required and is handled by capturing/retaking system logic.

### Gaps

**G-1 — Alien forces composition**
No requirement specifies the alien side's weapon types, costs, or how many forces spawn at entry points. This design invents a minimal spec (see Section 7.2) as a necessary assumption; exact values are tunable constants.

**G-2 — Bot knowledge scope**
FR-063 says the bot queries "current (ground-truth) game state." This design assumes the bot has unrestricted read access to the full `GameState` struct (no information delay), consistent with the VisionStatement's "the alien bot has access to ground-truth game state."

**G-3 — Fleet order queueing (OQ-004)**
What happens when the player issues a new command to a system that already has orders en-route? This design implements: each command is independent. Multiple orders can be en-route simultaneously to the same system. They execute in arrival order. If an earlier order invalidates a later one (e.g., a move order for a fleet that was meanwhile destroyed), a failed-execution event is generated.

**G-4 — Initial state visibility**
FR-010 says "no alien-held systems appear at game start." But systems that are human-held at start — is their initial status immediately known to the player? This design assumes yes: the player begins with complete knowledge of the initial configuration (human-held systems, their economic levels, and uninhabited systems) since this is baseline common knowledge. Alien presence is only revealed after combat events arrive.

### Untestable Requirements

**U-1 — NFR-003 "without perceptible stuttering"**
Redefined as: game clock display updates at ≥ 10 Hz in real time; Three.js renders at ≥ 30 FPS during interaction (OrbitControls in motion); event sidebar appends new entries within 500 ms of their computed `arrivalYear`.

---

## 4. Constraints and Assumptions

### Constraints

- **Go standard library only** (FR-003): No third-party Go packages. Uses `net/http`, `encoding/json`, `encoding/csv`, `math`, `math/rand`, `sync`, `embed`, `io/fs`, `time`.
- **Three.js + OrbitControls** (IR-001): These are bundled by Vite from the `three` npm package. No other 3D framework.
- **Server port 8080, localhost only** (FR-001): `net.Listen("tcp", "127.0.0.1:8080")`.
- **Single-player, no auth, no HTTPS** (NFR-006).
- **No database** (Section 5.4): All state in-memory.
- **Go 1.24.2** (as declared in `go.mod`): `//go:embed`, `http.FileServerFS`, `fs.Sub` are all available.
- **Existing prototype retained**: `proto/` is unchanged; the full game lives in `cmd/`, `internal/`, and `web/`.

### Assumptions

- **Build order**: The developer must run `go run ./tools/gendata` (from the project root) first, then `cd web && npm run build`, then `go build ./cmd/spacegame`. A `Makefile` at the repo root documents and enforces this order. The gendata step generates both `proto/src/stardata.js` and `internal/game/stardata.json`; skipping it will leave stale or missing data files.
- **Star system IDs**: Derived from the catalog name (lowercase, spaces replaced by `-`), e.g., `"gj-551"` for GJ 551, `"sol"` for Sol. These IDs are stable across game sessions.
- **Co-located stars**: Two or more stars with identical RA, Dec, and distance strings in `nearest.csv` are grouped into a single game entity with the combined names (consistent with `proto/design.md`). The game treats the group as one system.
- **Planet ring**: The existing CSS2D planet ring (a white bordered circle) is retained for human-held systems with confirmed planets; ring color changes to match system status color.
- **Economic level Gaussian**: mean = 2.5, σ = 1.0, clamped to [1, 5], round to nearest integer. Earth is always 5.
- **Alien side construction**: Aliens do not build weapons in-game. Alien forces "spawn" at their entry points at configured intervals, representing forces dispatched from the off-map alien empire.
- **Alien information model**: The bot has no information delay. It reads ground-truth `GameState` directly. The player sees the alien side only through events that have propagated to Earth.
- **Reporter behavior on combat**: When combat begins in a system containing one or more Reporter vessels, the Reporters immediately depart toward Sol. They do not participate in combat. A Reporter fleet is created in transit (Sol-bound) with `arrivalYear = combatYear + dist(system, Sol) / FLEET_SPEED_C`. The reporter fleet is consumed (removed) on arrival at Sol; its `arrivalYear` becomes the `arrivalYear` of the combat-result event.
- **Uninhabited systems**: Systems that are neither human-held at start nor alien entry points begin as uninhabited (no forces, no economic level). They can be colonized or captured in future game versions; for MVP their status is purely visual.
- **gendata tool**: `tools/gendata/` remains in the repo unchanged for use with the `proto/` prototype. The full game server does not use it; the server reads CSVs directly at startup.

---

## 5. Architecture

### 5.1 Major Components

| Component | Where | What It Does |
|-----------|-------|--------------|
| **Go HTTP Server** | `internal/server/` | Accepts HTTP connections, routes requests, serves embedded SPA, manages SSE client registry |
| **Game Engine** | `internal/game/engine.go` | Runs the authoritative game loop; processes commands; dispatches events |
| **Game State** | `internal/game/state.go` | In-memory store of all star systems, fleets, event log, clock, faction states |
| **Combat Resolver** | `internal/game/combat.go` | Stochastic combat resolution; produces combat events |
| **Economic System** | `internal/game/economy.go` | Output accumulation; construction order validation and execution |
| **Event Manager** | `internal/game/events.go` | Event creation, event log, SSE broadcasting to connected clients |
| **CSV Loader** | `internal/game/loader.go` | Parses `nearest.csv` and `planets.csv`; builds initial `GameState` |
| **Bot Agent** | `internal/game/bot.go` | Alien AI; implements `BotAgent` interface; runs as a goroutine |
| **SPA Frontend** | `web/src/` | Three.js star map, event sidebar, context menus, dialogs, SSE client |
| **Embedded Assets** | `web/embed.go` | `//go:embed dist`; makes the built SPA available to the server at compile time |

### 5.2 Component Interaction Diagram

```
┌─────────────────────────────────────────────────────────────┐
│  Browser (SPA)                                              │
│  ┌────────────┐  ┌──────────┐  ┌───────┐  ┌────────────┐  │
│  │  starmap   │  │ sidebar  │  │  ui   │  │  api / SSE │  │
│  │  (Three.js)│  │          │  │       │  │  client    │  │
│  └─────┬──────┘  └────┬─────┘  └───┬───┘  └─────┬──────┘  │
│        └──────────────┴────────────┴─────────────┘         │
│                         state.js                            │
└────────────────────────────┬────────────────────────────────┘
                             │  HTTP / SSE  (localhost:8080)
                             │  GET /api/stars  → JSON
                             │  GET /api/state  → JSON
                             │  GET /api/events → SSE stream
                             │  POST /api/command → JSON
                             │  POST /api/pause   → JSON
┌────────────────────────────┴────────────────────────────────┐
│  Go Server (localhost:8080)                                 │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  HTTP Server (internal/server/)                       │  │
│  │   handlers.go  ─── routes → game engine calls        │  │
│  │   SSE registry ─── fan-out events to clients         │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                           │                                  │
│  ┌────────────────────────┴─────────────────────────────┐  │
│  │  Game Engine (internal/game/)                         │  │
│  │  ┌──────────┐ ┌────────┐ ┌──────────┐ ┌──────────┐  │  │
│  │  │ engine   │ │ combat │ │ economy  │ │ events   │  │  │
│  │  │ (ticker) │ │        │ │          │ │          │  │  │
│  │  └────┬─────┘ └────────┘ └──────────┘ └─────┬────┘  │  │
│  │       │                                       │        │  │
│  │  ┌────┴─────────────────────────────────┐    │        │  │
│  │  │  GameState (in-memory)               │────┘        │  │
│  │  └──────────────────────────────────────┘             │  │
│  │  ┌───────────┐                                         │  │
│  │  │ BotAgent  │ (goroutine)                             │  │
│  │  └───────────┘                                         │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
         ↑
  nearest.csv, planets.csv  (read once at startup)
  web/dist/ (embedded via //go:embed)
```

### 5.3 Data Flow

**Startup**:
1. Server loads `nearest.csv` and `planets.csv` → `loader.Initialize()` → `GameState`
2. Server embeds `web/dist/` via `//go:embed`; serves at `/`
3. Game engine goroutine starts (`engine.Run()`)
4. Bot goroutine starts (`bot.Run()`)

**Game loop (every 100 ms real time)**:
1. `engine.Tick()` advances `GameState.Clock` by `YearsPerTick`
2. Engine processes all pending commands whose `executeYear ≤ Clock`
3. Engine checks for combat in any system (alien + human forces present)
4. `economy.AccumulateOutput()` increments each system's `AccumOutput`
5. Engine checks victory/defeat conditions
6. `events.BroadcastMatured()` pushes newly-matured events (those whose `arrivalYear ≤ Clock`) to all SSE clients

**Player command**:
1. Browser → `POST /api/command` with JSON body
2. Handler validates command against current known state (quick check)
3. Command is enqueued with `executeYear = Clock + dist / COMMAND_SPEED_C`
4. Handler responds 200 with `commandId` and `estimatedArrivalYear`
5. At `executeYear`, engine's Tick() processes the command against ground-truth state
6. If execution fails, a `command_failed` event is logged and will propagate to Earth

**Bot command**:
1. `bot.Tick(state, clock)` is called by the engine on every Nth tick (configurable cadence)
2. Bot returns a slice of `BotCommand`; engine applies them immediately (no travel delay)
3. Bot-generated events are logged with appropriate `arrivalYear` like any other event

**SSE event delivery**:
1. `events.BroadcastMatured()` finds events where `arrivalYear ≤ Clock` and `!alreadyBroadcast`
2. Each event is marshaled to JSON and written to every registered `http.ResponseWriter` in the SSE registry
3. Client's `EventSource` receives and dispatches events to JavaScript handlers

---

## 6. Detailed Design

### 6.1 `internal/game/constants.go`

**Purpose**: Single location for all tunable game parameters (NFR-005).

**Satisfies**: NFR-005, FR-012, FR-041, FR-044, FR-056, FR-057

**Interface** (exported constants and vars):

```go
package game

const (
    // Time
    TimeScaleYearsPerSecond = 10.0 / 180.0   // 10 in-game years per 3 real minutes
    TickIntervalMs          = 100             // real milliseconds per game tick
    TickIntervalSec         = 0.1             // = TickIntervalMs / 1000
    YearsPerTick            = TimeScaleYearsPerSecond * TickIntervalSec // ≈ 0.005556

    // Fleet and command speed
    FleetSpeedC   = 0.8  // fraction of c; LY per in-game year
    CommandSpeedC = 0.8  // same as fleet speed (FR-031)

    // Economic output (output units per in-game year, indexed by level 0..5; index 0 unused)
    // Level 1 = 1.0/yr, Level 2 = 2.0/yr, ..., Level 5 = 16.0/yr (geometric)

    // Initial economic level distribution
    EconLevelMean   = 2.5
    EconLevelStddev = 1.0

    // Periphery definition for alien entry points
    PeripheryFraction = 0.75   // systems with dist > PeripheryFraction * maxDist
    AlienEntryCount   = 2      // number of alien entry points per game

    // Alien spawn behavior
    AlienSpawnIntervalYears = 20.0   // in-game years between alien force arrivals
    AlienInitialUnits       = 15     // units placed at each entry point at game start
    AlienSpawnUnitsPerWave  = 10     // units added per spawn event

    // Alien exhaustion
    AlienExhaustionThreshold = 400   // cumulative alien units destroyed → exhaustion

    // Victory/defeat
    HumanWinRetentionFraction = 0.60  // fraction of initial human systems to retain (FR-056)
    AlienWinCaptureFraction   = 0.40  // fraction of all systems for alien win (FR-057)

    // Bot tick cadence (call bot every N engine ticks)
    BotTickCadence = 10
)

// EconOutputRate[level] = economic output units per in-game year. Index 0 unused.
var EconOutputRate = [6]float64{0, 1.0, 2.0, 4.0, 8.0, 16.0}

// WeaponDefs defines all weapon type parameters.
var WeaponDefs = map[WeaponType]WeaponDef{
    WeaponOrbitalDefense: {Cost: 10,  MinLevel: 1, Attack: 2,  Defense: 4,  CanMove: false, Reports: false},
    WeaponInterceptor:    {Cost: 25,  MinLevel: 2, Attack: 4,  Defense: 3,  CanMove: false, Reports: false},
    WeaponReporter:       {Cost: 20,  MinLevel: 2, Attack: 0,  Defense: 1,  CanMove: true,  Reports: true},
    WeaponEscort:         {Cost: 75,  MinLevel: 3, Attack: 6,  Defense: 4,  CanMove: true,  Reports: false},
    WeaponBattleship:     {Cost: 200, MinLevel: 4, Attack: 12, Defense: 6,  CanMove: true,  Reports: false},
}
```

**Behavior**: This file contains only declarations. No logic.

**Error handling**: N/A.

**Dependencies**: None.

---

### 6.2 `internal/game/types.go`

**Purpose**: Shared type definitions used across all game packages.

**Satisfies**: FR-040, FR-043, FR-045 (data representation)

**Interface**:

```go
package game

type SystemStatus string
const (
    StatusHuman      SystemStatus = "human"
    StatusAlien      SystemStatus = "alien"
    StatusContested  SystemStatus = "contested"  // combat reported but no clear victor yet
    StatusUninhabited SystemStatus = "uninhabited"
    StatusUnknown    SystemStatus = "unknown"     // no information has reached Earth
)

type Owner string
const (
    HumanOwner Owner = "human"
    AlienOwner  Owner = "alien"
)

type WeaponType string
const (
    WeaponOrbitalDefense WeaponType = "orbital_defense"
    WeaponInterceptor    WeaponType = "interceptor"
    WeaponReporter       WeaponType = "reporter"
    WeaponEscort         WeaponType = "escort"
    WeaponBattleship     WeaponType = "battleship"
)

// WeaponTypeOrder is the canonical display order (ascending cost).
var WeaponTypeOrder = []WeaponType{
    WeaponOrbitalDefense, WeaponInterceptor, WeaponReporter, WeaponEscort, WeaponBattleship,
}

type WeaponDef struct {
    Cost     float64  // economic output units
    MinLevel int      // minimum system economic level to construct
    Attack   int      // combat attack rating
    Defense  int      // combat defense rating
    CanMove  bool     // interstellar capable
    Reports  bool     // triggers combat reporting to Earth when present
}

type EventType string
const (
    EventFleetArrival    EventType = "fleet_arrival"
    EventCombatOccurred  EventType = "combat_occurred"   // reporter present: full details
    EventCombatSilent    EventType = "combat_silent"     // no reporter: internal only, never broadcast
    EventSystemCaptured  EventType = "system_captured"
    EventSystemRetaken   EventType = "system_retaken"
    EventConstructionDone EventType = "construction_done"
    EventCommandFailed   EventType = "command_failed"
    EventReporterReturn  EventType = "reporter_return"   // reporter arrived at Earth
    EventAlienSpawn      EventType = "alien_spawn"       // internal only
    EventAlienExhausted  EventType = "alien_exhausted"
    EventGameOver        EventType = "game_over"
)

type CommandType string
const (
    CmdConstruct CommandType = "construct"
    CmdMove      CommandType = "move"
    CmdPause     CommandType = "pause"
)
```

---

### 6.3 `internal/game/state.go`

**Purpose**: In-memory authoritative game state and its accessor methods.

**Satisfies**: FR-004, FR-011, FR-015, FR-043, FR-045, FR-046

**Interface**:

```go
package game

import "sync"

type GameState struct {
    mu sync.RWMutex  // protects all fields below

    Clock       float64
    Paused      bool
    GameOver    bool
    Winner      Owner   // "" if not over
    WinReason   string

    Systems     map[string]*StarSystem  // key: system ID (e.g., "gj-551")
    SystemOrder []string                // insertion order (Sol first, then by distance)
    Fleets      map[string]*Fleet       // key: fleet ID
    Events      []*GameEvent            // all events, chronological by eventYear
    PendingCmds []*PendingCommand       // commands not yet arrived at target

    Human       HumanFaction
    Alien       AlienFaction

    nextFleetNum  int
    nextEventID   int
    nextCmdID     int
}

type StarSystem struct {
    ID              string
    DisplayName     string
    X, Y, Z         float64
    DistFromSol     float64
    HasPlanets      bool

    // Ground truth (authoritative, only engine writes these)
    Status          SystemStatus
    EconLevel       int          // 0 for uninhabited/alien-held
    AccumOutput     float64
    LocalUnits      map[WeaponType]int  // stationary units (OrbitalDefense, Interceptor)
    FleetIDs        []string            // IDs of fleets currently present

    // Last known state (derived from events with arrivalYear ≤ currentClock)
    KnownStatus     SystemStatus
    KnownAsOfYear   float64
    KnownEconLevel  int
    KnownLocalUnits map[WeaponType]int
    KnownFleetIDs   []string
}

type Fleet struct {
    ID          string
    Name        string
    Owner       Owner
    Units       map[WeaponType]int
    LocationID  string    // system ID if stationed; "" if in transit
    DestID      string    // "" if not in transit
    DepartYear  float64
    ArrivalYear float64
    InTransit   bool
}

type GameEvent struct {
    ID          string
    EventYear   float64
    ArrivalYear float64      // math.MaxFloat64 if event never reaches Earth
    SystemID    string
    Type        EventType
    Description string
    Broadcast   bool         // true once pushed to SSE clients
    Details     interface{}  // type-specific payload (see Section 7)
}

type PendingCommand struct {
    ID          string
    ExecuteYear float64
    OriginID    string   // always "sol" for player commands
    TargetID    string
    Type        CommandType
    WeaponType  WeaponType  // for CmdConstruct
    Quantity    int          // for CmdConstruct
    FleetID     string       // for CmdMove
    DestID      string       // for CmdMove
    IsBot       bool
}

type HumanFaction struct {
    InitialSystemIDs []string  // systems held at game start (for win condition)
}

type AlienFaction struct {
    TotalLost        int
    Exhausted        bool
    EntryPointIDs    []string
    NextSpawnYear    float64
}
```

**Key methods on `*GameState`** (all acquire `mu` appropriately):

```go
func (s *GameState) Tick(deltaYears float64)
func (s *GameState) ApplyCommand(cmd *PendingCommand) error
func (s *GameState) RecordEvent(evt *GameEvent)
func (s *GameState) UpdateKnownStates(clock float64)  // applies matured events to KnownState fields
func (s *GameState) CheckVictory() (over bool, winner Owner, reason string)
func (s *GameState) NewFleetID() string   // returns "fleet-1", "fleet-2", etc.
func (s *GameState) NewFleetName() string // returns "Fleet Alpha", "Fleet Bravo", etc.
func (s *GameState) NewEventID() string
func (s *GameState) NewCommandID() string
```

**Error handling**: Methods that mutate state return `error` for validation failures (e.g., insufficient output). Engine treats these as `command_failed` events, not panics.

**Dependencies**: `sync`, `math`, `fmt`

---

### 6.4 `internal/game/loader.go`

**Purpose**: Parse `nearest.csv` and `planets.csv`; build the initial `GameState`.

**Satisfies**: FR-005, FR-006, FR-007, FR-008, FR-009, FR-010

**Interface**:

```go
package game

func Initialize(nearestCSVPath, planetsCSVPath string) (*GameState, error)
```

**Behavior** (pseudocode):

```
Initialize(nearestPath, planetsPath):
    hasPlanets = loadPlanets(planetsPath)     // returns set of normalized star names with planets
    rawStars   = loadStars(nearestPath)        // parses RA/Dec/dist → StarGroup list (reuse tools/gendata logic)
    maxDist    = max(star.DistFromSol for star in rawStars)

    state = &GameState{Systems: {}, Fleets: {}, ...}

    for each rawStar:
        id = toID(rawStar.DisplayName)
        sys = &StarSystem{
            ID: id, DisplayName: rawStar.DisplayName,
            X: rawStar.X, Y: rawStar.Y, Z: rawStar.Z,
            DistFromSol: rawStar.DistFromSol,
            HasPlanets: hasPlanets.contains(rawStar names),
        }

        // Determine initial status (FR-006, FR-007)
        if sys.isSol:
            sys.Status = StatusHuman
            sys.EconLevel = 5
        else if sys.HasPlanets:
            sys.Status = StatusHuman
        else if sys.DistFromSol <= maxDist / 2.0:
            sys.Status = StatusHuman
        else:
            sys.Status = StatusUninhabited

        // Assign economic level (FR-008)
        if sys.Status == StatusHuman && !sys.isSol:
            sys.EconLevel = clamp(round(gaussianSample(EconLevelMean, EconLevelStddev)), 1, 5)

        // Initialize known state = initial state (G-4 assumption)
        sys.KnownStatus   = sys.Status
        sys.KnownEconLevel = sys.EconLevel
        sys.KnownAsOfYear  = 0.0

        state.Systems[id] = sys

    // Record initial human systems (FR-056 win condition)
    state.Human.InitialSystemIDs = [id for sys in state.Systems if sys.Status == StatusHuman]

    // Select alien entry points (FR-009)
    peripheral = [sys for sys in state.Systems if sys.DistFromSol > PeripheryFraction * maxDist]
    shuffle(peripheral)
    state.Alien.EntryPointIDs = peripheral[:AlienEntryCount].IDs

    // Place initial alien forces at entry points (G-1 assumption)
    for each entryID in state.Alien.EntryPointIDs:
        fleet = &Fleet{
            ID: state.NewFleetID(),
            Name: state.NewFleetName(),
            Owner: AlienOwner,
            Units: {WeaponEscort: 5, WeaponBattleship: 5, WeaponReporter: 0, ...},
            LocationID: entryID,
            InTransit: false,
        }
        state.Fleets[fleet.ID] = fleet
        state.Systems[entryID].FleetIDs = append(..., fleet.ID)
        state.Systems[entryID].Status = StatusAlien  // entry point starts alien-held (FR-010: not visible at start)

    state.Alien.NextSpawnYear = AlienSpawnIntervalYears

    return state, nil
```

**CSV parsing**: Reuse the same RA/Dec parsing and coordinate conversion logic from `tools/gendata/main.go` (copied, not imported, to keep the tools package separate). Extract this into a shared internal helper or duplicate it in loader.go.

**Gaussian sampling**: Box-Muller transform using `math/rand` (seeded with `time.Now().UnixNano()`).

**Error handling**:
- If either CSV cannot be opened: return `nil, fmt.Errorf("loading %s: %w", path, err)`.
- If a CSV row has too few columns: skip row, log to stderr.
- If `nearest.csv` yields zero stars: return `nil, errors.New("nearest.csv: no usable star records")`.
- If fewer than `AlienEntryCount` peripheral systems exist: use as many as are available, log a warning.

**Dependencies**: `encoding/csv`, `math`, `math/rand`, `time`, `os`, `bufio`, `strconv`

---

### 6.5 `internal/game/engine.go`

**Purpose**: Game loop; processes ticks, pending commands, spawns; coordinates all subsystems.

**Satisfies**: FR-011, FR-012, FR-013, FR-049, FR-051, FR-055, FR-058, FR-062

**Interface**:

```go
package game

type Engine struct {
    State    *GameState
    Bot      BotAgent
    Events   *EventManager
}

func NewEngine(state *GameState, bot BotAgent, events *EventManager) *Engine

// Run blocks; call in a goroutine. Stops when ctx is cancelled.
func (e *Engine) Run(ctx context.Context)

// Pause sets paused state; safe to call from HTTP handlers.
func (e *Engine) SetPaused(paused bool)

// EnqueueCommand validates and enqueues a player command.
// Returns commandID and estimated arrival year, or error.
func (e *Engine) EnqueueCommand(cmd *PendingCommand) (string, float64, error)
```

**Behavior** (`Run` pseudocode):

```
Run(ctx):
    ticker = time.NewTicker(TickIntervalMs * time.Millisecond)
    tickCount = 0
    defer ticker.Stop()

    loop:
        select:
            case <-ctx.Done(): return
            case <-ticker.C:
                state.mu.Lock()
                if !state.Paused && !state.GameOver:
                    e.tick(state)
                state.mu.Unlock()

tick(state):
    state.Clock += YearsPerTick

    // Process matured pending commands
    for each cmd in state.PendingCmds where cmd.ExecuteYear <= state.Clock:
        err = state.ApplyCommand(cmd)
        if err != nil:
            logCommandFailed(state, cmd, err)
        remove cmd from state.PendingCmds

    // Accumulate economic output (FR-046)
    economy.AccumulateOutput(state, YearsPerTick)

    // Check for combat (FR-049)
    for each system in state.Systems:
        if humanForcesPresent(system) && alienForcesPresent(system):
            combat.Resolve(state, system)

    // Alien spawning (FR-055, G-1)
    if !state.Alien.Exhausted && state.Clock >= state.Alien.NextSpawnYear:
        spawnAlienForces(state)
        state.Alien.NextSpawnYear += AlienSpawnIntervalYears

    // Bot tick (FR-062)
    tickCount++
    if tickCount % BotTickCadence == 0:
        cmds = e.Bot.Tick(state, state.Clock)
        for each botCmd in cmds:
            applyBotCommand(state, botCmd)  // immediate, no travel delay

    // Update known states for all systems (FR-018)
    state.UpdateKnownStates(state.Clock)

    // Broadcast matured events via SSE (FR-017)
    e.Events.BroadcastMatured(state)

    // Check victory/defeat (FR-056, FR-057, FR-058)
    if over, winner, reason = state.CheckVictory(); over:
        state.GameOver = true
        state.Winner   = winner
        state.WinReason = reason
        state.Paused   = true
        e.Events.BroadcastGameOver(winner, reason)
```

**Error handling**:
- `EnqueueCommand` returns error for: unknown system ID, player trying to command alien-held system, system status "unknown" (no information), insufficient projected output for construction.
- `tick` never panics; all subsystem calls recover from panics and log to stderr.

**Dependencies**: `context`, `time`, `internal/game/combat`, `internal/game/economy`, `internal/game/events`

---

### 6.6 `internal/game/combat.go`

**Purpose**: Stochastic combat resolution.

**Satisfies**: FR-049, FR-050, FR-051, FR-052, FR-053, FR-054

**Interface**:

```go
package game

// Resolve resolves all combat in the given system for the current tick.
// It mutates system forces and logs events.
func Resolve(state *GameState, sys *StarSystem)
```

**Behavior** (pseudocode):

```
Resolve(state, sys):
    humanUnits = collectAllHumanUnits(sys)    // flatten local units + fleet units
    alienUnits = collectAllAlienUnits(sys)

    if len(humanUnits) == 0 || len(alienUnits) == 0: return

    // Check for and extract Reporters before combat (they flee immediately)
    reportersFled = false
    for each humanFleet in sys with Reporter units:
        reportersFled = true
        departing = createReporterFleet(humanFleet.Reporter units → Sol)
        departing.ArrivalYear = state.Clock + sys.DistFromSol / FleetSpeedC
        state.Fleets[departing.ID] = departing
        remove reporter units from humanFleet

    // Round-based combat
    rng = rand.New(rand.NewSource(time.Now().UnixNano()))
    maxRounds = 20  // safety cap
    for round = 0; round < maxRounds && len(humanUnits) > 0 && len(alienUnits) > 0; round++:
        // Each human unit fires at a random alien unit
        for each attacker in humanUnits (shuffle order):
            target = randomAlienUnit(alienUnits, rng)
            p = hitProbability(attacker, target)  // Attack / (Attack + Defense)
            if rng.Float64() < p:
                eliminate(target from alienUnits)
                state.Alien.TotalLost++

        // Each surviving alien unit fires at a random human unit
        for each attacker in alienUnits (shuffle order):
            target = randomHumanUnit(humanUnits, rng)
            p = hitProbability(attacker, target)
            if rng.Float64() < p:
                eliminate(target from humanUnits)

    // Determine outcome
    humanWon = len(alienUnits) == 0 && len(humanUnits) > 0
    alienWon = len(humanUnits) == 0 && len(alienUnits) > 0
    draw     = len(humanUnits) == 0 && len(alienUnits) == 0

    // Apply outcome to system
    if alienWon || draw:
        sys.Status = StatusAlien
        clearHumanForces(sys)
        sys.EconLevel = 0
    if humanWon:
        sys.Status = StatusHuman
        clearAlienForces(sys)

    // Write-back surviving unit counts to sys (from unit slices)
    reconcileForces(sys, humanUnits, alienUnits)

    // Log events (FR-052, FR-053)
    silentEvt = &GameEvent{
        EventYear:   state.Clock,
        ArrivalYear: math.MaxFloat64,   // never reaches Earth
        SystemID:    sys.ID,
        Type:        EventCombatSilent,
        Description: summarizeCombat(humanWon, alienWon, draw, casualties),
    }
    state.RecordEvent(silentEvt)

    if reportersFled:
        // Reporter will carry the combat result; arrivalYear set on reporter fleet
        reportEvt = &GameEvent{
            EventYear:   state.Clock,
            ArrivalYear: state.Clock + sys.DistFromSol / FleetSpeedC,  // 0.8c travel
            SystemID:    sys.ID,
            Type:        EventCombatOccurred,
            Description: summarizeCombat(humanWon, alienWon, draw, casualties),
        }
        state.RecordEvent(reportEvt)
```

**`hitProbability(attacker, target)`**:
```
p = float64(WeaponDefs[attacker.Type].Attack) /
    float64(WeaponDefs[attacker.Type].Attack + WeaponDefs[target.Type].Defense)
// clamped to [0.05, 0.95] to avoid guaranteed hits/misses
```

**Unit representation during combat**: A unit is `struct { Type WeaponType; Owner Owner }`. Slices are built by expanding `map[WeaponType]int` into individual unit structs. This makes round-by-round elimination straightforward.

**Error handling**: If a system has both sides' forces but all units of one side are zero (shouldn't happen due to guard at top), function returns immediately. No panic.

**Dependencies**: `math/rand`, `time`, `math`

---

### 6.7 `internal/game/economy.go`

**Purpose**: Economic output accumulation and construction order execution.

**Satisfies**: FR-045, FR-046, FR-047, FR-048, FR-034, FR-035, FR-036

**Interface**:

```go
package game

// AccumulateOutput adds output to each human-held system proportional to deltaYears.
func AccumulateOutput(state *GameState, deltaYears float64)

// ValidateConstruct checks whether a construction command can execute.
// Returns nil if valid, error describing the rejection reason if not.
func ValidateConstruct(sys *StarSystem, wt WeaponType, qty int) error

// ExecuteConstruct applies an approved construction order to the system.
func ExecuteConstruct(state *GameState, sys *StarSystem, wt WeaponType, qty int)

// ProjectedOutput returns estimated accumulated output at futureYear,
// given current output and the system's econ level.
func ProjectedOutput(sys *StarSystem, futureYear float64) float64
```

**Behavior**:

```
AccumulateOutput(state, deltaYears):
    for each sys in state.Systems where sys.Status == StatusHuman:
        sys.AccumOutput += EconOutputRate[sys.EconLevel] * deltaYears

ValidateConstruct(sys, wt, qty):
    def = WeaponDefs[wt]
    if sys.EconLevel < def.MinLevel:
        return error("economic level %d required, system has %d", def.MinLevel, sys.EconLevel)
    totalCost = def.Cost * float64(qty)
    if sys.AccumOutput < totalCost:
        return error("insufficient output: need %.1f, have %.1f", totalCost, sys.AccumOutput)
    return nil

ExecuteConstruct(state, sys, wt, qty):
    def = WeaponDefs[wt]
    sys.AccumOutput -= def.Cost * float64(qty)
    if WeaponDefs[wt].CanMove:
        // Create a fleet for interstellar-capable units
        fleet = &Fleet{
            ID: state.NewFleetID(), Name: state.NewFleetName(),
            Owner: HumanOwner,
            Units: {wt: qty},
            LocationID: sys.ID, InTransit: false,
        }
        state.Fleets[fleet.ID] = fleet
        sys.FleetIDs = append(sys.FleetIDs, fleet.ID)
    else:
        sys.LocalUnits[wt] += qty
    // Log construction complete event (propagates at c)
    evt = &GameEvent{
        EventYear: state.Clock, ArrivalYear: state.Clock + sys.DistFromSol,
        SystemID: sys.ID, Type: EventConstructionDone,
        Description: fmt.Sprintf("Constructed %d %s", qty, wt),
    }
    state.RecordEvent(evt)

ProjectedOutput(sys, futureYear):
    // futureYear is the year the command arrives
    deltaYears = futureYear - currentClock   // approximate
    return sys.AccumOutput + EconOutputRate[sys.EconLevel] * deltaYears
```

**Error handling**: `ValidateConstruct` returns descriptive errors; callers log these as `command_failed` events. `ExecuteConstruct` panics if called with an invalid weapon type (programming error, not a runtime error).

---

### 6.8 `internal/game/events.go`

**Purpose**: Event creation helpers, event log management, SSE fan-out.

**Satisfies**: FR-015, FR-016, FR-017, FR-025, FR-026, FR-028

**Interface**:

```go
package game

import "net/http"

type EventManager struct {
    mu      sync.Mutex
    clients map[string]chan []byte  // key: client ID, value: buffered channel
}

func NewEventManager() *EventManager

// Register adds an SSE client. Returns a channel that receives JSON-encoded event lines.
func (m *EventManager) Register(clientID string) <-chan []byte

// Unregister removes a disconnected SSE client.
func (m *EventManager) Unregister(clientID string)

// BroadcastMatured sends all newly-matured events to all registered clients.
// Called on every engine tick.
func (m *EventManager) BroadcastMatured(state *GameState)

// BroadcastClockSync sends a clock synchronization event to all clients.
func (m *EventManager) BroadcastClockSync(state *GameState)

// BroadcastGameOver sends the game-over event to all clients.
func (m *EventManager) BroadcastGameOver(winner Owner, reason string)
```

**SSE wire format** (each SSE message is a `text/event-stream` frame):

```
event: clock_sync
data: {"gameYear":42.500,"paused":false}

event: game_event
data: {"id":"evt-42","arrivalYear":45.200,"systemId":"gj-551","type":"combat_occurred","description":"Human forces repelled alien attack at Proxima Centauri"}

event: system_update
data: {"systemId":"gj-551","knownStatus":"human","knownAsOfYear":40.100,"knownEconLevel":3,"knownLocalUnits":{"orbital_defense":2},"knownFleets":[]}

event: game_over
data: {"winner":"human","reason":"Alien forces exhausted. Earth and 68% of systems retained."}
```

**`BroadcastMatured` behavior**:
```
BroadcastMatured(state):
    for each evt in state.Events where !evt.Broadcast && evt.ArrivalYear <= state.Clock
                                    && evt.ArrivalYear < math.MaxFloat64:
        payload = marshal SSE "game_event" frame from evt
        broadcast payload to all client channels
        evt.Broadcast = true

        // Also send updated known state for the event's system
        sys = state.Systems[evt.SystemID]
        sysPayload = marshal SSE "system_update" frame from sys.KnownState fields
        broadcast sysPayload to all client channels
```

**Client channel**: Buffered channel of size 64. If a client's channel is full (client is slow), the event is dropped for that client (they will re-sync on reconnect via `GET /api/state`).

**Clock sync cadence**: `BroadcastClockSync` is called by the engine every 100 real ticks (every 10 real seconds). Also called immediately on pause/unpause.

**Error handling**: Writing to a closed channel is recovered with `recover()` in the broadcast loop; the client is then unregistered.

---

### 6.9 `internal/game/bot.go`

**Purpose**: Defines the `BotAgent` interface and provides the default alien bot implementation.

**Satisfies**: FR-062, FR-063, FR-064

**Interface**:

```go
package game

// BotAgent is the interface the engine uses to drive the alien side.
// An alternative bot replaces only this implementation.
type BotAgent interface {
    // Initialize is called once before the game loop starts.
    Initialize(state *GameState)

    // Tick is called every BotTickCadence engine ticks.
    // It returns zero or more commands to execute immediately (no travel delay).
    // state is passed read-locked; the bot must not write to it directly.
    Tick(state *GameState, currentYear float64) []BotCommand

    // OnEvent is called for every newly-broadcast event. May be called
    // concurrently with Tick; the implementation must be thread-safe.
    OnEvent(evt *GameEvent)
}

type BotCommand struct {
    Type       CommandType
    SystemID   string       // source system for CmdMove
    WeaponType WeaponType   // for CmdConstruct
    Quantity   int
    FleetID    string       // for CmdMove
    DestID     string       // for CmdMove
}

// DefaultBot is the built-in alien bot.
type DefaultBot struct {
    // internal state: target priorities, fleet assignments, etc.
    mu      sync.Mutex
    targets []string   // system IDs prioritized for attack
}

func NewDefaultBot() *DefaultBot
func (b *DefaultBot) Initialize(state *GameState)
func (b *DefaultBot) Tick(state *GameState, year float64) []BotCommand
func (b *DefaultBot) OnEvent(evt *GameEvent)
```

**Bot strategy (DefaultBot)**:
The DefaultBot implements a simplified aggressive strategy:
1. **Prioritize Earth-adjacent systems**: Sort human-held systems by distance from the alien entry point, ascending (attack closest first).
2. **Move all available alien forces**: For each alien fleet at an entry point or alien-held system, dispatch it toward the highest-priority target that has no alien fleet already en-route.
3. **Do not construct** (alien forces spawn only via `spawnAlienForces` in the engine).

This strategy is intentionally simple. It is the substitution point for future improved bots.

**Error handling**: If `Tick` panics, the engine's goroutine recovers and logs the panic; the bot command is dropped.

---

### 6.10 `internal/server/server.go`

**Purpose**: HTTP server setup, routing, embedded asset serving.

**Satisfies**: FR-001, FR-002, FR-003, FR-004, NFR-002, NFR-004

**Interface**:

```go
package server

import "context"

type Server struct {
    engine  *game.Engine
    events  *game.EventManager
    state   *game.GameState
    httpSrv *http.Server
}

func New(engine *game.Engine, events *game.EventManager, state *game.GameState) *Server

// ListenAndServe starts the server on 127.0.0.1:8080. Blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error
```

**Routing table**:

| Method | Path | Handler |
|--------|------|---------|
| `GET` | `/` | Serve `index.html` from embedded FS |
| `GET` | `/assets/*` | Serve from embedded FS |
| `GET` | `/api/stars` | `handleStars` |
| `GET` | `/api/state` | `handleState` |
| `GET` | `/api/events` | `handleEvents` (SSE) |
| `POST` | `/api/command` | `handleCommand` |
| `POST` | `/api/pause` | `handlePause` |

**Embedded asset serving** (`web/embed.go`):

```go
// web/embed.go
package webui

import "embed"

//go:embed dist
var DistFS embed.FS
```

In `server.go`:
```go
import (
    "io/fs"
    webui "github.com/gmofishsauce/SpaceGame/web"
)

subFS, _ := fs.Sub(webui.DistFS, "dist")
fileServer := http.FileServerFS(subFS)
// Register fileServer for "/" AFTER API routes
```

**`handleEvents` (SSE)**:
1. Set headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`.
2. Register client with `events.Register(clientID)`.
3. Immediately send current full state as `connected` event.
4. Loop: receive from client channel, write to `w`, flush (`w.(http.Flusher).Flush()`).
5. On client disconnect (channel closed or context done): `events.Unregister(clientID)`.

**CORS**: Not needed (same origin: server serves the SPA).

**Error handling**:
- If a handler panics: recover and return 500.
- If `net.Listen` fails: return the error from `ListenAndServe`.
- If client disconnects mid-SSE: detected via `r.Context().Done()`.

---

### 6.11 `internal/server/handlers.go`

**Purpose**: Individual HTTP handler functions.

**Satisfies**: FR-029, FR-031, FR-034, FR-037

**Interface** (method receivers on `*Server`):

```go
func (s *Server) handleStars(w http.ResponseWriter, r *http.Request)
func (s *Server) handleState(w http.ResponseWriter, r *http.Request)
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request)
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request)
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request)
```

**`handleStars`**:
- Acquires `state.mu.RLock()`
- Returns JSON array of star positions for Three.js rendering (permanent data, no game state):
  ```json
  [{"id":"gj-551","displayName":"Proxima Centauri","x":-1.546,"y":1.183,"z":3.769,"hasPlanets":true,"isSol":false}]
  ```
- Releases lock
- Response cached with `Cache-Control: max-age=86400` (positions never change)

**`handleState`**:
- Acquires `state.mu.RLock()`
- Returns full player-visible state snapshot:
  ```json
  {
    "gameYear": 42.5, "paused": false, "gameOver": false,
    "systems": [{ "id": "...", "knownStatus": "human", "knownAsOfYear": 38.0, ... }],
    "events": [{ "id": "...", "arrivalYear": 40.1, "systemId": "...", "type": "...", "description": "..." }],
    "fleets": []
  }
  ```
- Filters events to only those with `arrivalYear ≤ gameYear` and `type != EventCombatSilent`
- Returns only `KnownState` fields of systems, not ground truth

**`handleCommand`**:
1. Decode JSON body into `CommandRequest`
2. Validate: system exists, command type is valid, system's known status permits the action
3. Compute `executeYear = state.Clock + dist(Sol, system) / CommandSpeedC` (or 0 for Sol)
4. Call `engine.EnqueueCommand(pendingCmd)`
5. If error: return 400 with JSON error body
6. If ok: return 200:
   ```json
   {"ok":true,"commandId":"cmd-42","estimatedArrivalYear":47.3}
   ```

**`handlePause`**:
- Decode `{"paused": true}` or `{"paused": false}`
- Call `engine.SetPaused(paused)`
- Broadcast `clock_sync` event immediately
- Return 200 `{"ok":true}`

**Error response format** (all errors):
```json
{"ok":false,"error":"insufficient economic output: need 75.0, have 43.2"}
```

---

### 6.12 `internal/server/types.go`

**Purpose**: JSON request/response struct definitions.

```go
package server

type CommandRequest struct {
    Type       game.CommandType  `json:"type"`
    SystemID   string            `json:"systemId"`
    WeaponType game.WeaponType   `json:"weaponType,omitempty"`
    Quantity   int               `json:"quantity,omitempty"`
    FleetID    string            `json:"fleetId,omitempty"`
    DestID     string            `json:"destinationId,omitempty"`
}

type CommandResponse struct {
    OK                  bool    `json:"ok"`
    CommandID           string  `json:"commandId,omitempty"`
    EstimatedArrivalYear float64 `json:"estimatedArrivalYear,omitempty"`
    Error               string  `json:"error,omitempty"`
}

type PauseRequest struct {
    Paused bool `json:"paused"`
}

type StarDTO struct {
    ID          string  `json:"id"`
    DisplayName string  `json:"displayName"`
    X           float64 `json:"x"`
    Y           float64 `json:"y"`
    Z           float64 `json:"z"`
    HasPlanets  bool    `json:"hasPlanets"`
    IsSol       bool    `json:"isSol"`
}

type SystemDTO struct {
    ID              string          `json:"id"`
    DisplayName     string          `json:"displayName"`
    KnownStatus     game.SystemStatus `json:"knownStatus"`
    KnownAsOfYear   float64         `json:"knownAsOfYear"`
    KnownEconLevel  int             `json:"knownEconLevel"`
    KnownLocalUnits map[string]int  `json:"knownLocalUnits"`
    KnownFleets     []FleetDTO      `json:"knownFleets"`
}

type FleetDTO struct {
    ID         string         `json:"id"`
    Name       string         `json:"name"`
    Owner      game.Owner     `json:"owner"`
    Units      map[string]int `json:"units"`
    InTransit  bool           `json:"inTransit"`
    DestID     string         `json:"destinationId,omitempty"`
    ArrivalYear float64       `json:"arrivalYear,omitempty"`
}

type EventDTO struct {
    ID          string  `json:"id"`
    ArrivalYear float64 `json:"arrivalYear"`
    SystemID    string  `json:"systemId"`
    Type        string  `json:"type"`
    Description string  `json:"description"`
}

type StateResponse struct {
    GameYear  float64     `json:"gameYear"`
    Paused    bool        `json:"paused"`
    GameOver  bool        `json:"gameOver"`
    Winner    string      `json:"winner,omitempty"`
    WinReason string      `json:"winReason,omitempty"`
    Systems   []SystemDTO `json:"systems"`
    Events    []EventDTO  `json:"events"`
}
```

---

### 6.13 `cmd/spacegame/main.go`

**Purpose**: Entry point. Wires together loader, engine, bot, event manager, and server.

**Satisfies**: FR-001, NFR-002

**Behavior**:

```go
func main():
    ctx, cancel = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    state, err = game.Initialize("nearest.csv", "planets.csv")
    if err: log.Fatal(err)

    events = game.NewEventManager()
    bot    = game.NewDefaultBot()
    engine = game.NewEngine(state, bot, events)

    go engine.Run(ctx)      // game loop goroutine
    go bot.run(ctx)         // bot main goroutine (if needed for async events)

    srv = server.New(engine, events, state)
    log.Printf("SpaceGame listening on http://127.0.0.1:8080")
    if err = srv.ListenAndServe(ctx); err != nil && !errors.Is(err, http.ErrServerClosed):
        log.Fatal(err)
```

**Error handling**: Fatal on CSV load failure. On signal, context cancels, engine and server shut down gracefully (server has a 5-second shutdown timeout via `http.Server.Shutdown(ctx)`).

---

### 6.14 `web/src/main.js`

**Purpose**: SPA entry point. Initializes all client-side modules.

**Satisfies**: FR-002, FR-014, FR-017, FR-019

```javascript
import { StarMap } from './starmap.js'
import { Sidebar } from './sidebar.js'
import { UIController } from './ui.js'
import { APIClient } from './api.js'
import { ClientState } from './state.js'

const state = new ClientState()
const api = new APIClient(state)
const starMap = new StarMap(state, api)
const sidebar = new Sidebar(state)
const ui = new UIController(state, api, starMap)

async function init() {
    const [stars, gameState] = await Promise.all([api.fetchStars(), api.fetchState()])
    state.initStars(stars)
    state.initGameState(gameState)
    starMap.init(stars)
    sidebar.init()
    ui.init()
    api.connectSSE()
}

document.addEventListener('DOMContentLoaded', init)
```

---

### 6.15 `web/src/starmap.js`

**Purpose**: Three.js 3D star map. Extends the prototype directly.

**Satisfies**: FR-019, FR-020, FR-021, FR-022, FR-023, FR-024, FR-027

**Extending the prototype**:
- The prototype's Points object, CSS2DRenderer, OrbitControls, axis lines, dashed-line hover, and demand-rendering pattern are all reused verbatim.
- The following are added:
  1. **Per-vertex colors** on the Points geometry (status-based coloring, FR-020).
  2. **Extended hover popup** (status, year, econ, forces — FR-021).
  3. **Right-click handler** for context menus (FR-029).
  4. **Destination-selection mode** (left-click selects fleet destination).
  5. **Sidebar highlight**: a method `highlightStar(systemId)` that temporarily brightens a star's vertex color.

**Status colors** (tunable constants in `web/src/constants.js`):

| Status | Color (hex) |
|--------|------------|
| human | `0x4488ff` (blue) |
| alien | `0xff3333` (red) |
| contested | `0xff8800` (orange) |
| unknown | `0x888888` (gray) |
| uninhabited | `0xffffff` (white) |
| Sol | `0xffff88` (yellow-white) |

**Vertex color update** (`updateStarColors()`):
```javascript
updateStarColors() {
    const colors = this.pointsGeometry.attributes.color.array
    for (let i = 0; i < STAR_DATA.length; i++) {
        const systemId = STAR_DATA[i].id
        const status = this.state.getKnownStatus(systemId)
        const c = new THREE.Color(STATUS_COLORS[status])
        colors[i*3]   = c.r
        colors[i*3+1] = c.g
        colors[i*3+2] = c.b
    }
    this.pointsGeometry.attributes.color.needsUpdate = true
    this.requestRender()
}
```

**Hover popup** (extended CSS2D label):
```html
<div class="star-popup">
  <div class="star-name">Proxima Centauri</div>
  <div class="star-status">Status: Human-held (as of year 38.2)</div>
  <div class="star-econ">Economy: Level 3</div>
  <div class="star-forces">Forces: 2 Orbital Defense, Fleet Alpha</div>
</div>
```

**Right-click**:
```javascript
renderer.domElement.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    const star = this.getStarAtMouse(e)
    if (star) ui.showContextMenu(e.clientX, e.clientY, star)
})
```

**Demand rendering** is retained from the prototype. `updateStarColors()` calls `requestRender()`.

---

### 6.16 `web/src/sidebar.js`

**Purpose**: Scrolling event log sidebar.

**Satisfies**: FR-025, FR-026, FR-027, FR-028

**Behavior**:
- A `<div id="sidebar">` contains `<div id="event-log">`.
- `appendEvent(evt)` creates an entry:
  ```html
  <div class="event-entry" data-system-id="gj-551">
    <span class="evt-year">Year 45.2</span>
    <span class="evt-system">Proxima Centauri</span>
    <span class="evt-desc">Human forces repelled alien attack. 3 units lost.</span>
  </div>
  ```
- Auto-scroll: if the sidebar was scrolled to the bottom before the new entry, scroll to bottom after append. Detect "at bottom" as `scrollTop + clientHeight >= scrollHeight - 10`.
- Mouseover on entry: calls `starMap.highlightStar(systemId)`.
- Mouseout on entry: calls `starMap.unhighlightStar()`.

---

### 6.17 `web/src/ui.js`

**Purpose**: Year counter, pause indicator, context menus, construction and fleet-command dialogs, keyboard handler.

**Satisfies**: FR-013, FR-014, FR-029, FR-030, FR-031, FR-032, FR-034, FR-035, FR-036, FR-037, FR-038, FR-039, FR-058

**Year counter**: A fixed `<div id="year-display">Year: 0.0</div>` updated by `ClientState`. The client advances its local year display using `setInterval(100ms)` based on server-provided time scale, reconciling with `clock_sync` SSE events.

**Pause indicator**: A `<div id="pause-overlay">⏸ PAUSED</div>` shown/hidden based on `state.paused`.

**Escape key handler**: Sends `POST /api/pause` toggling `paused`. If in destination-selection mode, Escape exits that mode instead.

**Context menu** (`showContextMenu(x, y, star)`):
1. Close any existing context menu.
2. Determine valid actions from `state.getKnownState(star.id)`:
   - If human-held: show "Construct…" if econ level ≥ min buildable, "Command…" if fleets present.
   - If alien-held or unknown: no actions available (show "No actions available").
   - If Sol: always show "Construct…" and "Command…" if applicable.
3. Render a `<div class="context-menu">` at `(x, y)`.
4. Clicking outside closes menu.

**Construction dialog** (`showConstructDialog(star)`):
1. Compute `commandTravelYears = star.distFromSol / COMMAND_SPEED_C` (displayed to player).
2. Compute `projectedOutput = state.getProjectedOutput(star.id, commandTravelYears)`.
3. Show modal with weapon type table: Name, Cost, Min Level, "Can Build?" (checked if econ level ≥ min and projected output ≥ cost).
4. On confirm: `POST /api/command` with `{type:"construct", systemId, weaponType, quantity:1}`.
5. Quantity is always 1 in MVP (dialog shows one unit at a time).

**Fleet command dialog** (`showFleetCommandDialog(star)`):
1. List known fleets at `star` with `canMove = true`.
2. Player selects one fleet by click.
3. Dialog closes; map enters destination-selection mode (cursor: crosshair; `ui.destinationMode = true`).
4. Next left-click on a star → show confirmation modal: "Send [fleetName] from [origin] to [destination]? Fleet arrives in ~N years."
5. On confirm: `POST /api/command` with `{type:"move", systemId, fleetId, destinationId}`.
6. Escape cancels destination-selection mode.

**Game-over screen** (`showGameOverScreen(winner, reason)`):
1. Cover the map with a semi-transparent overlay.
2. Show centered modal: "VICTORY" or "DEFEAT", reason string, "Close" button.
3. "Close" removes the overlay but game remains paused.

---

### 6.18 `web/src/api.js`

**Purpose**: All browser-to-server communication.

**Satisfies**: FR-017, FR-029

```javascript
export class APIClient {
    constructor(state) { this.state = state }

    async fetchStars() {
        const r = await fetch('/api/stars')
        return r.json()
    }

    async fetchState() {
        const r = await fetch('/api/state')
        return r.json()
    }

    async sendCommand(cmd) {
        const r = await fetch('/api/command', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(cmd)
        })
        return r.json()
    }

    async setPaused(paused) {
        const r = await fetch('/api/pause', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({paused})
        })
        return r.json()
    }

    connectSSE() {
        this.es = new EventSource('/api/events')
        this.es.addEventListener('clock_sync', e => this.state.onClockSync(JSON.parse(e.data)))
        this.es.addEventListener('game_event', e => this.state.onGameEvent(JSON.parse(e.data)))
        this.es.addEventListener('system_update', e => this.state.onSystemUpdate(JSON.parse(e.data)))
        this.es.addEventListener('game_over', e => this.state.onGameOver(JSON.parse(e.data)))
        this.es.onerror = () => {
            // On disconnect, attempt reconnect after 2 seconds.
            // EventSource does this automatically.
        }
    }
}
```

**Error handling**: If `sendCommand` returns `{ok: false}`, `ui.showCommandError(response.error)` is called (a non-modal toast notification).

---

### 6.19 `web/src/state.js`

**Purpose**: Client-side state store. Processes SSE events; notifies UI components.

**Satisfies**: FR-017, FR-018, FR-020

```javascript
export class ClientState {
    constructor() {
        this.stars = []         // static, from /api/stars
        this.systems = {}       // key: systemId → SystemDTO
        this.events = []        // EventDTOs, sorted by arrivalYear
        this.gameYear = 0.0
        this.paused = false
        this.gameOver = false
        this.listeners = []     // {event: string, fn: function}
    }

    on(event, fn) { this.listeners.push({event, fn}) }
    emit(event, data) { this.listeners.filter(l => l.event === event).forEach(l => l.fn(data)) }

    initStars(stars) { this.stars = stars }

    initGameState(gs) {
        this.gameYear = gs.gameYear
        this.paused = gs.paused
        this.gameOver = gs.gameOver
        gs.systems.forEach(s => this.systems[s.id] = s)
        this.events = gs.events.sort((a,b) => a.arrivalYear - b.arrivalYear)
        this.emit('stateLoaded', this)
    }

    onClockSync(data) {
        this.gameYear = data.gameYear
        this.paused = data.paused
        this.localClockBase = data.gameYear
        this.localClockBaseTime = Date.now()
        this.emit('clockSync', data)
    }

    onGameEvent(evt) {
        this.events.push(evt)
        this.emit('newEvent', evt)
    }

    onSystemUpdate(upd) {
        Object.assign(this.systems[upd.systemId], upd)
        this.emit('systemUpdated', upd.systemId)
    }

    onGameOver(data) {
        this.gameOver = true
        this.emit('gameOver', data)
    }

    // Local time interpolation between clock_sync events
    getLocalYear() {
        if (this.paused) return this.gameYear
        const elapsed = (Date.now() - this.localClockBaseTime) / 1000.0
        return this.localClockBase + elapsed * TimeScaleYearsPerSecond
    }

    getKnownStatus(systemId) { return this.systems[systemId]?.knownStatus ?? 'unknown' }
    getKnownState(systemId)  { return this.systems[systemId] }
    getProjectedOutput(systemId, deltaYears) {
        const sys = this.systems[systemId]
        if (!sys) return 0
        const rate = ECON_OUTPUT_RATE[sys.knownEconLevel] ?? 0
        return (sys.knownAccumOutput ?? 0) + rate * deltaYears
    }
}
```

**Local year interpolation**: Between `clock_sync` events, the client advances its displayed year using `getLocalYear()` (called by `setInterval` in `ui.js` at 100 ms), providing smooth display without 10 SSE messages per second.

---

### 6.20 `web/src/constants.js`

**Purpose**: Client-side display constants (colors, sizes). Mirrors relevant server constants.

```javascript
export const TimeScaleYearsPerSecond = 10.0 / 180.0
export const CommandSpeedC = 0.8
export const FleetSpeedC = 0.8

export const STATUS_COLORS = {
    human:      0x4488ff,
    alien:      0xff3333,
    contested:  0xff8800,
    unknown:    0x888888,
    uninhabited: 0xffffff,
}
export const SOL_COLOR = 0xffff88

export const ECON_OUTPUT_RATE = [0, 1.0, 2.0, 4.0, 8.0, 16.0]

// Mirrored weapon defs for UI display (costs and min levels)
export const WEAPON_DEFS = {
    orbital_defense: { displayName: 'Orbital Defense', cost: 10,  minLevel: 1 },
    interceptor:     { displayName: 'Interceptor',     cost: 25,  minLevel: 2 },
    reporter:        { displayName: 'Reporter',         cost: 20,  minLevel: 2 },
    escort:          { displayName: 'Escort',           cost: 75,  minLevel: 3 },
    battleship:      { displayName: 'Battleship',       cost: 200, minLevel: 4 },
}
```

---

## 7. Data Model

### 7.1 Server-Side Go Structs (persistent / significant in-memory)

All structs defined in Section 6.3 (`internal/game/state.go`). Key data flows:

| Data | Created | Read | Updated | Deleted |
|------|---------|------|---------|---------|
| `StarSystem` | `loader.Initialize` | every tick, HTTP handlers | `engine.Tick` (status, forces, accum), `combat.Resolve` | never (systems are permanent) |
| `Fleet` | `loader.Initialize`, `economy.ExecuteConstruct`, `engine.spawnAlienForces` | every tick, HTTP handlers | `engine.Tick` (location on arrival), `combat.Resolve` | when fleet is destroyed in combat |
| `GameEvent` | `combat.Resolve`, `economy.ExecuteConstruct`, `engine.Tick` | `events.BroadcastMatured`, HTTP `/api/state` | `Broadcast = true` when pushed | never |
| `PendingCommand` | `engine.EnqueueCommand` | every tick | never (consumed) | when `executeYear ≤ Clock` |
| `AlienFaction` | `loader.Initialize` | `engine.Tick` | `TotalLost++` on alien unit death, `Exhausted=true` at threshold | never |

### 7.2 Alien Force Composition (G-1 assumption)

Alien forces use the same `WeaponType` enum. Alien units have attack/defense values from `WeaponDefs` (same table as human weapons). Initial spawn and per-wave composition:

| Spawn Type | Composition |
|------------|------------|
| Initial (per entry point) | 3 Escorts, 2 Battleships, 5 Interceptors |
| Wave spawn | 4 Escorts, 1 Battleship, 2 Interceptors |

These are all in `constants.go` as named constants.

### 7.3 Event `Details` Payloads (type-specific)

```go
type CombatDetails struct {
    HumanLosses  map[WeaponType]int `json:"humanLosses"`
    AlienLosses  map[WeaponType]int `json:"alienLosses"`
    HumanWon     bool               `json:"humanWon"`
    AlienWon     bool               `json:"alienWon"`
}

type ConstructionDetails struct {
    WeaponType WeaponType `json:"weaponType"`
    Quantity   int        `json:"quantity"`
}

type CommandFailedDetails struct {
    CommandType CommandType `json:"commandType"`
    Reason      string      `json:"reason"`
}

type FleetArrivalDetails struct {
    FleetID   string         `json:"fleetId"`
    FleetName string         `json:"fleetName"`
    Units     map[WeaponType]int `json:"units"`
}
```

### 7.4 Save/Load (Deferred — FR-059 to FR-061)

`GameState` must be JSON-serializable for future save/load. Design constraint: do not use unexported fields for any data that must persist. The `mu` field is excluded from JSON marshaling (`json:"-"`). Save format: a single JSON file containing the marshaled `GameState`.

### 7.5 Coordinate System

Identical to the prototype (from `proto/design.md`):

```
js_x = astro_x
js_y = astro_z    (celestial north = up in Three.js)
js_z = -astro_y
```

The conversion is applied in `loader.go` using the same formula as `tools/gendata/main.go`.

---

## 8. Key Design Decisions

| Decision | Alternatives Considered | Choice | Rationale |
|----------|------------------------|--------|-----------|
| **Server-to-client communication** | HTTP polling (every N seconds), WebSocket, SSE | **SSE** (`text/event-stream`) | SSE works with Go standard library (`net/http`); EventSource API is supported in all target browsers; unidirectional (server-to-client only) which is sufficient since client-to-server commands use regular HTTP POST. WebSocket requires a library or `net/http` hijacking. |
| **Embedded vs. disk-served assets** | `http.FileServer(http.Dir("web/dist"))`, `//go:embed` | **`//go:embed`** in `web/embed.go` | Embedding makes the binary self-contained (no dependency on working directory at runtime). The `embed` package is part of the Go standard library. The `web/embed.go` trick (embed the `dist` subdirectory from within the `web/` package) avoids the `..` path restriction on `//go:embed`. |
| **Star data delivery to browser** | Pre-generated `stardata.js` (prototype approach), `GET /api/stars` endpoint | **`GET /api/stars` endpoint** | Server already parses CSVs at startup; serving via API avoids a separate build step (no `go run ./tools/gendata` needed for the full game). Simplifies the build pipeline. |
| **Bot architecture** | In-process goroutine, external HTTP process | **In-process goroutine implementing `BotAgent` interface** | FR-064 requires easy substitution: the interface boundary satisfies this. In-process avoids IPC overhead and network serialization. The `BotAgent` interface would be the contract for an external bot if ever needed. |
| **Bot information model** | Bot subject to information delay (symmetric), bot has ground-truth (asymmetric) | **Ground-truth access** | Explicitly stated in `requirements.md` Section 7 Assumptions: "The alien bot has access to ground-truth game state." This asymmetry is a deliberate design choice in the game's fiction. |
| **Combat model** | Deterministic (higher attack wins), simultaneous fire, sequential fire | **Round-based simultaneous fire** with per-unit stochastic hit rolls | "Stochastic function of types and quantities" (FR-050). Round-based allows combat to play out over multiple ticks even though in this design it resolves in one engine tick (multiple rounds within a single tick). Hit probability formula is simple and tunable. |
| **Planet ring inheritance** | Remove rings after status change, change ring color, keep rings only for human systems | **Ring color matches system status color** | Planet rings are a visual asset from the prototype. Changing their color (border color of the CSS2D div) to match status provides continuity and additional visual information. |
| **Command travel speed** | Commands travel at c (laser), commands travel at 0.8c (courier ship) | **0.8c (FR-031 explicit)** | FR-031 specifies "0.8" as the divisor; VisionStatement notes that "massive lasers/masers" could in principle transmit at c, but FR-031 is authoritative. |
| **Local clock interpolation** | Continuous SSE clock events, JS-side interpolation | **JS interpolation + periodic `clock_sync`** | Sending 10 SSE frames/second solely for clock updates wastes bandwidth and adds complexity. The client interpolates locally (using the known time scale) and reconciles on each `clock_sync` event (every 10 real seconds) and on pause/unpause. |
| **Fleet naming** | Sequential numbers ("Fleet 1"), NATO phonetic ("Fleet Alpha"), random names | **Sequential with NATO alphabet prefix** | "Fleet Alpha", "Fleet Bravo", ... "Fleet Zulu", "Fleet 27", "Fleet 28", ... Provides unique, memorable names with minimal logic. |
| **Single Points object with vertex colors** | Separate Points per status group, individual Sprite per star | **Single Points + vertex colors** | Reuses the prototype's single-Points approach. Vertex colors (BufferAttribute) allow per-star color control. Updating `needsUpdate = true` on the color attribute triggers a GPU re-upload, which is fast for ~108 stars. |

---

## 9. File and Directory Plan

### New Files to Create

- `cmd/spacegame/main.go` — **CREATE** — Server entry point; wires all subsystems, starts goroutines
- `internal/game/constants.go` — **CREATE** — All tunable game parameters (NFR-005)
- `internal/game/types.go` — **CREATE** — Shared enums and structs
- `internal/game/state.go` — **CREATE** — `GameState`, `StarSystem`, `Fleet`, `GameEvent`, `PendingCommand`
- `internal/game/loader.go` — **CREATE** — CSV parsing and initial state construction
- `internal/game/engine.go` — **CREATE** — Game loop goroutine (`Run`), tick dispatch, command queue
- `internal/game/combat.go` — **CREATE** — Stochastic combat resolver
- `internal/game/economy.go` — **CREATE** — Output accumulation, construction validation/execution
- `internal/game/events.go` — **CREATE** — Event log, SSE client registry, broadcast
- `internal/game/bot.go` — **CREATE** — `BotAgent` interface + `DefaultBot` implementation
- `internal/server/server.go` — **CREATE** — HTTP server, routing, embedded asset serving
- `internal/server/handlers.go` — **CREATE** — HTTP handler functions
- `internal/server/types.go` — **CREATE** — JSON request/response DTOs
- `web/embed.go` — **CREATE** — `//go:embed dist` declaration; package `webui`
- `web/package.json` — **CREATE** — npm config; Three.js dependency; build scripts
- `web/vite.config.js` — **CREATE** — Vite config; dev proxy to localhost:8080 for `/api/`
- `web/index.html` — **CREATE** — SPA shell HTML; `<script type="module" src="/src/main.js">`
- `web/src/main.js` — **CREATE** — App entry point; module wiring
- `web/src/starmap.js` — **CREATE** — Three.js scene (extends prototype logic)
- `web/src/sidebar.js` — **CREATE** — Event log sidebar
- `web/src/ui.js` — **CREATE** — Year counter, pause, context menus, dialogs
- `web/src/api.js` — **CREATE** — fetch and SSE client wrappers
- `web/src/state.js` — **CREATE** — Client-side state store and event handlers
- `web/src/constants.js` — **CREATE** — Client display constants
- `Makefile` — **CREATE** — Encodes build order: `npm run build` then `go build`

### Existing Files (Unchanged)

- `proto/` — **UNCHANGED** — Prototype; kept for reference
- `tools/gendata/main.go` — **UNCHANGED** — Used only by prototype
- `tools/gendata/README.md` — **UNCHANGED**
- `nearest.csv` — **UNCHANGED** — Source data
- `planets.csv` — **UNCHANGED** — Source data
- `VisionStatement.md` — **UNCHANGED**
- `requirements.md` — **UNCHANGED**
- `.gitignore` — **UNCHANGED** (already ignores `node_modules/` and `proto/dist/`)

### Existing Files to Modify

- `go.mod` — **MODIFY** — No new external dependencies; may need updated module path comment. No changes to `go 1.24.2` directive.
- `README.md` — **MODIFY** — Add build instructions: `cd web && npm install && npm run build`, then `go run ./cmd/spacegame`
- `.gitignore` — **MODIFY** — Add `web/dist/` to ignored paths

---

## 10. Requirement Traceability

| Requirement | Design Section | Files |
|-------------|---------------|-------|
| FR-001 | 6.10 Server | `internal/server/server.go`, `cmd/spacegame/main.go` |
| FR-002 | 6.10, 6.14 | `internal/server/server.go`, `web/embed.go`, `web/index.html` |
| FR-003 | 6.10, §4 Constraints | All `internal/` Go files |
| FR-004 | 6.3, 6.10 | `internal/game/state.go`, `internal/server/handlers.go` |
| FR-005 | 6.4 | `internal/game/loader.go` |
| FR-006 | 6.4 | `internal/game/loader.go` |
| FR-007 | 6.4 | `internal/game/loader.go`, `internal/game/constants.go` |
| FR-008 | 6.4 | `internal/game/loader.go`, `internal/game/constants.go` |
| FR-009 | 6.4 | `internal/game/loader.go`, `internal/game/constants.go` |
| FR-010 | 6.4 | `internal/game/loader.go` (`KnownState` not set for alien entry points) |
| FR-011 | 6.3, 6.5 | `internal/game/state.go`, `internal/game/engine.go` |
| FR-012 | 6.5, 6.1 | `internal/game/engine.go`, `internal/game/constants.go` |
| FR-013 | 6.5, 6.17 | `internal/game/engine.go`, `internal/server/handlers.go`, `web/src/ui.js` |
| FR-014 | 6.17, 6.19 | `web/src/ui.js`, `web/src/state.js` |
| FR-015 | 6.8 | `internal/game/events.go`, `internal/game/state.go` |
| FR-016 | 6.8, 6.6 | `internal/game/events.go`, `internal/game/combat.go` |
| FR-017 | 6.8, 6.11 | `internal/game/events.go`, `internal/server/handlers.go` |
| FR-018 | 6.3 | `internal/game/state.go` (`UpdateKnownStates`) |
| FR-019 | 6.15 | `web/src/starmap.js` |
| FR-020 | 6.15, 6.20 | `web/src/starmap.js`, `web/src/constants.js` |
| FR-021 | 6.15 | `web/src/starmap.js` |
| FR-022 | 6.15 | `web/src/starmap.js` (prototype hover logic retained) |
| FR-023 | 6.19 | `web/src/state.js` (Sol always returns live `gameYear`) |
| FR-024 | 6.15 | `web/src/starmap.js` (OrbitControls retained) |
| FR-025 | 6.16 | `web/src/sidebar.js` |
| FR-026 | 6.16 | `web/src/sidebar.js` |
| FR-027 | 6.15, 6.16 | `web/src/starmap.js` (`highlightStar`), `web/src/sidebar.js` |
| FR-028 | 6.16 | `web/src/sidebar.js` |
| FR-029 | 6.15, 6.17 | `web/src/starmap.js` (right-click), `web/src/ui.js` (context menu) |
| FR-030 | 6.17 | `web/src/ui.js` (`showContextMenu` filtering) |
| FR-031 | 6.5, 6.11 | `internal/game/engine.go`, `internal/server/handlers.go` |
| FR-032 | 6.17, 6.19 | `web/src/ui.js`, `web/src/state.js` (`getProjectedOutput`) |
| FR-033 | 6.5, 6.8 | `internal/game/engine.go`, `internal/game/events.go` |
| FR-034 | 6.17 | `web/src/ui.js` (`showContextMenu`) |
| FR-035 | 6.17 | `web/src/ui.js` (`showConstructDialog`) |
| FR-036 | 6.7, 6.11 | `internal/game/economy.go`, `internal/server/handlers.go` |
| FR-037 | 6.17 | `web/src/ui.js` (`showContextMenu`) |
| FR-038 | 6.17 | `web/src/ui.js` (`showFleetCommandDialog`) |
| FR-039 | 6.5, 6.11 | `internal/game/engine.go`, `internal/server/handlers.go` |
| FR-040 | 6.1, 7.1 | `internal/game/constants.go`, `internal/game/types.go` |
| FR-041 | 6.1 | `internal/game/constants.go` (`FleetSpeedC`) |
| FR-042 | 6.3 | `internal/game/state.go` (`NewFleetName`) |
| FR-043 | 6.3 | `internal/game/state.go` (`StarSystem.LocalUnits`, `StarSystem.FleetIDs`) |
| FR-044 | 6.1 | `internal/game/constants.go` (`WeaponDefs[].MinLevel`) |
| FR-045 | 6.3 | `internal/game/state.go` (`StarSystem.EconLevel`) |
| FR-046 | 6.7 | `internal/game/economy.go` (`AccumulateOutput`) |
| FR-047 | 6.7 | `internal/game/economy.go` (`ValidateConstruct`, `ExecuteConstruct`) |
| FR-048 | §4 Assumptions | (EconLevel not modified except by capture/retake) |
| FR-049 | 6.5, 6.6 | `internal/game/engine.go`, `internal/game/combat.go` |
| FR-050 | 6.6 | `internal/game/combat.go` |
| FR-051 | 6.5 | `internal/game/engine.go` (combat check only on stationed forces) |
| FR-052 | 6.6 | `internal/game/combat.go` (always logs `EventCombatSilent`) |
| FR-053 | 6.6, 6.8 | `internal/game/combat.go` (reporter logic), `internal/game/events.go` |
| FR-054 | 6.6 | `internal/game/combat.go` (status update on outcome) |
| FR-055 | 6.5 | `internal/game/engine.go` (`spawnAlienForces`, exhaustion check) |
| FR-056 | 6.3, 6.1 | `internal/game/state.go` (`CheckVictory`), `internal/game/constants.go` |
| FR-057 | 6.3, 6.1 | `internal/game/state.go` (`CheckVictory`), `internal/game/constants.go` |
| FR-058 | 6.5, 6.17 | `internal/game/engine.go`, `web/src/ui.js` (`showGameOverScreen`) |
| FR-059 | Deferred | — |
| FR-060 | Deferred | — |
| FR-061 | Deferred | — |
| FR-062 | 6.9 | `internal/game/bot.go` |
| FR-063 | 6.9 | `internal/game/bot.go` (`BotAgent` interface) |
| FR-064 | 6.9 | `internal/game/bot.go` (interface substitution point) |
| NFR-001 | 6.14–6.19 | `web/src/*.js`, Vite bundling |
| NFR-002 | 6.10, 6.13 | `internal/server/server.go`, `cmd/spacegame/main.go` |
| NFR-003 | 6.5, 6.8, 6.19 | Engine tick 100ms; SSE push; JS interpolation |
| NFR-004 | 6.10, `web/package.json` | `//go:embed dist`; Vite bundles Three.js locally |
| NFR-005 | 6.1, 6.20 | `internal/game/constants.go`, `web/src/constants.js` |
| NFR-006 | 6.10 | `net.Listen("tcp", "127.0.0.1:8080")` |

---

## 11. Testing Strategy

### Unit Tests

| Module | Test Cases |
|--------|-----------|
| `loader.go` | Load valid CSVs → correct system count (~108); Sol at (0,0,0); co-located stars grouped; Earth economic level = 5; systems with planets → human-held; systems within half max dist, no planets → human-held; systems beyond half max dist, no planets → uninhabited. |
| `economy.go` | `AccumulateOutput` over 1 year at each level matches `EconOutputRate`; `ValidateConstruct` rejects if econ level < min; rejects if insufficient output; `ExecuteConstruct` deducts correct cost; `ProjectedOutput` matches manual calculation. |
| `combat.go` | 100 humans vs 0 aliens → no combat; zero humans: aliens win; equal forces: stochastic outcome (run N=1000, check both sides win ≥ 5% of the time); Reporter present → `EventCombatOccurred` logged with finite `arrivalYear`; no Reporter → `EventCombatSilent` logged with `arrivalYear = MaxFloat64`. |
| `state.go` | `UpdateKnownStates` at year T only applies events with `arrivalYear ≤ T`; `CheckVictory` returns human win when conditions met; returns alien win on Earth capture; returns false when neither. |
| `loader.go` (Gaussian) | Run 10,000 samples, verify all results in [1,5], mean within 0.1 of 2.5. |

### Integration Tests

| Scenario | How to Verify |
|----------|--------------|
| Server starts and serves `GET /` with 200 | `go test` with `httptest.NewServer` |
| `POST /api/command` construct → event appears in `/api/state` after appropriate `executeYear` | Advance mock clock to `executeYear`; poll `/api/state` |
| SSE stream delivers `game_event` when matured | Connect `EventSource` in test; advance engine clock; assert event received |
| Combat in system with Reporter → event in `GET /api/state` events list after `arrivalYear` | Set up system with both sides + reporter; advance clock to arrivalYear; check |
| Alien exhaustion → human win condition | Run engine to `AlienExhaustionThreshold` alien losses; assert `GameOver == true, Winner == "human"` |

### End-to-End (Manual)

These cannot be automated easily given Three.js; manual QA checklist:

1. Load game in browser; star map renders; Sol labeled; planet rings visible.
2. Right-click a human-held system; "Construct…" dialog shows weapons filtered by econ level and available output.
3. Issue construction order; check event sidebar shows entry after appropriate delay.
4. Right-click a system with a fleet; "Command…" dialog; select fleet; click destination; confirmation dialog shows transit time.
5. Pause with Escape; year counter freezes; resume with Escape; counter advances.
6. Hover a star: popup shows status, year, econ level, forces; dotted lines appear.
7. Hover sidebar entry: corresponding star brightens.
8. Run game to alien exhaustion (adjust `AlienExhaustionThreshold` to a small value for testing); game-over screen appears.

### Edge Cases

- System with `dist` exactly equal to `maxDist / 2` (boundary condition for FR-007): should be human-held.
- Fleet in transit when its origin system is captured: fleet continues to its destination unaffected.
- Player commands to a system that is captured before command arrives: `command_failed` event generated.
- All peripheral systems already chosen as alien entry points (unlikely with ≥ 2 candidates at 75% threshold, but guard in `loader.go`).
- Bot issues move command for a fleet that no longer exists (was destroyed in combat): `applyBotCommand` returns error, logs it, no panic.
- SSE client buffer full (slow client): event dropped; client reconnects, gets full state from `GET /api/state`.
- Two combat events in same system on same tick (shouldn't happen since combat resolves fully in one call per tick, but `Resolve` should be idempotent if called twice with no forces on one side).

---

## 12. Open Questions

These items must be resolved before the corresponding code is implemented. The developer should not start on affected modules until the author provides answers.

| ID | Question | Affects | Default (if author does not respond) |
|----|----------|---------|--------------------------------------|
| **OQ-001** | What are the exact numerical thresholds for human victory (`HumanWinRetentionFraction`) and alien victory (`AlienWinCaptureFraction`)? (FR-056, FR-057) | `constants.go`, `state.go` | 0.60 / 0.40 as listed in this document |
| **OQ-002** | What are the Gaussian parameters (mean, σ) for economic level initialization? (FR-008) | `constants.go`, `loader.go` | mean = 2.5, σ = 1.0 as listed |
| **OQ-003** | Does alien exhaustion track total units destroyed or total units ever spawned? Does it ramp smoothly (reduce spawn rate) or trigger at a single threshold (stop spawning entirely)? (FR-055) | `engine.go`, `constants.go` | Single threshold on total units destroyed; spawning stops entirely |
| **OQ-004** | When a player issues a fleet command while a previous command to the same system is already en-route: should they be queued (both execute in order), supersede (only the newest executes), or is this forbidden (player must cancel)? (OQ-004 from requirements) | `engine.go`, `ui.js` | Queue: both execute in arrival order |
| **OQ-005** | What visual treatment should distinguish stale information from current information? (e.g., faded color, italic label, timestamp in popup) (OQ-005 from requirements) | `constants.js`, `starmap.js` | Italic system name in popup; no color fade (information freshness shown via "as of year X" in popup) |
| **OQ-006** | Should the game support difficulty levels in the MVP? (OQ-006 from requirements) | `bot.go`, `constants.go` | Single difficulty only |
| **OQ-007** | Are Orbital Defense and Interceptor truly immobile (cannot be included in a fleet and moved)? The requirements are clear (FR-040 table) but the vision statement's ordering was different. | `types.go`, `ui.js` | Per FR-040: Orbital Defense = not interstellar; Interceptor = not interstellar |
| **OQ-008** | FR-016 states that reporter events use "the reporter's travel time." Does this mean: (a) the reporter travels at 0.8c and report arrives at `eventYear + dist/0.8`, or (b) the reporter departs immediately and the travel time is the round trip? The design assumes (a) (one-way reporter travel). | `combat.go`, `events.go` | One-way: `arrivalYear = eventYear + dist / FleetSpeedC` |
