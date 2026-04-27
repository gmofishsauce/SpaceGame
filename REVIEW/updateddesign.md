# Design: SpaceGame Dual-State Refactor

## 1. Overview

SpaceGame is a real-time interstellar war strategy game in which the player at Sol issues commands across a sphere of star systems, hampered by light-speed information delay. The game must maintain two distinct world states at all times: an **omniscient ground truth** (used by the engine and by the alien bot) and **Sol's view** (the player-visible state, advanced only by reports that have propagated to Sol at finite speed).

The current implementation collapses both states onto a single `StarSystem` struct using paired `X` / `KnownX` fields. The discipline is convention-only and has several known leaks (live fleet objects exposed through the player API; fleet departures broadcast without light-speed delay; mobile-unit construction never reflected in the known view; wealth deductions never reflected in the known view).

This design replaces the paired-field approach with two strictly separated types — `Truth` and `SolView` — bridged by a single `Event` propagation primitive. The HTTP layer can reach `SolView` only; handlers and SSE serializers cannot, by package boundary, see or accidentally serialize ground truth. All state changes the player should learn about flow through `Event`s with explicit `ArrivalYear` gating; the engine itself is the only writer of both `Truth` and `SolView`.

The refactor is large but mechanical. It does not introduce new gameplay features. It does fix the four highest-severity leaks/gaps identified in `architecturalreview.md` (defects #1–#4 in that document) and lays the groundwork for later work (channels, reporter coupling, event indexing).

## 2. Requirements Summary

`requirements.md` does not exist for this project. The original vision document is `OLD_SPECS/`, which `CLAUDE.md` declares non-authoritative. The authoritative inputs are:

- The `ReviewPrompt` file in the repository root (stakeholder vision narrative).
- `CLAUDE.md` (architecture summary, build/run commands, key game concepts).
- The existing codebase (declared authoritative).
- `architecturalreview.md` (the architect's review of the current implementation).

The requirements below were **derived by the architect** from those inputs. They use the prefix **DR-** (Design Requirement) to make clear they were not produced by a separate requirements analyst. Each is testable.

### Functional requirements

- **DR-1 (Strict separation).** The system shall maintain two strictly separated representations of game state at all times: a **Truth** representation (omniscient, used by the engine and by the alien bot) and a **SolView** representation (the player-visible state). Code in the `srv/internal/server` package shall not, at compile time, be able to reach Truth.

- **DR-2 (Single propagation primitive).** Every change to SolView shall be the result of applying a single `Event` whose `ArrivalYear` is less than or equal to the engine clock at the moment of application. There shall be no other path that mutates SolView. (This makes the model self-consistent: matured events fully describe what the player knows.)

- **DR-3 (Light-speed gating of all player-visible changes).** Every state change occurring at a system other than Sol — fleet departure, fleet arrival, combat outcome, construction completion, economic growth, status change — shall be visible to the player only after `EventYear + dist/c` (with comm laser present) or `EventYear + dist/0.8c` (with surviving reporter), whichever is earlier; or never, if neither reporting mechanism existed at the time of the event. Sol's own state (`sol`) is exempt; the player sees Sol's ground truth.

- **DR-4 (Snapshot fleets in SolView).** The player-visible representation of any fleet shall include a copy of that fleet's unit composition as of `AsOfYear`. Subsequent ground-truth changes to the fleet shall not retroactively alter the player's view of it.

- **DR-5 (Mobile-unit construction is reportable).** Construction of mobile units (Reporter, Escort, Battleship, Comm Laser) at a remote system, when reportable, shall update the player's known fleet composition for that system. (Fixes architectural review defect #3.)

- **DR-6 (Wealth changes are reportable).** Wealth deductions caused by construction, when reportable, shall update the player's known wealth for the system. Continuous wealth accumulation between reports may continue to be projected on the client side from `(KnownWealth, KnownEconLevel, KnownAsOfYear)`. (Fixes architectural review defect #4.)

- **DR-7 (Bot omniscience preserved).** The alien bot shall continue to read Truth directly. No light-speed gate is imposed on alien decision-making. (DR-1 only protects the player view.)

- **DR-8 (API contract preserved).** The HTTP/SSE API documented in `server_api.md` shall continue to function. Field names and shapes in `/api/stars`, `/api/state`, `/api/events`, `/api/command`, `/api/pause` shall be preserved. New optional fields may be added (e.g. `unitsAsOfYear` on a fleet DTO); no field shall be removed or have its type changed.

- **DR-9 (Test compatibility).** Existing Go tests in `srv/internal/game/game_test.go` shall continue to pass after the refactor. New tests shall enforce the separation (DR-1) and the propagation invariant (DR-2).

### Non-functional requirements

- **DR-10 (Engine performance).** The engine tick (currently every 100 ms) shall not become asymptotically slower in the number of recorded events. Today's `O(N_events)` per-tick scans (in `UpdateKnownStates` and `BroadcastMatured`) shall be replaced with an indexed structure; concretely, propagation work per tick shall be `O(matured-this-tick)` plus `O(log N)` per event for an insertion into a heap.

- **DR-11 (Determinism seam).** The game engine shall accept an injected `*rand.Rand` so that tests can drive deterministic combat outcomes. The default behavior (time-seeded RNG) is unchanged.

### Out of scope (explicit non-goals)

- This refactor does not introduce new gameplay (no new weapon types, no new commands, no rule changes).
- This refactor does not change the JavaScript SPA's rendering logic. Client-side code may need trivial adjustments where DTO field names change, but no architectural change is required on the client.
- Reporter survival coupling (review item E), bot light-speed pipeline (review item #8), explicit propagation channels (review item D), and SSE batching (review item H) are **deferred to a later phase** described in §6.7. The developer should not implement these unless explicitly directed.

## 3. Requirements Issues

Because the architect derived these requirements rather than receiving them from a separate analyst, there are no inconsistencies between the architect's understanding and the document; ambiguities found in the source vision (`ReviewPrompt`) are listed here for the user's awareness.

- **Ambiguity (resolved by assumption).** `ReviewPrompt` describes a scenario where a fleet-assembly command is issued to S1 and a move command from S1 to S3, and S1 is later discovered to have been captured. It is ambiguous whether the player's *visible* arrows for the in-flight orders should disappear at the (then-imagined) execution time, or remain visible until corrected by news from S1. **Assumption:** order-arrows disappear at the *expected* execution time computed from issue, with corrections (`EventCommandFailed`) appearing later when the news arrives at Sol. The current implementation already does this; no change is proposed.

- **Ambiguity (resolved by assumption).** It is unclear whether construction of a Comm Laser at a remote system should be retroactively visible (i.e., does the construction event itself, which preceded the laser's installation, get to use the new laser to report? or only future events?). **Assumption:** the construction-completion event itself rides home at light speed if the laser is operational at the moment of completion. The current implementation already does this in `ExecuteConstruct`; no change is proposed.

- **Gap (deferred).** No requirement covers what happens if a reporter fleet is destroyed in transit. Currently impossible (reporters don't engage). Deferred per non-goal above.

- **Untestable as stated (resolved).** The `ReviewPrompt` requirement that "two completely distinct game states" be maintained is not directly testable as written. It is operationalized here as DR-1 (compile-time separation) plus DR-2 (single propagation path), both of which are testable.

## 4. Constraints and Assumptions

### Constraints

- **Language / platform:** Go 1.x (server) and JavaScript ES modules + Three.js (client), per `CLAUDE.md`. No new languages introduced.
- **Build:** `go build -o spacegame srv/cmd/spacegame/main.go` must continue to work. Frontend continues to be built with Vite into `web/dist/`, embedded via `web/embed.go`.
- **API contract:** Existing endpoints under `/api/*` retain their URLs and JSON shapes (DR-8).
- **Single-process, single-host:** the game still runs as one Go process listening on `127.0.0.1:8080`. No external services.
- **Concurrency model:** the engine remains the sole writer; HTTP handlers continue to use `state.RLock()`. The single-mutex model is preserved.
- **CSV inputs:** `nearest.csv` and `planets.csv` must continue to load via the loader.

### Assumptions

- The existing `web/src/state.js` consumes `system_update`, `game_event`, `clock_sync`, `fleet_departed`, `connected`, and `game_over` SSE events. The refactor must continue to send all of these. (Confirmed by reading `web/src/state.js`.)
- `Sol`'s display always shows ground truth (no light-speed delay to itself). This is preserved.
- The set of `EventType` values does not need to grow as part of this refactor. Existing event types are sufficient once the propagation pipeline is unified. A new `EventFleetDeparted` *is* added (see §6.4).
- The number of stars and events in a typical game is small enough that any in-memory data structure (slice, map, heap) is acceptable. We are not designing for >10⁶ events.
- Per CLAUDE.md, after frontend changes, `scripts/build-frontend.sh` is run and `web/dist/` is committed alongside source. This refactor will require minor frontend adjustments only if any DTO field name changes (it should not).

## 5. Architecture

### 5.1 High-level structure

The refactor introduces three new types and reorganizes one existing type. Components and their relationships:

```
+-----------------------------+
|   srv/internal/server       |  HTTP / SSE handlers
|   (handlers.go, server.go)  |
+--------------+--------------+
               |  reads only:
               v
       +-------+-------+
       |   *SolView    |   <--- player-visible state
       +-------+-------+
               ^
               | applied by Propagator
               |
       +-------+-------+        +-------------+
       |   *EventLog   |<-------|   *Truth    |   <--- ground truth
       +-------+-------+        +------+------+
               ^                       ^
               |                       |
               +---- engine.Tick() ----+
                           ^
                           |
                  +--------+---------+
                  |   BotAgent       |   reads Truth
                  +------------------+
```

### 5.2 Data flow

A single tick of the engine:

1. **Advance clock.** `state.Clock += YearsPerTick`.
2. **Process command arrivals.** For each `PendingCommand` with `ExecuteYear ≤ Clock`: validate against `Truth`; on success, mutate `Truth` and append zero or more `Event`s to `EventLog`; on failure, append an `EventCommandFailed`.
3. **Process fleet arrivals at destinations.** Mutate `Truth.Fleets` and `Truth.Systems[dest]`; record `EventFleetArrival` (and possibly `EventSystemConquered`) into `EventLog`.
4. **Resolve combat in any system with both factions.** Mutate `Truth`; record `EventCombatOccurred` (or `EventCombatSilent`) into `EventLog`. Reporter fleets continue to be spawned as today.
5. **Accumulate wealth, advance econ levels.** Mutate `Truth`; record `EventEconGrowth` into `EventLog`.
6. **Spawn alien forces if due.** Mutate `Truth`; record internal `EventAlienSpawn`.
7. **Bot tick.** Bot reads `Truth`, returns `BotCommand`s; engine applies them directly to `Truth`.
8. **Propagate matured events.** `Propagator.Propagate(state)` pops every `Event` from `EventLog` whose `ArrivalYear ≤ Clock`, applies it to `SolView`, marks it `AppliedToView=true`, and (if not internal) emits an SSE frame to all clients.
9. **Periodic clock sync.** Every `ClockSyncCadence` ticks, broadcast a `clock_sync` SSE.
10. **Victory check.** On change of `GameOver`, broadcast `game_over` and pause.

The engine holds `state.mu.Lock()` for the entire tick. HTTP handlers take `state.mu.RLock()` and read only fields reachable from `SolView` and a small set of other public fields (`Clock`, `Paused`, `GameOver`, etc.).

### 5.3 What is new vs. modified vs. unchanged

- **New files:** `srv/internal/game/truth.go`, `srv/internal/game/solview.go`, `srv/internal/game/catalog.go`, `srv/internal/game/eventlog.go`, `srv/internal/game/propagator.go`.
- **Heavily modified:** `srv/internal/game/state.go` (struct shape), `srv/internal/game/engine.go` (uses new types), `srv/internal/game/loader.go` (constructs new types), `srv/internal/game/economy.go`, `srv/internal/game/combat.go` (mutate Truth, record events), `srv/internal/game/events.go` (becomes mostly SSE plumbing; propagation logic moves to `propagator.go`), `srv/internal/game/types.go` (cleanup).
- **Lightly modified:** `srv/internal/server/handlers.go` (reads from `SolView` instead of `StarSystem.Known*`), `srv/internal/server/types.go` (DTOs gain `unitsAsOfYear` snapshot field on fleets).
- **Unchanged:** `srv/cmd/spacegame/main.go` (other than passing an `*rand.Rand` for DR-11), CSV files, web frontend (other than possibly a one-line adjustment in `web/src/state.js` if a DTO field is renamed; ideally none).

## 6. Detailed Design

### 6.1 `StarCatalog` (new — `srv/internal/game/catalog.go`)

- **Name:** `StarCatalog`, `CatalogEntry`.
- **Purpose:** Hold static, never-changing star data (positions, distances, names, planet flag) separately from anything that can vary. Both `Truth` and `SolView` reference the catalog by ID; star geometry is the same in both views.
- **Satisfies:** DR-1 (separation by removing geometry from the mutable state types).
- **Interface:**

```go
type StarCatalog struct {
    Order   []string                  // ID order: "sol" first, then by distance
    Entries map[string]*CatalogEntry
}

type CatalogEntry struct {
    ID          string
    DisplayName string
    X, Y, Z     float64
    DistFromSol float64
    HasPlanets  bool
    IsSol       bool
}

func (c *StarCatalog) Get(id string) *CatalogEntry  // nil if missing
func (c *StarCatalog) MaxDist() float64             // computed once at load
```

- **Behavior:** Built once during `Initialize` from `nearest.csv` + `planets.csv`. Immutable thereafter; no mutex needed because no field is ever written after construction.
- **Error handling:** `Get` returns nil for unknown IDs. Callers panic-or-warn at their discretion. Loader errors propagate as today.
- **Dependencies:** none beyond standard library.

### 6.2 `Truth` (new — `srv/internal/game/truth.go`)

- **Name:** `Truth`, `TrueSystem`, `TrueFleet`.
- **Purpose:** Represent the omniscient world state. Engine mutates this; bot reads this; HTTP handlers must not see it.
- **Satisfies:** DR-1, DR-7.
- **Interface (read-only methods):**

```go
type Truth struct {
    Systems map[string]*TrueSystem
    Fleets  map[string]*TrueFleet
}

type TrueSystem struct {
    ID             string
    Status         SystemStatus
    EconLevel      int
    Wealth         float64
    EconGrowthYear float64
    LocalUnits     map[WeaponType]int
    FleetIDs       []string
    PrimaryFleetID string
    FleetCount     int
}

type TrueFleet struct {
    ID, Name           string
    Owner              Owner
    Units              map[WeaponType]int
    LocationID         string
    SourceID, DestID   string
    DepartYear, ArrivalYear float64
    InTransit          bool
}

// Read-only convenience methods (do not mutate).
func (t *Truth) System(id string) *TrueSystem
func (t *Truth) Fleet(id string)  *TrueFleet
```

- **Behavior:** Pure data containers. All mutation happens in package-internal functions (engine, combat, economy). The bot's read-only contract is enforced by code review (no field is unexported, since bot needs to read most of them).
- **Error handling:** `System` and `Fleet` return nil on miss; callers handle.
- **Dependencies:** `WeaponType`, `Owner`, `SystemStatus` from `types.go`.

### 6.3 `SolView` (new — `srv/internal/game/solview.go`)

- **Name:** `SolView`, `KnownSystem`, `KnownFleet`, `KnownTransit`.
- **Purpose:** Represent everything Sol knows. Mutated only by `Propagator`. Read by HTTP/SSE handlers.
- **Satisfies:** DR-1, DR-2, DR-4.
- **Interface:**

```go
type SolView struct {
    Systems   map[string]*KnownSystem
    Fleets    map[string]*KnownFleet      // SNAPSHOTS — no aliasing into Truth
    InTransit map[string]*KnownTransit    // human transits known to Sol
}

type KnownSystem struct {
    ID         string
    Status     SystemStatus
    AsOfYear   float64               // year of latest news from this system
    EconLevel  int
    Wealth     float64               // last reported wealth (not extrapolated server-side)
    LocalUnits map[WeaponType]int
    FleetIDs   []string              // IDs into SolView.Fleets, NOT Truth.Fleets
}

type KnownFleet struct {
    ID, Name   string
    Owner      Owner
    Units      map[WeaponType]int    // SNAPSHOT at AsOfYear
    LocationID string                // "" if in transit (then look in InTransit)
    AsOfYear   float64
}

type KnownTransit struct {
    FleetID            string
    Name               string
    Owner              Owner
    Units              map[WeaponType]int  // SNAPSHOT at departure-as-known
    SourceID, DestID   string
    DepartYear, ArrivalYear float64
    AsOfYear           float64
}

func (v *SolView) System(id string) *KnownSystem
func (v *SolView) Fleet(id string)  *KnownFleet
```

- **Behavior:** Constructed at load time as a copy of the initial Truth (per `loader.go` today, which seeds `Known*` from ground truth at year 0 — preserved). Subsequently, only `Propagator.applyEventToView` writes to it.

  **Sol exception:** `SolView.Systems["sol"]` is a special case. The propagator updates it from internal events as today; the HTTP layer additionally **synthesizes Sol's wealth and units at request time** by reading `Truth.Systems["sol"]`. This is the *only* place where the server package is permitted to peek at Truth. It is encapsulated in a single helper `(state *GameState) ReadSolGroundTruth() SolGroundTruthSnapshot`. (See §6.6.) This preserves DR-1 in spirit: handlers cannot reach Truth other than this single explicit affordance.

- **Error handling:** Get methods return nil on miss.
- **Dependencies:** `Truth` (for the Sol exception), `WeaponType`, `Owner`, `SystemStatus`.

### 6.4 `EventLog` and `Event` (new + modified — `srv/internal/game/eventlog.go`, `events.go`)

- **Name:** `EventLog`, `Event`, `MaturedHeap`.
- **Purpose:** Single propagation primitive. Indexed for efficient maturation.
- **Satisfies:** DR-2, DR-10.
- **Interface:**

```go
type Event struct {
    ID          string
    EventYear   float64
    ArrivalYear float64           // math.MaxFloat64 if never reportable
    SystemID    string
    Type        EventType
    Description string
    Details     interface{}
    Internal    bool              // true: never broadcast, never applied to view
    Broadcast   bool              // true once SSE-sent
    AppliedToView bool            // true once applied to SolView
}

type EventLog struct {
    All      []*Event              // chronological by record time
    BySystem map[string][]*Event   // for future "events at this system" queries
    pending  *eventHeap            // min-heap by ArrivalYear, only unmatured + non-internal
}

func NewEventLog() *EventLog
func (l *EventLog) Record(e *Event)            // assigns ID; pushes onto pending if reportable
func (l *EventLog) PopMatured(clock float64) []*Event  // removes and returns events with ArrivalYear ≤ clock
```

- **Behavior:**
  - `Record`: appends to `All`; appends to `BySystem[e.SystemID]`; if `!e.Internal && e.ArrivalYear < math.MaxFloat64`, pushes onto the heap. Internal events (e.g. `EventAlienSpawn`, `EventCombatSilent`) never enter the heap and never broadcast/apply.
  - `PopMatured`: pops from the heap while the top's `ArrivalYear ≤ clock`. Returns the popped events in heap-pop order (which is non-decreasing `ArrivalYear`). Each popped event is the caller's responsibility to mark `AppliedToView=true` and `Broadcast=true`.
- **Error handling:** No errors raised; missing maps are lazily allocated on first `Record`.
- **Dependencies:** `container/heap` (Go stdlib).

`Event` itself is a renaming/cleanup of today's `GameEvent`. The field `CanReport` is removed (it was redundant with `ArrivalYear < MaxFloat64`). A new event type is added:

```go
const EventFleetDeparted EventType = "fleet_departed"
```

This replaces the out-of-band `BroadcastFleetDeparted` path and is the principal mechanism by which DR-3 is enforced for departures.

### 6.5 `Propagator` (new — `srv/internal/game/propagator.go`)

- **Name:** `Propagator`. (One concrete type; no interface needed.)
- **Purpose:** The single bridge between `EventLog` and `SolView` + the SSE broadcast layer. This is the only writer of `SolView`.
- **Satisfies:** DR-2, DR-3, DR-4, DR-5, DR-6.
- **Interface:**

```go
type Propagator struct {
    Events *EventManager   // SSE broadcast plumbing
}

func NewPropagator(em *EventManager) *Propagator

// Propagate pops all matured events, applies each to SolView, broadcasts each.
// Caller must hold state.mu (write lock).
func (p *Propagator) Propagate(state *GameState)
```

- **Behavior:**

```
for evt := range state.Events.PopMatured(state.Clock):
    p.applyEventToView(state.SolView, state.Catalog, evt)
    evt.AppliedToView = true
    if !evt.Internal:
        p.Events.broadcastEvent(evt)
        p.Events.broadcastSystemUpdate(state, evt.SystemID)
        evt.Broadcast = true
```

- **`applyEventToView` cases (the heart of the refactor):**

  - **`EventCombatOccurred`** with `*CombatDetails`: update `KnownSystem.Status` per outcome flags. Decrement `KnownSystem.LocalUnits` by the non-mobile entries of `HumanLosses`. For each `KnownFleet` at this system, decrement its `Units` by the mobile entries of `HumanLosses` proportionally; if `Units` reaches zero, remove the fleet from `SolView.Fleets` and from the system's `FleetIDs`. Set `KnownSystem.AsOfYear = max(AsOfYear, EventYear)`. (Today's code zeroes losses on local units only and ignores fleet-side losses; this captures both by mirroring what the truth-side combat resolver did.)

  - **`EventSystemCaptured`** / **`EventSystemRetaken`** / **`EventSystemConquered`**: set `KnownSystem.Status` and (for conquered) reset `KnownEconLevel=0`, `KnownWealth=0`. Bump `AsOfYear`.

  - **`EventConstructionDone`** with `*ConstructionDetails`: subtract `def.Cost * Quantity` from `KnownSystem.Wealth` (DR-6). If `def.CanMove` is **false**, add `Quantity` to `KnownSystem.LocalUnits[wt]`. If `def.CanMove` is **true** (DR-5), apply to a `KnownFleet`:

    - If the system's `KnownSystem.PrimaryFleetID` exists in `SolView.Fleets`, add to it; bump that fleet's `AsOfYear`.
    - Otherwise, create a new `KnownFleet` with snapshot units `{wt: Quantity}`, owner `HumanOwner`, location = system ID, name = `<DisplayName>-1st Fleet`, `AsOfYear = EventYear`. Append the new fleet ID to `KnownSystem.FleetIDs`; set `KnownSystem.PrimaryFleetID`.

    The `ConstructionDetails` struct gains a field `PrimaryFleetID string` (the truth-side primary at the moment of construction) so the propagator can mirror the truth-side decision rather than recomputing it.

  - **`EventFleetArrival`** with `*FleetArrivalDetails`: take a snapshot. Insert/update `SolView.Fleets[FleetID]` with `Units = copy(Details.Units)`, `Name = Details.FleetName`, `LocationID = SystemID`, `AsOfYear = EventYear`. Append to `KnownSystem.FleetIDs` (de-duplicated). Remove from `SolView.InTransit[FleetID]` if present.

  - **`EventFleetDeparted`** (new) with `*FleetDepartureDetails` (new payload, mirrors `KnownTransit`): create a `KnownTransit`, snapshot units, remove the fleet from `KnownSystem.FleetIDs` of the source system. Insert into `SolView.InTransit`. Remove the `KnownFleet` from `SolView.Fleets` (it's now in transit).

  - **`EventEconGrowth`** with `*EconGrowthDetails`: `KnownSystem.EconLevel = NewLevel`. Bump `AsOfYear`.

  - **`EventCommandArrived`** / **`EventCommandExecuted`** / **`EventCommandFailed`**: no `SolView` mutation beyond `AsOfYear`. Broadcast normally so the client can prune its `pendingCommands` and display failure reasons.

  - **`EventReporterReturn`**: no `SolView` mutation; this event is decorative for the player. Broadcast normally.

  - **`EventAlienExhausted`**: a global event; `SystemID` is "sol". Broadcast normally; the SolView is unaffected (the engine itself flips `state.Alien.Exhausted` and that flag is read directly by `CheckVictory`).

  - **`EventAlienSpawn`**, **`EventCombatSilent`**: `Internal=true`; never reach the propagator. (`EventLog.Record` keeps them out of the heap.)

- **Error handling:** Each case is defensive: if the referenced `KnownSystem` does not exist (should not happen for existing IDs), a single `log.Printf` warns and the event is skipped. The propagator never panics; it must keep the engine alive.

- **Dependencies:** `EventLog`, `SolView`, `EventManager`, `WeaponDefs`.

### 6.6 `GameState` and engine refactor (`state.go`, `engine.go`)

- **`GameState`** becomes:

```go
type GameState struct {
    mu sync.RWMutex

    Clock     float64
    Paused    bool
    GameOver  bool
    Winner    Owner
    WinReason string

    Catalog *StarCatalog
    truth   *Truth      // unexported; server package cannot see it
    SolView *SolView    // exported; server package reads via state.SolView

    Events  *EventLog

    Human       HumanFaction
    Alien       AlienFaction
    PendingCmds []*PendingCommand

    rng *rand.Rand      // injected (DR-11)

    nextFleetNum int
    nextEventID  int
    nextCmdID    int
}

// Engine-only accessor; usable only from inside package game.
func (s *GameState) Truth() *Truth { return s.truth }

// Server-package use (DR-1 escape hatch for Sol's own state).
func (s *GameState) ReadSolGroundTruth() SolGroundTruthSnapshot {
    sol := s.truth.Systems["sol"]
    return SolGroundTruthSnapshot{
        Status:     sol.Status,
        EconLevel:  sol.EconLevel,
        Wealth:     sol.Wealth,
        LocalUnits: copyUnits(sol.LocalUnits),
        FleetIDs:   append([]string(nil), sol.FleetIDs...),
    }
}
```

The lowercase `truth` field is the compile-time barrier required by DR-1: code outside package `game` cannot dereference it. The exported `Truth()` method exists for engine-internal helpers that live in package `game` but reach the engine through interfaces. (This pattern is already used for `Lock`/`Unlock` helpers today.)

- **`Engine.tick`** is restructured to follow the data-flow ordering in §5.2. Notably:
  - The synchronous `BroadcastFleetDeparted` call is **deleted**. Move execution now records an `EventFleetDeparted` with the proper `ArrivalYear` (DR-3, fixes review defect #2).
  - `e.State.UpdateKnownStates(e.State.Clock)` is **deleted**. Replaced by `e.Propagator.Propagate(e.State)`.
  - The truth-side fleet object's mutation order is: clear from source `FleetIDs`, set `InTransit=true`, set `DepartYear`, etc. — same as today, but the SSE side-effect is removed.

### 6.7 Phase-2 work (deferred, sketch only)

The following are explicitly **not** part of this design's scope but are listed so the developer recognizes them as follow-ons (and does not pre-emptively refactor in a direction that conflicts):

- **Channels (review item D):** introduce a `Channel` enum so the propagator can model multiple competing reports per event.
- **Reporter survival coupling (review item E):** condition `Event.ArrivalYear` on the survival of an associated reporter fleet.
- **Bot light-speed pipeline (review item #8):** route `BotCommand`s through `PendingCmds`.
- **SSE batching and recovery (review item H):** tick-coalesced frames; client recovery on buffer overflow.
- **Event-log compaction (review item F second half):** drop or cold-tier old events.

The data structures in this design accommodate all five without further restructuring.

## 7. Data Model

### 7.1 In-memory data (post-refactor)

Top-level: `GameState` (see §6.6).

```
GameState
├── Catalog : *StarCatalog
│   └── Entries: map[id]*CatalogEntry  (immutable post-load)
├── truth : *Truth
│   ├── Systems: map[id]*TrueSystem
│   └── Fleets:  map[id]*TrueFleet
├── SolView : *SolView
│   ├── Systems:   map[id]*KnownSystem   (FleetIDs reference SolView.Fleets only)
│   ├── Fleets:    map[id]*KnownFleet    (snapshots; AsOfYear set on each write)
│   └── InTransit: map[fleetId]*KnownTransit
├── Events : *EventLog
│   ├── All:      []*Event
│   ├── BySystem: map[id][]*Event
│   └── pending:  min-heap of *Event by ArrivalYear (excludes Internal events)
├── PendingCmds : []*PendingCommand
└── Human, Alien, Clock, Paused, GameOver, Winner, WinReason, rng
```

### 7.2 DTO changes (`server_api.md` impact)

- **`/api/state`** continues to return `SystemDTO` and `FleetDTO` arrays. `FleetDTO` gains an optional `unitsAsOfYear: number` field carrying `KnownFleet.AsOfYear` (or, for an in-transit fleet, the transit's `AsOfYear`). For Sol's own fleets, the field is set to `state.Clock`. Clients that ignore it continue to work; the SPA in `web/src/state.js` and `web/src/sidebar.js` may surface it later but is not required to.
- **SSE `system_update`** continues to carry the same fields it carries today, sourced from `KnownSystem` (and from `SolGroundTruthSnapshot` for `sol`).
- **SSE `fleet_departed`** continues to be sent, but now via the propagator at the moment the departure event matures at Sol — not at the moment of physical departure at the source system.

### 7.3 Persisted data

None. There is no database; CSV inputs are unchanged.

### 7.4 Migration strategy

This is an in-memory refactor. There is no data migration. On version bump, in-flight games are lost (acceptable; today's server has no persistence either).

## 8. Key Design Decisions

| Decision | Alternatives Considered | Choice | Rationale |
|---|---|---|---|
| Where does the player-visible state live? | (a) Keep `Known*` fields on `StarSystem`. (b) Two structs in the same package. (c) Two structs in two packages. | (b) Two structs (`Truth`, `SolView`) in package `game`; `truth` field unexported in `GameState`. | (a) is the status quo and has demonstrated leak surface. (c) imposes import cycles since the engine writes both. (b) gives compile-time separation at the package boundary that matters (server package) without restructuring the engine. |
| How to bridge Truth → SolView? | (a) Direct mutation by every engine subsystem. (b) Single `Event` propagation through one apply function. | (b) Single propagator. | (a) is what we have, with leaks. (b) makes the rule "everything visible flows through Event" structurally enforceable and unit-testable. |
| How to handle Sol's special case (player sees Sol ground truth)? | (a) Mirror Sol's truth into SolView every tick. (b) HTTP handler reads truth via a single explicit accessor. | (b) `ReadSolGroundTruth()`. | (a) duplicates writes and is easy to forget. (b) is one easily-grepped escape hatch with a clear name. |
| How to store fleet snapshots in SolView? | (a) Embed snapshot units inside `KnownSystem.Fleets`. (b) Separate `SolView.Fleets` map indexed by fleet ID, referenced from systems by ID. | (b). | (b) matches the existing client model (`web/src/state.js` keeps `inTransitFleets` separately from systems) and makes `KnownTransit` a natural sibling. |
| Event maturation index? | (a) Linear scan with cursor. (b) Min-heap by ArrivalYear. (c) Separate matured / unmatured slices. | (b) Min-heap. | (a) is fragile because events with later EventYear can have earlier ArrivalYear (closer system + comm laser vs. distant system). (b) is `O(log n)` per insert/extract and uses Go's stdlib `container/heap`. (c) needs partition cost on every tick. |
| Fleet-departure visibility? | (a) Keep `BroadcastFleetDeparted` synchronous (status quo, leak). (b) New `EventFleetDeparted` event type, propagated normally. | (b). | (a) violates DR-3. (b) reuses the propagation primitive and is symmetric with `EventFleetArrival`. |
| RNG injection? | (a) Keep `time.Now().UnixNano()` seeding internally. (b) Inject `*rand.Rand` from `main.go`, default time-seeded. | (b). | (b) preserves current default behavior (time-seeded) but allows tests to seed deterministically (DR-11, fixes review defect #12). One-line change to `main.go`. |
| Backward compatibility for client? | (a) Allow DTO field renames in this refactor. (b) Strict additive-only. | (b) Additive-only. | (a) drags in a frontend change. (b) keeps the refactor server-only. |
| Where should `applyEventToView` live? | (a) Method on `SolView`. (b) Method on `Propagator`. | (b). | (a) mixes mutation with the data type. (b) keeps `SolView` a pure data container; the `Propagator` is the only writer, which makes DR-2 grep-able (`grep -r 'state\.SolView\.' srv/internal/game/` should return only propagator code plus loader). |
| Snapshot copy semantics for `Units`? | (a) Share map by reference. (b) Deep copy on each write. | (b). | (a) reintroduces the aliasing leak we are fixing. (b) costs a few microseconds per event; trivial. Use the existing `copyUnits` helper. |

## 9. File and Directory Plan

All paths are relative to repo root.

- `srv/internal/game/catalog.go` — **CREATE** — `StarCatalog`, `CatalogEntry`, accessors. ~80 LOC.
- `srv/internal/game/truth.go` — **CREATE** — `Truth`, `TrueSystem`, `TrueFleet`. ~120 LOC.
- `srv/internal/game/solview.go` — **CREATE** — `SolView`, `KnownSystem`, `KnownFleet`, `KnownTransit`, `SolGroundTruthSnapshot`. ~150 LOC.
- `srv/internal/game/eventlog.go` — **CREATE** — `Event` (renamed from `GameEvent`), `EventLog`, internal `eventHeap` implementing `heap.Interface`. ~200 LOC.
- `srv/internal/game/propagator.go` — **CREATE** — `Propagator`, `applyEventToView` switch over `EventType`. ~300 LOC.

- `srv/internal/game/state.go` — **MODIFY** — `GameState` now embeds `*Truth`, `*SolView`, `*StarCatalog`, `*EventLog`. Remove `Systems`, `SystemOrder`, `Fleets`, `Events`. Remove `UpdateKnownStates`, `applyEventToKnownState`. Add `Truth()`, `ReadSolGroundTruth()`. Keep ID generators, `RecordEvent`, `ApplyCommand`, `CheckVictory`. The `ApplyCommand` body is rewritten to mutate `state.truth` and to record `Event`s (no more direct mutation of `Known*` fields).
- `srv/internal/game/engine.go` — **MODIFY** — Tick uses `state.truth` for mutation, `state.Events` for recording, `state.Propagator` for propagation. Delete `BroadcastFleetDeparted` calls; replace with `Event` recording. Engine constructor takes a `*rand.Rand`.
- `srv/internal/game/loader.go` — **MODIFY** — Build `*StarCatalog` first, then `*Truth`, then seed `*SolView` from initial Truth (preserving G-4 / today's behavior). Initial alien presence at entry-point systems is recorded in `Truth` only; `SolView` reflects the player's pre-game view (no alien knowledge). Initial fleets at human systems are mirrored into `SolView.Fleets` as snapshots. Take an optional `*rand.Rand`.
- `srv/internal/game/economy.go` — **MODIFY** — `AccumulateWealth`, `AdvanceEconLevels`, `ApplyEconomicCombatPenalty`, `ValidateConstruct`, `ExecuteConstruct`, `ExecuteCreateFleet`, `ExecuteReassign` operate on `*Truth` (passed in) and record `*Event`s (passed-in `*EventLog`). The `ConstructionDetails` struct gains `PrimaryFleetID string`.
- `srv/internal/game/combat.go` — **MODIFY** — `Resolve`, helpers operate on `*Truth`. The reporter-spawn behavior is preserved as today (deferred; not part of this refactor).
- `srv/internal/game/events.go` — **MODIFY** — Becomes purely SSE plumbing. `EventManager` keeps client registry, sse-frame formatting, `BroadcastConnected`. Remove `BroadcastMatured` (replaced by `Propagator.Propagate`). Remove `BroadcastFleetDeparted`. The `eventToMap`, `fleetToMap`, `systemToMap`, `fullStateMap` serializers are rewritten to read from `*SolView` (and from `ReadSolGroundTruth()` for `sol`).
- `srv/internal/game/types.go` — **MODIFY** — Add `EventFleetDeparted` constant. Add `FleetDepartureDetails` struct. Add `PrimaryFleetID` to `ConstructionDetails`. Move static catalogue-y types to `catalog.go`.
- `srv/internal/game/bot.go` — **MODIFY** — `DefaultBot.Tick(state, year)` reads `state.Truth()` instead of bare `state.Systems`/`state.Fleets`. Logic unchanged.

- `srv/internal/server/handlers.go` — **MODIFY** — `handleStars` reads `state.Catalog`. `handleState` and `buildSystemDTO` read `state.SolView` plus `state.ReadSolGroundTruth()` for sol. No reference to `state.Truth()` from the server package.
- `srv/internal/server/types.go` — **MODIFY** — Add `UnitsAsOfYear *float64` to `FleetDTO` (omitempty).

- `srv/internal/game/game_test.go` — **MODIFY** — Update existing tests to construct the new types via the loader (or via a new test-helper `NewTestState(t)`). Add new tests per §11.

- `srv/cmd/spacegame/main.go` — **MODIFY** — Construct `rng := rand.New(rand.NewSource(time.Now().UnixNano()))`; pass to `Initialize` and `NewEngine`. Optional `--seed` flag.

- `web/src/state.js` — **MODIFY (defensive)** — If `unitsAsOfYear` is present on a fleet DTO, retain it (for future UI use). No display change required.

- `server_api.md` — **MODIFY** — Document the optional `unitsAsOfYear` field on fleet DTOs and the unchanged shape of every other endpoint.

## 10. Requirement Traceability

| Requirement | Design Section | Files |
|---|---|---|
| DR-1 (Strict separation) | §5.1, §6.2, §6.3, §6.6 | `truth.go`, `solview.go`, `state.go`, `handlers.go` |
| DR-2 (Single propagation primitive) | §5.2 step 8, §6.4, §6.5 | `eventlog.go`, `propagator.go`, `engine.go` |
| DR-3 (Light-speed gating of all changes) | §6.4 (new `EventFleetDeparted`), §6.5 (apply step), §6.6 (engine restructure) | `eventlog.go`, `propagator.go`, `engine.go`, `combat.go`, `economy.go` |
| DR-4 (Snapshot fleets) | §6.3, §6.5 (`EventFleetArrival`/`EventFleetDeparted` cases) | `solview.go`, `propagator.go` |
| DR-5 (Mobile-unit construction reportable) | §6.5 (`EventConstructionDone` mobile case) | `propagator.go`, `economy.go`, `types.go` |
| DR-6 (Wealth deductions reportable) | §6.5 (`EventConstructionDone` wealth update) | `propagator.go` |
| DR-7 (Bot omniscience) | §5.1, §6.2, §6.6 (`Truth()` accessor) | `bot.go`, `state.go` |
| DR-8 (API contract preserved) | §7.2 | `handlers.go`, `server/types.go`, `server_api.md` |
| DR-9 (Test compatibility) | §11 | `game_test.go` |
| DR-10 (Engine perf: indexed events) | §6.4 | `eventlog.go` |
| DR-11 (Determinism seam) | §6.6, §9 (main.go) | `engine.go`, `loader.go`, `main.go` |

## 11. Testing Strategy

### 11.1 Unit tests

- **`solview_test.go`** — round-trip a single `EventFleetArrival` through the propagator and assert the resulting `KnownFleet` has the snapshot units, correct `AsOfYear`, and is referenced by `KnownSystem.FleetIDs`. Mutate the source `TrueFleet` afterward and assert the snapshot does not change (DR-4).
- **`propagator_test.go`** — one test per `EventType` case in `applyEventToView`. Each test seeds a tiny `Truth` + initial `SolView`, records one event with deterministic `ArrivalYear`, runs `Propagate`, and asserts the expected `SolView` mutation and SSE side-effect. Cases must include:
  - `EventConstructionDone` for a non-mobile weapon: `LocalUnits` updated, `Wealth` decremented (DR-6).
  - `EventConstructionDone` for a mobile weapon at a system whose primary fleet exists: primary fleet's `Units` increased, `AsOfYear` advanced, `Wealth` decremented (DR-5, DR-6).
  - `EventConstructionDone` for a mobile weapon when no primary fleet exists: a new `KnownFleet` is created and listed in `KnownSystem.FleetIDs`.
  - `EventFleetDeparted`: source system's `KnownSystem.FleetIDs` no longer lists the fleet; `SolView.InTransit` lists it; `SolView.Fleets` no longer contains it.
  - `EventCombatOccurred` with the human side losing all mobile units in a fleet: that fleet is removed from `SolView.Fleets` and from the system's `FleetIDs`.
- **`eventlog_test.go`** — `Record` an event with `Internal=true` and assert it does not enter the heap. `Record` events with non-monotonic `ArrivalYear` (close + comm laser, then far + comm laser) and assert `PopMatured` returns them in `ArrivalYear` order. Record an unreportable event (`ArrivalYear=MaxFloat64`) and assert it never matures.

### 11.2 Integration tests

- **Light-speed gating (DR-3).** Build a 2-system game (Sol + remote at 10 LY with comm laser). Issue a construct command at Sol. Run the engine until `Clock = 9`. Assert `SolView.Systems[remote]` shows no construction. Run until `Clock = 21`. Assert the construction is now reflected in `SolView`. (Command travels 10 yrs to arrive, executes, and the result travels 10 yrs back; total 20.)
- **No live aliasing (DR-4).** Same setup. After construction is reported, mutate `state.Truth().Fleets[remoteFleetID].Units` directly in test code; assert `SolView.Fleets[remoteFleetID].Units` is unchanged.
- **Departure gating (DR-3 fleet case).** Build a 2-system game with a stationed remote fleet. Issue a `CmdMove`. Run the engine to just before the order's arrival year; assert `SolView.InTransit` is empty. Run until just after; assert `SolView.InTransit` lists the fleet only after `EventFleetDeparted` matures at Sol (≈ another `dist/c` years if comm laser; never if no laser).
- **Property test (review item J).** Random-seeded short game (year 0 to year 50). After the run, replay every broadcast `Event` from a clean `SolView` initialized to the same starting state; assert the replay's `SolView` is byte-identical (via reflect.DeepEqual on a deterministic snapshot) to the engine's `SolView`. Failure indicates either a leak (state in `SolView` not derivable from broadcasts) or a gap (events not applied symmetrically).

### 11.3 Existing tests (DR-9)

`srv/internal/game/game_test.go` must compile and pass. Where today's tests reach `state.Systems[id]` or `state.Fleets[id]`, replace with `state.Truth().Systems[id]` / `state.Truth().Fleets[id]`. Where they reach `sys.KnownStatus`, replace with `state.SolView.Systems[id].Status`. Test semantics do not change.

### 11.4 Manual / smoke

After build, `./spacegame` and visit `http://localhost:8080`. Run a typical session for ~5 minutes. The starmap and sidebar should render the same way they do today; in particular, in-flight order arrows, fleet-arrival arrows, and reported combat should appear at the same wall-clock moments. (Manual smoke is not a substitute for the integration tests above; it is a sanity check that the SSE wiring still reaches the client.)

### 11.5 How to verify each requirement

- DR-1 — `grep -nR 'state\.truth' srv/internal/server/` returns nothing. `grep -nR 'state\.Truth()' srv/internal/server/` returns nothing.
- DR-2 — `grep -nR 'state\.SolView' srv/internal/game/` returns matches in `propagator.go` and `loader.go` only.
- DR-3 — Integration test "Light-speed gating" passes. Integration test "Departure gating" passes.
- DR-4 — Unit test `solview_test.go` passes; integration test "No live aliasing" passes.
- DR-5 — `propagator_test.go` mobile-construction cases pass.
- DR-6 — `propagator_test.go` wealth case passes.
- DR-7 — `bot_test.go` (existing) still passes; bot reads truth fleet positions correctly.
- DR-8 — `server_api.md` review; client SPA boots without console errors.
- DR-9 — `go test ./srv/...` is green.
- DR-10 — Microbenchmark: simulate a game to year 200 and measure tick latency. Compare to a baseline build; tick latency must not increase super-linearly in event count.
- DR-11 — A test seeds the engine deterministically and asserts a specific combat outcome; running it twice produces the same result.

## 12. Open Questions

None blocking. The design assumes:

1. The new optional `unitsAsOfYear` DTO field is acceptable to the user. (If not, it can be omitted; the snapshot semantics on the server are unaffected.)
2. The `ConstructionDetails.PrimaryFleetID` field added in §6.5 is acceptable. (It is needed so the propagator can mirror the truth-side primary-fleet decision when applying construction; without it, the player's view could create a different fleet than the truth.)
3. The decision to keep reporter behavior unchanged in this refactor (deferring review item E) is acceptable. Today reporters do not actually condition the report; that is preserved as a known limitation.

If any of these three is unacceptable, raise it before implementation begins.
