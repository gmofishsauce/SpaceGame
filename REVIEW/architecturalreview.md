# Architectural Review: SpaceGame Dual-State Model

## Bottom line

The dual-state architecture **is implemented in form** but is **leaky in substance**. The code maintains both ground-truth fields and `Known*` fields on each `StarSystem`, and gates HTTP responses by `Known*` fields — that part works. But the design has several places where the player's view aliases ground-truth objects, where state propagation bypasses the light-speed model, and where the "known" projection silently drops important events. The result is that a careful player paying attention to the SSE stream and the UI will sometimes see information they should not have, and will sometimes miss information they should have.

I would describe the implementation as **adequate for an MVP demo** but **not adequate to deliver the experience the ReviewPrompt describes**. The fixes are not catastrophic — most are local — but there is one structural change worth doing now while the code is still small.

---

## Concrete defects in the existing dual-state implementation

I found these by reading `srv/internal/game/state.go`, `engine.go`, `events.go`, `combat.go`, `economy.go`, and `srv/internal/server/handlers.go`. They are listed roughly by severity.

### 1. The player's view aliases live ground-truth `Fleet` objects (severe leak)

`StarSystem.KnownFleetIDs` is a `[]string` of fleet IDs, and the SSE serializer (`buildKnownFleets` in `events.go`) and the REST serializer (`buildSystemDTO` in `handlers.go`) both look up those IDs in the *single global* `state.Fleets` map and serialize the fleet's **current** units. There is no snapshot of the fleet's composition at the time the player learned about it.

Consequences:
- After a combat at a distant system reduces a fleet from 6 to 2 battleships, the player's UI updates to "2 battleships" *immediately at engine time of combat*, regardless of light-speed delay or whether a reporting mechanism existed.
- The same applies to in-transit fleets: `humanFleetsInTransit` and `BroadcastFleetDeparted` send live `Fleet` objects to all clients the moment the fleet departs at the *destination's* clock (not Sol's).
- Cross-cuts the entire stated requirement that the player only sees what has propagated to Sol.

### 2. `BroadcastFleetDeparted` ignores light-speed (severe leak)

In `engine.go` `tick()`, when a `CmdMove` matures and is applied at a distant system, `e.Events.BroadcastFleetDeparted(f)` fires immediately at the current engine clock. The player learns the fleet has departed at engine_clock = ExecuteYear, but in Sol's frame they should learn this at engine_clock + distFromSol (or never, if no comm laser is at the source).

This is the cleanest example of "ground truth pushed straight to the client". The fix is to record this as a normal `GameEvent` with the proper `ArrivalYear` and `CanReport` gating, the same as a fleet *arrival*.

### 3. Construction of mobile units is invisible to the player's known-state (gap)

`applyEventToKnownState` in `state.go` handles `EventConstructionDone` like this:

```go
if !WeaponDefs[d.WeaponType].CanMove {
    sys.KnownLocalUnits[d.WeaponType] += d.Quantity
}
```

That branch silently drops the event for any mobile unit (Reporter, Escort, Battleship, CommLaser). New battleships built at a distant system are never reflected in `KnownLocalUnits` *or* in `KnownFleetIDs` — they only appear when (and if) that fleet eventually arrives somewhere the player can observe. The player cannot tell that their construction order was successfully fulfilled at a system that has a comm laser, even though the event itself broadcasts.

### 4. `KnownWealth` is never updated by any event

Search `applyEventToKnownState`: it updates known status, units, econ level, fleets — but no case mutates `KnownWealth`. Wealth changes happen in `AccumulateWealth` (continuous) and `ExecuteConstruct` (deduct) — both ground-truth-only. The client's `getProjectedWealth` extrapolates from `KnownWealth` × `knownEconLevel` × elapsed years, but the base it extrapolates from is never advanced by reported construction. Outcome: after a remote system has built a comm laser and reported it, its wealth display is permanently stale (off by the cost of the laser, plus all subsequent construction).

### 5. Reporter fleets do not actually carry information

`extractAndSendReporters` in `combat.go` spawns a new in-transit fleet toward Sol, but the *combat event* it is supposed to deliver is a separate `GameEvent` whose `ArrivalYear` is computed by `reportArrivalYear` at the time of combat. The reporter fleet's only function on arrival (`processFleetArrivals`) is to emit a decorative `EventReporterReturn`. If the reporter fleet were intercepted en route — currently impossible because they have no combat presence anywhere they pass through — the report would still arrive. The two should be coupled: the combat event's broadcast should be conditional on the reporter actually surviving the trip.

### 6. The event store is unbounded and re-scanned per tick

`UpdateKnownStates` and `EventManager.BroadcastMatured` both scan the entire `state.Events` slice every tick (100 ms). `Broadcast` and `AppliedToKnown` are flipped to `true` on each, but the slice itself grows forever. With 200 systems and a long game this becomes O(N) work per tick at the engine's hot path. There is also no per-system index, so any future feature like "show the last 10 events at this system" requires another full scan.

### 7. Distinct propagation channels are conflated into a single `ArrivalYear`

There are at least three propagation channels:
1. Comm laser at *c*
2. Surviving reporter at 0.8 *c*
3. Unreported (the "silent" frame)

All three collapse into one `float64` field on `GameEvent`. This works for binary "did it arrive yet?" gating but loses any future ability to model lossy/competing reports (e.g., reporter delivers, then a later contradictory comm-laser-borne update arrives), or to model uncertainty intervals.

### 8. Bot commands are instantaneous and bypass the command pipeline

`applyBotCommand` runs immediately (`// no travel delay`). For symmetry with the model and for future flexibility (alien comm channels, alien comm-jamming, etc.), the bot should produce real `PendingCommand` records traveling at the alien-equivalent of `CommandSpeedC`. The implementation gap is small but the conceptual gap is large.

### 9. Rebroadcast on every matured event (chatty)

For every matured event, `BroadcastMatured` sends a `game_event` *and* a `system_update` for the event's system. Multi-event ticks get redundant `system_update`s. The system snapshot itself contains the entire fleet array, so a system with two reported fleets re-emits both fleets each time anything changes. Clients that lose buffer space (`safeSend` returns false) silently lose events with no recovery path.

### 10. `Status` vs `KnownStatus` discipline relies on convention

Engine code touches `sys.Status`; handlers should touch `sys.KnownStatus`; bot is allowed to touch `sys.Status` (alien is omniscient). There is no compile-time barrier preventing handlers from reading `sys.Status` by accident. The fields are 4 lines apart in a struct of 21 fields.

### 11. Initial alien intelligence leak via `KnownAsOfYear`

In `loader.go`, every system's `KnownAsOfYear = 0.0` is set, but for entry-point systems whose `Status` is silently flipped to `StatusAlien` after that initialization, the `KnownStatus` remains "uninhabited"/"human" — good. However the initial alien fleet is added to `state.Fleets` but **not** to any `KnownFleetIDs`. Because of defect #1, however, the alien fleets in `state.Fleets` would surface to the client through any code path that walks `state.Fleets` directly. Today there are none on the player path, but it is one unguarded loop away.

### 12. No reproducibility seam

`rand.New(rand.NewSource(time.Now().UnixNano()))` in `Initialize`, again in `engine.NewEngine`. No way to seed for tests, no way to record a game seed for replay. Combat is non-deterministic; a regression test for combat outcome is impossible.

---

## Architectural recommendations

I would prioritize these in roughly this order. Items 1 and 2 are the only ones that change the structure of the code; the rest are local fixes inside the structure.

### A. Separate "ground truth" from "Sol's view" into two distinct types

This is the single most valuable refactor. Today `StarSystem` has 12 mixed fields. Replace with:

```
type Truth struct {
    // omniscient world state — engine writes, bot reads
    Systems map[string]*TrueSystem
    Fleets  map[string]*TrueFleet
    Clock   float64
}

type SolView struct {
    // what Sol knows, advanced by reported events maturing at Sol's clock
    Systems    map[string]*KnownSystem
    Fleets     map[string]*KnownFleet   // snapshots, not pointers into Truth
    KnownAsOf  map[string]float64       // per-system "last news from" year
    Pending    []*PendingCommand
    InFlight   []*KnownFleetTransit     // separate from KnownFleet
}
```

The HTTP layer takes a read lock on `SolView` and never even imports `Truth`. The engine writes to `Truth`, generates `Event`s, and a single propagation step copies matured events into `SolView`. This:

- makes leak #1 and #2 *structurally* impossible — handlers cannot see `Truth.Fleets` even by mistake,
- replaces the convention "use the right field" with a type-system guarantee,
- gives you a clean place to snapshot `Fleet.Units` at the moment of report,
- makes it obvious where to plug in additional channels later (item D).

Cost: two days of refactoring, mostly mechanical. The sooner the better — every new feature that touches state has to pick the right field, and the field count is growing.

### B. Promote `Event` to be the single propagation primitive

Right now the code has three parallel propagation paths:
1. `GameEvent` with `ArrivalYear`, applied via `applyEventToKnownState` and broadcast by `BroadcastMatured`.
2. `BroadcastFleetDeparted` — fires synchronously, no light-speed gate.
3. `BroadcastClockSync`, `BroadcastGameOver` — out-of-band global state.

(1) is the real model. (2) and (3) leak around it. Make every state change at a remote system go through path (1). A fleet departure from S becomes:

```
EventFleetDeparted{
    EventYear: cmd.ExecuteYear,
    SystemID:  S,
    ArrivalYear: arrivalYearFor(EventYear, S.DistFromSol, hasCommLaser),
    Details: { fleetSnapshot, destination, expectedArrival },
}
```

Then `BroadcastMatured` picks it up at Sol's clock and emits the `fleet_departed` SSE only when it actually arrives. This is the same plumbing already used for `fleet_arrival`; reuse it.

### C. Snapshot, don't reference

When a `Known*` view records that a fleet exists, store a frozen copy of its composition at the time the report was generated, not a pointer or ID-into-truth. `FleetArrivalDetails` already has the right shape (`Units map[WeaponType]int`); commit to it everywhere. `KnownFleet` should be a value type with a `(asOfYear, units, location)` triple.

### D. Make propagation channels explicit

Replace `arrivalYearFor(clock, dist, hasCommLaser bool)` with:

```go
type Channel int
const (
    ChannelInternal  Channel = iota // never reported; truth-only
    ChannelCommLaser               // c
    ChannelReporter                // 0.8c, conditional on reporter survival
)

func (c Channel) Arrival(clock, dist float64) (float64, bool) { ... }
```

Then an event is generated with a list of `(Channel, requirements)` and the engine schedules the earliest one whose requirements are met (e.g., reporter fleet has not been destroyed). This sets you up for richer scenarios — e.g., a reporter that arrives carrying outdated news two years after a comm-laser report has already updated Sol's view; today both collapse into "earliest year wins".

### E. Couple reporter survival to the report

A reporter dispatched from a distant combat is currently a fire-and-forget fleet; the report is a separately-keyed event with its own arrival year and is unconditional. Either:

- **Simple fix**: when the reporter fleet is destroyed in transit (today: never; future: possible), invalidate any pending events keyed to its arrival.
- **Better**: the report *is* the fleet. The fleet carries the event payload; the event matures only on fleet arrival. This collapses items D and E.

### F. Index and bound the event log

- Add `state.EventsBySystem map[string][]*GameEvent` populated as events are recorded.
- Maintain a single integer cursor `nextUnmatured` — events strictly before it are guaranteed broadcast/applied. Walk forward from the cursor each tick.
- Consider a ring or compacting strategy: events whose `ArrivalYear < clock - K` and `Broadcast && AppliedToKnown` can be moved to a cold tier or dropped if you do not need a historical log.

### G. Make randomness seedable

Have `Initialize` and `NewEngine` accept `*rand.Rand` (or a `Source`) injected by `main.go`. Default to time-based, but allow a `--seed N` flag. Tests then become deterministic, which is the only way you will catch combat regressions.

### H. SSE: dirty-system batching

After a tick, broadcast at most one `system_update` per distinct system that had at least one matured event, and batch matured events into a single `game_event` array per tick. This both reduces redundant frames and gives the client a consistent view at each tick boundary. While you're there, give `safeSend` a real recovery path — when a buffer overflows, mark the client "lost", drop them, and let the client reconnect via `/api/state`.

### I. Ground truth for the bot is fine

The bot legitimately reads `Truth` directly — aliens are omniscient by design choice (`bot.go` `humanTargetsByProximity` reads `sys.Status`). Keep that. Just make it impossible for handlers and SSE serializers to do the same, by giving them only `SolView`.

### J. Keep the dual representation honest with a property test

Add a test that, for a randomly seeded game played to year T, the events that have been broadcast to a hypothetical client are sufficient to reconstruct `SolView` from scratch. If reconstruction diverges, you have a leak (information present in `SolView` not in events) or a gap (events not applied). This is the cheapest way to keep the structural separation honest as the code evolves.

---

## Summary table

| Issue | Severity | Fix difficulty |
|---|---|---|
| Live-fleet aliasing in `KnownFleetIDs` (#1) | High — direct leak | Local + part of refactor A/C |
| `BroadcastFleetDeparted` skips light-speed (#2) | High — direct leak | Local; covered by refactor B |
| Mobile-unit construction not in known state (#3) | High — gap | Local fix in `applyEventToKnownState` |
| `KnownWealth` never advanced (#4) | Medium — gap | Local fix in `applyEventToKnownState` |
| Reporter does not really carry the report (#5) | Medium — model error | Local; refactor E |
| Unbounded event log scan per tick (#6) | Medium — perf | Refactor F |
| Conflated propagation channels (#7) | Low for now, structural later | Refactor D |
| Bot commands instantaneous (#8) | Low | Local |
| Chatty / lossy SSE (#9) | Medium | Refactor H |
| `Status`/`KnownStatus` discipline by convention (#10) | Medium — latent leak risk | Refactor A |
| Alien fleet leakage potential (#11) | Latent | Refactor A removes it |
| Non-reproducible RNG (#12) | Medium for testability | Refactor G |

---

## My recommendation

Do **A** (split `Truth` from `SolView`), **B** (single propagation primitive), **C** (snapshot fleets), and the local fixes for **#3** and **#4** before you build any more game features. After that, the implementation will match the vision in the ReviewPrompt — and crucially, will keep matching it as new features are added, because the type system will enforce it instead of the next person remembering to type `KnownStatus` instead of `Status`. **F**, **G**, **H** can wait until they bite. **D** and **E** become natural once **A**+**B** are in place.

I have not produced a `design.md` because there is no `requirements.md` — the request was for an architectural opinion, and the codebase is authoritative per `CLAUDE.md`. If you want me to convert these recommendations into a formal design document (with requirement IDs, file/directory plan, and traceability matrix) so a developer can implement it without further consultation, point me at a requirements file or tell me to derive one from the ReviewPrompt + `CLAUDE.md`, and I will produce one.
