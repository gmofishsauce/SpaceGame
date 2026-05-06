# SpaceGame Server API

The Go server listens on `http://127.0.0.1:8080` and exposes two surfaces:

1. **Static SPA** — `GET /` and `GET /assets/*` serve the compiled Three.js front-end.
2. **Game API** — five endpoints under `/api/` described below.

All API responses use `Content-Type: application/json` unless noted.
All error responses share the same shape: `{"ok": false, "error": "<message>"}`.

---

## Player identification

All `/api/*` endpoints **except `GET /api/stars`** require a `?player=` query parameter
identifying the requesting player. The two valid values are `human` and `alien`.

```
GET  /api/state?player=human
GET  /api/events?player=alien
POST /api/command?player=human
POST /api/pause?player=human
```

Missing or unrecognised values return **HTTP 400**:

```json
{"ok": false, "error": "unknown player"}
```

The state and event stream returned by each endpoint are **scoped to the requesting player**:
each player sees only the events and system knowledge their own faction has accumulated.
Owner labels in the response (`"owner": "human"`, `"owner": "alien"`) are absolute identifiers,
not relative to the caller.

---

## Endpoints

### `GET /api/stars`

Returns the static star catalogue used to render the Three.js map.
No `?player` parameter is required or used.
This data never changes during a session; the response carries `Cache-Control: max-age=86400`.

The catalogue includes stars from both the human and alien regions of the game.

**Response** — JSON array of star objects:

```json
[
  {
    "id":          "sol",
    "displayName": "Sol",
    "x":           0.0,
    "y":           0.0,
    "z":           0.0,
    "distFromSol": 0.0,
    "hasPlanets":  false,
    "isSol":       true
  },
  {
    "id":          "proxima-centauri",
    "displayName": "Proxima Centauri",
    "x":           -1.546,
    "y":            1.183,
    "z":           -3.769,
    "distFromSol":  4.242,
    "hasPlanets":  true,
    "isSol":       false
  }
]
```

**Fields**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Stable system identifier (lowercase, spaces → hyphens) |
| `displayName` | string | Human-readable name |
| `x`, `y`, `z` | float64 | Three.js Cartesian position in light-years (`x`=astro\_x, `y`=astro\_z, `z`=−astro\_y) |
| `distFromSol` | float64 | Distance from Sol in light-years (for human stars); distance from 61 Ursae Majoris (for alien stars) |
| `hasPlanets` | bool | True if confirmed exoplanets in catalogue |
| `isSol` | bool | True for the Solar System entry only |

---

### `GET /api/state?player=<player>`

Returns a full snapshot of the current game state **as seen by the requesting player**.
Each system shows only information derived from events that have propagated to that player's
home system at light speed or via reporter fleets.
The player's own home system always reflects the current ground-truth state.

**Response**

```json
{
  "gameYear":  42.5,
  "paused":    false,
  "gameOver":  false,
  "winner":    "",
  "winReason": "",
  "systems": [
    {
      "id":              "sol",
      "displayName":     "Sol",
      "knownStatus":     "human",
      "knownAsOfYear":   42.5,
      "knownEconLevel":  5,
      "knownWealth":     1320.0,
      "knownLocalUnits": {"orbital_defense": 3, "interceptor": 2},
      "knownFleets": [
        {
          "id":           "fleet-4",
          "name":         "Fleet Delta",
          "owner":        "human",
          "units":        {"escort": 2, "battleship": 1},
          "inTransit":    true,
          "destinationId":"proxima-centauri",
          "arrivalYear":  47.3
        }
      ]
    }
  ],
  "events": [
    {
      "id":          "evt-12",
      "arrivalYear": 38.1,
      "systemId":    "proxima-centauri",
      "type":        "combat_occurred",
      "description": "Human forces victorious. 3 alien units and 1 human unit lost."
    }
  ],
  "pendingCommands": [
    {
      "id":          "cmd-7",
      "type":        "construct",
      "originId":    "sol",
      "targetId":    "proxima-centauri",
      "executeYear": 47.3,
      "description": "Construct escort at Proxima Centauri"
    }
  ],
  "fleetsInTransit": [
    {
      "id":           "fleet-4",
      "name":         "Fleet Delta",
      "owner":        "human",
      "units":        {"escort": 2},
      "inTransit":    true,
      "sourceId":     "sol",
      "destinationId":"proxima-centauri",
      "departYear":   40.0,
      "arrivalYear":  47.3
    }
  ]
}
```

**`knownStatus` values**

| Value | Meaning |
|-------|---------|
| `human` | Human-held as of last report |
| `alien` | Alien-held as of last report |
| `contested` | Last report showed combat with no clear victor |
| `uninhabited` | No faction holds this system |
| `unknown` | No information has reached the player's home yet |

**`winner` values** (non-empty only when `gameOver` is true)

| Value | Meaning |
|-------|---------|
| `human` | Human player won |
| `alien` | Alien player won |
| `draw` | Game ended at the year cap with no victor |

**Event `type` values**

| Type | Description |
|------|-------------|
| `fleet_arrival` | A fleet arrived at the system |
| `combat_occurred` | Combat took place (reporter or comm laser was present) |
| `system_captured` | System fell to the opponent |
| `system_retaken` | System retaken by the human player |
| `system_conquered` | An uninhabited system was claimed by a fleet carrying a comm laser |
| `construction_done` | Construction order completed |
| `command_arrived` | A player command reached its target system |
| `command_executed` | Command executed successfully |
| `command_failed` | Command could not execute (insufficient wealth, etc.) |
| `reporter_return` | A reporter fleet returned to the player's home with intelligence |
| `game_over` | The game has ended |

> **Note:** `combat_silent` events are internal-only and never included in API responses.

---

### `GET /api/events?player=<player>` (SSE stream)

Opens a persistent Server-Sent Events connection. The server pushes events as they mature
(i.e. their propagation delay from the event location to the player's home has elapsed)
without polling. The stream is **scoped to the requesting player** — each player receives
only events routed to their home.

The engine starts paused and auto-unpauses when **both** players have an open SSE connection.
If either player's last SSE connection closes, the engine pauses and logs:
`server: <player> disconnected; game paused`.

**Headers set by server:**
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

**On connect** the server immediately sends a `connected` event carrying the full current
state as seen by the requesting player (same shape as `GET /api/state`):

```
event: connected
data: {"gameYear":42.5,"paused":false,...}
```

**Subsequent event types:**

#### `clock_sync`
Sent every ~10 real seconds, and immediately on pause/unpause.
```
event: clock_sync
data: {"gameYear":42.500,"paused":false}
```

#### `game_event`
Sent when an event's propagation delay has elapsed for the requesting player.
```
event: game_event
data: {
  "id":          "evt-42",
  "arrivalYear": 45.200,
  "systemId":    "proxima-centauri",
  "type":        "combat_occurred",
  "description": "Human forces repelled alien attack. 3 alien units and 2 human units lost."
}
```

#### `system_update`
Sent alongside each `game_event`, carrying the updated known state of the affected system.
```
event: system_update
data: {
  "id":              "proxima-centauri",
  "displayName":     "Proxima Centauri",
  "knownStatus":     "human",
  "knownAsOfYear":   40.100,
  "knownEconLevel":  3,
  "knownLocalUnits": {"orbital_defense": 2},
  "knownFleets":     []
}
```

#### `game_over`
Sent when a victory, defeat, or draw condition is reached.
```
event: game_over
data: {"winner":"human","winReason":"Human forces captured 61 Ursae Majoris."}
```

**Reconnection:** The browser `EventSource` API reconnects automatically on disconnect.
On reconnection the `connected` event re-syncs any state missed during the gap, since the
SSE stream does not replay past events. Reconnect support in the bot client is not
implemented in v1 — a stream drop causes the bot to exit (which pauses the engine).

---

### `POST /api/command?player=<player>`

Dispatches a command from the requesting player's home system. The command travels at 0.8c
and executes at `gameYear + dist(home, target) / 0.8`. Commands targeting the player's own
home execute immediately at `gameYear`.

Validation is performed against the **player's known view** at the time the command is
issued. Ground-truth execution at arrival time may still fail if the situation changed
while the command was in transit; such failures are recorded as `command_failed` events
and propagated back to the player's home.

**Request body**

```json
{
  "type":          "construct",
  "systemId":      "proxima-centauri",
  "weaponType":    "escort",
  "quantity":      1
}
```

```json
{
  "type":          "move",
  "systemId":      "proxima-centauri",
  "fleetId":       "fleet-4",
  "destinationId": "barnards-star"
}
```

**Fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | ✓ | `"construct"`, `"move"`, or `"create_fleet"` |
| `systemId` | string | ✓ | Target system (must be known to be held by the issuing player) |
| `weaponType` | string | for `construct` | One of `orbital_defense`, `interceptor`, `reporter`, `escort`, `battleship`, `comm_laser` |
| `quantity` | int | for `construct` | Number of units to build (defaults to 1 if omitted) |
| `fleetId` | string | for `move` | Fleet to dispatch |
| `destinationId` | string | for `move` | Destination system ID |

**Success response (200)**

```json
{
  "ok":                   true,
  "commandId":            "cmd-42",
  "estimatedArrivalYear": 47.3
}
```

**Error response (400)**

```json
{
  "ok":    false,
  "error": "insufficient wealth: need 32.0, have 18.5"
}
```

**Common error reasons**

- Unknown system ID
- System is known to be held by the opponent
- Insufficient accumulated wealth
- Economic level too low for requested weapon type
- Fleet not found or already in transit

---

### `POST /api/pause?player=<player>`

Pauses or unpauses the game clock and simulation. A `clock_sync` SSE event is broadcast
to all connected clients immediately.

**Only the human player may call this endpoint.** Requests with `?player=alien` return
**HTTP 403**:

```json
{"ok": false, "error": "only the human player may pause"}
```

**Request body**

```json
{"paused": true}
```

**Response (200)**

```json
{"ok": true}
```

---

### `GET /api/debug/state`

Returns the complete authoritative (ground-truth) game state for debugging. No `?player`
parameter is required. This endpoint is not subject to the light-speed information delay
and should not be used by game clients; it exists for developer tooling and the optional
`--debug-truth` bot flag.

**Response** — `{"gameYear": <float>, "events": [...]}`

---

## Weapon Types Reference

| ID | Display Name | Cost | Min Level | Attack | Vulnerability | Mobile | Reports | Comm |
|----|--------------|-----:|----------:|-------:|-------------:|--------|---------|------|
| `orbital_defense` | Orbital Defense | 1 | 1 | low | high | No | No | No |
| `interceptor` | Interceptor | 2 | 1 | medium | medium | No | No | No |
| `reporter` | Reporter | 4 | 1 | none | medium | Yes | Yes | No |
| `escort` | Escort | 8 | 2 | medium | medium | Yes | No | No |
| `battleship` | Battleship | 32 | 3 | high | low | Yes | No | No |
| `comm_laser` | Comm Laser | 64 | 4 | none | high | Yes | No | Yes |

- **Mobile** — fleet-capable; can be ordered to other systems.
- **Reports** — when present at combat start, flees at 0.8c and carries the combat result back to the owning player's home (`arrivalYear = eventYear + dist / 0.8`).
- **Comm** — when present, all events in the system are immediately reported to the system's current owner's home at light speed (`arrivalYear = eventYear + dist`). Comm lasers report events using the ownership at the time of the event, even if subsequently destroyed in the same combat.

Attack/vulnerability numeric values: `none=0`, `low=1`, `medium=3`, `high=10`.
Hit probability per shot: `attackPower / (attackPower + vulnerability)`, clamped to `[0.05, 0.95]`.

---

## Economic Wealth Rates

| Level | Wealth / in-game year |
|------:|----------------------:|
| 0 | 1 |
| 1 | 2 |
| 2 | 4 |
| 3 | 8 |
| 4 | 16 |
| 5 | 32 |

Both home systems start at level 5 with 64 wealth. Non-home owned systems start at a
Gaussian-distributed level (mean 2.5, σ 1.0, clamped to [1, 5]). Economic level rises by
1 per 100 in-game years without combat; any combat reduces level by 1 and resets the clock.
Both human-held and alien-held systems accumulate wealth each tick.

---

## Victory Conditions

The engine checks for victory each tick, in priority order:

1. **Home capture** — a player who holds the opponent's home system wins immediately.
2. **Initial-systems fraction** — a player who holds ≥ 60% of the opponent's initial
   system count wins.
3. **Draw** — if neither condition is met by game year 500, the game ends in a draw.

When a win and a draw would trigger on the same tick, the win takes precedence.

---

## Time Scale

| Parameter | Value |
|-----------|-------|
| In-game years per real second | 10/180 ≈ 0.0556 |
| Tick interval | 100 ms real = 0.00556 in-game years |
| Fleet / command speed | 0.8c (0.8 LY / in-game year) |
| Comm laser report speed | 1.0c (1.0 LY / in-game year) |
