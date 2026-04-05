# Requirements: BasicBot — External Alien Bot for SpaceGame
Version: 0.2

---

## Change Log

| Version | Date | Summary |
|---------|------|---------|
| 0.1 | 2025 | Initial draft |
| 0.2 | 2026-04-05 | Add kzinhome alien home world (§2.5); add `GET /api/alien/state` endpoint (§2.6); change initial alien fleet composition (§2.7); defer alien economic simulation (§6) |

---

## Background

SpaceGame's game engine contains a built-in alien bot (`DefaultBot`, in
`srv/internal/game/bot.go`) that drives the alien side. `DefaultBot` has
direct in-process access to the full `GameState` struct with no information
delay — it knows the ground truth about every system and every fleet.

This document specifies an **external standalone program** (`BasicBot`) that
replaces `DefaultBot` as the alien-side driver during a game session. The key
design principle is that `BasicBot` operates under **the same information
constraints as the human player**: it uses only the public REST/SSE API that
the game server already exposes to browsers, supplemented by three new
alien-specific endpoints (two read, one write). It never receives ground-truth
game state about human-held systems, uninhabited systems, or unreported
combat events.

Unknown to the human player, the alien empire's home world is **kzinhome**
(61 Ursae Majoris, ~31.1 light years from Sol). All alien strategic decisions
originate from kzinhome. Just as the human player at Sol experiences
speed-of-light information delay from distant systems, alien intelligence must
travel from the battlefield back to kzinhome — a baseline distance of 31 light
years — before it can inform bot decisions. This extra distance is a deliberate
gameplay handicap: the alien bot will be working with older information about
the human sphere than the human player is, particularly in the critical region
near Sol where most combat will occur.

`BasicBot` is explicitly a **debugging tool**, not a competitive opponent. Its
goal is to make enough legal, coherent moves that the game mechanics — fleet
movement, combat, event propagation, the economic system, and victory
conditions — can be exercised and observed. It does not need to play well.

---

## 1. Scope

These requirements cover two things:

1. **Server extensions**: the minimal additions to the existing game server
   (`srv/`) required to support an external bot.
2. **BasicBot program**: the standalone Go program that lives in `BasicBot/`
   at the repository root.

Changes to the browser SPA (`web/`) are out of scope.

---

## 2. Server Extension Requirements

### 2.1 New Endpoint: `GET /api/alien/fleets`

**SRV-001** The server must expose a new read endpoint at `GET /api/alien/fleets`
that returns the current position and composition of every alien fleet known
to the game engine.

**SRV-002** The response must include all alien fleets regardless of whether their
position has been reported back to Sol. This is the alien empire's own
logistical knowledge — it always knows where it sent its ships.

**SRV-003** The response body must be a JSON array of fleet objects with the
following fields:

| Field | Type | Description |
|-------|------|-------------|
| `fleetId` | string | Stable fleet identifier (e.g., `"fleet-3"`) |
| `locationId` | string | System ID of the fleet's current system; empty string if in transit |
| `inTransit` | bool | True if the fleet is currently between systems |
| `destId` | string | Destination system ID if `inTransit` is true; empty string otherwise |
| `arrivalYear` | float64 | In-game year of arrival at `destId`; 0 if not in transit |
| `units` | object | Map of weapon-type string to unit count, e.g., `{"escort": 5, "battleship": 2}` |

**SRV-004** The endpoint must acquire the state read lock (`state.mu.RLock`) for
the duration of the response construction, consistent with all other read
endpoints.

**SRV-005** If there are no alien fleets, the endpoint must return an empty JSON
array (`[]`), not `null` and not a 404.

---

### 2.2 New Endpoint: `POST /api/alien/move`

**SRV-006** The server must expose a new write endpoint at `POST /api/alien/move`
that accepts an alien fleet movement order and executes it immediately with no
travel delay, mirroring the behavior of an internal `BotCommand` of type
`CmdMove`.

**SRV-007** The request body must be a JSON object with the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fleetId` | string | ✓ | The alien fleet to move |
| `destId` | string | ✓ | Destination system ID |

**SRV-008** On success, the server must respond HTTP 200 with:
```json
{"ok": true}
```

**SRV-009** The endpoint must validate the request and return HTTP 400 with a
JSON error body (`{"ok": false, "error": "<reason>"}`) for any of the
following conditions:
- `fleetId` is absent or empty
- `destId` is absent or empty
- No fleet with `fleetId` exists in the game state
- The fleet identified by `fleetId` is not alien-owned
- The fleet is already in transit (`inTransit == true`)
- The fleet has zero total units
- No system with `destId` exists in the game state
- The fleet's current `locationId` equals `destId` (already there)

**SRV-010** A valid move command must be applied to the game state inside the
engine's state write lock, equivalent to how `engine.applyBotCommand` handles
a `BotCommand`. Specifically: the fleet's `InTransit`, `DestID`, `DepartYear`,
and `ArrivalYear` fields are set, and a `fleet_arrival` event is scheduled.

**SRV-011** The `ArrivalYear` of the dispatched fleet must be computed as:
`currentClock + dist(fleet.locationId, destId) / FleetSpeedC`, using the same
`FleetSpeedC` constant used throughout the engine.

**SRV-012** The `POST /api/alien/move` endpoint must use the engine's existing
state locking mechanism. It must not access `GameState` without holding the
write lock.

---

### 2.3 DefaultBot Suppression

**SRV-013** When the environment variable `SPACEGAME_EXTERNAL_BOT` is set to
the value `"1"`, the server must instantiate a `NullBot` in place of
`DefaultBot`. `NullBot` implements the `BotAgent` interface with no-op methods:
`Initialize` does nothing, `Tick` always returns nil, `OnEvent` does nothing.
The alien side will then be driven exclusively by commands arriving via
`POST /api/alien/move`.

**SRV-014** When `SPACEGAME_EXTERNAL_BOT` is not set or is set to any value
other than `"1"`, the server must instantiate `DefaultBot` as today, and the
`POST /api/alien/move` endpoint must still be registered and functional. This
allows manual testing of the alien move endpoint even with `DefaultBot` active
(though both would be driving alien forces simultaneously, which is only
appropriate for testing the endpoint itself).

**SRV-015** `NullBot` must be defined in `srv/internal/game/bot.go` alongside
`DefaultBot`. No new files are required in the `srv/` tree for this change.

**SRV-016** The `main.go` entry point must read `SPACEGAME_EXTERNAL_BOT` and
pass the appropriate bot to `game.NewEngine`. The log output at startup must
indicate which bot is active, e.g.:
```
bot: using DefaultBot (built-in alien AI)
```
or:
```
bot: SPACEGAME_EXTERNAL_BOT=1; using NullBot — external bot expected on /api/alien/move
```

---

### 2.4 Route Registration

**SRV-017** All three new endpoints must be registered in `buildMuxWithFS` in
`srv/internal/server/server.go`, wrapped in `recoverMiddleware`, alongside the
existing five routes. The routing table after this change:

| Method | Path | Handler |
|--------|------|---------|
| `GET` | `/api/stars` | `handleStars` (unchanged) |
| `GET` | `/api/state` | `handleState` (unchanged) |
| `GET` | `/api/events` | `handleEvents` (unchanged) |
| `POST` | `/api/command` | `handleCommand` (unchanged) |
| `POST` | `/api/pause` | `handlePause` (unchanged) |
| `GET` | `/api/alien/fleets` | `handleAlienFleets` *(new)* |
| `POST` | `/api/alien/move` | `handleAlienMove` *(new)* |
| `GET` | `/api/alien/state` | `handleAlienState` *(new)* |

---

### 2.5 Alien Home World: kzinhome

**SRV-018** The server must maintain a hidden `StarSystem` with ID `"kzinhome"`
and display name `"61 Ursae Majoris"` in `state.Systems`. This system
represents the alien home world and is the reference point for all alien
information-delay calculations. Its 3D coordinates must correspond to the
real astronomical position of 61 Ursae Majoris converted using the same
equatorial-to-Cartesian transform and Three.js axis remapping used for all
other systems in the loader (`jx = ax`, `jy = az`, `jz = -ay`). Its distance
from Sol is approximately 31.1 light years.

61 Ursae Majoris astronomical coordinates for the loader:
- RA: `11 41 03.0` (hours minutes seconds)
- Dec: `+34 12 06` (degrees arcminutes arcseconds)
- Distance: `31.1` light years

These values should be hardcoded as constants rather than read from
`nearest.csv`, because the star may appear in the CSV under various catalog
names and because its special role warrants explicit treatment.

**SRV-019** kzinhome must **not** be added to `state.SystemOrder`. Because
both `handleStars` and `handleState` iterate `state.SystemOrder` to build
their responses, this single exclusion is sufficient to hide kzinhome from
all player-facing endpoints without modifying those handlers. kzinhome is
accessible internally via `state.Systems["kzinhome"]`.

**SRV-020** If 61 Ursae Majoris appears in `nearest.csv` under any catalog or
common name, the loader must skip that row (not add it to the game's normal
system set) and instead use the hardcoded kzinhome entry. The loader must log
a message if it detects and skips such a row, e.g.:
```
loader: skipping 61 UMa / HD 101501 from nearest.csv; using hardcoded kzinhome
```
Detection: match any row whose distance field rounds to 31.1 LY and whose
RA/Dec are within 0.1° of the values above.

---

### 2.6 New Endpoint: `GET /api/alien/state`

This endpoint provides the game state as seen from kzinhome. It mirrors the
structure of `GET /api/state` but applies alien-perspective information delay:
status-changing events reach kzinhome after `dist(event_system, kzinhome)`
years, not `dist(event_system, sol)` years.

**SRV-021** The server must expose a new read endpoint at `GET /api/alien/state`.
The response body must use the same JSON structure as `GET /api/state`
(`StateResponse`): `gameYear`, `paused`, `gameOver`, `winner`, `winReason`,
`systems`, and `events`.

**SRV-022** The `gameYear`, `paused`, `gameOver`, `winner`, and `winReason`
fields in the response must reflect current ground truth (no information delay).

**SRV-023** The `systems` array must contain one entry per system in
`state.SystemOrder` (i.e., all normal game systems, excluding kzinhome). For
each system, the `knownStatus` field must reflect the **alien-perspective
known status** (`AlienKnownStatus`), defined in SRV-024 through SRV-027.
All other system fields (`knownEconLevel`, `knownWealth`, `knownLocalUnits`,
`knownFleets`) must be returned as zero/empty; the bot does not use them and
the server does not maintain them for the alien perspective.

**SRV-024** Each `StarSystem` must gain two new fields:

```go
AlienKnownStatus  SystemStatus  // alien-perspective known status
AlienKnownAsOfYear float64      // game year of the most recent event that updated AlienKnownStatus
```

These fields are not included in `GET /api/state` or any other player-facing
response. They are populated by `UpdateAlienKnownStates` (SRV-026).

**SRV-025** At game start (in `Initialize` in `loader.go`), `AlienKnownStatus`
must be set equal to `sys.Status` for every system. `AlienKnownAsOfYear` must
be set to `0.0`. This represents pre-war alien intelligence: the alien empire
knew the initial configuration of the human sphere before hostilities began.

**SRV-026** A new function `UpdateAlienKnownStates(kzinhome *StarSystem,
clock float64)` must be added to `state.go`. It must be called from the
engine's `tick()` function immediately after the existing
`UpdateKnownStates(clock)` call. Its behavior:

```
for each event evt in state.Events:
    if evt.AlienAppliedToKnown: skip
    alienArrivalYear = evt.EventYear + distBetween(state.Systems[evt.SystemID], kzinhome)
    if alienArrivalYear > clock: skip
    applyEventToAlienKnownState(state.Systems[evt.SystemID], evt)
    evt.AlienAppliedToKnown = true
```

The `AlienAppliedToKnown bool` field must be added to `GameEvent`.

**SRV-027** `applyEventToAlienKnownState` applies an event to a system's alien
known state. It must handle the following event types (all others are ignored):

| Event type | Effect on `AlienKnownStatus` |
|------------|------------------------------|
| `system_captured` | → `"alien"` |
| `system_retaken` | → `"human"` |
| `combat_occurred` (human won) | → `"human"` |
| `combat_occurred` (alien won) | → `"alien"` |
| `combat_occurred` (draw) | → `"contested"` |

`AlienKnownAsOfYear` must be updated to `evt.EventYear` whenever a change is
applied, mirroring the logic in `applyEventToKnownState`.

`EventCombatSilent` and `EventAlienSpawn` must be ignored (they are internal
and must not propagate to kzinhome).

**SRV-028** The `handleAlienState` handler must acquire the state read lock for
the duration of the response construction. It must look up kzinhome via
`state.Systems["kzinhome"]`. If kzinhome is not found (which should not happen
in a correctly initialised game), the handler must return HTTP 500 with a JSON
error body. The `events` field in the response must be returned as an empty
JSON array (`[]`); the bot does not use it, and computing alien-delayed event
lists is deferred.

---

### 2.7 Initial Alien Fleet Composition

**SRV-029** At game start, each alien entry point must be seeded with one fleet
consisting of **4 battleships and 12 escort vessels** (16 units total). This
is accomplished by changing the `AlienInitialComposition` constant in
`srv/internal/game/constants.go` from its current value to:

```go
var AlienInitialComposition = map[WeaponType]int{
    WeaponBattleship: 4,
    WeaponEscort:     12,
}
```

The interceptors previously in `AlienInitialComposition` are removed. With
two entry points (`AlienEntryCount = 2`), the alien side will field 32 units
total at game start. This gives the aliens a strong opening attack force
consistent with their role as the aggressor.

**SRV-030** The `AlienSpawnComposition` constant (used for periodic
reinforcement waves) is **not changed** by this requirement.

**SRV-031** The `AlienInitialUnits` constant in `constants.go` must be updated
from `15` to `16` to match the new `AlienInitialComposition` total. Although
`AlienInitialUnits` is not referenced by the loader (which uses
`AlienInitialComposition` directly), leaving it at `15` would create a
misleading inconsistency for future readers of the constants file.

---

## 3. BasicBot Program Requirements

### 3.1 Module and Location

**BOT-001** `BasicBot` must be a standalone Go program located at
`BasicBot/` in the repository root. It must have its own `go.mod` file
declaring module path `github.com/gmofishsauce/SpaceGame/BasicBot` and
specifying the same Go version as the main module (`go.mod` at repo root).

**BOT-002** `BasicBot` must use only the Go standard library. No third-party
packages. This mirrors `FR-003` of the game server and keeps the build
trivial.

**BOT-003** `BasicBot` must be buildable and runnable independently of the
`srv/` module. It must not import any packages from
`github.com/gmofishsauce/SpaceGame/srv/...`. All types it needs (fleet
shapes, state shapes) must be defined locally within the `BasicBot` module
using plain Go structs that match the server's JSON wire format.

---

### 3.2 Configuration

**BOT-004** `BasicBot` must accept the following command-line flags:

| Flag | Default | Description |
|------|---------|-------------|
| `-server` | `http://127.0.0.1:8080` | Base URL of the game server |
| `-interval` | `5` | Decision interval in real seconds between bot cycles |
| `-v` | false | Verbose: log each fleet decision to stderr |

**BOT-005** All flags must be parsed with the standard `flag` package. Unknown
flags must cause the program to print usage and exit with code 1.

---

### 3.3 Startup and Lifecycle

**BOT-006** On startup, `BasicBot` must verify that the game server is
reachable by issuing `GET /api/alien/state`. If the server is not reachable
within 10 seconds (with retries every 2 seconds), the program must print an
error to stderr and exit with code 1.

**BOT-007** `BasicBot` must handle `SIGINT` and `SIGTERM` gracefully: on
receipt of either signal, the program must complete any in-flight HTTP request,
log a shutdown message to stderr, and exit with code 0.

**BOT-008** `BasicBot` does not need to register with the server or maintain
any persistent session. Each decision cycle is stateless from the server's
perspective.

---

### 3.4 Decision Loop

**BOT-009** `BasicBot` must run a periodic decision loop. The loop fires every
`-interval` real seconds (default 5). Each iteration of the loop is called a
**cycle**.

**BOT-010** If the game is paused (`paused == true`) or over
(`gameOver == true`) as reported by `GET /api/alien/state`, the bot must skip
the cycle entirely and wait for the next interval.

**BOT-011** Each cycle must execute the following steps in order:
1. Fetch current alien-perspective game state via `GET /api/alien/state`.
   This provides pause/gameOver status, game year, and alien-perspective
   system known statuses used for target selection (§3.5).
2. Fetch current alien fleet positions via `GET /api/alien/fleets`.
3. Compute the target list (see §3.5).
4. Compute fleet assignments (see §3.5).
5. Issue one `POST /api/alien/move` request per assignment.

**BOT-012** If the `GET /api/alien/state` call fails (non-200 response or
network error), the bot must log the error to stderr and skip the rest of the
cycle. The same applies to `GET /api/alien/fleets`.

**BOT-013** If a `POST /api/alien/move` call returns a non-200 response or a
JSON body with `"ok": false`, the bot must log the error to stderr and
continue to the next fleet assignment. A failed move for one fleet must not
prevent move commands for other fleets in the same cycle.

---

### 3.5 Strategy: Nearest Human First

**BOT-014** The target list must be constructed as follows:
- From the `GET /api/alien/state` response, collect all systems whose
  `knownStatus` is `"human"`. This is the alien-perspective known status,
  which reflects information that has had time to travel 31+ light years to
  kzinhome. The bot may therefore be working with older intelligence than the
  human player, particularly for systems near Sol.
- Sort this list by Euclidean distance from Sol (ascending) — attack the
  nearest reported human-held systems first.
- Systems whose `knownStatus` is anything other than `"human"` (including
  `"alien"`, `"contested"`, `"uninhabited"`, or `"unknown"`) are excluded from
  the target list.

**BOT-015** The rationale for sorting by distance from Sol (rather than
distance from the fleet): the goal is to threaten Earth, and human-held
systems nearer Sol are higher-value targets. This produces a more coherent
attack pattern than nearest-to-fleet, while remaining equally simple to
implement.

**BOT-016** Fleet assignment must work as follows:
- Iterate over alien fleets (from `GET /api/alien/fleets`) in any stable order.
- Skip fleets where `inTransit == true`.
- Skip fleets where the total unit count is zero (sum of all values in
  the `units` map).
- For each eligible fleet, assign it the highest-priority target from the
  target list that:
  - Is not the fleet's own current `locationId` (already there).
  - Has not already been assigned to another fleet in this cycle (do not
    send two fleets to the same target in the same cycle).
- If no eligible target exists for a fleet, leave it idle (issue no move
  command).

**BOT-017** The "already assigned" exclusion in BOT-016 must be tracked only
within a single cycle. Across cycles there is no memory: if a target is still
human-held on the next cycle and another fleet is idle, that fleet may be
assigned the same target.

**BOT-018** The bot must also track in-transit alien fleets: a target system
that already has an alien fleet en route to it (i.e., `destId` matches the
system's ID in the `GET /api/alien/fleets` response) must be excluded from
assignment, even if it was not assigned in the current cycle. This prevents
multiple fleets from being dispatched to the same destination simultaneously.

**BOT-019** The Euclidean distance calculation must use the 3D positions
(`x`, `y`, `z`) of the systems as returned by `GET /api/stars`. The bot must
fetch `GET /api/stars` once at startup and cache the result for the lifetime
of the program. System positions never change during a session. kzinhome does
not appear in `GET /api/stars`; the bot does not need kzinhome's coordinates
(alien information delay is computed server-side).

---

### 3.6 Logging

**BOT-020** `BasicBot` must write all log output to stderr. It must not write
to stdout (stdout is reserved for possible future structured output).

**BOT-021** The following events must always be logged regardless of the `-v`
flag:
- Program startup (server URL, decision interval)
- First successful connection to the game server
- Each `POST /api/alien/move` error response
- Program shutdown

**BOT-022** When `-v` is set, the following additional events must be logged:
- Each cycle start, including the current in-game year and pause/running state
- Each fleet considered for assignment, and whether it was skipped or assigned
- Each target system considered, and why it was accepted or rejected
- Each `POST /api/alien/move` success response

---

### 3.7 Error Handling Summary

| Condition | Required behavior |
|-----------|-------------------|
| Server unreachable at startup | Retry for 10 s, then exit(1) |
| `GET /api/alien/state` fails mid-run | Log error, skip cycle |
| `GET /api/alien/fleets` fails | Log error, skip cycle |
| `POST /api/alien/move` returns error | Log error, continue to next fleet |
| `GET /api/stars` fails at startup | Log error, exit(1) |
| Invalid JSON in any response | Log error; treat as request failure |
| SIGINT / SIGTERM | Complete current HTTP request, then exit(0) |
| Panic in decision loop | Recover, log to stderr, continue |

---

## 4. Non-Functional Requirements

**NFR-BOT-001** `BasicBot` must build with `go build ./...` from `BasicBot/`
with no errors and no warnings.

**NFR-BOT-002** The `BasicBot` binary must start and make its first decision
cycle within 3 real seconds of the game server being available.

**NFR-BOT-003** Each decision cycle must complete within 2 real seconds under
normal operating conditions (game server on localhost, fewer than 200 systems).
This is not a hard real-time constraint; it is a sanity bound for debugging use.

**NFR-BOT-004** `BasicBot` must not write to or read from any file on disk
during normal operation. All configuration is via flags and environment.

**NFR-BOT-005** `BasicBot` must not use goroutines beyond those required for
signal handling. The decision loop runs in the main goroutine. This keeps the
program easy to trace and debug.

---

## 5. Constraints

- **Language**: Go, same version as declared in the repo root `go.mod`.
- **Standard library only**: No third-party packages in `BasicBot/` or in the
  server extension code (consistent with `FR-003`).
- **No shared code with `srv/`**: `BasicBot` is fully independent; it
  reconstructs the JSON DTOs it needs as local structs.
- **No database, no files**: All state is in-memory and ephemeral.
- **Localhost only**: `BasicBot` is a development tool; security is not a
  concern. No authentication, no TLS.

---

## 6. Out of Scope

The following are explicitly excluded from this requirements document:

- Any improvement to `DefaultBot`'s strategy.
- Any change to the browser SPA (`web/`).
- A `POST /api/alien/construct` endpoint (alien forces spawn via the engine's
  `spawnAlienForces`, not via construction orders; this does not change).
- Save/load compatibility for `BasicBot` session state.
- Any form of registration, authentication, or session token between
  `BasicBot` and the server.

### Deferred: Alien Economic Simulation

The following requirements are acknowledged as necessary for long-term game
balance but are **explicitly deferred** to keep the initial `BasicBot`
implementation simple and get the game online quickly. They should be
addressed in a future version of this document before `BasicBot` is considered
feature-complete.

**DEFERRED-001 — Alien star systems**: The alien empire should control star
systems of its own, not shown in the human UI, that provide industrial and
strategic depth to the alien side. kzinhome (now defined as a hidden system)
is the foundation for this, but it currently has no economic or military role
beyond serving as an information relay point.

**DEFERRED-002 — Alien economy**: kzinhome and any future alien-held systems
should accumulate wealth and advance economic levels over time, mirroring
the human economic model. This wealth should fund alien weapons construction,
allowing the alien side to build forces rather than only receiving periodic
spawn waves from `spawnAlienForces`.

**DEFERRED-003 — Alien construction orders**: `BasicBot` should be able to
issue construction commands (analogous to `POST /api/command` with type
`construct`) to build new units at alien-held systems. A companion server
endpoint (e.g., `POST /api/alien/construct`) and corresponding bot logic
would be required. Until this is implemented, alien force generation remains
entirely under the engine's `spawnAlienForces` mechanism.

Until these deferred items are implemented, the alien side is economically
passive: it receives spawn waves on a fixed schedule but cannot independently
grow its industrial base. The initial fleet composition (SRV-029) partially
compensates by giving the aliens a strong opening force.
