# Symmetric Multiplayer — Investigation Overview

> **Status:** Resolved. This document is the input for the requirements
> pass. It replaces `MULTI/MultiplayerPrompt` as the working specification
> for symmetric multiplayer. All open questions originally raised in §7
> have been resolved through stakeholder discussion and are recorded in
> place; no further input is required before the requirements analyst
> begins. No code is to be written from this document.
>
> Sections 1–5 describe the agreed change. Section 6 surveys the existing
> code. Section 7 records the resolved decisions and their rationale.
> Section 8 fences the work that is explicitly out of scope for this round.
> The "Summary of resolved decisions" immediately below is a one-page
> recap; the bodies of §§7.1–7.11 are the source of truth.

---

## Summary of resolved decisions

A scannable index of every decision made in §7. Numbers in parentheses
reference the originating question.

**Identity, lifecycle, pause**
- Player identity: required `?player=human|alien` query parameter on every
  `/api/*` request except `/api/stars`. (Q1)
- Game start: engine ticks automatically once both SSE subscriptions are
  open; paused before that. (Q2)
- Mid-game disconnect: engine pauses and writes a console log line; no
  event broadcast beyond the existing `clock_sync`. (Q3)
- Reconnect: not supported in v1; deferred. (Q4)
- Pause authority: human only; `/api/pause` returns 403 to the bot. (Q5)
- Pause behavior: SSE streams stay open; no `game_event`/`system_update`
  frames flow while paused; the existing `clock_sync` carries the
  transition. (Q6)

**Victory and draw**
- Win conditions exactly mirrored: capture opponent's home OR hold ≥ X%
  of opponent's initial systems, single shared threshold. (Q7)
- Exhaustion counter retained only if needed for draw detection. (Q8)
- Draw condition exists; exact form deferred to playtesting; first
  implementation is a game-length cap. (Q9)

**Initial knowledge**
- Full mutual knowledge of every system's *initial* state at t=0 ("pre-
  war radio monitoring" flavor); light-speed delay applies only to
  changes during the war. (Q10–Q12)
- Reporter scouting extension: not added in v1; revisit only if
  playtesting shows the game stalls. (Q12a)

**Star catalog**
- Alien-side data files: `alien-nearest.csv` and `alien-planets.csv` in
  repo root, mirroring the existing pair. (Q13)
- Catalog merging: data-prep contract guarantees same star → same name
  → same ID; the loader detects-and-fails on duplicate IDs but does no
  runtime resolution. (Q14)
- Coordinate frame: stays Sol-relative; 61 UMa is another entry; alien-
  side distances computed via `Catalog.Distance`. (Q15)

**Bot strategy and parity**
- v1 bot deliberately simple: replenish escorts at home; attack toward
  the opponent system with highest `KnownSystem.AsOfYear`; slow reporter
  sweep. (Q16)
- Bot cannot pause; must tolerate being late. (Q17)
- Single developer affordance: optional `--debug-truth` flag for
  diagnostic logging only; the bot's decision logic must not consume it.
  (Q18)

**Economy and weapon parity**
- Single shared `WeaponDefs`; identical economy rules; no side-specific
  tunables. (Q19)
- 61 UMa seeded identically to Sol: econ level 5, wealth 64, 1 comm
  laser, 1st Fleet with 2 reporters. (Q20)
- Reporter and comm-laser semantics identical for both sides; reports
  flow to the **owner's** home; ownership at the moment of reporting
  determines destination. (Q21)

**Process model for the bot**
- Bot is a separate Go binary in this repo, communicating only over the
  public HTTP/SSE API. (Q22)
- Launched by a `scripts/run-game.sh` shell script (server first, then
  bot); the server does **not** supervise the bot. Manual two-terminal
  launch always supported. (Q23)
- Layout: `srv/cmd/spacebot/main.go` plus `srv/internal/bot/`. (Q24)
- RL-bot future: out of scope for v1; the v1 split is RL-friendly by
  accident; headless tick mode and reset endpoint are flagged as future
  additions, not v1 requirements.

**API shape**
- Identity carrier: query parameter (restated from Q1). (Q25)
- `/api/stars` shared and exempt from the player-required rule. (Q26)
- Wire labels stay absolute `human`/`alien`; the client derives
  self/foe locally. (Q27)

**Naming and cleanup**
- Code keeps `Human`/`Alien` identifiers; the two faction *types* unify
  into one `Faction` used twice. (Q28)
- `OLD_SPECS/` archived as historical; not updated. (Q29)
- Full deletion of asymmetric machinery (`bot.go`, alien
  spawn/dormancy/exhaustion constants, alien faction fields,
  `EventAlienSpawn`, `EventAlienExhausted`, the loader's entry-point
  seeding). (Q30)
- No parallel asymmetric mode preserved. (Q31)

---

## 1. Goal

Convert SpaceGame from an **asymmetric** game (one human plus an omniscient
in-process bot) to a **symmetric** game (two clients, both subject to the
same light-speed information delay, where one client happens to be a bot
process).

In the new model:

- The server is the authoritative simulator. Two local clients connect to it
  over the existing HTTP/SSE API.
- One client is the human's web SPA, served from `127.0.0.1:8080` as today.
- The other client is a **separate, standalone bot process** that speaks the
  same API. It runs on the same machine and connects over loopback.
- Neither client has access to ground truth. Each sees only what its home
  star (Sol for the human, 61 UMa for the bot) has learned through
  light-speed-delayed reports and reporter/comm-laser intelligence.
- All omniscient-bot machinery is removed.

The aliens become a player, not a scripted hazard. The asymmetric features
they currently enjoy (entry-point spawning, dormancy, in-process omniscient
target selection, zero command-travel-time) all go away.

## 2. What "symmetric" means concretely

### 2.1 Both sides have a home, an empire, and an economy

| Aspect | Human (today) | Human (after) | Alien (today) | Alien (after) |
|--------|---------------|---------------|----------------|----------------|
| Home star | Sol | Sol | none — appears at peripheral entry points | 61 UMa |
| Initial systems | systems near Sol from `nearest.csv` w/ planets or within ½ max distance | unchanged | none (just spawn waves at entry points) | systems near 61 UMa from a new pair of CSV files |
| Economy | per-system wealth + econ level; Sol always 5 | unchanged | none | symmetric to human (61 UMa always 5; other home-region systems use the existing Gaussian rule) |
| Construction | yes, via `/api/command` | unchanged | no — units appear via spawn | yes, via the same API the human uses |
| Reporters / comm lasers | yes | unchanged | n/a | yes (symmetric) |
| Command travel delay | `dist(Sol, target) / 0.8c` | unchanged | none — bot writes Truth directly | `dist(home, target) / 0.8c` |
| Information about the world | only events arrived at Sol | unchanged | full omniscience over Truth | only events arrived at 61 UMa |
| Dormancy / spawn cycle | n/a | n/a | `AlienDormancyYears`, `AlienSpawnIntervalYears`, `AlienSpawnComposition` | **removed** |

### 2.2 Both sides win or lose the same way

Today: alien wins by capturing Sol or holding ≥ X% of the human's initial
systems; human wins by exhausting the alien (a counter that ticks up as
alien units are destroyed) plus retaining Sol and ≥ Y% of initial systems.

After: each player wins by capturing the opponent's home, or by holding
≥ X% of the opponent's initial systems (whatever the symmetric formulation
is — see §7). The "alien exhausted" counter goes away unless we want to
preserve it as a bilateral attrition-victory rule.

### 2.3 Both sides get a per-player view

Today there is one `SolView`, written by one `Propagator` from the single
`EventLog`, and broadcast on one SSE stream.

After there are two views (call them `PlayerView` indexed by player ID), two
propagation paths (one per home star, since arrival times differ), and two
SSE streams (one per connected client, each filtered by what has matured at
that player's home).

Each event in the log effectively has **two arrival years** — one per home —
plus an internal/silent flag. An event matures into a player's view when
the clock crosses that player's arrival year for that event.

## 3. Player identity, connections, and lifecycle

### 3.1 Identity

The server must know which player a request comes from. Options to
discuss in §7:

- A query-string or header tag (e.g. `?player=human`, `X-SpaceGame-Player: bot`).
- A token issued at a `POST /api/join` step.
- Inferred from the route (`/api/human/state` vs `/api/bot/state`).
- Inferred from connection order (first SSE = human, second = bot) — not
  recommended; brittle.

### 3.2 Lifecycle

- Server starts with no game running; or starts the game immediately and
  uses two pre-issued tokens; or waits for both clients to join before the
  engine starts ticking. (Discussion in §7.)
- Pause is currently a global flag. With two clients we must decide whether
  either client may pause, only the human can pause, or only an admin
  request can.
- Reconnect: today the SPA reconnects via `EventSource` and re-syncs by
  calling `GET /api/state`. The bot needs the same affordance.
- Disconnect: if the bot dies, does the engine pause, keep ticking, or
  abort the game? Same question for the human.

### 3.3 Local-only

Per the original prompt, the server only accepts loopback connections, so
authentication is unnecessary. We still need *identification* (who is who)
but not security.

## 4. The bot becomes a client

### 4.1 Process model

The bot is a separate executable. Suggested options to discuss:

- A second Go binary in this repo (`srv/cmd/spacebot/main.go`) launched
  manually or by the main server.
- A subprocess auto-spawned by `spacegame` if a `--with-bot` flag is set.
- A separate program in any language (Python, etc.) that calls the API.

Whatever we pick, the bot:

- Authenticates/identifies as the alien player.
- Subscribes to its SSE stream to learn what its home knows.
- Calls `GET /api/state` (filtered to its POV) for snapshots.
- Issues `POST /api/command` like the human, paying the same `dist/0.8c`
  arrival delay.
- Has no access to Truth.

### 4.2 Bot policy / strategy

The bot must now plan under uncertainty. The current `DefaultBot` reads
`state.Truth().Systems` directly to find every human-held system and target
the closest one. That algorithm is invalid in the new model. The bot will
need to:

- Maintain its own model of what the opponent might be doing based on
  delayed reports.
- Decide where to send reporters and comm lasers to gather intelligence.
- Decide what to construct, given economic constraints.
- Decide when to attack vs. defend.

For the **first pass** of symmetric play we should aim for a deliberately
simple bot — e.g. "build escorts at home; periodically dispatch them
toward the most recently known opponent system; build reporters and send
them outward in a sweep." Stronger AI is a follow-on. (See §7.)

## 5. Star catalog and starting positions

### 5.1 Catalog union

The current catalog is loaded from `nearest.csv` (catalog of stars near
Sol) and `planets.csv` (which of them have known planets). Both players
share one catalog.

The new alien-side data files (per the prompt: similar in form to
`nearest.csv` and `planets.csv`, but produced separately as a data task)
define the alien home region. The catalog at game time is the **union** of
both regions, deduplicated by stable ID. 61 UMa is ~31.2 LY from Sol
(9.58 pc), so it is well outside the very-nearest-stars regime, but a
human-region catalog of any meaningful extent and an alien-region catalog
around 61 UMa are still likely to overlap on intermediate stars. The
merge must handle that.

### 5.2 Starting holdings

Each side's starting systems come from the side's own data files, applying
analogous rules:

- Home star: always econ level 5, 1 comm laser, etc.
- Sibling home-region systems with planets: human-owned (or alien-owned),
  Gaussian econ level.
- Sibling home-region systems without planets but inside half-max-distance
  from home: same.
- Anything else in the catalog: uninhabited.

What about a star that qualifies as both a human-region system and an
alien-region system? Tie-breaker rules needed (§7). Plausible default:
the closer home wins.

### 5.3 Coordinates and distances

Star positions are absolute in the catalog (today expressed as Three.js
cartesian relative to Sol). For symmetry we need:

- Distance from any star to any other star (already supported by
  `StarCatalog.Distance(a, b)`).
- Distance from each star to **each home** (today `CatalogEntry.DistFromSol`
  is precomputed; we need the same for `DistFromAlienHome` or, more
  generally, `DistFromHome(playerID)`).
- The SPA renders the map in Sol-relative coordinates today. We can keep
  the same absolute coordinate system; the human player still looks "out
  from Sol" because that's their home. The bot doesn't render anything.

## 6. Codebase impact survey

This is not a design — it is a heat map of where the change will press
hardest.

### 6.1 Areas that need real surgery

- **`srv/internal/game/state.go`**
  - `HumanFaction` and `AlienFaction` are currently asymmetric struct types.
    They should fuse into a single `Faction` (or `Player`) type used twice,
    keyed by player ID. `EntryPointIDs`, `NextSpawnYear`, `Exhausted`, and
    `TotalLost` are likely dropped.
  - `ReadSolGroundTruth` has `"sol"` baked in. Becomes
    `ReadHomeGroundTruth(playerID)` or similar.
  - `truth` remains the single ground truth. `SolView` is replaced by a
    map of two `PlayerView` instances.

- **`srv/internal/game/truth.go`** and `solview.go`
  - `Truth` is largely fine; it is already player-agnostic.
  - `SolView` is renamed `PlayerView`. Two of them live on `GameState`,
    keyed by player ID.

- **`srv/internal/game/propagator.go`**
  - The propagator currently reads matured events from one log and writes
    one view. After: it must propagate each event into each player's view
    when *that* player's arrival time matures.
  - Today `Event.ArrivalYear` is a single number meaning "arrival at Sol."
    Becomes either `ArrivalYearHuman` + `ArrivalYearAlien` or a small map
    keyed by player ID. `arrivalYearFor` (in state.go) is hard-coded to
    `distFromSol`; needs a per-player computation.
  - The `EventLog.PopMatured(clock)` cursor model assumes a single
    "matured" frontier. We need per-player frontiers.
  - For each non-internal event the propagator currently broadcasts the
    `game_event` and `system_update` once. After: it broadcasts to a
    specific player's SSE stream when that player's arrival fires; both
    players may eventually see the same event but at different real times.

- **`srv/internal/game/engine.go`**
  - `EnqueueCommand` hardcodes `OriginID = "sol"` and computes
    `distance("sol", target)`. Becomes per-player.
  - `processFleetArrivals` special-cases `dest.DestID == "sol"` for
    reporter return; becomes "destination is the fleet owner's home."
  - `spawnAlienForces`, `AlienSpawnIntervalYears`, `AlienSpawnComposition`,
    `AlienDormancyYears`, `AlienInitialComposition`: deleted.
  - The bot tick (`Bot.Tick`, `applyBotCommand`) is deleted entirely;
    bot commands now arrive as ordinary `POST /api/command` requests
    from the bot client.
  - `CheckVictory` becomes symmetric (see §2.2).

- **`srv/internal/game/bot.go`**
  - Deleted. The replacement lives in a new package (or new module / new
    binary; see §4.1).

- **`srv/internal/game/loader.go`**
  - Today seeds Sol, populates one side, picks alien entry points, and
    plants alien fleets at those entry points without informing SolView.
  - After: load **two** home-region datasets, populate Truth from both,
    seed each `PlayerView` from the parts of Truth that player would
    legitimately know at game start, and skip the entry-point/spawn logic
    entirely.
  - `gaussianEconLevel`, `EconGrowthIntervalYears`, etc. are reused
    as-is, applied to both sides.
  - Open question: at t=0, does each player know their opponent exists,
    where their home is, or anything about their starting systems? The
    most internally-consistent answer is "each player knows their own
    home region only; opponent systems show as `unknown`." But that
    leaves the human staring at a sparse map for a long time; gameplay
    tuning needed (§7).

- **`srv/internal/server/handlers.go`** and `server.go`
  - Every handler that reads or returns state must accept a player
    identity and serve the correct view.
  - SSE: each subscriber is a player; events are filtered per subscriber.
  - `POST /api/command` validates against the *requesting* player's known
    state, not always SolView.
  - `POST /api/pause`: who may call it? (See §7.)

- **`server_api.md`**
  - Re-spec the API to be player-aware. The wire shape can stay almost
    identical; what changes is that endpoints are scoped to a player and
    that "human" / "alien" become "self" / "foe" relative to the
    requesting player.

### 6.2 Areas that are largely fine

- `combat.go`, `economy.go`, `eventlog.go` event types, weapon
  definitions, `constants.go` (most of it), `catalog.go` — these are
  already faction-agnostic or close to it.
- The SPA (`web/src/`) likely needs no behavioral change; it just keeps
  consuming `/api/state` and `/api/events`. Identification details may
  add one HTTP header. The bot does not use the SPA at all.

### 6.3 Tests

A lot of game tests assume the asymmetric world (alien spawns, alien
dormancy, omniscient bot moves). Those tests will need to be rewritten
for symmetric play. Some property/integration tests probably stay valid
(combat math, economy math, propagation correctness).

## 7. Open Questions for stakeholder discussion

These are the questions that must be answered before specifications and
then a detailed design can be written. They are deliberately spelled out;
the goal is to get answers, not guesses.

### 7.1 Player identification — RESOLVED

1. **Identity carrier.** Required query parameter `?player=human|alien`
   on every `/api/*` request, with the single exception of `/api/stars`
   (the static catalogue is player-independent). Values other than
   `human` or `alien` return `400 {"ok":false,"error":"unknown player"}`.
   No token, no join handshake, no path prefix. A middleware (sibling of
   `recoverMiddleware`) validates the parameter once and attaches the
   player ID to the request context; handlers read it from context.

2. **Game start.** The engine begins ticking automatically once both
   players have an open SSE subscription on `/api/events`. There is no
   explicit "begin" signal. Until the second subscription is open, the
   engine is paused (no ticks, clock not advancing).

3. **Mid-game disconnect.** If either player's SSE subscription drops,
   the engine pauses (sets `Paused=true`) and writes a clearly-marked
   message to the server's stdout — the server is expected to be
   running in a console window the human player can see. Suggested
   format:

   ```
   server: <player> disconnected; game paused
   ```

   No event is broadcast on the surviving stream beyond the normal
   `clock_sync` that pause already emits.

4. **Reconnect.** Not supported in v1. Once a player disconnects the
   game session is effectively over; the server may continue running
   (paused) but does not accept a new subscription as the same player.
   Revisiting reconnect is deferred to a later change.

### 7.2 Pause semantics — RESOLVED

5. **Who may pause.** Only the human. `POST /api/pause` returns
   `403 {"ok":false,"error":"only the human player may pause"}` when
   `?player=alien`. The disconnect-triggered pause described in 7.1.3
   is a server-internal action and does not go through `/api/pause`.

6. **Behavior while paused.** Both SSE streams remain open. While
   paused, the server sends no `game_event` or `system_update` frames
   on either stream. The existing `clock_sync` event — which is already
   broadcast immediately on pause and unpause by `Engine.SetPaused` —
   is the sole signal both clients need to track pause state.

   Keeping the streams open is effectively forced by 7.1.4: tearing
   them down on pause would require a reconnect, which v1 does not
   support.

### 7.3 Symmetric victory conditions — RESOLVED

7. **Win conditions are exactly mirrored.** A player wins by either
   (a) capturing the opponent's home star, or (b) holding ≥ X% of the
   opponent's initial systems, where X is the same threshold for both
   sides. The current asymmetric values (`AlienWinCaptureFraction`,
   `HumanWinRetentionFraction`) collapse into a single shared constant.
   `CheckVictory` is rewritten to evaluate (a) and (b) for each player
   in turn rather than encoding human-vs-alien asymmetry.

8. **Exhaustion counter: kept only if needed for draw detection.** If a
   draw condition (per Q9) is derivable purely from positional state —
   e.g. game-length cap, or a stalemate detector based on holdings —
   the exhaustion counter is dropped entirely. If a draw rule needs
   "neither side has the productive capacity to finish the other off,"
   then a mirrored per-player units-destroyed counter is reintroduced.
   The decision is contingent on Q9 and on what playtesting reveals.

9. **A draw condition exists.** Its exact form is **deferred to
   playtesting** rather than fixed in the design now. The simplest
   plausible rule, and the one we should plan to implement first, is a
   game-length cap: if neither player has won by some elapsed in-game
   year, the game ends in a draw. Other candidates (mutual exhaustion,
   stable equilibrium) can be added if playtesting shows the simple
   length cap is unsatisfying. The requirements document should call
   out that the threshold values (length cap, exhaustion levels if
   used) are explicitly tunable parameters, not load-bearing constants.

### 7.4 Initial knowledge — RESOLVED

**Decision: full mutual knowledge of the *initial* state of every
system on both sides at t=0.** The flavor justification is that each
side has spent a long pre-war period monitoring the other's radio
emissions and has deduced the layout of the opponent's empire. The
light-speed delay applies only to *changes* during the war, not to the
initial snapshot.

10. **Opponent home star.** Each player knows the opponent's home star
    at t=0, knows it is opponent-held, and knows its initial econ level,
    wealth, and unit composition. `KnownSystem.Status` for that star is
    set to the opposing-owner status (e.g. `alien` from the human's
    point of view) with `AsOfYear=0`.

11. **Catalog visibility.** Both players see the full union catalog at
    t=0 (positions, display names, has-planets). Unchanged from today.

12. **Initial system holdings.** Not hidden. At game start each player's
    `PlayerView` mirrors `Truth` for *every* system on the map — their
    own initial systems, the opponent's initial systems, and any
    uninhabited systems — with `AsOfYear=0`. There is no `unknown`
    status at t=0. The status `unknown` may still appear later if the
    design ever produces a system that comes into existence after t=0
    and has not yet been observed, but for v1 the resolved model has no
    such case, so `unknown` is effectively dead at t=0.

12a. **Reporter scouting extension.** Not needed for v1. The combat-
     triggered reporter, the comm laser, and capture-based knowledge
     are jointly sufficient under full-initial-knowledge, because there
     are no opaque systems to scout at t=0 — only stale intelligence
     about what the opponent has *done since*. The reporter behavior in
     the current code stays as-is. (If playtesting shows that opponents
     can hide their internal moves indefinitely and the game stalls, we
     revisit by adding a passive-scout mechanic, but only then.)

**Loader implication.** The current loader's carve-out — "seed SolView
from Truth EXCEPT alien presence at entry points is suppressed" —
disappears. Under the resolved rule, each `PlayerView` is seeded as a
straight mirror of `Truth` at t=0. This is a simplification.

**Persistence-of-stale-knowledge implication.** A `KnownSystem` for an
opponent-held system that the player never touches will stay pinned at
`AsOfYear=0` for the entire game. That is intentional: the player's
view of the opponent's empire is exactly as accurate as it was on the
day the war began, until they go look. This is the core gameplay
tension and the resolved model preserves it cleanly.

### 7.5 Star catalog merging — RESOLVED

13. **Alien-side data file layout.** Two new files in the repo root,
    parallel to the existing `nearest.csv` and `planets.csv`:

    - `alien-nearest.csv` — alien-region star catalog, same column
      schema as `nearest.csv`.
    - `alien-planets.csv` — alien-region planet annotations, same
      column schema as `planets.csv`.

    The contents of these files are produced as a separate data task
    and are not part of this design.

14. **Duplicate handling = data-prep contract, not runtime
    reconciliation.** It is an *external* (data-prep) responsibility to
    ensure that any star physically present in both regions appears in
    both files with the same preferred display name, so that
    `toSystemID` produces the same stable ID for it. Under this
    contract there is no runtime conflict resolution to perform; the
    merge becomes a concatenation.

    The loader's only obligation is **detection, not resolution**: it
    must fail fast and loudly if duplicate IDs appear in the union of
    the two catalogs, naming the offending ID(s) in the error message,
    so that a data-prep mistake is immediately obvious. Silently
    coalescing or arbitrarily picking one entry is forbidden.

    Properties guaranteed by the contract:

    - No star can be a starting system for both sides (one ID, one
      owning file).
    - No two physically distinct stars can collide on a single ID.
    - Coordinate values for a given star are sourced consistently
      (data prep uses one source of truth; the loader does not attempt
      to reconcile coordinate drift).

    The existing same-file grouping in `loadStars` (which collapses
    co-located catalog rows like Alpha Centauri A and B by string-
    equal RA/Dec/distance) is preserved. The cross-file duplicate-ID
    check is the only new guard.

15. **Coordinate frame stays Sol-relative.** The merged catalog
    preserves the existing Three.js cartesian frame with Sol at the
    origin. 61 UMa and its neighbors become additional catalog entries
    at their actual positions in that frame. No re-origining, no dual
    coordinate systems.

    Implications:

    - `CatalogEntry.DistFromSol` remains valid for every entry,
      including alien-region entries (it is just the literal distance
      from Sol).
    - For the alien player, "distance from home" is computed via
      `Catalog.Distance("61-uma", targetID)` on demand, the same way
      the existing code already uses `Catalog.Distance` for inter-
      system travel. We may also choose to precompute and cache a
      `DistFromHome[playerID]` table on `GameState` for hot paths
      (command travel time, comm-laser arrival time), since these
      computations happen on every event.
    - The SPA continues to render in Sol-relative coordinates because
      that is its natural frame; the human player is at Sol. The bot
      doesn't render anything, so frame choice is invisible to it.

### 7.6 Bot strategy and parity — RESOLVED

16. **v1 bot is deliberately simple.** Target behavior, in priority
    order:

    1. Replenish escorts at the home star (61 UMa) using accumulated
       wealth, capped to keep some reserve for other production.
    2. Periodically dispatch escort/battleship fleets toward the most
       recently *known* opponent system — i.e. the opponent system
       whose `KnownSystem.AsOfYear` is highest. This drives action
       along the gradient of recent intelligence rather than against
       a uniformly stale picture.
    3. Sweep reporters outward from home to candidate opponent
       systems on a slow cadence, preferring systems that have not
       been observed for the longest time.

    No tree search, no opponent modeling, no economic optimization.
    The goal is a bot that *plays the same game the human plays*
    well enough to be a meaningful opponent, not one that wins.
    Stronger AI is a follow-on effort.

17. **Bot cannot pause.** Pause is human-only per 7.2.5. The bot has
    no `/api/pause` affordance and no internal pause concept; if its
    own decision-making takes longer than a tick, the engine simply
    advances without bot input that tick. The bot must tolerate
    arriving late to its own decisions.

18. **One developer affordance: a debug-truth flag, off by default.**
    The bot binary takes a `--debug-truth` flag (or equivalent
    environment variable). When set, the bot calls `/api/debug/state`
    — an endpoint that already exists in `server.go` — and writes
    the ground truth to its own log for diagnostic purposes only.
    The bot's decision logic must **not** consume the debug-truth
    output; it is for human inspection of the bot's behavior versus
    reality, not for the bot's own play. With the flag off (the
    default), the bot uses only player-visible state, identical to
    the human.

    The endpoint stays available to the human as well (it is already
    in the route table); whether the SPA exposes it is a UI decision
    out of scope here. Symmetry is preserved: the affordance is a
    *developer* affordance, not a *bot* affordance.

### 7.7 Economy and weapon parity — RESOLVED

19. **Weapon and economy rules are identical for both sides.** A
    single shared `WeaponDefs` table governs both players: same
    costs, same attack/vulnerability, same mobility, same minimum
    economic level. Economic growth follows the existing rule for
    both sides: `EconLevelMean=2.5`, `EconLevelStddev=1.0`, clamped
    to `[1, 4]` for non-home systems; level rises by 1 per
    `EconGrowthIntervalYears` of peace, falls by 1 on combat. No
    side-specific tunables.

20. **61 UMa mirrors Sol at game start.** The alien home star is
    seeded with the same fixed initial state Sol gets today:

    - `Status` = alien-held
    - `EconLevel` = 5 (always max, does not grow further)
    - `Wealth` = 64
    - `LocalUnits` = 1 comm laser
    - One initial fleet ("1st Fleet" in the home system's name)
      stationed at home, containing 2 reporters

    The loader's existing Sol special-case in `loadStars` /
    `Initialize` becomes a parameterised "home-system seed" applied
    once per player, with the home ID supplied per side
    (`"sol"` for the human, `"61-uma"` or whatever ID
    `toSystemID("61 Ursae Majoris")` produces for the alien — the
    actual ID is determined by the alien-side CSV's preferred
    display name).

21. **Reporter and comm-laser semantics are identical for both
    sides, but reports flow to the *owner's* home, not to Sol.**
    Today's `arrivalYearFor(clock, distFromSol, hasCommLaser)` is
    hardcoded to Sol's frame; under symmetry the function must
    compute arrival time relative to whichever player owns the
    reporting system at the moment the event fires. Concretely:

    - **Comm laser** at a system owned by player P reports events at
      that system to P's home at 1.0c
      (`arrivalYearFor(clock, dist(systemID, homeOf(P)), true)`).
    - **Reporter** present at a system owned by P at the moment
      combat begins flees to P's home at 0.8c, delivering its
      report on arrival (`eventYear + dist(systemID, homeOf(P)) /
      0.8`).
    - **Owner determination at the moment of reporting** matters:
      if a system changes hands during combat, the comm-laser report
      goes to the *previous* owner's home (the laser fires its
      report and is then destroyed in the same combat, per existing
      semantics), and the reporter flees to the side it was loyal
      to at combat start.

    This generalises one Sol-specific detail in the propagator and
    in `arrivalYearFor`; everything else stays the same.

### 7.8 Process model for the bot — RESOLVED

22. **Separate Go binary built from this repo.** The bot is a
    second `main` package in this module. It speaks the public HTTP/SSE
    API only. It links to no `srv/internal/game` types directly; its
    request/response shapes come from a small client package that
    knows the wire format. Sharing source with the server is a code-
    reuse convenience; the bot must remain implementable in any
    language by anyone willing to write the wire shapes themselves.

23. **Launched by a shell script in `scripts/`.** A new
    `scripts/run-game.sh` (or similar) starts the server, waits for it
    to be reachable on `127.0.0.1:8080`, then starts the bot pointed at
    that endpoint. The two processes run side by side; either can be
    killed independently for debugging.

    The server itself does **not** spawn the bot. We deliberately
    avoid `--with-bot`-style server-as-supervisor coupling: child-
    process lifecycle, signal forwarding, and log multiplexing are a
    real engineering tax that buys us nothing in this single-machine
    setup. Manual launch (run the server, run the bot in another
    terminal) is also always supported and is the default development
    workflow.

24. **Bot lives at `srv/cmd/spacebot/`** with shared bot code at
    `srv/internal/bot/`. This mirrors the existing
    `srv/cmd/spacegame/` + `srv/internal/game/` + `srv/internal/server/`
    layout, so a developer who knows where the server lives can find
    the bot without surprise.

    Suggested package shape:

    - `srv/cmd/spacebot/main.go` — entry point, flag parsing,
      lifecycle.
    - `srv/internal/bot/agent.go` — the v1 strategy from 7.6.
    - `srv/internal/bot/client.go` — HTTP/SSE client for the public
      API. Used by the bot, but designed to be reusable if we ever
      build a CLI or a second client.
    - `srv/internal/bot/state.go` — the bot's local model of what its
      home knows (a mirror of what would arrive on its SSE stream).

#### Future direction — reinforcement-learning bot (deferred)

A stronger bot trained by reinforcement learning is an explicit future
goal, deliberately out of scope for v1. The decisions in this section
are RL-friendly by accident, not by design:

- The server/bot split through the public API is exactly the
  observation/action boundary an RL agent would want. The bot binary
  becomes one possible *agent harness*; an RL agent is just a
  different bot binary speaking the same API.
- `/api/state` (per-player view) is a natural observation. The
  command shapes are a natural action space. `game_over` provides
  episode termination.

What v1 does **not** provide, and what an RL effort would later have
to add (and is not designed for here):

- A headless / accelerated tick mode that decouples the engine from
  real time. Today the engine ticks on a `time.Ticker`; RL training
  needs faster-than-realtime simulation.
- Deterministic seeding for repeatable rollouts. The RNG is already
  injected (DR-11), so this is a small flag-plumbing exercise, not a
  design change.
- A reset/episode-restart endpoint. v1 has no concept of starting a
  fresh game without restarting the server process.

These are flagged here only so the v1 work doesn't accidentally make
them harder to add later. They are **not** part of the v1
requirements.

### 7.9 API shape changes — RESOLVED

25. **Identity carrier: `?player=human|alien` query parameter.**
    Already decided by 7.1.1; restated here for completeness. No
    header, no token, no path prefix.

26. **`/api/stars` stays shared.** Already implied by 7.1.1, which
    exempts `/api/stars` from the player-required rule. The star
    catalogue is geometric and player-independent; both clients
    receive the same response and may cache it (the existing
    `Cache-Control: max-age=86400` header is preserved).

27. **Wire labels stay absolute: `human` / `alien`, not `self` /
    `foe`.** Every `knownStatus` value, every `Owner` field on
    fleets, every `winner` field, and every `description` string in
    events keeps the absolute owner name. The client derives
    self/foe from its own known identity if it wants; the server
    does not relabel per request.

    Justification:

    - Server-side code already uses `StatusHuman`/`StatusAlien` and
      `HumanOwner`/`AlienOwner` pervasively. Per-request relabeling
      is pure churn.
    - Logs and traces stay unambiguous. "System X became alien-held"
      reads the same way regardless of who is looking; "System X
      became foe-held" raises "foe of whom?".
    - The bot derives `self`/`foe` from one line of code given its
      own player ID; pushing that one line into the server,
      multiplied across every endpoint and every event type, is a
      bad trade.
    - Three of the five `knownStatus` values (`contested`,
      `uninhabited`, `unknown`) are already POV-neutral. Only
      `human` and `alien` are owner-specific, and those are exactly
      the ones whose meaning is most easily preserved as absolute
      names.
    - Aligns with 7.10.28 (keep `Human`/`Alien` in code).

    The wire shapes documented in `server_api.md` therefore change
    very little. The substantive change is that `/api/state` and
    `/api/events` are scoped to the requesting player's known view;
    the field values inside those responses keep their absolute
    owner names.

### 7.10 Naming and terminology — RESOLVED

28. **Code keeps `Human` / `Alien`.** All existing identifiers stay:
    `HumanOwner`, `AlienOwner`, `StatusHuman`, `StatusAlien`,
    `HumanFaction`, `AlienFaction`, and so on. Symmetry is a
    behavioral property, not a naming property. Renaming to
    `Player1`/`Player2` or similar would be pure churn with no
    functional benefit, and would discard the game flavor that
    pervades the codebase, tests, docstrings, log messages, and event
    description strings.

    The two sides *are* unified at the type level — the asymmetric
    `HumanFaction` and `AlienFaction` structs are refactored into a
    single `Faction` (or `Player`) type used twice per 6.1 — but the
    *labels* on the two instances remain `Human` and `Alien`. This
    aligns with 7.9.27 (wire labels stay absolute).

29. **`OLD_SPECS/` is archived as historical, not updated.** The
    directory continues to live in the repo as a record of the
    asymmetric design's original requirements. It is not
    authoritative after the symmetric work lands; `CLAUDE.md` already
    states this ("Specs in OLD_SPECS/ are historical only"), and that
    statement remains accurate. The new requirements and design
    documents land at the repo root or under `MULTI/` (exact location
    to be decided by the requirements pass) — they do not modify
    `OLD_SPECS/`.

### 7.11 Cleanup scope — RESOLVED

30. **Full deletion of all listed asymmetric machinery.** Deleted, not
    deprecated. No back-compat shims, no feature flags, no parallel
    code paths. The concrete delete list:

    From `srv/internal/game/bot.go`:
    - `BotAgent` interface
    - `DefaultBot` struct and methods
    - `BotCommand` struct
    - `humanTargetsByProximity`, `alienInboundTargets`, `totalUnits`
      helpers (move any that turn out to be reused; otherwise delete)
    - The file likely becomes empty and can be removed entirely. The
      bot lives in its own binary per 7.8.

    From `srv/internal/game/engine.go`:
    - `Engine.Bot` field and the parameter on `NewEngine`
    - The bot tick branch in `Engine.tick()`
    - `applyBotCommand()` and its call site
    - `spawnAlienForces()`
    - The alien-spawn cadence check
    - The reporter-fleet-arrives-at-Sol special case is **generalised**
      (not deleted) to "reporter fleet arrives at the fleet owner's
      home" per 7.7.21.

    From `srv/internal/game/constants.go`:
    - `BotTickCadence`
    - `AlienDormancyYears`
    - `AlienSpawnIntervalYears`
    - `AlienSpawnComposition`
    - `AlienInitialComposition`
    - `AlienEntryCount`
    - `PeripheryFraction`

    From `srv/internal/game/state.go`:
    - `AlienFaction.EntryPointIDs`
    - `AlienFaction.NextSpawnYear`
    - `AlienFaction.TotalLost`
    - `AlienFaction.Exhausted`

    The `HumanFaction` and `AlienFaction` types themselves are not
    deleted but are unified into a single `Faction`/`Player` type
    used twice (per 6.1 and 7.10.28).

    From `srv/internal/game/events.go` (or wherever event types live):
    - `EventAlienSpawn` event type and every code path producing it
    - `EventAlienExhausted` event type and every code path producing
      it (the `alien_exhausted` wire event in `server_api.md` is
      removed accordingly)

    From `srv/internal/game/loader.go`:
    - The peripheral-systems-as-entry-points selection block
    - The deliberate `SolView` carve-out that hides alien presence at
      game start (per 7.4 the seeding becomes a straight Truth mirror)
    - The "alien fleets at entry points" seeding

    Tests that exercise any of the deleted machinery are deleted or
    rewritten against symmetric semantics. Tests for player-agnostic
    behavior (combat math, economy growth, propagator correctness,
    catalog loading) are preserved and may need parameterisation
    over player ID.

31. **No parallel asymmetric mode is preserved.** The asymmetric game
    is fully replaced. There is no `--legacy-bot` flag, no branching
    code path, no preserved single-player mode for "quick
    playtesting." Developers who want to play against a weak opponent
    for testing simply run the v1 simple bot from 7.6.

## 8. Out of scope for this round

The following items are explicitly *not* part of the symmetric-multiplayer
investigation. They are listed here so they don't accidentally creep in.

- Network play across hosts (the prompt commits to local-only).
- Authentication or authorization beyond "loopback only."
- More than two players.
- A spectator client.
- The actual content of the alien-side `nearest.csv` / `planets.csv`
  analogues. The original prompt says these are produced as a separate
  data task.
- A new bot strategy beyond the simple v1 described in §4.2.
- Any change to weapon definitions, combat math, or economic rates
  unrelated to symmetry.
- UI changes in the SPA beyond whatever the API rename forces.

## 9. Suggested next step after this discussion

Once the questions in §7 have answers, the next step is to produce:

1. A revised `requirements.md` covering the symmetric game (functional and
   non-functional requirements, with IDs).
2. A revised `design.md` driven from those requirements, naming concrete
   files, types, and interfaces.

Code changes follow from the design, not from this document.
