# Design: Symmetric Multiplayer

## 1. Overview

Convert SpaceGame from an asymmetric one-player-versus-omniscient-bot game
to a symmetric two-player-over-loopback game. Both sides — the human SPA
and a new standalone bot binary — connect to the same Go server over the
public HTTP/SSE API and play under identical rules and identical
information delays. The omniscient in-process bot, the alien spawn/dormancy
machinery, and the alien-only "exhaustion" win path are removed entirely.
This document specifies the concrete code changes required to implement
the requirements in `MULTI/requirements.md`.

---

## 2. Requirements Summary

The IDs below are reproduced from `MULTI/requirements.md`. Where two
requirements are tightly coupled they are summarised together; the
authoritative wording is in the requirements document. This design must
satisfy each of the following.

### Functional — game model and symmetry

- **FR-1.** A single two-player game session: one human (home: Sol), one
  alien (home: 61 UMa). No more than two players.
- **FR-2.** Identical mechanics for both players: economy, weapons, fleet
  movement, combat, reporters, comm lasers, command travel delay, info
  delay.
- **FR-3.** The server is the sole authoritative simulator; clients
  cannot mutate `GameState`.
- **FR-4 / FR-5.** 61 UMa and Sol are seeded identically at t=0:
  status=that side held, econ level 5 (no further growth), wealth 64,
  1 comm laser, one initial fleet at home with 2 reporters.
- **FR-6.** Non-home starting systems are selected and economically
  initialised by the existing rules, applied per side: home-region
  systems with planets, plus home-region systems within half-max
  distance from home. Econ levels via the existing Gaussian rule.
- **FR-7.** All other catalogue entries are uninhabited.

### Functional — faction and identity

- **FR-8.** Unify `HumanFaction` and `AlienFaction` into a single
  `Faction` type used twice.
- **FR-9.** Code identifiers retain `Human`/`Alien` labels; do not
  rename to generic `Player1`/`Player2`.
- **FR-10.** Wire-format owner labels stay absolute (`human`, `alien`).
  No per-request `self`/`foe` relabelling.

### Functional — truth, views, and information delay

- **FR-11.** Exactly one `Truth`.
- **FR-12.** Exactly two views (one per player); `SolView` is replaced.
- **FR-13.** Each event carries a per-player arrival year plus the
  internal flag.
- **FR-14.** An event matures into a player's view only when the engine
  clock crosses *that* player's arrival year for it.
- **FR-15.** Per-player matured cursors; no shared frontier.
- **FR-16.** Each non-internal event is broadcast on a player's SSE
  stream when it matures into that player's view, and not on the other
  player's stream.
- **FR-17.** At t=0 each `PlayerView` is a complete mirror of `Truth`
  for every system, with `AsOfYear=0` ("pre-war radio monitoring").
- **FR-18.** After t=0 only matured events update a `PlayerView`. Stale
  initial knowledge persists.
- **FR-19.** `KnownSystem.Status = unknown` may exist in the type
  system but MUST NOT appear at t=0.

### Functional — reporters and comm lasers

- **FR-20.** Reports flow to the *owner's* home, not unconditionally to
  Sol. Comm laser at speed 1.0c; reporter at 0.8c.
- **FR-21.** Owner used for routing the report is the system's owner
  *at the moment of reporting*.
- **FR-22.** Other reporter / comm-laser semantics unchanged.
- **FR-23.** The "reporter fleet arrives at Sol" special case is
  generalised to "reporter fleet arrives at owner's home."

### Functional — commands

- **FR-24.** Identical command request/response shapes for both
  players.
- **FR-25.** Commands originate at issuer's home.
- **FR-26.** Travel delay = `dist(home, target)/0.8c` for both players.
- **FR-27.** Validate against issuer's `PlayerView`, not against
  `Truth` and not against the other player's view.
- **FR-28.** Late ground-truth failure semantics preserved.

### Functional — star catalogue

- **FR-29.** Load four files: `nearest.csv`, `planets.csv`,
  `alien-nearest.csv`, `alien-planets.csv` (same column schemas).
- **FR-30.** Runtime catalogue is the union, single keyed by stable ID.
- **FR-31.** Loader must fail fast on duplicate IDs across the union,
  naming offenders. Existing same-file co-located grouping preserved.
- **FR-32.** Coordinate frame stays Sol-relative.
- **FR-33.** `CatalogEntry.DistFromSol` valid for all entries.
- **FR-34.** Capability to compute distance from a player's home for
  any system.
- **FR-35.** `GET /api/stars` exempt from player-identity rule;
  identical response for both; existing 24h cache preserved.

### Functional — player identity on the API

- **FR-36/37/38.** Required `?player=human|alien` query parameter on
  every `/api/*` request except `/api/stars`. Anything else →
  `400 {"ok":false,"error":"unknown player"}`. Validation in
  middleware; ID attached to context.
- **FR-39.** `/api/stars` ignores `player`.
- **FR-40.** No headers, tokens, joins, or path prefixes.

### Functional — state / events

- **FR-41.** `GET /api/state` returns the requesting player's view.
- **FR-42.** `GET /api/events` is scoped to requester; existing frame
  shapes unchanged.
- **FR-43.** At most one active SSE per player; a second concurrent
  one for the same player MUST NOT be treated as the *other* player.
- **FR-44.** `/api/debug/state` exposes `Truth`; available to either
  client.

### Functional — game lifecycle

- **FR-45.** Engine paused on startup.
- **FR-46.** Engine begins ticking automatically when both players
  have an open SSE subscription.
- **FR-47.** On any SSE drop mid-game: pause, log
  `server: <player> disconnected; game paused`; only `clock_sync` on
  the surviving stream.
- **FR-48.** Reconnect not supported in v1.
- **FR-49.** No "new game" detection; restart = restart the process.

### Functional — pause

- **FR-50.** `POST /api/pause` accepted only for `?player=human`;
  `?player=alien` → `403 {"ok":false,"error":"only the human player may pause"}`.
- **FR-51.** Paused → no `game_event` or `system_update` on either
  stream.
- **FR-52.** Both streams stay open while paused.
- **FR-53.** `clock_sync` is the sole pause-state signal.

### Functional — victory / draw

- **FR-54.** Win by capturing opponent's home.
- **FR-55.** Win by holding ≥ `WinRetentionFraction` of opponent's
  initial systems (single shared constant).
- **FR-56.** `CheckVictory` evaluates each player symmetrically.
- **FR-57.** Draw condition exists; v1 form is a game-length cap
  `DrawYearCap`.
- **FR-58.** `WinRetentionFraction` and `DrawYearCap` are exposed as
  tunable constants.
- **FR-59.** Old "alien exhausted" counter and rule removed.
- **FR-60.** `game_over.winner` ∈ {`human`, `alien`, `draw`}.

### Functional — bot binary

- **FR-61.** New Go binary built from `srv/cmd/spacebot/`.
- **FR-62.** Bot logic under `srv/internal/bot/` (`agent.go`,
  `client.go`, `state.go`).
- **FR-63.** Bot speaks public HTTP/SSE only; no `srv/internal/game`
  imports.
- **FR-64.** Bot identifies as `?player=alien`.
- **FR-65.** Bot subscribes to SSE; uses `GET /api/state` for
  snapshots.
- **FR-66.** Bot issues commands via `POST /api/command?player=alien`,
  paying `dist/0.8c` delay.
- **FR-67.** Bot cannot pause; tolerates being late.
- **FR-68.** Server does not spawn or supervise the bot.
- **FR-69.** New `scripts/run-game.sh` launches server-then-bot;
  manual two-terminal launch remains supported.

### Functional — bot strategy v1

- **FR-70.** Three behaviours, in order: (1) replenish escorts at
  home; (2) dispatch fleets toward opponent system with highest
  `KnownSystem.AsOfYear`; (3) slow reporter sweep favouring oldest
  observation.
- **FR-71.** No tree search, no opponent modelling, no economic
  optimisation.
- **FR-72.** Bot plans only against its `PlayerView`.
- **FR-73.** Optional `--debug-truth` flag: bot calls
  `/api/debug/state` and logs Truth; decision logic ignores it.

### Functional — cleanup

- **FR-74.** Delete: `BotAgent`, `DefaultBot`, `BotCommand`, the bot
  helpers in `bot.go`; `Engine.Bot` field, bot tick branch,
  `applyBotCommand`, `spawnAlienForces`, alien-spawn cadence; the
  alien-cluster constants in `constants.go`; the alien faction
  fields in `state.go`; `EventAlienSpawn` and `EventAlienExhausted`;
  the loader's entry-point selection, the `SolView` carve-out, and
  the alien-fleets-at-entry-points seeding. The Sol special-case in
  `processFleetArrivals` is *generalised*, not deleted.
- **FR-75.** No parallel asymmetric mode preserved.
- **FR-76.** Asymmetric tests deleted or rewritten; player-agnostic
  tests preserved.

### Functional — documentation

- **FR-77.** Update `server_api.md` for `?player`, the 400 / 403
  responses, removed `alien_exhausted`, scoped state/events, and the
  retained absolute owner labels.
- **FR-78.** `OLD_SPECS/` not modified.

### Non-functional

- **NFR-1.** Engine remains sole writer; handlers hold only RLock.
- **NFR-2.** Per-player propagation MUST NOT introduce non-determinism
  for a given seed.
- **NFR-3.** Existing tick cadence (~100 ms wall, ~0.056 game years/s)
  unchanged.
- **NFR-4.** Loopback-only (`127.0.0.1:8080`).
- **NFR-5.** No new auth; `?player` is identification, not
  authentication.
- **NFR-6.** `go build` paths preserved; new `spacebot` binary builds
  the same way.
- **NFR-7.** `go test ./srv/...` passes.
- **NFR-8.** Existing package-boundary discipline preserved; bot
  follows the analogous layout.
- **NFR-9.** Existing naming conventions preserved.
- **NFR-10/11/12.** SPA continues to function with one extra query
  param; map renders alien-region stars; frontend build path
  unchanged.
- **NFR-13.** Disconnect-driven pause logged unambiguously to stdout.
- **NFR-14.** No routine logging of `Truth` or `PlayerView`; debug
  endpoints stay opt-in.
- **NFR-15.** `WinRetentionFraction` and `DrawYearCap` (and any
  future tunables) live in the constants file.
- **NFR-16.** Don't gratuitously foreclose future headless / RL
  additions.

---

## 3. Requirements Issues

### Ambiguities

- **A-1. Sol's home econ level today is 4, not 5.** `srv/internal/game/loader.go:86`
  hard-codes `ts.EconLevel = 4` for Sol, while `server_api.md`'s
  weapon table and `EconWealthRate` array claim "Sol is always level
  5", and `gaussianEconLevel` clamps non-home systems to `[1, 4]`.
  `requirements.md` (FR-4 / FR-5) and the overview's Q20 specify
  `EconLevel = 5` for both home stars at t=0.
  **This design assumes** both Sol and 61 UMa are seeded at level 5,
  consistent with the requirements. The loader will be changed to
  set Sol to 5 (which is a behavioural change for the human side
  vs. today). If the stakeholder intended Sol to stay at level 4
  and merely wants 61 UMa to mirror Sol, FR-4 / FR-5 / Q20 would
  need to be amended; flagging here so a developer who notices the
  diff does not silently "correct" it back.

- **A-2. Definition of "initial systems" in FR-55.** FR-55 says a
  player wins by holding ≥ X% of the *opponent's initial systems*
  at any point. "Initial" is taken to mean "the systems that
  player owned at t=0" (i.e. recorded once in
  `Faction[Owner].InitialSystemIDs` at load time and never recomputed).
  This is the analogue of today's `state.Human.InitialSystemIDs`.
  This design adopts that interpretation. An alternative reading
  would be "the opponent's *currently held home-region* systems";
  that is rejected because it makes the threshold drift as the
  opponent loses systems, which is not what the win condition
  intends.

- **A-3. Capture of the *empty* opponent home.** FR-54 says "win by
  capturing the opponent's home star." If the opponent's home has
  changed `TrueSystem.Status` away from that opponent's owner — for
  any reason — does the capturing side win immediately? This design
  treats `Truth.Systems[homeOf(opponent)].Status == ownerOf(self)`
  as the unambiguous check, mirroring today's
  `if sol.Status == StatusAlien` test in `CheckVictory`. Combat at
  home is the only realistic path to that condition.

### Contradictions

- None found. The requirements are self-consistent. The mismatch
  between `requirements.md` and the existing Sol-at-level-4 code is
  recorded under A-1 (a code-vs-requirement gap, not a self-
  contradiction in the requirements).

### Gaps

- **G-1. Initial fleet name for 61 UMa.** FR-4 says 61 UMa is seeded
  with one initial fleet ("1st Fleet") containing 2 reporters. The
  current loader names that fleet `<DisplayName>-1st Fleet` where
  `<DisplayName>` is the home-system display name. This design
  follows the same convention: the alien initial fleet is named
  `<61-UMa-DisplayName>-1st Fleet`, where `<DisplayName>` is whatever
  `alien-nearest.csv` produces (e.g. "61 Ursae Majoris" → fleet name
  "61 Ursae Majoris-1st Fleet"). If the stakeholder wants a fixed
  name like "Alien-1st Fleet" instead, FR-4 should be amended.

- **G-2. Behaviour of an alien second SSE subscription as `human`.**
  FR-43 says a second concurrent subscription as the *same* player
  MUST NOT be treated as the other player, but does not say what
  happens. The pre-existing `EventManager.Register` makes a fresh
  channel per `clientID`; the unsuspecting second subscriber would
  simply receive the same stream. In practice the bot opens one
  subscription and the SPA opens one. This design preserves that
  behaviour: a second concurrent subscription as the same player
  succeeds and gets its own channel; `Engine` treats "at least one
  subscription" as "this player is connected."

- **G-3. Wealth growth at alien-held systems.** FR-2 says rules are
  identical, but the existing `economy.AccumulateWealth` only
  accumulates wealth for `StatusHuman` systems. FR-2 forces this to
  also apply to `StatusAlien` systems. The design specifies this in
  §6 (`economy.go`) — flagged here as an implication of FR-2 that is
  not separately spelled out in the requirements.

### Untestable requirements

- **U-1. NFR-9 ("respect existing conventions").** Subjective; not
  directly testable. Treated as a code-review check, not as a
  build-time gate.
- **U-2. NFR-16 ("don't gratuitously foreclose future RL/headless").**
  Subjective. Treated as a design-review check that is satisfied by
  isolating the bot in its own binary against the public API.

---

## 4. Constraints and Assumptions

### Constraints

- **Language.** Go 1.x for the server and the bot; JavaScript ES
  modules (Vite) for the SPA. Module path:
  `github.com/gmofishsauce/SpaceGame`.
- **Network.** Server binds only `127.0.0.1:8080` (NFR-4).
- **Concurrency.** `GameState.mu` is the single coarse lock. Engine
  is sole writer (NFR-1). Handlers hold only `RLock`.
- **Determinism.** RNG is injected (DR-11 in the existing code); per-
  player propagation must not break per-seed determinism (NFR-2).
- **Tick cadence.** 100 ms wall-clock; ~0.005556 game years per tick
  (NFR-3).
- **Backwards compatibility.** None. The asymmetric game is replaced;
  no parallel mode, no legacy flag (FR-75).
- **Existing conventions.** snake_case in tests/log strings is
  mostly absent; the code is idiomatic Go (CamelCase exported,
  camelCase unexported). The design preserves that. Existing
  identifiers `HumanOwner`, `AlienOwner`, `StatusHuman`,
  `StatusAlien` are preserved (FR-9, FR-10).

### Assumptions

- **AS-1.** `alien-nearest.csv` and `alien-planets.csv` exist at the
  process working directory at startup time, with the same column
  schema as their Sol-side counterparts. They are produced by a
  separate data task (out of scope per `requirements.md` §3 and the
  overview's §8).
- **AS-2.** `alien-nearest.csv` contains a row whose `commonName`
  field resolves (via `toSystemID`) to a stable ID for 61 UMa. This
  design uses whatever `toSystemID` yields (likely `61-ursae-majoris`)
  rather than hard-coding `61-uma`. The constant `AlienHomeID` is
  populated at load time, not compile time.
- **AS-3.** Both CSV files are encoded such that any star physically
  present in both regions appears with the same preferred display
  name in both, and therefore yields the same `toSystemID` output.
  The loader detects ID collisions across the union (FR-31). It
  does not silently coalesce.
- **AS-4.** Per Q21, fleet ownership in `TrueFleet.Owner` is the
  authoritative answer to "whose home does a report flow to?" at
  the moment of reporting. Local-unit reporting (e.g. comm-laser
  destruction in combat) routes by `TrueSystem.Status` at the moment
  the laser fires its report — i.e. the value of `Status` *before*
  combat resolution writes the new owner.
- **AS-5.** A single shared `WinRetentionFraction` of 0.60 is used
  initially. (Today's `HumanWinRetentionFraction` = 0.60 already
  matches the human side; `AlienWinCaptureFraction` = 0.40 is
  collapsed away. The shared value is a tuning parameter per Q9.)
- **AS-6.** Initial `DrawYearCap` is 500 in-game years. This is a
  guess pending playtesting (the overview explicitly defers tuning);
  the constant is a single-line change. Flagged so a developer is
  not surprised by the magic number.
- **AS-7.** The loader's existing same-file co-located grouping in
  `loadStars` (collapsing string-equal RA/Dec/distance rows) is
  applied per-file, *before* the cross-file union. This avoids
  mistakenly grouping Sol-region rows with alien-region rows that
  share textual coordinates.

---

## 5. Architecture

### 5.1 High-level component diagram

```
                      ┌──────────────────────────────────────────────┐
                      │             Process: spacegame               │
                      │                                              │
   ┌──────────┐ HTTP  │  ┌────────────────────┐                      │
   │ Browser  ├──────►│  │  net/http mux      │                      │
   │  (SPA)   │ SSE   │  │  + playerMiddleware│                      │
   │          │       │  └─────────┬──────────┘                      │
   └──────────┘       │            │                                 │
                      │            ▼                                 │
                      │  ┌────────────────────────────┐              │
                      │  │  server/handlers.go        │              │
                      │  │  (per-player views/SSE)    │              │
                      │  └─────┬───────────┬──────────┘              │
                      │        │           │                         │
   ┌──────────┐ HTTP  │        ▼           ▼                         │
   │ spacebot ├──────►│  ┌─────────┐  ┌──────────────┐               │
   │ (Go bin) │ SSE   │  │ Engine  │◄─┤ EventManager │               │
   │          │       │  │ + tick  │  │ (per-player  │               │
   └──────────┘       │  │ loop    │  │   channels)  │               │
                      │  └────┬────┘  └──────────────┘               │
                      │       │                                      │
                      │       ▼                                      │
                      │  ┌────────────────────────────┐              │
                      │  │ GameState: mu, Truth,      │              │
                      │  │ Views[player]=PlayerView,  │              │
                      │  │ EventLog (per-player heaps)│              │
                      │  └────────────────────────────┘              │
                      └──────────────────────────────────────────────┘
```

The bot runs as a sibling process; nothing in `spacegame` knows it
exists beyond the fact that "alien" is one of two SSE subscribers.

### 5.2 What is new, modified, unchanged

**New code:**

- `srv/cmd/spacebot/main.go` — bot entry point (FR-61).
- `srv/internal/bot/agent.go` — strategy (FR-62, FR-70).
- `srv/internal/bot/client.go` — typed HTTP/SSE client (FR-62,
  FR-63).
- `srv/internal/bot/state.go` — bot's local mirror (FR-62).
- `scripts/run-game.sh` — orchestrator (FR-69).

**Modified code:**

- `srv/cmd/spacegame/main.go` — drop `NewDefaultBot()` wiring, take
  alien CSV paths.
- `srv/internal/server/server.go` — register
  `playerMiddleware` (FR-38). Wire the engine connect/disconnect
  hooks into `handleEvents`.
- `srv/internal/server/handlers.go` — every handler except
  `handleStars` and `handleDebugState` consumes the player ID from
  context. `handleState` returns the requesting player's view.
  `handlePause` 403s on alien. `handleCommand` writes the issuer's
  origin and reads the issuer's view for validation.
- `srv/internal/server/types.go` — minor; possibly add fields to
  DTOs.
- `srv/internal/game/state.go` — replace `HumanFaction`/`AlienFaction`
  with `Faction` × 2; replace `SolView` with `Views[Owner]`; replace
  `ReadSolGroundTruth` with `ReadHomeGroundTruth(player Owner)`;
  rewrite `CheckVictory` symmetrically.
- `srv/internal/game/truth.go` — unchanged in shape; comments
  updated.
- `srv/internal/game/solview.go` — rename file/type to
  `playerview.go` / `PlayerView`. (See §6.4.)
- `srv/internal/game/propagator.go` — per-player propagation; the
  propagator now writes to two views.
- `srv/internal/game/eventlog.go` — per-player heaps and per-player
  `PopMatured`.
- `srv/internal/game/engine.go` — drop `Engine.Bot`, the bot tick
  branch, `applyBotCommand`, `spawnAlienForces`; generalise the
  reporter-arrives-at-Sol special case to "arrives at owner's home";
  add `OnPlayerConnected` / `OnPlayerDisconnected`.
- `srv/internal/game/events.go` — broadcasts target a specific
  player's channel set, not all clients indiscriminately. Drop the
  `alien_exhausted` event.
- `srv/internal/game/types.go` — drop `EventAlienSpawn`,
  `EventAlienExhausted`.
- `srv/internal/game/economy.go` — `AccumulateWealth` and
  `AdvanceEconLevels` apply to both `StatusHuman` and `StatusAlien`
  systems (G-3). `ValidateConstruct` accepts either side as long as
  it's the issuer's side.
- `srv/internal/game/combat.go` — report routing uses the system /
  fleet owner's home, not Sol.
- `srv/internal/game/loader.go` — load four CSVs, merge with
  duplicate-ID detection, seed both homes, mirror both views from
  truth (FR-17).
- `srv/internal/game/constants.go` — drop the asymmetric constants;
  add `WinRetentionFraction`, `DrawYearCap`.
- `server_api.md` — rewrite per FR-77.

**Deleted code:**

- `srv/internal/game/bot.go` — entire file.

**Unchanged code:**

- `srv/internal/game/catalog.go` — already player-agnostic.
- The Vite app, `web/src/*` — only acquires `?player=human` on its
  fetches/EventSource (NFR-10). Owner labels stay absolute, so the
  rendering logic is untouched.

### 5.3 Data flow and control flow

**Tick (engine, every 100 ms):**

1. `processFleetArrivals` — owner-aware (was Sol-only special).
2. Process matured commands (`PendingCommand.ExecuteYear ≤ Clock`).
3. `AccumulateWealth` and `AdvanceEconLevels` for both sides.
4. Combat in any system with both sides present. Reports route to
   system's owner's home (or fleet owner's home for reporters that
   flee).
5. `Propagator.Propagate(state)` — for each player, pop matured
   events from that player's heap; apply to that player's view;
   broadcast on that player's SSE channel set.
6. Periodic `clock_sync` to all subscribers.
7. `CheckVictory` symmetrically; on win/draw, set `GameOver`.

**Player connect (SSE):**

1. Middleware extracts `?player=human|alien` and attaches to context.
2. `handleEvents` registers a channel under the player ID, sends
   the `connected` snapshot of *that* player's view, calls
   `Engine.OnPlayerConnected(playerID)`.
3. `OnPlayerConnected` increments a per-player counter; when both
   counters > 0 and the engine is paused for the "waiting" reason,
   unpause.

**Player disconnect:**

1. `handleEvents` returns (request context cancelled, or send
   failure). It calls `EventManager.Unregister(playerID, clientID)`.
2. Unregister returns a "subscriber count went to zero" boolean.
3. If that boolean is true and `GameOver` is false, the handler
   calls `Engine.OnPlayerDisconnected(playerID)` which logs
   `server: <player> disconnected; game paused` and pauses the
   engine.

**Command (player):**

1. Middleware → context has `playerID`.
2. `handleCommand` decodes JSON, builds a `PendingCommand` with
   `OriginID = homeOf(playerID)`, calls `Engine.EnqueueCommand`.
3. `EnqueueCommand` validates against the issuer's view and computes
   `ExecuteYear = Clock + dist(home, target)/0.8c`.

---

## 6. Detailed Design

This section is in dependency order: types and constants first, then
state, then the engine subsystems, then HTTP, then the bot.

### 6.1 Constants and faction model

#### `srv/internal/game/constants.go`

**Purpose.** Single home for tunable parameters (NFR-15). Drop
asymmetric ones; add symmetric ones.

**Satisfies.** FR-58, FR-59, FR-74 (constants subset), FR-75, NFR-15.

**Delete:**

```go
PeripheryFraction
AlienEntryCount
AlienSpawnIntervalYears
AlienExhaustionThreshold
HumanWinRetentionFraction
AlienWinCaptureFraction
AlienDormancyYears
BotTickCadence
AlienInitialComposition  // map var
AlienSpawnComposition    // map var
```

**Add:**

```go
// WinRetentionFraction is the shared per-side threshold for FR-55:
// a player wins if it holds at least this fraction of the opponent's
// initial systems. Single tunable; identical for both sides. (FR-55,
// FR-58, NFR-15)
const WinRetentionFraction = 0.60

// DrawYearCap is the in-game year at which an unfinished game ends
// in a draw. Subject to playtesting. (FR-57, FR-58, NFR-15)
const DrawYearCap = 500.0
```

**Keep unchanged:** `TickIntervalMs`, `YearsPerTick`, `FleetSpeedC`,
`CommandSpeedC`, `EconGrowthIntervalYears`, `EconLevelMean`,
`EconLevelStddev`, `ClockSyncCadence`, `MaxCombatRounds`,
`WealthPenaltyMaxFraction`, `EconWealthRate`, `WeaponDefs`.

#### `srv/internal/game/types.go`

**Purpose.** Owner / status / event-type enums.

**Satisfies.** FR-9, FR-10, FR-19, FR-74 (events subset).

**Delete:**

```go
EventAlienSpawn
EventAlienExhausted
```

**Add nothing.** `Owner` retains `HumanOwner`, `AlienOwner`. The
existing `StatusUnknown` value is retained per FR-19.

**Add helper (anywhere player-aware code is needed):**

```go
// HomeIDOf returns the home system ID for the given player. Both home
// IDs are populated at load time on GameState; this helper centralises
// the lookup so that no caller hard-codes "sol" or "61-uma".
func (s *GameState) HomeIDOf(p Owner) string { return s.Homes[p] }

// OpposingOwner returns the other player. (FR-54, FR-55)
func OpposingOwner(p Owner) Owner {
    if p == HumanOwner { return AlienOwner }
    return HumanOwner
}
```

#### Faction unification

`HumanFaction` / `AlienFaction` collapse into a single `Faction`
type used twice (FR-8). The replacement lives in `state.go`.

```go
// Faction holds per-player state. Used twice — once for human,
// once for alien. (FR-8)
type Faction struct {
    InitialSystemIDs []string  // systems held by this side at t=0 (FR-55)
}
```

Asymmetric fields (`Exhausted`, `TotalLost`, `EntryPointIDs`,
`NextSpawnYear`) are deleted (FR-59, FR-74).

### 6.2 GameState

#### `srv/internal/game/state.go`

**Purpose.** Authoritative in-memory state, sole writer is the
engine.

**Satisfies.** FR-11, FR-12, FR-25, FR-44 (via `ReadHomeGroundTruth`),
FR-54–FR-57, FR-60, FR-74.

**Interface (changes only):**

```go
type GameState struct {
    mu sync.RWMutex

    Clock     float64
    Paused    bool
    GameOver  bool
    Winner    Owner   // "" if not over; "draw" carried as a sentinel below
    WinReason string

    Catalog *StarCatalog
    truth   *Truth

    // Replaces SolView (FR-12).
    Views map[Owner]*PlayerView

    Events *EventLog

    Propagator *Propagator

    // Replaces Human / Alien fields (FR-8).
    Factions map[Owner]*Faction

    // Home star IDs, populated at load time (AS-2).
    Homes map[Owner]string

    PendingCmds []*PendingCommand

    rng *rand.Rand

    nextFleetNum int
    nextCmdID    int
}

// ReadHomeGroundTruth replaces ReadSolGroundTruth. The home of player
// p is read directly from Truth; this reflects the fact that events
// at one's own home propagate at zero distance (and continuously
// growing wealth is not an event). (FR-44 partial; serves as the
// continuous-state escape hatch for the *issuer's* home.)
func (s *GameState) ReadHomeGroundTruth(p Owner) HomeGroundTruthSnapshot
```

`HomeGroundTruthSnapshot` is `SolGroundTruthSnapshot` renamed. Same
fields.

**Special sentinel for draws.** `Winner` is `Owner`. To carry "draw"
on the wire (FR-60), introduce:

```go
const DrawWinner Owner = "draw"
```

This is a sentinel — `DrawWinner` is not used as a real owner. The
DTO layer encodes `state.Winner` as a string; "draw" passes through
unchanged.

**`CheckVictory` (rewritten).**

```go
// CheckVictory evaluates win conditions symmetrically. (FR-54, FR-55,
// FR-56, FR-57, FR-60)
func (s *GameState) CheckVictory() (over bool, winner Owner, reason string) {
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
        initial := s.Factions[opp].InitialSystemIDs
        if len(initial) == 0 { continue }
        held := 0
        for _, id := range initial {
            if sys, ok := s.truth.Systems[id]; ok && statusToOwner(sys.Status) == p {
                held++
            }
        }
        frac := float64(held) / float64(len(initial))
        if frac >= WinRetentionFraction {
            return true, p, fmt.Sprintf(
                "%s holds %.0f%% of opponent's initial systems.", p, frac*100)
        }
    }

    // 3. Draw on game-length cap.
    if s.Clock >= DrawYearCap {
        return true, DrawWinner, fmt.Sprintf("Game ended at year %.1f without a victor.", s.Clock)
    }

    return false, "", ""
}

// statusToOwner converts a SystemStatus to the corresponding Owner,
// or "" for non-owned statuses (uninhabited / contested / unknown).
func statusToOwner(s SystemStatus) Owner {
    switch s {
    case StatusHuman: return HumanOwner
    case StatusAlien: return AlienOwner
    default: return ""
    }
}
```

**`PendingCommand` updates.**

```go
type PendingCommand struct {
    ID            string
    ExecuteYear   float64
    OriginID      string       // = state.Homes[Issuer] for player commands
    Issuer        Owner        // NEW: which player issued this command (FR-25, FR-27)
    TargetID      string
    Type          CommandType
    WeaponType    WeaponType
    Quantity      int
    FleetID       string
    DestID        string
    SourceFleetID string
    TargetFleetID string
    ReassignUnits map[WeaponType]int
    // IsBot field is REMOVED. Bot commands are now ordinary player
    // commands distinguished by Issuer == AlienOwner.
}
```

**Behavior of `ApplyCommand` for events.** Each `Event` recorded
inside `ApplyCommand` (e.g. `EventCommandArrived`) is given per-
player arrival times by `state.recordEvent` (a thin wrapper —
see §6.4 below).

**Error handling.** Unchanged: any failure returns an `error`; the
engine logs a `command_failed` event.

**Dependencies.** `Catalog`, `Truth`, `EventLog`, `Faction` map.

#### Removed methods

`ReadSolGroundTruth` is renamed to `ReadHomeGroundTruth(p Owner)` and
the call site in `handleState` / `systemToMap` is updated to pass the
requesting player.

### 6.3 Truth

#### `srv/internal/game/truth.go`

**Purpose.** Authoritative world state. Already player-agnostic.

**Satisfies.** FR-11.

**Interface.** Unchanged. Comments updated to remove "alien bot
reads it directly" — under the new model, nothing outside `package
game` reads `Truth` except via `ReadHomeGroundTruth` (and the debug
endpoint).

### 6.4 Player views and event maturation

#### `srv/internal/game/playerview.go` (renamed from `solview.go`)

**Purpose.** A player's information state (FR-12, FR-17, FR-18).

**Satisfies.** FR-12, FR-17, FR-18.

**Interface:**

```go
// PlayerView is the per-player information state. Two of these live
// on GameState, one per Owner. Mutated only by the Propagator
// (loader excepted; it seeds the initial mirror of Truth, FR-17).
type PlayerView struct {
    Owner     Owner                       // which player this view belongs to
    Systems   map[string]*KnownSystem
    Fleets    map[string]*KnownFleet
    InTransit map[string]*KnownTransit
}

// HomeGroundTruthSnapshot is returned by GameState.ReadHomeGroundTruth.
type HomeGroundTruthSnapshot struct {
    Status     SystemStatus
    EconLevel  int
    Wealth     float64
    LocalUnits map[WeaponType]int
    FleetIDs   []string
}
```

`KnownSystem`, `KnownFleet`, `KnownTransit` shapes are unchanged.
The fact that `KnownSystem.FleetIDs` includes opponent fleets that
the player has observed is *new* — today's `SolView` only tracks
human fleets, but under symmetric play each player must be able to
"know" the opponent has units somewhere it once observed them.
Specifically:

- A `KnownFleet` may have `Owner == HumanOwner` *or* `AlienOwner`.
- A `KnownSystem.FleetIDs` may include either.
- The view-rendering code iterates over `view.Fleets[fid]` without
  filtering by owner; old code that filtered with
  `if f.Owner != HumanOwner { continue }` is dropped (it lives in
  `events.go` and `handlers.go`; see §6.7).

#### `srv/internal/game/eventlog.go`

**Purpose.** Append-only record of events plus per-player matured
heaps.

**Satisfies.** FR-13, FR-14, FR-15.

**Interface (changes):**

```go
type Event struct {
    ID          string
    EventYear   float64
    SystemID    string
    Type        EventType
    Description string
    Details     interface{}
    Internal    bool

    // Per-player arrival times. math.MaxFloat64 = never reportable
    // to that player. (FR-13)
    Arrival     map[Owner]float64

    // Per-player flags. (FR-13)
    AppliedToView map[Owner]bool
    Broadcast     map[Owner]bool
}

type EventLog struct {
    All      []*Event
    BySystem map[string][]*Event

    // Per-player min-heap by Arrival[player]. Only unmatured +
    // non-internal events. (FR-15)
    pending map[Owner]*eventHeap

    nextID int
}

func NewEventLog() *EventLog
func (l *EventLog) Record(e *Event)               // pushes onto each player's heap if Arrival[player] < ∞
func (l *EventLog) PopMatured(clock float64, p Owner) []*Event
```

**Algorithm — `Record`:**

```pseudocode
e.ID := next ID if empty
e.AppliedToView := {Human: false, Alien: false}
e.Broadcast := {Human: false, Alien: false}
append e to All; index BySystem
if e.Internal: return
for each p in {Human, Alien}:
    if e.Arrival[p] < MaxFloat64:
        push e onto pending[p]
```

**Algorithm — `PopMatured(clock, p)`:** identical to today's
single-heap `PopMatured`, but on `pending[p]`. Determinism (NFR-2)
follows from heap-by-arrival-year + tie-breaking by record order
(equal arrival years preserve insertion order via stable sort
of equal keys; today's heap is not strictly stable, so we add a
secondary key on `ID` numerical order to make ordering
deterministic).

**Helper for recording (added to GameState or a helpers file).**
Centralises the per-player arrival-time computation so every
call site doesn't replicate the formula:

```go
// RecordEvent computes per-player arrival times for an event
// originating at sys at time eventYear, with the given report
// path (comm laser → 1.0c, reporter-fled → 0.8c, neither →
// unreportable). Then records the event.
//
// reportTo identifies the home that should receive the report —
// typically derived from the system or fleet owner per FR-20/FR-21.
// If reportTo == "" (e.g. an EventCommandArrived for a command
// failing at a system the issuer can't see), routing falls back
// to the issuer if known, else unreportable to both sides.
func (s *GameState) RecordEvent(e *Event, reportTo Owner, hasCommLaser bool, reporterFled bool) {
    arrivals := map[Owner]float64{
        HumanOwner: math.MaxFloat64,
        AlienOwner: math.MaxFloat64,
    }
    if reportTo != "" {
        d := s.Catalog.Distance(e.SystemID, s.Homes[reportTo])
        switch {
        case hasCommLaser:
            arrivals[reportTo] = e.EventYear + d           // 1.0c (FR-20)
        case reporterFled:
            arrivals[reportTo] = e.EventYear + d/FleetSpeedC // 0.8c (FR-20)
        }
    }
    e.Arrival = arrivals
    s.Events.Record(e)
}
```

**Important.** Reports route to **one** player (the owner / fleet-
owner per FR-20/FR-21), not both. The overview is explicit on
this. If a future change introduces multi-player observability of a
single event, `Arrival` is already a per-player map, so the
extension is mechanical.

**Migration of existing call sites.** Every place that today calls
`state.Events.Record(...)` with a single `ArrivalYear` field is
rewritten to call `state.RecordEvent(...)` with the appropriate
`(reportTo, hasCommLaser, reporterFled)`. A find-and-replace pass
over `combat.go`, `engine.go`, `economy.go`, and `state.go` covers
every call site; see §9 for the file list.

#### `srv/internal/game/propagator.go`

**Purpose.** Sole writer of `PlayerView`s; sole emitter of
`game_event` and `system_update` SSE frames.

**Satisfies.** FR-14, FR-16, FR-23 partial.

**Interface:**

```go
type Propagator struct {
    Events *EventManager
}

func NewPropagator(em *EventManager) *Propagator

// Propagate applies all events matured for either player. Caller
// holds state.mu (write lock).
func (p *Propagator) Propagate(state *GameState)
```

**Algorithm:**

```pseudocode
for player in {Human, Alien}:
    matured := state.Events.PopMatured(state.Clock, player)
    for evt in matured:
        view := state.Views[player]
        applyEventToView(view, state.Catalog, evt)
        evt.AppliedToView[player] = true
        if not evt.Internal:
            Events.broadcastEvent(player, evt)
            Events.broadcastSystemUpdate(player, state, evt.SystemID)
            evt.Broadcast[player] = true
```

**`applyEventToView`** — same body as today, but parameterised over
`view *PlayerView`. Two specific changes:

- The `EventFleetArrival` and `EventFleetDeparted` cases that today
  filter on `fleet.Owner == HumanOwner` no longer filter — both
  players track both owners' fleets (subject to having seen them).
  Each fleet appears in a player's view only if a matured event
  said it does.
- The `EventAlienExhausted` case is deleted.

**Error handling.** Unchanged: a missing `KnownSystem` for the event's
`SystemID` is a logged warning; the propagator does not panic.

**Dependencies.** `EventLog`, `EventManager`, `PlayerView`,
`StarCatalog`.

### 6.5 Engine

#### `srv/internal/game/engine.go`

**Purpose.** Tick loop, command enqueuing, fleet arrivals, victory
check.

**Satisfies.** FR-23, FR-25, FR-26, FR-27, FR-45, FR-46, FR-47,
FR-49, FR-74 (engine subset), NFR-1.

**Interface (changes):**

```go
type Engine struct {
    State  *GameState
    Events *EventManager
    rng    *rand.Rand

    // Per-player subscription counts, used by OnPlayerConnected /
    // OnPlayerDisconnected to drive auto-pause/auto-unpause.
    subMu     sync.Mutex
    subCount  map[Owner]int
    everPaired bool   // true once both have been simultaneously connected
}

// NewEngine drops the BotAgent argument. (FR-74)
func NewEngine(state *GameState, events *EventManager, rng *rand.Rand) *Engine

func (e *Engine) Run(ctx context.Context)

func (e *Engine) SetPaused(paused bool)

// EnqueueCommand validates against issuer's view and stamps Issuer/
// OriginID. Returns commandID, executeYear, error. (FR-25, FR-26,
// FR-27)
func (e *Engine) EnqueueCommand(issuer Owner, cmd *PendingCommand) (string, float64, error)

// OnPlayerConnected and OnPlayerDisconnected manage the auto-pause
// lifecycle. (FR-45, FR-46, FR-47)
func (e *Engine) OnPlayerConnected(p Owner)
func (e *Engine) OnPlayerDisconnected(p Owner)
```

**`EnqueueCommand` algorithm:**

```pseudocode
state.Lock(); defer state.Unlock()

cat := state.Catalog.Get(cmd.TargetID)
if cat == nil: return "", 0, errf("unknown system %q", cmd.TargetID)

// Validate against the issuer's view (FR-27).
home := state.Homes[issuer]
if cmd.TargetID != home:
    ks := state.Views[issuer].System(cmd.TargetID)
    if ks != nil and ks.Status is the opposing player's status:
        return "", 0, errf("system %q known opponent-held", cmd.TargetID)

// Compute execute year (FR-25, FR-26).
if cmd.TargetID == home:
    cmd.ExecuteYear = state.Clock
else:
    cmd.ExecuteYear = state.Clock +
        state.Catalog.Distance(home, cmd.TargetID) / CommandSpeedC

cmd.ID = state.NewCommandID()
cmd.Issuer = issuer
cmd.OriginID = home
state.PendingCmds = append(state.PendingCmds, cmd)
return cmd.ID, cmd.ExecuteYear, nil
```

**`tick` (delta from current):**

- Drop the `Alien.Exhausted`/`spawnAlienForces` block.
- Drop the `BotTickCadence` / `Bot.Tick` block.
- Add: at the end of the tick, if `Clock >= DrawYearCap` and not
  `GameOver`, `CheckVictory` will return draw; the existing wrapper
  fires `BroadcastGameOver(DrawWinner, reason)`.

**`processFleetArrivals` (delta).** The Sol-only special case for
reporter consumption becomes owner-aware:

```pseudocode
ownerHome := state.Homes[fleet.Owner]
if fleet.DestID == ownerHome and fleetIsReporterOnly(fleet):
    delete(state.truth.Fleets, fleet.ID)
    state.RecordEvent(&Event{
        EventYear: fleet.ArrivalYear,
        SystemID:  ownerHome,
        Type:      EventReporterReturn,
        Description: ...,
    }, fleet.Owner /*reportTo*/, true /*home has comm laser*/, false)
    continue
```

This satisfies FR-23.

**`OnPlayerConnected` / `OnPlayerDisconnected` algorithm:**

```pseudocode
OnPlayerConnected(p):
    subMu.Lock()
    subCount[p]++
    bothNow := subCount[Human] > 0 and subCount[Alien] > 0
    subMu.Unlock()
    if bothNow:
        State.Lock()
        everPaired = true
        if State.Paused and not State.GameOver:
            State.Paused = false
        State.Unlock()
        Events.BroadcastClockSync(state)  // FR-46

OnPlayerDisconnected(p):
    subMu.Lock()
    subCount[p]--
    nowZero := subCount[p] == 0
    subMu.Unlock()
    if nowZero and everPaired and not State.GameOver:
        State.Lock(); State.Paused = true; State.Unlock()
        log.Printf("server: %s disconnected; game paused", p)  // FR-47
        Events.BroadcastClockSync(state)
```

`SetPaused` (called by `POST /api/pause`) only works for human
players — see §6.7.

**Error handling.**

- A panic in `tick` is recovered and logged (existing behaviour).
- A panic in `applyCommand` is logged via `command_failed`.
- The bot tick is gone, so there's no longer a recover for it.

**Dependencies.** `GameState`, `EventManager`, `*rand.Rand`,
`StarCatalog`.

### 6.6 Combat, economy, loader

#### `srv/internal/game/combat.go`

**Purpose.** Resolve combat at one system per tick.

**Satisfies.** FR-2, FR-20, FR-21, FR-22, FR-74 (counter removal).

**Changes:**

- Drop all references to `state.Alien.TotalLost` and
  `AlienExhaustionThreshold` and the resulting `EventAlienExhausted`.
- `extractAndSendReporters` is generalised to "extract reporters
  for *each* side that has them, send them to the side's home."
  Concretely: the function takes both `humanUnits` and `alienUnits`
  and walks each fleet at the system; if `fleet.Units[Reporter] > 0`,
  the reporters are removed and a new in-transit reporter fleet is
  spawned heading toward `state.Homes[fleet.Owner]`. This supports
  Q21's "the reporter flees to the side it was loyal to at combat
  start."
- `reportArrivalYear` becomes per-player — but in practice only the
  owner of the *system* receives a comm-laser report (lasers belong
  to the system), and the owner of each *reporter-carrying fleet*
  receives a reporter report. The combat event itself is recorded
  with `Arrival` populated by `state.RecordEvent`:
  - If the system has a comm laser: `reportTo = ownerOfSystemAtCombatStart`,
    `hasCommLaser = true`. (FR-21 — comm laser fires its report
    *before* the combat resolves, hence the pre-combat owner.)
  - Else if a reporter fled belonging to player P: a separate
    `EventCombatOccurred` is *not* duplicated — instead the single
    combat event's `Arrival[P]` is set via the reporter's path
    (`eventYear + dist/0.8c`), and the comm-laser path remains
    `Arrival[ownerOfSystem] = eventYear + dist/1.0c` if applicable.
  - Else: `Arrival = {math.MaxFloat64, math.MaxFloat64}`, the
    event is internal-but-recorded (`EventCombatSilent`).
- `clearHumanForces` / `clearAlienForces` are unchanged in shape
  but no longer cause any view-side filtering changes (the views
  now hold both sides' fleets).

**Note on the deferred reporter-survival hazard.** The existing
`extractAndSendReporters` doc-comment in `combat.go` notes a
"reporter survival is not coupled to report delivery" hazard —
that hazard is inherited unchanged by this design. Symmetric play
does not introduce in-transit combat, so the existing safety
condition still holds. This design does not address it.

#### `srv/internal/game/economy.go`

**Purpose.** Wealth growth and econ-level progression.

**Satisfies.** FR-2 (G-3), FR-19 economic side, FR-27.

**Changes:**

- `AccumulateWealth`: replace `if sys.Status == StatusHuman` with
  `if sys.Status == StatusHuman || sys.Status == StatusAlien`.
- `AdvanceEconLevels`: same broadening. Determine which player owns
  each system from `Status`; use that to select the home for the
  econ-growth report. The growth event's `reportTo` is the system's
  owner; `hasCommLaser` is `systemHasCommLaser(truth, sys)`.
- `ValidateConstruct`: replace `if sys.Status != StatusHuman` with
  `if statusToOwner(sys.Status) != cmd.Issuer` (uses the
  command's `Issuer` so each player can only construct in their own
  systems — FR-27).
- `ExecuteConstruct`: identical body, but `EventConstructionDone`
  is recorded with `reportTo = sys.Owner` (derived from `Status`).
  `applyMobileConstructionToFleet` (in propagator.go) no longer
  hard-codes `Owner: HumanOwner`; it reads from
  `ConstructionDetails.Owner` (a new field on the details payload).

`ConstructionDetails` gains:

```go
type ConstructionDetails struct {
    WeaponType WeaponType
    Quantity   int
    FleetID    string
    FleetName  string
    Owner      Owner   // NEW: owner of the system / fleet at construction
}
```

#### `srv/internal/game/loader.go`

**Purpose.** Load CSVs, build catalogue, seed `Truth` and both
`PlayerView`s.

**Satisfies.** FR-4, FR-5, FR-6, FR-7, FR-17, FR-29, FR-30, FR-31,
FR-32, FR-33, FR-74 (loader subset).

**Interface:**

```go
// Initialize replaces today's two-arg signature. (FR-29)
//
// Loads four CSVs, builds the merged catalogue, fails fast on
// duplicate IDs (FR-31), seeds Truth with both home regions, and
// mirrors both PlayerViews from Truth at AsOfYear=0 (FR-17).
func Initialize(rng *rand.Rand,
    humanNearestCSV, humanPlanetsCSV string,
    alienNearestCSV, alienPlanetsCSV string,
) (*GameState, error)
```

**Algorithm:**

```pseudocode
1. Load human-side: hasPlanetsH := loadPlanets(humanPlanetsCSV)
                    groupsH, maxDistH := loadStars(humanNearestCSV, hasPlanetsH, asSol=true)
   Load alien-side: hasPlanetsA := loadPlanets(alienPlanetsCSV)
                    groupsA, maxDistA := loadStars(alienNearestCSV, hasPlanetsA, asSol=false)

2. Build catalogue entries from groupsH + groupsA. For each group g:
       id := toSystemID(g.DisplayName)
       entry := CatalogEntry{ID:id, ...}
   Detect duplicates: maintain a set of seen IDs while iterating
   the union; if a second insertion attempt occurs, FATAL with
   `fmt.Errorf("loader: duplicate system ID %q across human and alien catalogs", id)`
   listing the offending ID. (FR-31)

   Coordinate frame is Sol-relative (FR-32); both files express
   coordinates in the same frame, so no transformation is needed.

3. Determine home IDs:
       solID    := toSystemID("Sol")             // = "sol"
       alienHome := the IsAlienHome row in groupsA  (see "asSol=false" loader change)

   We extend `loadStars` to accept a flag `asHomeRegion bool` and
   require exactly one row in the region to be marked as the home.
   In `nearest.csv` the existing convention is "the row whose
   `catalogName == 'SUN'`" — we generalise to "the row whose
   `catalogName == 'SUN'`" for human and "the row whose
   `commonName` matches a configurable token in alien-nearest.csv"
   for alien. The simpler concrete choice: `alien-nearest.csv` MUST
   contain a row whose `commonName` is "61 Ursae Majoris", and
   `loadStars` returns the corresponding group ID as `alienHome`.

4. state.Homes = { Human: solID, Alien: alienHome }

5. Build Truth.Systems for every catalog entry:
       homeOf maps groupID → Owner (or "" if uninhabited).

       If g is the human home (Sol):       seedHome(StatusHuman, ...)
       elif g is the alien home (61 UMa):  seedHome(StatusAlien, ...)
       elif g came from human file with planets, or within
            maxDistH/2 of Sol:              owned by Human, gaussian econ
       elif g came from alien file with planets, or within
            maxDistA/2 of 61 UMa:           owned by Alien, gaussian econ
       else:                                Uninhabited

   If a system qualifies as both sides' starting system per the
   above rules, the data-prep contract (Q14) forbids this — but
   FR-31's duplicate-ID check would already have caught the
   collision at step 2. Defensive: at step 5, if a system somehow
   ends up qualifying for both, log fatal.

6. Per-side initial fleet (FR-4 / FR-5 / FR-6):
       For each system owned by Human at game start:
           create one fleet "<DisplayName>-1st Fleet" with
           Units = {Reporter: 2}; primary=this fleet.
           If g.HasPlanets: ts.LocalUnits[CommLaser] = 1.
       Symmetric for Alien with the alien CSV.
       For the home stars specifically, force EconLevel=5 and
       Wealth=64 and 1 CommLaser regardless of HasPlanets (FR-4 /
       FR-5).

7. Record per-faction initial systems (FR-55):
       state.Factions = {
           Human: &Faction{InitialSystemIDs: <all StatusHuman IDs>},
           Alien: &Faction{InitialSystemIDs: <all StatusAlien IDs>},
       }

8. Seed PlayerViews (FR-17). For p in {Human, Alien}:
       view := PlayerView{Owner: p, Systems: {}, Fleets: {}, InTransit: {}}
       for each system in Truth.Systems:
           view.Systems[id] = &KnownSystem{
               ID:         id,
               Status:     ts.Status,        // straight mirror
               AsOfYear:   0,
               EconLevel:  ts.EconLevel,
               Wealth:     ts.Wealth,
               LocalUnits: copy(ts.LocalUnits),
               FleetIDs:   copy(ts.FleetIDs), // includes BOTH sides' fleets
           }
       for each fleet in Truth.Fleets:
           view.Fleets[fid] = &KnownFleet{
               ID: fid, Name: ..., Owner: tf.Owner, Units: copy(tf.Units),
               LocationID: tf.LocationID, AsOfYear: 0,
           }
       state.Views[p] = view

9. The peripheral-systems-as-entry-points selection block,
   the SolView carve-out, and the alien-fleets-at-entry-points
   seeding from today's loader are DELETED. (FR-74)
```

**Error handling.**

- File open / CSV parse errors propagate as errors from `Initialize`.
- Duplicate IDs across catalogues → fatal error from `Initialize`
  with the offending ID(s) named (FR-31).
- Missing alien home row in `alien-nearest.csv` → fatal error with
  message `loader: alien home star "61 Ursae Majoris" not found in
  alien-nearest.csv`.

**Dependencies.** `csv`, `os`, `*rand.Rand`, `StarCatalog`.

### 6.7 HTTP layer

#### `srv/internal/server/server.go`

**Purpose.** Mux setup; middleware chain.

**Satisfies.** FR-37, FR-38, FR-39.

**Changes:**

- Add a `playerMiddleware` that runs *before* `recoverMiddleware`
  (or wraps it) on every API route except `/api/stars`.
- The handler chain becomes:
  `mux.HandleFunc("/api/state", s.recoverMiddleware(s.playerMiddleware(s.handleState)))`
  with `playerMiddleware` defined as:

```go
type ctxKey struct{}
var playerCtxKey = ctxKey{}

// playerMiddleware enforces the required ?player=human|alien query
// parameter and attaches the player ID to the request context.
// (FR-36, FR-37, FR-38)
func (s *Server) playerMiddleware(h http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var p game.Owner
        switch r.URL.Query().Get("player") {
        case "human": p = game.HumanOwner
        case "alien": p = game.AlienOwner
        default:
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusBadRequest)
            json.NewEncoder(w).Encode(map[string]any{
                "ok": false, "error": "unknown player",
            })
            return
        }
        r = r.WithContext(context.WithValue(r.Context(), playerCtxKey, p))
        h(w, r)
    }
}

// playerOf is the handler-side accessor.
func playerOf(r *http.Request) game.Owner {
    return r.Context().Value(playerCtxKey).(game.Owner)
}
```

`/api/stars` is registered with only `recoverMiddleware`, not
`playerMiddleware`, per FR-39.

`/api/debug/state` is registered with `playerMiddleware` so the
caller's identity is recorded (and the bot can pass `?player=alien`
when it uses the flag), but the handler returns the same Truth
to either caller — FR-44.

#### `srv/internal/server/handlers.go`

**Purpose.** All HTTP/SSE request handling.

**Satisfies.** FR-24, FR-25, FR-27, FR-41, FR-42, FR-44, FR-50–53.

**Concrete signature changes:**

- Every handler that today reads `s.state.SolView` reads
  `s.state.Views[playerOf(r)]` instead.
- `buildSystemDTO`'s special-case for `sysID == "sol"` becomes
  `sysID == s.state.Homes[player]`. The DTO for the player's *own*
  home is sourced from `state.ReadHomeGroundTruth(player)`; for
  every other system, from `state.Views[player].System(sysID)`.
- `handlePause`:

  ```go
  if playerOf(r) != game.HumanOwner {
      w.Header().Set("Content-Type", "application/json")
      w.WriteHeader(http.StatusForbidden)
      json.NewEncoder(w).Encode(map[string]any{
          "ok": false,
          "error": "only the human player may pause",
      })
      return
  }
  // remainder as today
  ```
  (FR-50)

- `handleEvents`:

  ```go
  player := playerOf(r)
  // ... SSE setup as today ...
  clientID := fmt.Sprintf("client-%d", clientSeq.Add(1))
  ch := s.events.Register(player, clientID)
  defer func() {
      lastInPlayer := s.events.Unregister(player, clientID)
      if lastInPlayer && !s.state.GameOver {
          s.engine.OnPlayerDisconnected(player)
      }
  }()

  s.state.RLock()
  s.events.BroadcastConnected(player, clientID, s.state)
  s.state.RUnlock()

  s.engine.OnPlayerConnected(player)
  // streaming loop unchanged
  ```
  (FR-42, FR-46, FR-47)

- `handleCommand`: build the `PendingCommand` with no `OriginID`
  (the engine sets it from `state.Homes[issuer]`); pass
  `playerOf(r)` to `EnqueueCommand`. The "is bot?" filter in
  `buildPendingCommandDTOs` is replaced by "is mine?":

  ```go
  for _, cmd := range state.PendingCmds {
      if cmd.Issuer != viewer { continue }
      // emit DTO
  }
  ```

- `handleDebugState`: unchanged in shape; no need to read player from
  context for the response.

**SSE filtering.** `EventManager.Register(player, clientID)` keeps a
`map[Owner]map[string]chan []byte`. `broadcastEvent(player, evt)` and
`broadcastSystemUpdate(player, state, sysID)` send only to channels
under that player. `BroadcastClockSync` broadcasts to *all* clients
on both maps (it must reach both sides — FR-53). `BroadcastGameOver`
likewise broadcasts to all (both sides need to know the game ended).
`BroadcastConnected(player, clientID, state)` looks up the channel
on the requested player's map only.

**Connected snapshot.** `fullStateMap` (in `events.go`) is
parameterised over `player Owner`, so the initial frame for a
freshly subscribed alien sees alien systems / fleets / pending
commands.

**Error handling.** Identical patterns to today: 400 for malformed
input, 405 for wrong method, 500 (caught by `recoverMiddleware`)
for panics. Adds 403 on `POST /api/pause` for alien (FR-50) and
400 on missing/invalid `?player`.

#### Wire labels

Per FR-10, all owner labels in JSON / SSE stay absolute (`human`,
`alien`). The `winner` field gains a fourth possible value `draw`
(FR-60); the existing string-encoding path emits whatever
`state.Winner` is set to, and `DrawWinner = "draw"` flows through
unchanged.

#### `server_api.md`

**Changes per FR-77:**

- Document `?player=human|alien` requirement on every endpoint
  except `GET /api/stars`.
- Document the `400 unknown player` response.
- Document the `403 only the human player may pause` response on
  `POST /api/pause` for alien.
- Remove `alien_exhausted` from the event-types table.
- State explicitly that `/api/state` and `/api/events` are scoped
  to the requesting player; owner labels remain absolute.
- Update the `winner` enum to include `draw`.

### 6.8 Bot binary

#### `srv/cmd/spacebot/main.go`

**Purpose.** Bot process entry point.

**Satisfies.** FR-61, FR-64, FR-69, FR-73.

**Interface (CLI):**

```
spacebot [flags]

Flags:
    --server         Base URL of the server (default: http://127.0.0.1:8080)
    --debug-truth    If set, also fetch /api/debug/state periodically
                     and write the response to the bot log. Decision
                     logic does NOT consume this. (FR-73)
    --log            Path to log file (default: stderr)
```

`--server` exists so the bot can be pointed at an alternate port
during tests; defaults to the production loopback address.

**Algorithm:**

```pseudocode
parse flags
ctx, cancel := signal.NotifyContext(context.Background(), SIGINT, SIGTERM)
defer cancel()

client := bot.NewClient(*serverFlag, "alien")
local  := bot.NewLocal(client)
agent  := bot.NewAgent(client, local, *debugTruthFlag)

// SSE keeps Local fresh; Agent decides commands.
go local.Run(ctx)   // streams events, updates Local

agent.Run(ctx)      // decides on a cadence; calls client.PostCommand
```

#### `srv/internal/bot/client.go`

**Purpose.** Typed HTTP/SSE client over the public API.

**Satisfies.** FR-63, FR-64, FR-65, FR-66, FR-73.

**Interface:**

```go
// Client speaks the public HTTP/SSE API as one specific player.
// It does NOT import any srv/internal/game types. Wire shapes are
// declared locally in this package. (FR-63)
type Client struct {
    BaseURL  string
    Player   string             // "human" or "alien"
    HTTP     *http.Client
}

func NewClient(baseURL, player string) *Client

func (c *Client) GetStars(ctx context.Context) ([]Star, error)
func (c *Client) GetState(ctx context.Context) (*StateSnapshot, error)
func (c *Client) PostCommand(ctx context.Context, cmd CommandRequest) (*CommandResponse, error)
func (c *Client) GetDebugState(ctx context.Context) (*DebugSnapshot, error)

// StreamEvents opens an SSE subscription and calls handler for each
// frame until ctx is cancelled or the connection drops. Returns the
// terminal error.
func (c *Client) StreamEvents(ctx context.Context, handler func(EventFrame)) error
```

**Wire-shape mirrors** (separate file or top of `client.go`):

```go
type Star struct { ID, DisplayName string; X, Y, Z, DistFromSol float64; HasPlanets, IsSol bool }
type StateSnapshot struct {
    GameYear  float64
    Paused    bool
    GameOver  bool
    Winner    string
    WinReason string
    Systems   []SystemView
    Events    []EventView
    PendingCommands       []PendingCommandView
    HumanFleetsInTransit  []FleetView   // sic; field name retained from server (see note)
}
type SystemView struct {
    ID, DisplayName string
    KnownStatus     string
    KnownAsOfYear   float64
    KnownEconLevel  int
    KnownWealth     float64
    KnownLocalUnits map[string]int
    KnownFleets     []FleetView
}
// etc.
```

(Note: the server's `humanFleetsInTransit` field name is symmetric-
unfriendly. Per FR-77 / FR-10 we rename the DTO field to
`fleetsInTransit` (player-scoped: contains the requesting player's
in-transit fleets). The bot uses that renamed field.)

**Algorithm — `StreamEvents`:**

```pseudocode
url := BaseURL + "/api/events?player=" + Player
req := http.NewRequestWithContext(ctx, GET, url, nil)
req.Header.Set("Accept", "text/event-stream")
resp := HTTP.Do(req); defer resp.Body.Close()
scanner := bufio.NewReader(resp.Body)
loop:
    read SSE frames (eventName + JSON data); call handler(EventFrame{name, data})
on error: return err
```

**Error handling.**

- HTTP non-2xx → return error wrapping the body.
- SSE drop → return; caller decides whether to retry. Per FR-48
  reconnect is not supported, so the bot's response to a stream
  drop is to log and exit. (The bot is "the alien" — when it goes
  away, the engine pauses anyway.)
- A 400 `unknown player` should never happen and is treated as a
  fatal misconfiguration.

**Dependencies.** `net/http`, `bufio`, `encoding/json`, `context`.

#### `srv/internal/bot/state.go`

**Purpose.** Maintain the bot's local mirror of its `PlayerView` by
applying SSE frames as they arrive.

**Satisfies.** FR-65, FR-72.

**Interface:**

```go
type Local struct {
    mu        sync.Mutex
    Year      float64
    Paused    bool
    GameOver  bool
    Winner    string
    Stars     map[string]Star
    Systems   map[string]SystemView
    Fleets    map[string]FleetView
    InTransit map[string]FleetView
}

func NewLocal(client *Client) *Local
func (l *Local) Run(ctx context.Context) error         // streams + applies
func (l *Local) Snapshot() LocalSnapshot               // copy under mu
```

**Algorithm — `Run`:**

```pseudocode
stars := client.GetStars(ctx)        // FR-35
l.Stars = index(stars)
client.StreamEvents(ctx, func(frame EventFrame) {
    l.mu.Lock(); defer l.mu.Unlock()
    switch frame.Name:
    case "connected": l.applyConnected(frame.Data)   // initial snapshot
    case "clock_sync": l.Year, l.Paused = ...
    case "game_event": l.applyGameEvent(frame.Data)
    case "system_update": l.applySystemUpdate(frame.Data)
    case "game_over": l.GameOver = true; l.Winner = ...
})
```

`applyConnected` replaces `Systems`/`Fleets` wholesale.
`applySystemUpdate` overwrites `Systems[sysID]`.
`applyGameEvent` is decorative — used for logging — because every
state-shifting frame is followed by a `system_update` from the
server.

**Error handling.** Decode errors are logged and skipped; the goal
is to never crash the agent on a malformed frame. A stream-level
error returns from `Run` and the bot exits.

#### `srv/internal/bot/agent.go`

**Purpose.** The v1 alien strategy.

**Satisfies.** FR-70, FR-71, FR-72, FR-73.

**Interface:**

```go
type Agent struct {
    Client     *Client
    Local      *Local
    DebugTruth bool
    Logger     *log.Logger

    // Internal cadence
    ticker     *time.Ticker
    homeID     string             // = "61-uma" or whatever AS-2 yields
    rng        *rand.Rand

    // Per-tick reserve for non-escort production (FR-70.1)
    minWealthReserve float64
}

func NewAgent(c *Client, l *Local, debugTruth bool) *Agent

func (a *Agent) Run(ctx context.Context) error
```

**Algorithm — `Run`:**

```pseudocode
homeID := find(stars, "61 Ursae Majoris")  // by display name
a.homeID = homeID

ticker := time.NewTicker(BotDecisionInterval)  // e.g. 2 real seconds
defer ticker.Stop()

for:
    select ctx.Done(): return
           ticker.C:   a.decide(ctx)
           // optional debug-truth fetch every ~10s if enabled
```

**Algorithm — `decide(ctx)`:**

```pseudocode
snap := Local.Snapshot()
if snap.Paused or snap.GameOver: return

home := snap.Systems[homeID]  // ground-truth-of-home, as the server projects it

// FR-70.1 — replenish escorts at home
if home.KnownWealth >= EscortCost && reserveOK(home):
    Client.PostCommand(ctx, CommandRequest{
        Type: "construct", SystemID: homeID,
        WeaponType: "escort", Quantity: 1,
    })

// FR-70.2 — pick most-recently-known opponent system; dispatch fleet
opponentSys := argMaxAsOfYear(snap.Systems, status="human")
if opponentSys != nil:
    fleet := pickReadyFleet(snap, atSystem=homeID,
                            requiring=[escort, battleship])
    if fleet != nil:
        Client.PostCommand(ctx, CommandRequest{
            Type: "move", SystemID: homeID,
            FleetID: fleet.ID, DestinationID: opponentSys.ID,
        })

// FR-70.3 — slow reporter sweep (every Nth decide tick)
if a.tickCount % ReporterSweepCadence == 0:
    target := oldestUnobservedOpponent(snap)
    if target != nil:
        // Build a reporter at home; queue its move next tick.
        Client.PostCommand(ctx, CommandRequest{
            Type: "construct", SystemID: homeID,
            WeaponType: "reporter", Quantity: 1,
        })
        // Subsequent ticks: dispatch the new reporter via PostCommand
        // CmdMove. (Tracked via fleet AsOfYear deltas.)
```

The exact dispatch logic for sending an existing fleet vs.
creating one is implementation detail; the algorithm above is the
skeleton.

**Per FR-71** the agent has no tree search, no opponent modelling
beyond reading `KnownSystem.AsOfYear`, and no economic optimisation.

**Per FR-72** every read is from `Local.Snapshot()`, never from
`Client.GetDebugState`.

**Per FR-73** if `DebugTruth` is set, a separate goroutine (in
`Run`) calls `Client.GetDebugState` every 10 s and writes the
result to `Logger`. The agent's `decide` does not see this output.

**Error handling.** A `PostCommand` rejection (400) is logged and
the bot moves on. A 5xx is logged and the bot continues. Repeated
failures over a window are not specially handled in v1.

**Dependencies.** `Client`, `Local`, `time`, `context`, `log`,
`math/rand`.

### 6.9 SPA changes

**Modified files:** `web/src/api.js`, `web/src/main.js` (or wherever
the `EventSource` is opened).

**Required change.** Append `?player=human` to every `/api/*` URL
the SPA constructs. `/api/stars` MAY also include `?player=human`
(harmless — middleware is not registered there) but the design
recommends omitting it to match the server contract precisely.

**No change** to rendering: owner labels are still `human` /
`alien`, `knownStatus` values are unchanged, the catalogue is
already merged on the server side (FR-30), and Three.js renders
whatever is in `/api/stars`.

**Re-build.** Per `CLAUDE.md`, after editing `web/src/*` run
`scripts/build-frontend.sh` and commit `web/dist/` together with the
source change.

### 6.10 Launch script

#### `scripts/run-game.sh`

**Purpose.** Start server, wait, start bot.

**Satisfies.** FR-69.

**Behaviour:**

```bash
#!/usr/bin/env bash
set -euo pipefail

# Build (idempotent).
go build -o ./spacegame ./srv/cmd/spacegame
go build -o ./spacebot  ./srv/cmd/spacebot

# Server.
./spacegame &
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true' EXIT

# Wait until reachable.
for i in {1..50}; do
    if curl -fsS http://127.0.0.1:8080/api/stars >/dev/null 2>&1; then
        break
    fi
    sleep 0.1
done

# Bot.
./spacebot --server http://127.0.0.1:8080
```

The bot exits when its SSE drops (server shut down or bot Ctrl-C'd);
the trap kills the server. Either side can be killed independently
for debugging — the surviving side stays up.

---

## 7. Data Model

### 7.1 In-memory data structures (changes)

| Type | Change | Notes |
|------|--------|-------|
| `GameState` | + `Views map[Owner]*PlayerView`, `Factions map[Owner]*Faction`, `Homes map[Owner]string`. − `SolView`, `Human`, `Alien`. | See §6.2 |
| `PlayerView` | New (rename of `SolView`). + `Owner` field. Fleets/InTransit hold both sides' fleets. | See §6.4 |
| `Event` | + `Arrival map[Owner]float64`, `AppliedToView map[Owner]bool`, `Broadcast map[Owner]bool`. − `ArrivalYear`, `AppliedToView` (scalar), `Broadcast` (scalar). | See §6.4 |
| `EventLog` | + `pending map[Owner]*eventHeap`. − single `pending`. | See §6.4 |
| `PendingCommand` | + `Issuer Owner`. − `IsBot`. | See §6.2 |
| `Faction` | New unified type. Single field `InitialSystemIDs []string`. Replaces `HumanFaction`, `AlienFaction`. | See §6.1 |
| `ConstructionDetails` | + `Owner Owner`. | See §6.6 |
| `EventManager` | + `clients map[Owner]map[string]chan []byte` (replaces flat map). | See §6.7 |

### 7.2 CSV data files (on disk)

- `nearest.csv`, `planets.csv` — unchanged.
- `alien-nearest.csv`, `alien-planets.csv` — NEW; same schema. The
  file-content production is out of scope per AS-1.

### 7.3 Wire-format additions / changes

| Endpoint | Change |
|----------|--------|
| All `/api/*` except `/api/stars` | Require `?player=human|alien` |
| `GET /api/stars` | Unchanged |
| `GET /api/state` | Scoped to requesting player; `humanFleetsInTransit` field renamed `fleetsInTransit` (player's own in-transit fleets only) |
| `GET /api/events` | Scoped to requesting player |
| `POST /api/command` | Origin is requesting player's home, not always `sol` |
| `POST /api/pause` | 403 if `player=alien` |
| `GET /api/debug/state` | Unchanged |
| Wire event `alien_exhausted` | REMOVED |
| Wire field `winner` on `game_over` | New value `draw` possible |

### 7.4 Migration strategy

Single landing: the asymmetric game stops working the moment this
work lands. There is no save-game format to migrate. The only on-
disk artefacts are the CSV files (new data files needed) and the
embedded `web/dist/` (rebuilt as part of this work).

---

## 8. Key Design Decisions

| Decision | Alternatives Considered | Choice | Rationale |
|----------|------------------------|--------|-----------|
| Per-player arrival times on a single shared `Event` | (a) Two parallel `EventLog`s, one per player. (b) A single `Event` with a per-player map of `Arrival` and `Broadcast` flags. | (b) | A single ground-truth event source matches the existing engine architecture; per-player state on the Event itself avoids cross-log synchronisation and keeps the dual-state refactor's invariants intact. (NFR-2) |
| Per-player heaps in `EventLog` | (a) One heap, scan for "matured for either player." (b) Two heaps. | (b) | Two heaps keep `PopMatured` O(log n) per player and avoid spurious wakeups when only one player's frontier moves. |
| Rename `SolView` → `PlayerView`, used twice | (a) Keep `SolView` and add `BotView` mirror. (b) One generic `PlayerView`. | (b) | FR-2 requires symmetry. One type used twice is the only way to ensure the two sides cannot drift in shape; matches FR-8's faction unification. |
| Player identity carrier: query string | (a) `X-SpaceGame-Player` header. (b) `POST /api/join` token. (c) Path prefix `/api/human/...`. (d) Query string. | (d) | Mandated by FR-36 / Q1. Lowest churn: trivially testable with `curl`, requires no client-side header injection in the SPA, and uniformly applies to GET/POST/SSE. |
| Identity validation in middleware | (a) Each handler parses the query string. (b) Middleware. | (b) | DRY; the validation happens once and the player ID flows via `context.Value`. Mirrors `recoverMiddleware` (FR-38). |
| Auto-start the engine when both SSE streams are open | (a) Explicit `POST /api/start`. (b) Auto-start on second SSE open. | (b) | Mandated by FR-46 / Q2. Avoids an extra wire endpoint and an extra state machine. |
| Auto-pause on disconnect; no reconnect | (a) Tear down session on disconnect. (b) Pause and wait. (c) Pause, write log line, do not accept reconnect. | (c) | Mandated by FR-47 / FR-48 / Q3 / Q4. Defers the reconnect engineering cost until v2 while keeping the session inspectable in its paused state. |
| Pause restricted to human | (a) Either may pause. (b) Human only. (c) Admin endpoint. | (b) | Mandated by FR-50 / Q5. The bot has no reason to pause and doing so without a UI gesture is a footgun. |
| Symmetric victory: capture-of-home OR ≥X% of opponent's *initial* systems | (a) Holdings-of-all-systems threshold. (b) Capture only. (c) Capture + initial-systems threshold. | (c) | Mandated by FR-54 / FR-55 / Q7. Mirrors today's human-side win exactly and produces a symmetric alien-side win. |
| Draw on game-length cap | (a) Mutual exhaustion counter. (b) Stable-equilibrium detector. (c) Game-length cap. | (c) | Mandated by FR-57 / Q9 (deferred to playtesting; simplest first). Keeps `CheckVictory` trivial and tunable. |
| Single shared `WeaponDefs` and economy rules | (a) Side-specific tunables. (b) One shared table. | (b) | Mandated by FR-2 / Q19. Side-specific tunables are exactly the asymmetry being removed. |
| Bot is a separate Go binary in this repo, public-API only | (a) In-process bot in a separate goroutine. (b) Auto-spawned subprocess. (c) Independent binary. | (c) | Mandated by FR-61, FR-63, FR-68 / Q22, Q23. Forces the public API to be the contract; sets up the RL future direction (NFR-16) without making it a v1 deliverable. |
| Manual launch script, not server-as-supervisor | (a) `--with-bot` flag spawning a child process. (b) Shell script. | (b) | Mandated by FR-68, FR-69 / Q23. Saves us the engineering tax of child-process lifecycle, signal forwarding, and log multiplexing in v1. |
| Keep `Human`/`Alien` labels in code and on the wire | (a) Rename to `Player1`/`Player2` or `Self`/`Foe`. (b) Keep absolute. | (b) | Mandated by FR-9, FR-10, Q27, Q28. Pure churn vs. zero functional benefit; logs and wire shapes stay unambiguous. |
| Reports route to *one* player (the owner) | (a) Both players see all reports they could plausibly see. (b) Owner-only. | (b) | Mandated by FR-20, FR-21, Q21. Matches the canonical "comm laser belongs to the system, reporter belongs to the fleet" intuition. Multi-player observability is not requested. |
| Loader fails fast on duplicate IDs | (a) Silently coalesce. (b) Pick first-seen. (c) Fail fast. | (c) | Mandated by FR-31 / Q14. Silent coalescing hides data-prep bugs and corrupts coordinates; explicit failure is the right tradeoff. |
| Coordinate frame stays Sol-relative | (a) Re-origin to a midpoint. (b) Dual frames. (c) Stay Sol-relative. | (c) | Mandated by FR-32 / Q15. The SPA's existing rendering already assumes this. |
| `ReadHomeGroundTruth(player)` for live home wealth | (a) Drive home wealth via events. (b) Live read for the home only. | (b) | Wealth accumulates continuously, not on events. The home-only live read is a single, grep-able escape hatch from `Truth`, mirroring today's `ReadSolGroundTruth`. (DR-1 spirit preserved.) |
| Sentinel `DrawWinner = "draw"` on `Owner` | (a) Separate enum for game outcome. (b) `Owner` sentinel. | (b) | One field, one wire shape, no DTO branching. The sentinel value is never used as a real owner; this is grep-clear from its single declaration site. |

---

## 9. File and Directory Plan

### CREATE

- `srv/cmd/spacebot/main.go` — bot binary entry point. (FR-61)
- `srv/internal/bot/agent.go` — v1 strategy. (FR-62, FR-70)
- `srv/internal/bot/client.go` — typed HTTP/SSE client. (FR-62,
  FR-63)
- `srv/internal/bot/state.go` — local mirror of bot's view. (FR-62,
  FR-72)
- `scripts/run-game.sh` — server-then-bot launcher. (FR-69)
- `MULTI/design.md` — this document.

### MODIFY

- `srv/cmd/spacegame/main.go` — drop `NewDefaultBot()` wiring; add
  alien CSV path env vars (`SPACEGAME_ALIEN_NEAREST_CSV`,
  `SPACEGAME_ALIEN_PLANETS_CSV`); call the new `Initialize` with
  four paths. (FR-29, FR-74)
- `srv/internal/server/server.go` — add `playerMiddleware` and
  apply it to all routes except `/api/stars`. (FR-37, FR-38, FR-39)
- `srv/internal/server/handlers.go` — every handler reads
  `playerOf(r)`; `handleState` returns the requesting player's
  view; `handleCommand` passes player ID to `EnqueueCommand`;
  `handlePause` 403s on alien; `handleEvents` registers per-player
  channel and calls engine connect/disconnect hooks. (FR-24, FR-25,
  FR-27, FR-41, FR-42, FR-46, FR-47, FR-50)
- `srv/internal/server/types.go` — rename
  `humanFleetsInTransit` → `fleetsInTransit`. (FR-10 / FR-77)
- `srv/internal/game/state.go` — `Views`, `Factions`, `Homes`
  fields; `Faction` type; `ReadHomeGroundTruth`; symmetric
  `CheckVictory`; `RecordEvent` helper; `PendingCommand.Issuer`.
  (FR-8, FR-12, FR-25, FR-44, FR-54–FR-57, FR-60, FR-74)
- `srv/internal/game/truth.go` — comments updated. (no functional
  change)
- `srv/internal/game/solview.go` → `srv/internal/game/playerview.go`
  (rename); `SolView` → `PlayerView`; remove
  `SolGroundTruthSnapshot` (replaced by `HomeGroundTruthSnapshot`).
  (FR-12)
- `srv/internal/game/eventlog.go` — per-player heaps;
  per-player matured cursor; new `Event.Arrival` map. (FR-13,
  FR-14, FR-15, NFR-2)
- `srv/internal/game/propagator.go` — per-player propagation;
  `applyEventToView` parameterised over view; remove
  `EventAlienExhausted` handler; remove `Owner == HumanOwner`
  filters in fleet handlers. (FR-14, FR-16, FR-74)
- `srv/internal/game/engine.go` — drop `Bot` field, drop bot tick,
  drop `applyBotCommand`, drop `spawnAlienForces`; generalise
  reporter-arrives-at-home; add
  `OnPlayerConnected`/`OnPlayerDisconnected`. (FR-23, FR-46, FR-47,
  FR-74)
- `srv/internal/game/events.go` — per-player channel registry;
  `broadcastEvent(player, evt)` and
  `broadcastSystemUpdate(player, state, sysID)`; drop
  `alien_exhausted`. (FR-16, FR-42, FR-74)
- `srv/internal/game/types.go` — drop `EventAlienSpawn`,
  `EventAlienExhausted`. (FR-74)
- `srv/internal/game/economy.go` — `AccumulateWealth` and
  `AdvanceEconLevels` apply to both sides; `ValidateConstruct`
  checks `cmd.Issuer` rather than `StatusHuman`;
  `ConstructionDetails.Owner`. (FR-2, FR-27)
- `srv/internal/game/combat.go` — drop exhaustion-counter and
  `EventAlienExhausted` recording; report routing per FR-20 /
  FR-21; `extractAndSendReporters` symmetric. (FR-20, FR-21,
  FR-22, FR-74)
- `srv/internal/game/loader.go` — load four CSVs, merge, detect
  duplicate IDs, seed both home regions, mirror both views. (FR-4,
  FR-5, FR-6, FR-7, FR-17, FR-29, FR-30, FR-31, FR-74)
- `srv/internal/game/constants.go` — add `WinRetentionFraction`,
  `DrawYearCap`; remove asymmetric constants. (FR-58, FR-59,
  FR-74, NFR-15)
- `web/src/api.js` (and any other client file constructing API
  URLs) — append `?player=human` to every `/api/*` URL. (NFR-10)
- `web/dist/*` — rebuilt by `scripts/build-frontend.sh`. (CLAUDE.md)
- `server_api.md` — full update per FR-77.
- Tests under `srv/internal/game/*_test.go` — see §11.

### DELETE

- `srv/internal/game/bot.go` — entire file. (FR-74)

### NOT MODIFIED

- `srv/internal/game/catalog.go`
- `OLD_SPECS/*` (FR-78)
- `MULTI/MultiplayerPrompt`, `MULTI/SymmetricMultiplayerOverview.md`,
  `MULTI/futureprompt.md`, `MULTI/requirements.md`
- `nearest.csv`, `planets.csv`
- `proto/*`, `tools/gendata/*`

---

## 10. Requirement Traceability

| Requirement | Design Section | Files |
|-------------|---------------|-------|
| FR-1 | 6.2, 6.7 | `state.go`, `handlers.go`, `server.go` |
| FR-2 | 6.6 | `economy.go`, `combat.go`, `constants.go` |
| FR-3 | 6.5 | `engine.go` (unchanged invariant) |
| FR-4, FR-5 | 6.6 | `loader.go`, `constants.go` |
| FR-6, FR-7 | 6.6 | `loader.go` |
| FR-8 | 6.1, 6.2 | `state.go` |
| FR-9, FR-10 | 6.1, 6.7 | `types.go`, `handlers.go`, wire shapes preserved everywhere |
| FR-11 | 6.3 | `truth.go` (unchanged shape) |
| FR-12 | 6.4 | `playerview.go`, `state.go` |
| FR-13, FR-14, FR-15 | 6.4 | `eventlog.go` |
| FR-16 | 6.4, 6.7 | `propagator.go`, `events.go` |
| FR-17 | 6.6 | `loader.go` |
| FR-18 | 6.4 | `propagator.go` |
| FR-19 | 6.1, 6.6 | `types.go` (retained), `loader.go` (does not produce it) |
| FR-20, FR-21, FR-22 | 6.6 | `combat.go`, `eventlog.go` (`RecordEvent`) |
| FR-23 | 6.5 | `engine.go` (`processFleetArrivals`) |
| FR-24 | 6.7 | `handlers.go` |
| FR-25, FR-26 | 6.5 | `engine.go` (`EnqueueCommand`) |
| FR-27 | 6.5, 6.6 | `engine.go`, `economy.go` (`ValidateConstruct`) |
| FR-28 | 6.5 | `engine.go` (existing arrival-time semantics preserved) |
| FR-29, FR-30, FR-31, FR-32, FR-33, FR-34 | 6.6 | `loader.go`, `catalog.go` |
| FR-35 | 6.7 | `server.go`, `handlers.go` (`/api/stars`) |
| FR-36, FR-37, FR-38, FR-39, FR-40 | 6.7 | `server.go` (`playerMiddleware`) |
| FR-41 | 6.7 | `handlers.go` (`handleState`) |
| FR-42 | 6.7 | `handlers.go` (`handleEvents`), `events.go` |
| FR-43 | 6.7 | `events.go` (`Register`) |
| FR-44 | 6.7 | `handlers.go` (`handleDebugState`) |
| FR-45, FR-46, FR-47 | 6.5, 6.7 | `engine.go`, `handlers.go` |
| FR-48, FR-49 | 6.5 | `engine.go` |
| FR-50, FR-51, FR-52, FR-53 | 6.5, 6.7 | `engine.go`, `handlers.go`, `events.go` |
| FR-54, FR-55, FR-56 | 6.2 | `state.go` (`CheckVictory`) |
| FR-57, FR-58, FR-59 | 6.2, 6.1 | `state.go`, `constants.go` |
| FR-60 | 6.2, 6.7 | `state.go` (`DrawWinner`), `events.go` |
| FR-61 | 6.8 | `srv/cmd/spacebot/main.go` |
| FR-62 | 6.8 | `srv/internal/bot/{agent,client,state}.go` |
| FR-63 | 6.8 | `client.go` (no `srv/internal/game` import) |
| FR-64, FR-65, FR-66 | 6.8 | `client.go`, `agent.go` |
| FR-67 | 6.8 | `agent.go` (no pause path; tolerates lateness) |
| FR-68, FR-69 | 6.10 | `scripts/run-game.sh` |
| FR-70, FR-71, FR-72 | 6.8 | `agent.go`, `state.go` (bot side) |
| FR-73 | 6.8 | `client.go`, `agent.go` |
| FR-74 | 6.1, 6.2, 6.5, 6.6 | `bot.go` (deleted), `engine.go`, `state.go`, `loader.go`, `constants.go`, `types.go`, `combat.go` |
| FR-75 | All | No alternate code path lands. |
| FR-76 | 11 | Tests rewritten / removed |
| FR-77 | — | `server_api.md` |
| FR-78 | — | `OLD_SPECS/` not touched |
| NFR-1 | 6.5 | `engine.go` (unchanged invariant) |
| NFR-2 | 6.4 | `eventlog.go` (heap with stable tiebreak) |
| NFR-3 | 6.1 | `constants.go` (cadence unchanged) |
| NFR-4 | 6.7 | `server.go` (unchanged listen address) |
| NFR-5 | 6.7 | `playerMiddleware` is identification only |
| NFR-6 | 9 | `srv/cmd/spacebot/main.go` build path |
| NFR-7 | 11 | Test suite |
| NFR-8 | 9 | Package layout preserved |
| NFR-9 | All | Code style preserved per existing convention |
| NFR-10, NFR-11, NFR-12 | 6.9 | `web/src/*` |
| NFR-13 | 6.5 | `engine.go` (`OnPlayerDisconnected` log line) |
| NFR-14 | 6.7, 6.8 | No routine logging of state added; debug-truth opt-in |
| NFR-15 | 6.1 | `constants.go` |
| NFR-16 | 6.8 | Bot is a separate binary speaking the public API |

All requirements are covered. There is no requirement marked
"deferred" or "covered by existing code without modification"
beyond what the asymmetry-removal already implies.

---

## 11. Testing Strategy

### 11.1 Layers

- **Unit tests** under `srv/internal/game/`:
  - `eventlog_test.go` — extend to cover per-player heaps:
    record an event with `Arrival[Human] < Arrival[Alien]`, pop
    matured at a clock between the two, assert the event appears
    only on the human's stream.
  - `playerview_test.go` (renamed from `solview_test.go`) — the
    initial mirror is exact for both views; subsequent application
    of a matured event mutates only the right view.
  - `propagator_test.go` — both heaps drained; broadcast routing
    correct; the `unknown` system case still warns + skips; opp
    fleet arriving at a system known to player but not player-
    owned is correctly stored in the player's view as an opponent
    fleet.
  - `combat_test.go` (new or moved tests from `game_test.go`) —
    a comm laser at an alien-held system reports to the alien's
    home; a reporter loyal to the human flees to Sol; ownership
    at the moment of reporting is preserved across same-tick
    capture.
  - `loader_test.go` (new) — duplicate-ID detection fails fast;
    mirrored views at t=0 are exact; both home stars seeded
    correctly; `Faction.InitialSystemIDs` populated for both
    sides.
  - `state_test.go` — `CheckVictory` returns the right `Owner`
    in each of: human captures 61 UMa, alien captures Sol, human
    holds X% of alien initial, alien holds X% of human initial,
    `Clock` exceeds `DrawYearCap` with neither condition met.

- **Integration tests** under `srv/internal/game/integration_test.go`
  (rewritten):
  - Spin up a `GameState` via a test loader (small fake CSVs);
    drive ticks; verify that two ground-truth events at far-apart
    home regions reach the correct player at the correct year.
  - Two-player command flow: human enqueues a `Move` from Sol;
    alien enqueues a `Move` from 61 UMa; arrival times differ by
    `dist/0.8c`.

- **Server-level tests** (new, optional but recommended) under
  `srv/internal/server/`:
  - `playerMiddleware` rejects missing / invalid `?player`.
  - `/api/stars` returns identical responses for both players and
    when `?player` is omitted.
  - `POST /api/pause` 403s for alien, succeeds for human.
  - `handleEvents` registers under the right player; a state-
    change event recorded at the human's home appears only on the
    human's channel; reciprocally for alien.
  - End-to-end: open both SSE subscriptions, observe engine
    auto-unpause; close one, observe auto-pause and the
    well-formatted log line.

- **Bot tests** under `srv/internal/bot/`:
  - `client_test.go` — round-trip a fake server; verify
    `?player=alien` is appended to every URL.
  - `state_test.go` — `applyConnected` / `applySystemUpdate` /
    `applyGameEvent` produce the expected mirror.
  - `agent_test.go` — given a hand-crafted `Local`, verify the
    expected `PostCommand` calls fire (FR-70.1, FR-70.2,
    FR-70.3 — three separate test cases).

- **Manual / smoke testing.** Run `scripts/run-game.sh` and verify:
  - Engine remains paused until both clients connect.
  - The SPA shows alien-region stars on the map.
  - The bot's `/api/state?player=alien` shows the alien view.
  - Disconnecting the SPA pauses the engine and prints the log
    line.
  - Capturing the opponent's home produces the right `game_over`
    event on both streams.

### 11.2 Edge cases

- An event whose `Arrival[Human]` and `Arrival[Alien]` are both
  finite but unequal (e.g. combat at a system equidistant from
  both homes within a small ε) — both heaps should mature it at
  the right clock; both broadcasts should fire at the right time.
- A reporter fleet that flees from a system at the moment of
  capture: `Owner` at fleet creation is the *previous* owner's
  faction (AS-4); the report routes to that side.
- Comm laser destruction concurrent with the report it carries
  (existing semantics; AS-4): the report is recorded before the
  laser is destroyed; routing uses pre-combat ownership.
- `Catalog.Distance` between identical IDs = 0; `dist/0.8c` = 0;
  command at one's own home executes the same tick.
- `WinRetentionFraction` reached on the same tick as
  `Clock >= DrawYearCap`: `CheckVictory` evaluates capture-of-
  home first, then initial-systems, then draw — so a real win
  pre-empts a draw at the same tick.
- Bot starts before second SSE subscription is open: bot's
  `StreamEvents` succeeds (server is listening, just paused);
  bot polls `Local.Snapshot` and sees `Paused=true`; `decide`
  returns early until it sees `Paused=false`.

### 11.3 Verifying each requirement

The traceability table in §10 maps each requirement to the design
section and file(s); the test cases above target each row at least
once. Specifically:

- FR-46, FR-47 by the auto-pause/unpause server-level test.
- FR-50 by the pause-403 server-level test.
- FR-31 by the duplicate-ID loader test.
- FR-17 / FR-18 by the loader and propagator tests.
- FR-20 / FR-21 by the combat tests.
- FR-54 / FR-55 / FR-57 / FR-60 by the `CheckVictory` state tests.
- FR-70 by three discrete agent tests, one per behaviour.
- FR-37 by middleware unit tests.

---

## 12. Open Questions

1. **A-1 (Sol econ level 5 vs. 4).** This design takes the
   requirements at face value (level 5). If the stakeholder
   intended Sol to remain at level 4, FR-4 and FR-5 (and Q20 in
   the overview) need amendment. Implementation is one constant
   per side; no architectural impact.

2. **AS-2 (alien home ID).** The data-prep contract assumes
   `alien-nearest.csv` will contain a row whose `commonName` is
   `61 Ursae Majoris`. If the data team chooses a different
   preferred name (e.g. just `61 UMa`), the loader's "find the
   alien home" logic needs to match the actual string. The design
   uses display-name matching against the constant
   `AlienHomeDisplayName = "61 Ursae Majoris"`; if this needs to
   change, it is a single-line edit but worth confirming with
   data prep.

3. **AS-6 (`DrawYearCap = 500`).** This is a guess. The overview
   explicitly defers tuning to playtesting. Implementation
   accepts whatever value; flagging here so the developer
   doesn't treat 500 as load-bearing.

4. **Bot decision interval (`BotDecisionInterval`)** in `agent.go`.
   The requirements do not specify how often the bot wakes up to
   decide. The design uses 2 real seconds as a starting point.
   This is a tuning parameter; not load-bearing.

These items should not block implementation. They are tunable
constants or string literals, all of which are localised and
clearly named in the design.
