# Requirements: Symmetric Multiplayer

## 0. Document conventions

- Each requirement has a stable ID. Functional requirements use the prefix
  `FR-`; non-functional requirements use the prefix `NFR-`. IDs are not
  reused, even if a requirement is later withdrawn.
- "MUST", "MUST NOT", "SHOULD", "SHOULD NOT", and "MAY" carry their RFC 2119
  meanings.
- "The server" means the Go process built from `srv/cmd/spacegame/`.
- "The bot" means the Go process built from `srv/cmd/spacebot/` (a new
  binary added by this work).
- "The SPA" means the JavaScript single-page application served from the
  server's embedded `web/dist/`.
- "Player" means one of the two participants in a game session, identified
  by the value `human` or `alien`.
- "Home star" means Sol for the human player and 61 UMa for the alien
  player.
- "Truth" refers to the authoritative `GameState.truth` held by the server.
- "View" refers to a player-specific filtered projection of Truth (the
  `PlayerView` introduced by this work).
- All requirements describe the system *after* the symmetric-multiplayer
  work has landed. There is no transitional state and no parallel mode
  with the previous asymmetric game.
- Source decisions in `MULTI/SymmetricMultiplayerOverview.md` are referenced
  parenthetically as `Q<n>` where useful; the overview's §7 is the source
  of truth for rationale.

---

## 1. Functional requirements

### 1.1 Game model and symmetry

- **FR-1.** The server MUST host a single two-player game session, with
  exactly one human player (home: Sol) and exactly one alien player
  (home: 61 UMa). More than two players are not supported. (Out-of-scope
  §8.)
- **FR-2.** Both players MUST be subject to identical game mechanics. The
  rules governing economy, weapon construction, fleet movement, combat,
  reporter behavior, comm-laser behavior, command travel delay, and
  information delay MUST be the same for both sides; no rule may apply to
  one player and not the other.
- **FR-3.** The server MUST be the sole authoritative simulator. Neither
  player's client process may mutate `GameState`; clients communicate with
  the server only over the public HTTP/SSE API.
- **FR-4.** The alien home star (61 UMa) MUST be seeded at game start with
  the same initial fixed state Sol receives: status = alien-held, econ
  level = 5 (and not subject to growth), wealth = 64, local units = 1
  comm laser, and one initial fleet stationed at home containing 2
  reporters. (Q20)
- **FR-5.** Sol MUST continue to be seeded at game start with its existing
  fixed initial state (status = human-held, econ level = 5, wealth = 64,
  1 comm laser, 1 fleet of 2 reporters). The Sol seeding is a special
  case of the general "home-system seed" applied once per player. (Q20)
- **FR-6.** All non-home starting systems on each side MUST be selected
  and economically initialised by the existing rules: home-region
  systems with planets are owned by that side; home-region systems
  without planets but within half the maximum catalogue distance from
  that side's home are also owned by that side; econ levels for non-home
  owned systems are drawn by the existing Gaussian rule
  (`EconLevelMean=2.5`, `EconLevelStddev=1.0`, clamped to `[1, 4]`). (Q19)
- **FR-7.** All other catalogue entries (stars not selected as starting
  systems for either side) MUST be initialised as uninhabited.

### 1.2 Faction and identity

- **FR-8.** The server MUST internally represent each player by a unified
  type (e.g. `Faction` or `Player`) used twice — once for the human and
  once for the alien — with no asymmetric struct fields. The legacy
  asymmetric struct types `HumanFaction` and `AlienFaction` MUST be
  unified into one type. (Q28)
- **FR-9.** Source-code identifiers for the two sides MUST retain the
  labels `Human` and `Alien` (e.g. `HumanOwner`, `AlienOwner`,
  `StatusHuman`, `StatusAlien`). Symmetry MUST NOT cause a rename to
  generic labels such as `Player1`/`Player2`. (Q28)
- **FR-10.** Wire-format owner labels (in JSON responses, SSE events, and
  log strings) MUST keep the absolute names `human` and `alien`. The
  server MUST NOT relabel owners as `self`/`foe` per request; clients
  derive self/foe locally. (Q27)

### 1.3 Truth, views, and information delay

- **FR-11.** The server MUST maintain exactly one `Truth` instance,
  representing the authoritative state of every system, fleet, and
  faction. Truth is faction-agnostic. (§6.1)
- **FR-12.** The server MUST maintain exactly two `PlayerView` instances —
  one for the human and one for the alien — keyed by player ID. The
  legacy single `SolView` MUST be replaced. (§6.1)
- **FR-13.** Each event recorded in the event log MUST carry an arrival
  time computed for *each* player's home, plus the existing
  internal/silent flag. The legacy single `ArrivalYear` field (meaning
  arrival at Sol) MUST be replaced by per-player arrival times (e.g. via
  separate `ArrivalYearHuman` / `ArrivalYearAlien` fields, or a small
  map keyed by player ID).
- **FR-14.** An event MUST mature into a given player's view when, and
  only when, the engine clock crosses that player's arrival time for
  that event. The two players MUST be able to receive the same event at
  different real (and in-game) times.
- **FR-15.** The event log's "matured" cursor MUST be per-player. There
  MUST NOT be a single shared "matured" frontier across both players.
- **FR-16.** The propagator MUST broadcast each non-internal event onto
  the requesting player's SSE stream when (and only when) that event
  matures into that player's view. It MUST NOT broadcast a player's
  matured event onto the other player's stream.
- **FR-17.** At t=0 (game start), each `PlayerView` MUST be a complete
  mirror of `Truth` for every system in the catalogue, with
  `KnownSystem.AsOfYear=0`. Each player therefore knows, at the
  beginning of the war, the owner, econ level, wealth, and unit
  composition of every system on the map — including the opponent's
  home, the opponent's other initial systems, and all uninhabited
  systems. (Q10–Q12)
- **FR-18.** After t=0, only events that mature into a player's view via
  the propagator MUST update that player's `PlayerView`. A
  `KnownSystem` for an opponent-held system that the player never
  observes again after t=0 MUST remain pinned at `AsOfYear=0` for the
  duration of the game. (Q12 implication)
- **FR-19.** The `KnownSystem.Status` value `unknown` MAY be retained in
  the type system but MUST NOT be produced for any system at t=0 under
  the v1 model. (Q12)

### 1.4 Reporters and comm lasers

- **FR-20.** Reports from a system MUST be delivered to the *owner's*
  home star, not unconditionally to Sol. Concretely:
  - A comm laser at a system owned by player P MUST report events at
    that system to P's home at 1.0c, with arrival time
    `eventYear + dist(systemID, homeOf(P))`.
  - A reporter present at a system owned by P at the moment combat
    begins MUST flee to P's home at 0.8c, delivering its report on
    arrival at `eventYear + dist(systemID, homeOf(P)) / 0.8`. (Q21)
- **FR-21.** The owner used to determine the report destination MUST be
  the system's owner *at the moment of reporting* (i.e. at the start of
  the combat that triggers the report). If the system changes hands
  during combat, the comm-laser report still goes to the previous
  owner's home (the laser fires its report and is destroyed in the same
  combat, per existing semantics), and the reporter still flees to the
  side it was loyal to at combat start. (Q21)
- **FR-22.** Reporter and comm-laser semantics — including which events
  they capture, the timing of report emission, the destruction of comm
  lasers in the combat that triggers their report, and reporter flight
  to the owning side's home — MUST otherwise be unchanged from current
  behavior.
- **FR-23.** The current "reporter fleet arrives at Sol" special case
  MUST be generalised to "reporter fleet arrives at the fleet owner's
  home." (Q30)

### 1.5 Commands

- **FR-24.** Each player MUST issue commands via `POST /api/command`,
  using identical request and response shapes for both players.
- **FR-25.** A command issued by player P MUST originate at P's home
  star. The legacy hard-coding of `OriginID = "sol"` MUST be replaced by
  a per-player origin.
- **FR-26.** A command's travel delay MUST be computed as
  `dist(homeOf(P), targetID) / 0.8c`. The alien player therefore incurs
  a travel delay on commands; the previous "bot writes Truth directly,
  zero delay" behavior is removed.
- **FR-27.** A command MUST be validated against the *requesting*
  player's own `PlayerView`, not against `Truth` and not against the
  other player's view. A command that the player's view does not
  support MUST be rejected with the existing error response shape.
- **FR-28.** A command MAY still fail on arrival if the situation at the
  destination has changed in the intervening travel time. The existing
  arrival-time failure semantics are preserved and apply to both
  players.

### 1.6 Star catalogue

- **FR-29.** The server MUST load four data files from the working
  directory at startup: `nearest.csv`, `planets.csv`, `alien-nearest.csv`,
  and `alien-planets.csv`. The two new files mirror the column schemas
  of the existing pair. (Q13)
- **FR-30.** The runtime catalogue MUST be the union of the four files,
  forming a single `StarCatalog` keyed by stable ID. No separate
  per-player catalogue exists at runtime. (Q14, Q15)
- **FR-31.** The loader MUST detect duplicate stable IDs across the union
  of the two catalogue inputs and fail startup with a clear error
  message naming the offending ID(s). It MUST NOT silently coalesce or
  arbitrarily pick one entry. The existing same-file grouping in
  `loadStars` (which collapses co-located rows by string-equal
  RA/Dec/distance) is preserved; the cross-file duplicate-ID check is
  the only new guard. (Q14)
- **FR-32.** The catalogue coordinate frame MUST remain Sol-relative
  (the existing Three.js cartesian frame with Sol at the origin). 61
  UMa and its neighbours MUST be ordinary catalogue entries at their
  actual Sol-relative positions. No re-origining and no dual coordinate
  systems. (Q15)
- **FR-33.** `CatalogEntry.DistFromSol` MUST remain valid for every
  entry, including alien-region entries.
- **FR-34.** The system MUST provide an efficient way to compute
  "distance from this player's home" for any system ID. This MAY be
  done via on-demand calls to `Catalog.Distance(homeOf(P), targetID)`
  or via a precomputed per-player distance table on `GameState`; the
  choice is a design decision, but the capability is required because
  it is invoked per-event for command travel time and report arrival
  time.
- **FR-35.** `GET /api/stars` MUST return the merged catalogue, exactly
  the same response for both players, and MUST be exempt from the
  player-identity rule (FR-37). The existing
  `Cache-Control: max-age=86400` header MUST be preserved. (Q26)

### 1.7 Player identity on the API

- **FR-36.** Every `/api/*` request *except* `GET /api/stars` MUST carry
  a query parameter `player` with value `human` or `alien`. (Q1, Q25)
- **FR-37.** A request that is missing the `player` parameter, or that
  carries any value other than `human` or `alien`, MUST be rejected
  with HTTP `400` and JSON body
  `{"ok":false,"error":"unknown player"}`. (Q1)
- **FR-38.** Identity validation MUST be performed by middleware (a
  sibling of `recoverMiddleware`) that attaches the player ID to the
  request context. Handlers MUST read player identity from context, not
  re-parse the query string. (Q1)
- **FR-39.** `GET /api/stars` MUST NOT require the `player` parameter
  and MUST behave identically regardless of any value supplied. (Q26)
- **FR-40.** No other identity carrier MUST be introduced: no header,
  no token, no `POST /api/join` handshake, and no path prefix such as
  `/api/human/...` or `/api/alien/...`. (Q1, Q25)

### 1.8 State and event endpoints (per-player views)

- **FR-41.** `GET /api/state` MUST return the requesting player's
  `PlayerView` snapshot, never `Truth` and never the other player's
  view.
- **FR-42.** `GET /api/events` MUST establish an SSE subscription scoped
  to the requesting player. Frames sent on this stream MUST be filtered
  to events that have matured into that player's view. The wire shape
  of `clock_sync`, `game_event`, `system_update`, and `game_over`
  frames is otherwise unchanged.
- **FR-43.** Each player MAY have at most one active SSE subscription at
  a time. Behavior on a second concurrent subscription as the same
  player is not specified by these requirements; the server MUST NOT
  treat such a second subscription as the *other* player.
- **FR-44.** A debug endpoint that exposes raw `Truth` MUST exist and
  MUST be reachable without an additional permissions check (it is
  already in the route table as `/api/debug/state`). It is a developer
  affordance, available to either client. (Q18)

### 1.9 Game lifecycle

- **FR-45.** On server startup the engine MUST be paused (clock not
  advancing, no ticks).
- **FR-46.** The engine MUST automatically begin ticking as soon as
  *both* players have an open SSE subscription on `/api/events`. There
  MUST NOT be any other "begin" signal (no `POST /api/start`, no admin
  command). (Q2)
- **FR-47.** If either player's SSE subscription drops mid-game, the
  engine MUST pause itself by setting `Paused=true`, MUST write a
  clearly-marked message to the server's stdout in a format equivalent
  to `server: <player> disconnected; game paused`, and MUST broadcast
  no event on the surviving stream beyond the existing `clock_sync`
  that pause already emits. (Q3)
- **FR-48.** Reconnect as the same player after a disconnect MUST NOT
  be supported in v1. After a disconnect, the server MAY continue
  running (paused) but MUST NOT accept a new SSE subscription as the
  disconnected player. (Q4)
- **FR-49.** The server MUST NOT attempt to detect a "new game"
  starting; restarting requires restarting the server process. (§7.8
  future direction)

### 1.10 Pause

- **FR-50.** `POST /api/pause` MUST be honoured only when the request
  carries `?player=human`. When the request carries `?player=alien` the
  server MUST return HTTP `403` and JSON body
  `{"ok":false,"error":"only the human player may pause"}`. (Q5)
- **FR-51.** While the engine is paused (whether by the human via
  `/api/pause` or by the disconnect-driven internal pause from FR-47),
  the server MUST NOT send any `game_event` or `system_update` frames
  on either SSE stream. (Q6)
- **FR-52.** Both SSE streams MUST remain open while the engine is
  paused. Pause MUST NOT terminate either subscription. (Q6, forced by
  FR-48)
- **FR-53.** The existing `clock_sync` event broadcast by
  `Engine.SetPaused` on every pause/unpause transition MUST remain the
  sole signal both clients rely on to track pause state. (Q6)

### 1.11 Victory and draw

- **FR-54.** A player MUST win immediately by capturing the opponent's
  home star. (Q7)
- **FR-55.** A player MUST also win immediately by holding at least
  `WinRetentionFraction` of the opponent's *initial* systems, where
  `WinRetentionFraction` is a single shared constant applied identically
  to both sides. The legacy distinct constants
  `AlienWinCaptureFraction` and `HumanWinRetentionFraction` MUST be
  collapsed into this single shared constant. (Q7)
- **FR-56.** `CheckVictory` MUST be rewritten to evaluate the win
  conditions (FR-54 and FR-55) for each player in turn, with no
  human-vs-alien asymmetry. (Q7)
- **FR-57.** A draw condition MUST exist. The v1 draw condition MUST be
  a game-length cap: if neither player has met FR-54 or FR-55 by an
  elapsed in-game year `DrawYearCap`, the game ends in a draw. (Q9)
- **FR-58.** `DrawYearCap` and `WinRetentionFraction` MUST be exposed
  as tunable constants (in `srv/internal/game/constants.go` or a clearly
  equivalent location). They are explicitly subject to playtesting and
  MUST NOT be load-bearing in code outside the victory check. (Q9)
- **FR-59.** The "alien exhausted" counter and any associated
  asymmetric attrition-victory rule MUST be removed from v1. If a
  future draw rule needs a units-destroyed counter, it MUST be a
  mirrored per-player counter; it MUST NOT be reintroduced as an alien-
  only mechanism. (Q8)
- **FR-60.** The `game_over` SSE event MUST carry a `winner` field whose
  value is one of `human`, `alien`, or `draw`. The wire labels follow
  FR-10.

### 1.12 Bot process

- **FR-61.** A new Go binary MUST be built from `srv/cmd/spacebot/`
  within this module. Its public-facing entry point is
  `srv/cmd/spacebot/main.go`. (Q22, Q24)
- **FR-62.** Bot logic MUST live under `srv/internal/bot/`, with a
  suggested package layout:
  - `srv/internal/bot/agent.go` — strategy implementation.
  - `srv/internal/bot/client.go` — HTTP/SSE client for the public API.
  - `srv/internal/bot/state.go` — the bot's local mirror of what its
    home knows. (Q24)
- **FR-63.** The bot MUST communicate with the server *only* over the
  public HTTP/SSE API. It MUST NOT import or link against
  `srv/internal/game` types directly. (Q22)
- **FR-64.** The bot MUST identify itself on every request via
  `?player=alien`, per FR-36.
- **FR-65.** The bot MUST subscribe to `/api/events?player=alien` and
  use it as its primary information source. It MUST be able to call
  `GET /api/state?player=alien` for full snapshots.
- **FR-66.** The bot MUST issue commands via
  `POST /api/command?player=alien`, paying the same `dist/0.8c` arrival
  delay as the human player.
- **FR-67.** The bot MUST NOT call `/api/pause` and MUST tolerate the
  engine advancing without bot input on a given tick if the bot's
  decision-making is slow. The bot MUST NOT have any internal pause
  concept. (Q17)
- **FR-68.** The server MUST NOT spawn or supervise the bot process.
  The bot is launched independently. (Q23)
- **FR-69.** A new launch script (e.g. `scripts/run-game.sh`) MUST be
  provided that starts the server first, waits for it to be reachable
  on `127.0.0.1:8080`, then starts the bot pointed at that endpoint.
  Either process MUST remain individually killable for debugging.
  Manual two-terminal launch MUST also remain supported as the default
  development workflow. (Q23)

### 1.13 Bot strategy (v1)

- **FR-70.** The v1 bot strategy MUST consist of the following behaviors,
  in priority order. (Q16)
  - **FR-70.1.** Replenish escorts at the home star (61 UMa) using
    accumulated wealth, capped to retain reserve for other production.
  - **FR-70.2.** Periodically dispatch escort/battleship fleets toward
    the opponent system whose `KnownSystem.AsOfYear` is highest among
    the bot's known opponent systems (i.e. the most recently known
    opponent system).
  - **FR-70.3.** Sweep reporters outward from home toward candidate
    opponent systems on a slow cadence, preferring systems that have
    not been observed for the longest time.
- **FR-71.** The v1 bot MUST NOT perform tree search, opponent
  modelling, or economic optimisation beyond the rules in FR-70. (Q16)
- **FR-72.** The bot MUST plan only against its own `PlayerView`; it
  MUST NOT consult `Truth` for any decision. (Q18)
- **FR-73.** The bot binary MUST accept an optional `--debug-truth`
  flag (or equivalent environment variable). When enabled, the bot MUST
  call `GET /api/debug/state?player=alien` and write the returned
  ground truth to its own log for diagnostic purposes only. The bot's
  decision logic MUST NOT consume the debug-truth output. With the
  flag off (the default) the bot MUST use only its `PlayerView`. (Q18)

### 1.14 Cleanup of asymmetric machinery

- **FR-74.** The omniscient in-process bot MUST be removed entirely. The
  following items MUST be deleted (not deprecated, not feature-flagged):
  - From `srv/internal/game/bot.go`: `BotAgent` interface, `DefaultBot`
    struct and methods, `BotCommand` struct, and the
    `humanTargetsByProximity`, `alienInboundTargets`, `totalUnits`
    helpers (any helper turning out to be reused elsewhere may be moved
    rather than deleted). The file becomes empty and MUST be removed.
  - From `srv/internal/game/engine.go`: the `Engine.Bot` field, the
    `Bot` parameter on `NewEngine`, the bot tick branch in
    `Engine.tick()`, `applyBotCommand()` and its call site,
    `spawnAlienForces()`, and the alien-spawn cadence check. The
    "reporter fleet arrives at Sol" special case is *generalised*
    (FR-23), not deleted.
  - From `srv/internal/game/constants.go`: `BotTickCadence`,
    `AlienDormancyYears`, `AlienSpawnIntervalYears`,
    `AlienSpawnComposition`, `AlienInitialComposition`,
    `AlienEntryCount`, `PeripheryFraction`.
  - From `srv/internal/game/state.go`:
    `AlienFaction.EntryPointIDs`, `AlienFaction.NextSpawnYear`,
    `AlienFaction.TotalLost`, `AlienFaction.Exhausted` (the latter two
    follow from FR-59).
  - From the events module (currently in `srv/internal/game/events.go`
    or `eventlog.go`): `EventAlienSpawn` and `EventAlienExhausted`
    event types and every code path that produces them. The
    corresponding `alien_exhausted` wire event MUST be removed from
    `server_api.md`.
  - From `srv/internal/game/loader.go`: the peripheral-systems-as-
    entry-points selection block, the deliberate `SolView` carve-out
    that suppresses alien presence at game start, and the "alien
    fleets at entry points" seeding. (Q30)
- **FR-75.** No parallel asymmetric mode MUST be preserved. There MUST
  NOT be a `--legacy-bot` flag, a branching code path, or a single-
  player mode kept for "quick playtesting." (Q31)
- **FR-76.** Tests that exercise deleted asymmetric machinery MUST be
  deleted or rewritten against symmetric semantics. Tests for player-
  agnostic behaviour (combat math, economy growth, propagator
  correctness, catalogue loading) MUST be preserved and MAY be
  parameterised over player ID. (Q30)

### 1.15 Documentation

- **FR-77.** `server_api.md` MUST be updated to:
  - Document the required `?player=human|alien` query parameter on all
    `/api/*` endpoints except `GET /api/stars`.
  - Document the `400 unknown player` error response shape.
  - Document the `403 only the human player may pause` response shape
    on `POST /api/pause` for `?player=alien`.
  - Remove the `alien_exhausted` event.
  - State that `/api/state` and `/api/events` are scoped to the
    requesting player's known view, and that the wire-format owner
    labels remain absolute (`human`, `alien`).
- **FR-78.** `OLD_SPECS/` MUST NOT be modified by this work. It remains
  archived as historical, consistent with the existing `CLAUDE.md`
  statement that specs there are not authoritative. (Q29)

---

## 2. Non-functional requirements

### 2.1 Determinism and engine semantics

- **NFR-1.** The engine MUST remain the sole writer of `GameState`. HTTP
  and SSE handlers MUST hold only read locks on `GameState` while
  serving requests, preserving the current concurrency model.
- **NFR-2.** Per-player propagation MUST NOT introduce non-deterministic
  ordering of event delivery within a single player's stream. For a
  given seeded run, the sequence of events delivered to a given player
  MUST be deterministic given the seeded RNG and identical command
  inputs.
- **NFR-3.** The existing tick cadence (~100 ms wall-clock; ~0.056 game
  years per real second) MUST be unchanged unless overridden by a
  future requirement. The simple v1 bot MUST be fast enough to keep up
  with this cadence in normal operation, but the engine MUST remain
  correct (per FR-67) if it does not.

### 2.2 Locality and security

- **NFR-4.** The server MUST continue to listen only on the loopback
  interface (`127.0.0.1:8080`). No requirement in this document
  introduces remote-host access. (§8)
- **NFR-5.** No authentication or authorisation mechanism MUST be added
  beyond the existing loopback-only restriction. The `?player`
  parameter is identification, not authentication; it is trusted on the
  loopback. (§3.3)

### 2.3 Compatibility and code organisation

- **NFR-6.** The build system MUST continue to support
  `go build -o spacegame srv/cmd/spacegame/main.go` and add an
  analogous `go build -o spacebot srv/cmd/spacebot/main.go` (path may
  vary, but the binary MUST live in this module).
- **NFR-7.** `go test ./srv/...` MUST pass after the change.
- **NFR-8.** The existing module/package boundaries MUST be preserved:
  game logic in `srv/internal/game/`, HTTP layer in
  `srv/internal/server/`, server entry point in
  `srv/cmd/spacegame/`. The new bot code MUST follow the analogous
  layout (`srv/internal/bot/` + `srv/cmd/spacebot/`).
- **NFR-9.** Code style and naming conventions of the existing codebase
  MUST be respected. New game-internal identifiers MUST follow the
  existing pattern (`StatusHuman`, `HumanOwner`, etc.); the unified
  faction type MAY be named `Faction` or `Player` at the
  designer's/implementer's discretion. (Q28)

### 2.4 SPA compatibility

- **NFR-10.** The SPA MUST continue to function against the new API
  with no behavioural changes beyond the addition of the
  `?player=human` query parameter on its requests. The SPA does not
  participate in the alien-side game.
- **NFR-11.** The Three.js star map MUST render correctly when alien-
  region stars are present in the merged catalogue. (Implied by
  FR-32.)
- **NFR-12.** The frontend build process described in `CLAUDE.md`
  (`scripts/build-frontend.sh` producing `web/dist/` for embedding)
  MUST continue to work.

### 2.5 Observability

- **NFR-13.** The server MUST log the disconnect-driven pause message
  required by FR-47 to stdout in a form that is unambiguous when read
  in a console window (e.g. `server: human disconnected; game paused`).
- **NFR-14.** The server MUST NOT log either player's `PlayerView`
  contents or `Truth` contents at routine log levels. The
  `--debug-truth` affordance (FR-73) is an explicit, opt-in exception
  on the bot side; the human-side `/api/debug/state` endpoint remains
  opt-in via direct request.

### 2.6 Tunability

- **NFR-15.** The following parameters MUST be defined as named
  constants in a single, easily-locatable file
  (`srv/internal/game/constants.go` or its equivalent):
  - `WinRetentionFraction` (FR-55).
  - `DrawYearCap` (FR-57).
  Any future draw or attrition tunables added per FR-59 MUST follow
  the same pattern.

### 2.7 Future-readiness (advisory, not load-bearing)

- **NFR-16.** The v1 design MUST NOT make it gratuitously harder to
  later add: a headless / accelerated tick mode, a deterministic
  reset/episode-restart endpoint, and an RL agent harness reusing
  `srv/internal/bot/client.go`. These items are explicitly out of scope
  (§8 / §7.8 future direction); this requirement only forbids
  v1 choices that *foreclose* them.

---

## 3. Out-of-scope items (recorded for traceability, not requirements)

The following items are explicitly **not** required by this document
and MUST NOT be implemented as part of this work:

- Network play across hosts. (§8)
- Authentication or authorisation beyond loopback-only. (§8)
- More than two players. (§8)
- A spectator client. (§8)
- The actual data content of `alien-nearest.csv` and `alien-planets.csv`
  (produced as a separate data task). (§8)
- A bot strategy beyond the v1 simple bot (FR-70). (§8)
- Any change to weapon definitions, combat math, or economic rates
  unrelated to symmetry. (§8)
- UI changes in the SPA beyond what FR-77 / NFR-10 force. (§8)
- Reconnect support after a disconnected player drops. (FR-48, Q4)
- A reporter scouting extension. (Q12a)
- A `--legacy-bot` or "quick playtest" parallel asymmetric mode.
  (FR-75)
- A headless tick mode, deterministic reset endpoint, or RL agent
  harness. (NFR-16, §7.8 future direction)
