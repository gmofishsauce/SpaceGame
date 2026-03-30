# Requirements: SpaceGame

## 1. Overview

SpaceGame is a single-player, real-time interstellar strategy game served as a single-page web application on localhost. The player commands humanity's defenses against an alien invasion across the ~108 nearest star systems, operating from Earth under a strict limited-information constraint: all data arrives and all commands travel at the speed of light. A built-in bot drives the alien side. A single session lasts 2–4 real-time hours and covers hundreds of in-game years. The game is intended to reward deliberate strategic thought rather than twitch responses.

---

## 2. Users and Roles

| Role | Description |
|------|-------------|
| **Human Player** | Sole interactive user. Plays from Earth's perspective, issuing construction and movement orders, viewing the star map and event log. |
| **Bot Opponent** | Software agent driving the alien side. Not visible or configurable by the player. Interacts with the game engine through a defined API. |
| **Developer** | Runs the server locally; may adjust bot behavior and game parameters in code. |

---

## 3. Functional Requirements

### 3.1 Web Server

- **FR-001:** The system shall include a Go HTTP server that listens on port 8080 on localhost only.
- **FR-002:** The server shall serve a single HTML page (with embedded or bundled CSS and JavaScript) at the root path `/`.
- **FR-003:** The server shall require no external Go dependencies beyond the standard library.
- **FR-004:** The server shall maintain authoritative game state and the internal event log; the client shall not be the source of truth for game state.

### 3.2 Game Initialization

- **FR-005:** At the start of a new game, the system shall load star data from `nearest.csv` and `planets.csv` and determine which systems are human-held at game start.
- **FR-006:** All star systems that have one or more planets in `planets.csv` shall be initialized as human-held.
- **FR-007:** All star systems without planets whose distance from Sol is less than or equal to half the distance of the farthest star in `nearest.csv` shall also be initialized as human-held.
- **FR-008:** Each human-held star system shall be assigned an initial economic level (1–5) drawn from a Gaussian distribution, except Earth (Sol), which shall always be initialized at economic level 5.
- **FR-009:** Alien attacks shall begin from randomized entry points at the periphery of human space; the entry point(s) shall be re-randomized at the start of each new game.
- **FR-010:** No alien-held systems shall appear on the map at game start; alien presence is revealed only through combat events reported back to Earth.

### 3.3 Game Time

- **FR-011:** The system shall maintain an internal game clock measured in pulsar-calibrated in-universe years, initialized to year 0 at game start.
- **FR-012:** Game time shall advance continuously at a fixed rate of 10 in-universe years per 3 real-time minutes (approximately 3.33 in-universe years per real-time minute). This rate shall be a named, tunable constant in the code.
- **FR-013:** The player shall be able to pause and unpause the game by pressing the Escape key. All game simulation, time advancement, and bot activity shall halt while paused.
- **FR-014:** The current in-universe year shall be displayed permanently in the game UI.

### 3.4 Information and the Event Log

- **FR-015:** The system shall maintain an internal event log. Every game event (combat occurrence, combat result, construction completion, fleet arrival, reporter return, failed command execution, alien attack) shall be recorded in the event log with the in-universe year in which it occurred and the star system in which it occurred.
- **FR-016:** The system shall compute, for each event, the earliest in-universe year at which that event's information could have reached Earth, based on the distance of the event's star system from Sol and a light-speed propagation rate. Events that originate from a reporter or similar vessel shall use the reporter's travel time rather than light-speed.
- **FR-017:** The player UI shall only display information about an event after the current game clock has reached or passed that event's Earth-arrival year. No future or in-transit information shall be shown to the player.
- **FR-018:** When the player views a star system's status (via mouseover or right-click), the system shall display the most recent information about that system that has had time to reach Earth, along with the in-universe year that information was current.

### 3.5 Star Map Display

- **FR-019:** The star map shall render all star systems from `nearest.csv` as a 3D interactive display using Three.js, with Sol at the origin, consistent with the existing prototype.
- **FR-020:** Each star system marker shall have a distinct visual appearance based on its most recently known status as received at Earth. Required states are: *human-held*, *alien-held*, *contested* (combat reported), *status unknown or stale*, and *uninhabited*.
- **FR-021:** When the player moves the mouse over a star system marker, the system shall display a popup or overlay showing: the system's name, its most recently known status, the in-universe year that information was current, its known economic level (if any), and known forces present (if any).
- **FR-022:** The existing prototype mouseover behavior (dotted axis-projection lines, name label) shall be retained and extended to include the game status information described in FR-021.
- **FR-023:** Sol shall always display current accurate information (no light-travel delay).
- **FR-024:** Camera navigation (orbit, zoom, pan) shall be retained from the prototype.

### 3.6 Event Sidebar

- **FR-025:** The UI shall include a sidebar displaying a scrolling log of events in the order they became known to Earth (i.e., sorted by Earth-arrival year).
- **FR-026:** Each entry in the event log sidebar shall include at minimum: the in-universe year the information arrived at Earth, the star system involved, and a short description of the event.
- **FR-027:** When the player moves the mouse over an event in the sidebar, the system shall visually highlight the corresponding star system marker in the star map.
- **FR-028:** The sidebar shall scroll automatically to show new events as they arrive, but shall allow the player to scroll back through history.

### 3.7 Player Actions — General

- **FR-029:** The player shall initiate all actions by right-clicking on a star system marker, which shall display a context menu of available actions for that system.
- **FR-030:** The context menu shall only present actions that are valid for the selected system given its most recently known state as received at Earth.
- **FR-031:** All player commands issued to a non-Sol star system shall be subject to a command-travel delay equal to the distance from Sol to the target system divided by 0.8 (in in-universe years), representing transmission at the speed of light. Commands to Sol take effect immediately.
- **FR-032:** When presenting construction or fleet movement options for a distant system, the UI shall display projected estimates of the system's state at the time the command is expected to arrive, not the current known state.
- **FR-033:** If a command arrives at a star system and cannot be executed as issued (e.g., insufficient economic output, forces no longer present), the system shall record a failed-execution event in the event log. This event shall propagate back to Earth subject to normal light-travel delay and appear in the player's event sidebar upon arrival.

### 3.8 Player Actions — Construction

- **FR-034:** When a system is capable of construction, the right-click context menu shall include a "Construct…" option.
- **FR-035:** Selecting "Construct…" shall open a dialog displaying the available weapon types and their costs, filtered to what the system's (projected) economic level can support.
- **FR-036:** The player shall select a weapon type to construct; the construction order shall be dispatched and arrive after the command-travel delay.

### 3.9 Player Actions — Fleet Command

- **FR-037:** When a system has one or more fleets with interstellar capability, the right-click context menu shall include a "Command…" option.
- **FR-038:** Selecting "Command…" shall open a dialog from which the player can select a fleet present in the system and a destination star system.
- **FR-039:** Upon confirmation, the fleet movement order shall be dispatched and arrive after the command-travel delay. The fleet shall then depart and travel to the destination at 0.8c.

### 3.10 Forces and Weapons

- **FR-040:** The system shall support the following weapon types, in ascending order of cost and capability:

| Weapon Type | Interstellar Capable | Notes |
|-------------|---------------------|-------|
| Orbital Defense | No | Automated local defense; lowest cost |
| Interceptor | No | Local combat ships; no interstellar drive |
| Reporter | Yes | Unarmed; returns to Earth automatically if combat occurs in its system |
| Escort | Yes | Armed interstellar vessel; mid-tier cost |
| Battleship | Yes | Armed interstellar vessel; highest cost and combat power |

- **FR-041:** Interstellar-capable ships (reporters, escorts, battleships) shall travel between star systems at 0.8c. This speed shall be a named, tunable constant in the code.
- **FR-042:** Ships with interstellar drives shall be grouped into named fleets. Fleet names shall be automatically assigned by the system (e.g., "Fleet 1", "2nd Battle Group").
- **FR-043:** Each star system shall track quantities of each weapon type present, grouped into fleets where applicable.
- **FR-044:** The minimum economic level required to construct each weapon type shall be a named, tunable constant in the code.

### 3.11 Economic System

- **FR-045:** Each human-held star system shall have an economic level between 1 and 5 inclusive.
- **FR-046:** Economic output shall accumulate over time in each system as a function of its economic level. The rate of accumulation per economic level shall be a named, tunable constant in the code.
- **FR-047:** Constructing a weapon shall deduct its cost from the system's accumulated economic output. Construction that exceeds available output shall not be permitted.
- **FR-048:** Economic levels and output shall not be directly actionable by the player; they are properties of the system that grow or change due to game events.

### 3.12 Combat

- **FR-049:** Combat shall occur automatically when alien and human forces are present in the same star system at the same in-universe time.
- **FR-050:** Combat shall be resolved entirely by the game engine as a stochastic function of the types and quantities of forces present. The player has no input into combat resolution.
- **FR-051:** Combat shall only occur within star systems, never during interstellar transit.
- **FR-052:** A combat event shall be recorded in the internal event log regardless of whether reporting forces are present.
- **FR-053:** A combat event's results shall only propagate to Earth (and appear in the event sidebar) if a reporter or other reporting-capable vessel was present in the system during combat. Otherwise, only the occurrence of combat (not its outcome) may eventually be inferred from other signals.
- **FR-054:** A star system captured by alien forces shall change its status to alien-held. Human forces may subsequently retake a system, returning it to human-held status.

### 3.13 Victory and Defeat

- **FR-055:** Alien attacks shall diminish over time as a function of cumulative alien losses, representing exhaustion of the alien empire.
- **FR-056:** The human player wins if alien attacks cease (alien exhaustion is reached) while Earth remains human-held and a sufficient number of human systems are retained. The exact threshold shall be a named, tunable constant.
- **FR-057:** The alien bot wins if it captures Earth, or if it captures a sufficient number of human systems before alien exhaustion. The exact threshold shall be a named, tunable constant.
- **FR-058:** When a victory or defeat condition is reached, the system shall pause the game and display a game-over screen identifying the outcome.

### 3.14 Save and Load

- **FR-059:** The player shall be able to save the current game state at any time during play.
- **FR-060:** The player shall be able to load a previously saved game, restoring all game state including the event log, all forces, all system statuses, and the game clock.
- **FR-061:** The system shall support at least one save slot. Multiple save slots are not required for the initial version.

### 3.15 Bot API

- **FR-062:** The alien bot shall interact with the game engine exclusively through a defined programmatic API. No bot logic shall be embedded directly in the game engine.
- **FR-063:** The bot API shall expose, at minimum: the ability to query current (ground-truth) game state, the ability to issue movement orders for alien forces, and the ability to receive notification of game events.
- **FR-064:** The bot API shall be designed so that an alternative bot implementation (including a future reinforcement-learning agent) can be substituted by replacing a single module, with no changes to the game engine.

---

## 4. Non-Functional Requirements

- **NFR-001:** The application shall run entirely in a modern desktop web browser (Chrome, Firefox, or Safari, current versions) with no browser plugins required.
- **NFR-002:** The Go server shall start and be ready to serve requests within 2 seconds of launch on a typical developer laptop.
- **NFR-003:** The game simulation shall run in real time without perceptible stuttering or lag on a modern developer laptop.
- **NFR-004:** The application shall not require an internet connection at runtime; all assets (including the Three.js library) shall be embedded or served locally.
- **NFR-005:** All tunable game parameters (time scale, ship speed, economic rates, weapon costs, victory thresholds) shall be defined as named constants in a single location in the code to facilitate gameplay tuning.
- **NFR-006:** The application is intended for single-user local use only. No authentication, HTTPS, or multi-user support is required.

---

## 5. Data Requirements

### 5.1 Source Files

| File | Contents |
|------|----------|
| `nearest.csv` | ~108 star records: RA, Dec, distance, catalog name, common name |
| `planets.csv` | Planet records keyed to stars in `nearest.csv` |

### 5.2 Star System Entity

Each star system in the game shall maintain:

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Catalog name or GJ designation |
| `displayName` | string | Common name if available, else catalog name |
| `position` | (x, y, z) float | Light-years from Sol, computed at startup |
| `economicLevel` | int 1–5 | Initialized at game start, may change |
| `accumulatedOutput` | float | Economic output available for construction |
| `status` | enum | human-held, alien-held, contested, uninhabited |
| `forcesPresent` | list | Fleets and local weapon counts |
| `lastKnownState` | struct | Most recent state received at Earth, with timestamp |

### 5.3 Event Log Entry

| Field | Type | Notes |
|-------|------|-------|
| `eventYear` | float | In-universe year the event occurred |
| `arrivalYear` | float | In-universe year info reaches Earth |
| `systemId` | string | Star system where event occurred |
| `eventType` | enum | combat, construction, arrival, reporter-return, command-failed, alien-attack |
| `details` | struct | Event-type-specific payload |

### 5.4 Volume

- ~108 star systems. No database required; all state held in server memory and persisted to a save file on demand.

---

## 6. Integration Requirements

- **IR-001:** The client shall use the Three.js library for 3D rendering and OrbitControls for camera navigation. The library shall be served locally.
- **IR-002:** No other external integrations are required.

---

## 7. Constraints and Assumptions

### Constraints
- Server language: Go, standard library only (no third-party Go packages).
- Frontend rendering: Three.js (no other 3D framework).
- Server port: 8080, localhost only.
- Single-player local application; no network play, no authentication.
- The existing prototype code in `Proto/` may be used as a foundation but will require significant extension.

### Assumptions
- The player is always logically located at Earth (Sol). All information delays are computed relative to Sol.
- Relativistic time dilation effects on crew or ships are not modeled; only signal/command travel delay is simulated.
- The alien bot has access to ground-truth game state (it "knows" what is happening everywhere); only the human player is subject to information delay.
- Star positions are static; stellar motion over game timescales is not modeled.
- Economic levels do not change during normal play unless altered by game events (e.g., a captured system).

---

## 8. MVP Scope

The following requirements constitute the minimum viable game — a playable session with core mechanics functioning:

**Must have for MVP:**
`FR-001` – `FR-016` (server, initialization, time, event log core)
`FR-019` – `FR-024` (star map display)
`FR-025` – `FR-028` (event sidebar)
`FR-029` – `FR-039` (player actions: construction and fleet command)
`FR-040` – `FR-048` (forces and economics)
`FR-049` – `FR-054` (combat)
`FR-055` – `FR-058` (victory/defeat)
`FR-062` – `FR-064` (bot API)
`NFR-001` – `NFR-005`

**Deferred post-MVP:**
- `FR-059` – `FR-061` (save/load) — important but not required for first playable
- Multiple save slots
- Varying star marker appearance by spectral type or magnitude
- Touch/mobile support
- Two-player or networked play

---

## 9. Open Questions

- **OQ-001:** What are the exact numerical thresholds for victory/defeat (number of systems, percentage of economic output captured, etc.)? To be determined from early gameplay. See FR-056, FR-057.
- **OQ-002:** What is the exact shape of the Gaussian distribution for initial economic levels? Parameters (mean, standard deviation, clamping) to be tuned in code.
- **OQ-003:** Does alien exhaustion accelerate smoothly with alien losses, or does it trigger at threshold events? Mechanics to be refined from gameplay.
- **OQ-004:** When a player issues a fleet command to a system that already has orders en route, do the new orders supersede the old ones, queue behind them, or require the player to cancel? To be designed.
- **OQ-005:** What visual treatment distinguishes the "stale" information state from "current" information on the star map — e.g., faded color, italic label, a timestamp indicator?
- **OQ-006:** Should the game support difficulty levels (affecting bot aggressiveness or alien exhaustion rate) in the initial version, or is a single difficulty sufficient?
- **OQ-007:** Are there any weapon types beyond the initial five that should be added to the hierarchy before first play? The original design had a larger hierarchy; to be revisited after early gameplay.

---

## 10. Glossary

| Term | Definition |
|------|------------|
| **Bot** | The software agent that controls the alien side of the game. |
| **Bot API** | The programmatic interface through which the bot interacts with the game engine. |
| **Economic Level** | An integer 1–5 representing the industrial and population capacity of a star system. Level 5 approaches Kardashev Type II. |
| **Earth-arrival year** | The in-universe year at which information about a distant event would reach Sol, computed as event year + (distance ÷ speed of light). |
| **Event Log** | The server's authoritative, ground-truth record of all game events, timestamped in pulsar-calibrated in-universe years. |
| **Fleet** | A named group of interstellar-capable ships traveling or stationed together. |
| **Interceptor** | A local combat vessel with no interstellar drive. |
| **Kardashev Level** | A scale of civilizational energy use. Level 2 represents full utilization of a star's output. |
| **Light-travel delay** | The time required for information or a command to travel from Sol to a distant star system at the speed of light. |
| **Orbital Defense** | Automated local defensive weapons, analogous to close-in weapon systems (CIWS). |
| **Pulsar time** | A universal time standard synchronized across human space using millisecond pulsars, allowing consistent timestamping despite the lack of FTL communication. |
| **Reporter** | An unarmed interstellar vessel that automatically returns to Earth to report combat events. |
| **Sol** | Earth's sun, located at the origin (0, 0, 0) of the star map. The player's logical location throughout the game. |
| **Star Tank** | In science fiction, a three-dimensional display showing nearby space and star systems, as found on the bridge of a starship. |
| **0.8c** | The default travel speed of interstellar-capable vessels: 80% of the speed of light. A tunable constant. |
