 ---
  SpaceGame — Code Review

  Summary

  This is a well-structured, thoughtfully-designed game server. The Go server is clean, the concurrency model
  (single writer/engine goroutine, RLock for HTTP handlers) is sound, and the SSE-based push architecture is a
   good fit for the real-time limited-information gameplay. There is one crash-causing bug in the JavaScript,
  a silent displacement bug in the SSE client registry, several design-to-implementation gaps worth knowing
  about, and a handful of nits.

  ---
  Critical Issues

  [critical] ui.js:295 — Non-existent method called on cancel; TypeError crash

  In confirmFleetDestination, the Cancel button's handler calls a method that does not exist:

  modal.content.appendChild(this._cancelButton(() => {
      modal.overlay.remove()
      this.starMap.enterDestinationMode(fleetId, originId)  // ← method does not exist
  }))

  StarMap has enterSelectionMode(mode, fleetId, originId), not enterDestinationMode. Clicking Cancel after
  choosing a fleet destination will throw a TypeError and leave the UI in a broken state (selection mode still
   active, no banner, no way to recover without a page reload).

  Fix:
  this.starMap.enterSelectionMode('fleet', fleetId, originId)

  ---
  Warning Issues

  [warning] handlers.go:95 — SSE client ID collision; second tab displaces first

  clientID := fmt.Sprintf("client-%s", r.RemoteAddr)

  r.RemoteAddr for a browser connecting from localhost will be something like 127.0.0.1:54321. A second tab
  gets a different ephemeral port, so two tabs don't literally collide — but if the browser reuses a port
  (rare but possible), or in any future multi-client scenario, Register silently overwrites the channel in the
   map. The old goroutine continues reading from a channel that will never receive again, and Unregister will
  close the new client's channel when the old one disconnects, immediately killing the second connection.

  A safer key is a UUID or an atomic counter:

  var clientSeq atomic.Int64
  // ...
  clientID := fmt.Sprintf("client-%d", clientSeq.Add(1))

  [warning] combat.go:26 and economy.go:37 — RNG re-seeded per call; same seed possible in same tick

  rng := rand.New(rand.NewSource(time.Now().UnixNano()))

  Resolve and ApplyEconomicCombatPenalty each create a fresh RNG seeded from wall-clock nanoseconds. Multiple
  systems fighting in the same tick (all within a 100 ms window) may produce identical seeds and therefore
  identical random sequences, silently biasing results. Keep one *rand.Rand on the Engine struct and pass it
  down:

  // engine.go
  type Engine struct {
      ...
      rng *rand.Rand
  }
  // NewEngine:
  rng: rand.New(rand.NewSource(time.Now().UnixNano())),

  Then pass e.rng to Resolve and ApplyEconomicCombatPenalty.

  [warning] state.go:334 — Victory denominator includes uninhabited systems

  totalSystems := len(s.Systems)
  // ...
  if float64(alienHeld)/float64(totalSystems) >= AlienWinCaptureFraction {

  The denominator is all systems, including uninhabited ones that the alien bot never targets and cannot
  "hold" in the meaningful sense. This makes the 40% threshold effectively harder than it reads; aliens must
  capture 40% of the entire map including barren systems. The design says "sufficient number of human
  systems." Using len(s.Human.InitialSystemIDs) as the denominator would match the intent.

  [warning] state.go:239 — UpdateKnownStates is O(N·events) per tick

  for _, evt := range s.Events {
      if evt.AppliedToKnown { continue }
      ...
  }

  This scans the entire event list from the beginning on every engine tick. At 10 ticks/second for a 4-hour
  session that's ~144,000 ticks. The event log will likely stay small enough that this is unnoticeable in
  practice, but a simple knownStateAppliedIdx int on GameState (advancing past already-applied events) would
  make this O(new events) with zero allocation.

  [warning] applyEventToKnownState — KnownEconLevel is never updated; AdvanceEconLevels produces no event

  AdvanceEconLevels silently increments sys.EconLevel (ground truth) every 100 in-game years, but no GameEvent
   is recorded for this transition. applyEventToKnownState therefore never updates sys.KnownEconLevel. The
  player's known economy level for remote systems is permanently frozen at its initialization value. They'll
  never see it grow even through a comm laser.

  The fix is either to emit an EventEconGrowth event in AdvanceEconLevels when comm lasers or reporters are
  present, or (simpler for now) to handle econ growth in applyEventToKnownState by checking
  EventConstructionDone for the new unit and cross-referencing level. The cleanest fix is the event: add
  EventEconGrowth to the type enum and emit it from AdvanceEconLevels when hasCommLaser, then handle it in
  applyEventToKnownState.

  [warning] api.js:7–15 — No error handling at startup; unhandled promise rejection

  async function init() {
      const [stars, gameState] = await Promise.all([api.fetchStars(), api.fetchState()])

  If either fetch fails (server not ready, network error, non-2xx response), Promise.all rejects, the await
  throws, DOMContentLoaded handler throws, and the page is silently broken with no user-facing message. The
  individual fetch methods also call .json() without checking r.ok, so a 500 response would try to parse the
  error HTML as JSON.

  async function init() {
      try {
          const [stars, gameState] = await Promise.all([api.fetchStars(), api.fetchState()])
          // ...
      } catch (err) {
          document.body.textContent = `Failed to connect to game server: ${err}`
      }
  }

  And in APIClient:
  async fetchStars() {
      const r = await fetch('/api/stars')
      if (!r.ok) throw new Error(`/api/stars: ${r.status}`)
      return r.json()
  }

  [warning] combat.go:196–203 — Comm lasers excluded from combat; design spec says high vulnerability

  for wt, count := range sys.LocalUnits {
      def := WeaponDefs[wt]
      if def.CommLaser {
          continue // comm laser does not participate in combat
      }

  The design adjustment doc explicitly assigns comm lasers a vulnerability of 10 (high) — they should be the
  first things destroyed. The current implementation makes them invulnerable during combat and only destroys
  them when clearHumanForces is called (i.e., total human defeat). A human-held system with only a comm laser
  and one interceptor effectively has an indestructible comm laser until the last interceptor falls.

  The correct behavior: include the comm laser in collectHumanUnits, let it take fire, and remove it from
  reconcileForces's "preserve" logic if it was destroyed. The pre-combat "report alien arrival" step should
  still fire unconditionally at the start of the round, so the design intent of "always reports at least
  arrival" is preserved.

  [warning] constants.go:29–30 — Unit count constants don't match the compositions

  AlienInitialUnits      = 15  // but AlienInitialComposition sums to 10
  AlienSpawnUnitsPerWave = 10  // but AlienSpawnComposition sums to 7

  These constants are never read by any code — the actual unit counts come from the map literals. They're
  orphaned documentation that is already wrong. Either delete them or derive them:

  // Delete AlienInitialUnits and AlienSpawnUnitsPerWave, or compute:
  func sumComposition(m map[WeaponType]int) int { ... }

  ---
  Nits

  [nit] combat.go:257–260 — Dead code: set-to-zero before the delete

  fleet.Units[WeaponReporter] = 0          // ← sets to 0
  if fleet.Units[WeaponReporter] == 0 {    // ← always true
      delete(fleet.Units, WeaponReporter)
  }

  Replace with:
  delete(fleet.Units, WeaponReporter)

  [nit] bot.go:36–37 — DefaultBot.mu and .targets are declared but unused

  b.mu is never locked or unlocked. b.targets is allocated in the struct but Tick recomputes targets from
  scratch each call without storing them. Remove both fields to reduce confusion for anyone implementing an
  alternative bot.

  [nit] main.go:48 — os.Exit(0) is unreachable

  if err := srv.ListenAndServe(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
      log.Fatalf("server: %v", err)
  }
  log.Printf("SpaceGame server stopped")
  os.Exit(0)   // main() returns normally anyway

  log.Fatalf calls os.Exit(1) internally. If ListenAndServe returns nil or ErrServerClosed, main would exit
  cleanly without os.Exit(0). Remove it.

  [nit] constants.js duplicates constants.go with no enforcement

  WEAPON_DEFS, ECON_WEALTH_RATE, CommandSpeedC, FleetSpeedC, and TimeScaleYearsPerSecond are mirrored in the
  JS client. Any tuning change on the server side must also be applied to the JS file, with no mechanism to
  catch a mismatch. Consider adding a /api/constants endpoint that the client fetches at startup and uses for
  its projections.

  ---
  Proposed Tests

  These cover the correctness issues above. I'll write them now — let me confirm you want them written to a
  new file before I do so.


