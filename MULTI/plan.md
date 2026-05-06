# Plan: Implement Symmetric Multiplayer (MULTI/design.md)

## Context

`MULTI/design.md` (2241 lines) and `MULTI/requirements.md` (78 FRs + 16 NFRs)
specify a major refactor that converts SpaceGame from an asymmetric
human-vs-omniscient-bot game into a symmetric two-player game. Both sides
play under identical rules, identical information delays, and over the
same public HTTP/SSE API. A new bot binary replaces the in-process
omniscient `DefaultBot`. There is **no backwards compatibility**: the
asymmetric machinery (alien spawn, exhaustion, in-process bot, single
`SolView`, single shared event frontier) is deleted, not flagged off.

This plan does **not** redesign anything; the design is already detailed
and the requirements are stable. The plan is purely about **execution
order** — how to land ~14 file modifications + 5 new files + 1 deletion
without leaving the tree in a non-building state for long, and how to
sequence the work so each milestone is independently reviewable in the
developer-mode workflow (small chunks, run `go build` and `go test
./srv/...` after each).

The only deviations from the design are:
- This plan **does not** ask the user to amend any open questions in §12
  of design.md. The four open items (Sol econ level 5 vs. 4, alien home
  display name, `DrawYearCap=500`, `BotDecisionInterval=2s`) are all
  flagged in design.md as tunable single-line constants that "should not
  block implementation"; we accept the design's choices.
- `alien-nearest.csv` and `alien-planets.csv` are an out-of-scope
  separate data task per requirements.md §3 and AS-1. The loader code
  ships with this work; the data files may not exist yet, in which case
  end-to-end manual testing against a real game is deferred until the
  data team produces them. Unit tests use small fake CSVs (per design
  §11.1).

## Approach

Bottom-up, in the dependency order design §6 already prescribes:
constants → types → state → eventlog → propagator → engine → combat /
economy / loader → HTTP → cleanup → bot binary → frontend → tests →
docs. After each milestone, run:

```bash
go build ./... && go test ./srv/...
```

Some milestones (M2, M3, M5) will deliberately leave the tree
non-building between sub-steps because the rename / signature change
cascades; in those cases the milestone is the unit of "back to green",
not the individual edit. Those milestones are flagged below.

The bot binary (M11) is built last because nothing else depends on it,
and it can be developed against a working server. The frontend
`?player=human` change (M10) is a near-trivial mechanical edit but
cannot ship before M7 (middleware), since the server will start
rejecting requests at that point.

---

## Milestones

### M1. Constants and unified Faction type
- **Files:** `srv/internal/game/constants.go`, `srv/internal/game/state.go`
- Add `WinRetentionFraction` (0.60), `DrawYearCap` (500). (FR-58, NFR-15)
- Remove asymmetric constants: `BotTickCadence`, `AlienDormancyYears`,
  `AlienSpawnIntervalYears`, `AlienSpawnComposition`,
  `AlienInitialComposition`, `AlienEntryCount`, `PeripheryFraction`,
  `AlienWinCaptureFraction`, `HumanWinRetentionFraction`. (FR-74)
- Define unified `Faction` type with fields `InitialSystemIDs []string`
  (and any other fields design §6.1 specifies — read it before editing).
  Replace `HumanFaction` / `AlienFaction` types in `state.go`.
- Replace `GameState.Human` / `GameState.Alien` with
  `Factions map[Owner]*Faction` (or two named fields per design §6.2 —
  follow whichever the design specifies). (FR-8)
- Drop `AlienFaction.EntryPointIDs`, `NextSpawnYear`, `TotalLost`,
  `Exhausted`. (FR-74)
- **Will break compile** until M5 (engine cleanup) since `bot.go`,
  `engine.go`, `loader.go` reference the deleted symbols. To keep M1
  reviewable, leave `bot.go` and dead engine paths in place compiling
  against `_ = state.Alien.NextSpawnYear` shims if needed, OR sequence
  M1 to be landed together with the deletes in M5. **Recommended**:
  treat M1 + M5 + M6 (loader rewrite) as one atomic landing reviewed
  in pieces but committed together.

### M2. Rename `SolView` → `PlayerView`, add `Views` map
- **Files:** rename `srv/internal/game/solview.go` →
  `srv/internal/game/playerview.go`; modify `state.go`, callers
  throughout.
- Rename type `SolView` → `PlayerView`. (FR-12)
- Replace `GameState.SolView *SolView` with
  `GameState.Views map[Owner]*PlayerView`. (FR-12)
- Replace `ReadSolGroundTruth` / `SolGroundTruthSnapshot` with
  `ReadHomeGroundTruth(player Owner)` / `HomeGroundTruthSnapshot`.
- **Tree won't build** mid-rename; the rename is the milestone, run
  build at the end.

### M3. Per-player event arrival times
- **Files:** `srv/internal/game/eventlog.go`,
  `srv/internal/game/types.go` (Event struct).
- Replace `Event.ArrivalYear float64` with per-player arrival
  representation (design §6.4 specifies the exact shape — likely
  `Arrival map[Owner]float64` or `ArrivalHuman` + `ArrivalAlien`
  fields; follow the design). (FR-13)
- Convert single shared min-heap into per-player heaps; per-player
  matured cursor. (FR-15, NFR-2)
- New `PopMatured(player Owner, clock float64)` (or equivalent
  signature from design §6.4).
- Preserve heap stable-tiebreak for determinism (NFR-2).
- Update `RecordEvent` callers — they will be touched again in M6
  (combat) for the routing logic; here we only change the storage
  shape.

### M4. Propagator per-player
- **Files:** `srv/internal/game/propagator.go`.
- `Propagate(state)` iterates over both players, drains each player's
  matured heap, calls `applyEventToView` against that player's view,
  and broadcasts on that player's channel set. (FR-14, FR-16, FR-18)
- Remove the existing `Owner == HumanOwner` filters in fleet handlers
  per design §6.4 / FR-74.
- Remove the `EventAlienExhausted` handler. (FR-74)

### M5. Engine cleanup + connect/disconnect hooks
- **Files:** `srv/internal/game/engine.go`,
  **delete** `srv/internal/game/bot.go` entirely.
- Drop `Engine.Bot` field, `Bot` parameter on `NewEngine`, the bot
  tick branch, `applyBotCommand`, `spawnAlienForces`, and the
  alien-spawn cadence check. (FR-74)
- Generalise the "reporter fleet arrives at Sol" branch in
  `processFleetArrivals` to "arrives at fleet owner's home." (FR-23)
- Add `OnPlayerConnected(player Owner)` and
  `OnPlayerDisconnected(player Owner)` per design §6.5. The connect
  hook auto-unpauses when both players are connected (FR-46); the
  disconnect hook pauses + logs `server: <player> disconnected; game
  paused` (FR-47, NFR-13).
- `EnqueueCommand` now takes `issuer Owner`; computes
  `OriginID = homeOf(issuer)`; computes
  `ExecuteYear = Clock + Catalog.Distance(home, target) / 0.8c`.
  (FR-25, FR-26)
- Validate against `Views[issuer]`, not Truth. (FR-27)

### M6. Combat, economy, loader
- **Files:** `srv/internal/game/combat.go`,
  `srv/internal/game/economy.go`, `srv/internal/game/loader.go`.
- **combat.go:**
  - Remove exhaustion-counter increments and
    `EventAlienExhausted` recording. (FR-74)
  - Report routing: comm-laser report uses pre-combat
    `TrueSystem.Status` to determine destination home (AS-4);
    reporter flight uses `TrueFleet.Owner` at combat start (FR-20,
    FR-21). `RecordEvent` (or its successor) sets per-player
    `ArrivalYear` map: report-receiving player gets the real arrival
    time; the *other* player gets `math.MaxFloat64`.
  - `extractAndSendReporters` symmetric for both sides.
- **economy.go:**
  - `AccumulateWealth` and `AdvanceEconLevels` apply to both
    `StatusHuman` and `StatusAlien` systems. (FR-2, G-3)
  - `ValidateConstruct` checks `cmd.Issuer` matches `TrueSystem.Status`
    rather than hard-coding `StatusHuman`. (FR-27)
- **loader.go:**
  - Take four CSV paths; load `nearest.csv`, `planets.csv`,
    `alien-nearest.csv`, `alien-planets.csv`. (FR-29)
  - Per-file co-located grouping preserved (AS-7), then union with
    duplicate-ID detection that fails fast naming offenders. (FR-31)
  - Seed both home stars identically: `EconLevel=5`, `Wealth=64`,
    1 comm laser, 1 fleet of 2 reporters at home. (FR-4, FR-5; A-1
    flagged: this *changes* Sol from 4 to 5)
  - Seed non-home owned systems by existing Gaussian rule per side.
    (FR-6)
  - Mirror **both** views from Truth at t=0 with `AsOfYear=0` for
    every system. **Remove** the SolView carve-out that hides
    aliens. (FR-17, FR-74)
  - Populate `AlienHomeID` at load time by display-name match
    against `AlienHomeDisplayName = "61 Ursae Majoris"` (AS-2; open
    question §12.2 — single-line edit if data team picks differently).
  - Delete the peripheral-systems-as-entry-points block and the
    alien-fleets-at-entry-points seeding. (FR-74)
- After M6 the engine should build, run, tick, and pass at least the
  player-agnostic tests; asymmetric tests will fail until M13.

### M7. HTTP middleware + per-player handlers
- **Files:** `srv/internal/server/server.go`,
  `srv/internal/server/handlers.go`, `srv/internal/server/types.go`.
- Add `playerMiddleware` as a sibling of `recoverMiddleware`. Reads
  `?player`, validates ∈ `{human, alien}`, attaches to `r.Context()`.
  Reject missing/invalid with HTTP 400 + JSON
  `{"ok":false,"error":"unknown player"}`. (FR-36, FR-37, FR-38)
- Apply middleware to all `/api/*` routes **except** `GET /api/stars`
  and `GET /api/debug/state` (debug stays open per FR-44; stars
  exempt per FR-39).
- `handleState`: return `Views[playerOf(r)]` snapshot, never Truth,
  never the other player's view. (FR-41)
- `handleCommand`: pass `playerOf(r)` to `Engine.EnqueueCommand`.
  Origin is the issuer's home, not hardcoded "sol". (FR-24, FR-25)
- `handlePause`: 403 + JSON
  `{"ok":false,"error":"only the human player may pause"}` when
  `playerOf(r) == AlienOwner`. (FR-50)
- `handleEvents`: register per-player SSE channel, send the
  *requesting* player's `connected` snapshot, call
  `Engine.OnPlayerConnected`. On request-context cancellation,
  unregister and call `Engine.OnPlayerDisconnected`. (FR-42, FR-46,
  FR-47)
- Rename `humanFleetsInTransit` DTO → `fleetsInTransit`. (FR-10)

### M8. Per-player SSE broadcast registry (events.go)
- **Files:** `srv/internal/game/events.go`.
- `EventManager.Register(player Owner, clientID string)` keys
  channels by player. (FR-43)
- `broadcastEvent(player, evt)`,
  `broadcastSystemUpdate(player, state, sysID)` — propagator calls
  these per player. (FR-16)
- Drop the `alien_exhausted` wire event entirely. (FR-74, FR-77)
- `Unregister` returns "subscriber count went to zero" so the handler
  can drive `OnPlayerDisconnected`. (per design §5.3)

### M9. Symmetric victory + game-over wire field
- **Files:** `srv/internal/game/state.go` (CheckVictory).
- Rewrite `CheckVictory` to evaluate, for each player P in turn:
  1. `Truth.Systems[homeOf(opponent(P))].Status == ownerOf(P)` →
     P wins. (FR-54, A-3)
  2. count of opponent's `InitialSystemIDs` whose current Status is
     `ownerOf(P)`, divided by `len(InitialSystemIDs)`,
     ≥ `WinRetentionFraction` → P wins. (FR-55, A-2)
  3. Else if `Clock >= DrawYearCap` → draw. (FR-57)
- `GameOver` event carries `winner ∈ {human, alien, draw}`. (FR-60)
- A `Draw` sentinel (e.g. `Owner = "draw"`, or a separate field per
  design §6.2 — follow the design) is plumbed through.

### M10. Frontend `?player=human`
- **Files:** `web/src/api.js` and any other client file constructing
  API URLs (per Explore: also `sidebar.js` polls `/api/debug/state`).
- Append `?player=human` to every `/api/*` URL except `/api/stars`
  (which the design says is exempt; harmless to omit). (NFR-10)
- Run `scripts/build-frontend.sh`; commit refreshed `web/dist/`
  alongside the source change (per CLAUDE.md). (NFR-12)

### M11. Bot binary
- **Files (all new):** `srv/cmd/spacebot/main.go`,
  `srv/internal/bot/client.go`, `srv/internal/bot/state.go`,
  `srv/internal/bot/agent.go`.
- Build with `go build -o spacebot srv/cmd/spacebot/main.go`. (NFR-6)
- `client.go`: typed HTTP/SSE client; appends `?player=alien` to
  every URL; **no `srv/internal/game` imports** (FR-63). Includes a
  conditional `GetDebugState()` only used when `--debug-truth` is
  set. (FR-65, FR-66, FR-73)
- `state.go`: bot's `Local` mirror — `applyConnected`,
  `applySystemUpdate`, `applyGameEvent`. (FR-72)
- `agent.go`: v1 strategy with three behaviours from FR-70:
  (1) replenish escorts at home; (2) dispatch fleets at last-known
  opponent system (highest `KnownSystem.AsOfYear`); (3) slow
  reporter sweep favouring oldest observation.
  `BotDecisionInterval = 2 * time.Second` (open question §12.4).
- Bot must tolerate engine-paused state by polling `Local.Snapshot`
  and skipping `decide` while `Paused=true`. (FR-67, edge case in
  design §11.2)
- No call to `/api/pause`. (FR-67)

### M12. Launch script
- **Files:** `scripts/run-game.sh` (new).
- Builds both binaries idempotently; starts `spacegame` first; polls
  `127.0.0.1:8080` until reachable; then starts `spacebot`. Either
  process individually killable. (FR-69)
- Script content per design §6.10.

### M13. Tests
- **Files:** under `srv/internal/game/*_test.go`,
  `srv/internal/server/*_test.go` (new), `srv/internal/bot/*_test.go`
  (new).
- Delete or rewrite asymmetric tests in `game_test.go`,
  `integration_test.go`, `propagator_test.go`. (FR-76)
- Per design §11.1:
  - `eventlog_test.go` — per-player heaps + per-player matured
    cursor.
  - `playerview_test.go` (renamed) — initial mirror exact for both;
    later events mutate only the right view.
  - `propagator_test.go` — both heaps drained; routing correct.
  - `combat_test.go` — comm laser at alien-held system reports to
    alien home; reporter loyalty across same-tick capture (AS-4).
  - `loader_test.go` (new) — duplicate-ID fail-fast; t=0 mirror;
    both homes seeded; `Faction.InitialSystemIDs` populated for
    both sides.
  - `state_test.go` — `CheckVictory` 5 cases per design §11.1.
  - `playerMiddleware` server-level tests; `/api/stars` ignores
    `?player`; `/api/pause` 403 for alien; auto-pause/unpause on
    SSE connect/disconnect.
  - `bot/client_test.go`, `bot/state_test.go`, `bot/agent_test.go`
    — three agent tests, one per FR-70 behaviour.
- Edge cases from design §11.2 (equidistant home arrival, reporter
  flee at moment of capture, comm-laser destruction concurrent with
  its own report, dist=0 self-target, win-vs-draw same-tick
  precedence, bot-starts-before-second-SSE).
- `go test ./srv/...` MUST pass. (NFR-7)

### M14. Documentation
- **Files:** `server_api.md`.
- Per FR-77:
  - Document required `?player=human|alien` on all `/api/*` except
    `GET /api/stars`.
  - Document `400 unknown player` shape.
  - Document `403 only the human player may pause` shape.
  - Remove the `alien_exhausted` event.
  - State that `/api/state` and `/api/events` are scoped to the
    requesting player; owner labels remain absolute (`human`,
    `alien`).
- `OLD_SPECS/` not touched. (FR-78)

---

## Critical files (most likely to need re-reading mid-execution)

- `MULTI/design.md` §6.1 (Faction unification — exact field set)
- `MULTI/design.md` §6.2 (GameState exact field set incl. Homes /
  Factions decision; CheckVictory pseudocode)
- `MULTI/design.md` §6.4 (PlayerView, eventlog per-player heap shape,
  propagator)
- `MULTI/design.md` §6.5 (engine connect/disconnect hooks, EnqueueCommand
  signature)
- `MULTI/design.md` §6.7 (handler signatures, middleware)
- `MULTI/design.md` §6.8 (bot package layout)
- `MULTI/design.md` §6.10 (launch script content)
- `srv/internal/game/loader.go` (heaviest single rewrite; AS-7 ordering
  matters)
- `srv/internal/game/eventlog.go` (NFR-2 determinism via stable
  tiebreak)

## Reusable existing utilities

- `Catalog.Distance(id1, id2)` — already player-agnostic (FR-32, FR-34
  satisfied by on-demand calls; no precomputed table needed).
- `gaussianEconLevel` — applied per side unchanged (FR-6).
- Per-file same-coordinate grouping in `loadStars` — preserved as-is,
  applied per-file before union (AS-7).
- `recoverMiddleware` — sibling pattern for `playerMiddleware` (FR-38).
- `EventManager.Register/Unregister` — extended with player keying, not
  rewritten (FR-43).

## Verification

End-to-end after each milestone where possible:
```bash
go build ./...
go test ./srv/...
```

After M14 (final), full smoke per design §11.1 "Manual / smoke
testing":
```bash
scripts/build-frontend.sh
go build -o spacegame srv/cmd/spacegame/main.go
go build -o spacebot  srv/cmd/spacebot/main.go
scripts/run-game.sh    # requires alien-nearest.csv / alien-planets.csv
```
Then in a browser at `http://localhost:8080`:
1. Engine remains paused until both clients connect (FR-45, FR-46).
2. SPA shows alien-region stars on the map (NFR-11).
3. `curl 'http://localhost:8080/api/state?player=alien'` returns the
   alien view, distinct from `?player=human` (FR-41).
4. `curl 'http://localhost:8080/api/state'` (no `?player`) returns
   `400 {"ok":false,"error":"unknown player"}` (FR-37).
5. `curl -XPOST 'http://localhost:8080/api/pause?player=alien'` returns
   `403 {"ok":false,"error":"only the human player may pause"}` (FR-50).
6. Closing the SPA tab pauses the engine and prints
   `server: human disconnected; game paused` to stdout (FR-47, NFR-13).
7. Capturing 61 UMa with a human fleet emits `game_over` with
   `winner: "human"` on both streams (FR-54, FR-60).

If `alien-nearest.csv` / `alien-planets.csv` are unavailable, smoke
test 1, 4, 5, 6 still run against minimal fake CSVs in unit tests; 2,
3, 7 wait on the data team (AS-1).

## Out of scope (per requirements.md §3)

Network play, auth, >2 players, spectator client, alien CSV data
content, bot strategy beyond FR-70, weapon/combat/economy changes
unrelated to symmetry, SPA UI changes beyond `?player=human`,
reconnect support, headless tick mode, RL agent harness, legacy-bot
flag.
