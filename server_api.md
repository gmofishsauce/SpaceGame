# SpaceGame Server API

The Go server listens on `http://127.0.0.1:8080` and exposes two surfaces:

1. **Static SPA** — `GET /` and `GET /assets/*` serve the compiled Three.js front-end.
2. **Game API** — five endpoints under `/api/` described below.

All API responses use `Content-Type: application/json` unless noted.
All error responses share the same shape: `{"ok": false, "error": "<message>"}`.

---

## Endpoints

### `GET /api/stars`

Returns the static star catalogue used to render the Three.js map.
This data never changes during a session; the response carries `Cache-Control: max-age=86400`.

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
| `distFromSol` | float64 | Distance from Sol in light-years |
| `hasPlanets` | bool | True if confirmed exoplanets in catalogue |
| `isSol` | bool | True for the Solar System entry only |

---

### `GET /api/state`

Returns a full player-visible snapshot of the current game state.
Each system shows only **known** information derived from events that have arrived at Sol (`arrivalYear ≤ gameYear`).
Sol is special-cased and always shows the current ground-truth state.

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
| `unknown` | No information has reached Sol yet |

**Event `type` values visible to the player**

| Type | Description |
|------|-------------|
| `fleet_arrival` | A fleet arrived at the system |
| `combat_occurred` | Combat took place (reporter or comm laser was present) |
| `system_captured` | System fell to alien forces |
| `system_retaken` | System retaken by human forces |
| `construction_done` | Construction order completed |
| `command_arrived` | A player command reached its target system |
| `command_executed` | Command executed successfully |
| `command_failed` | Command could not execute (insufficient wealth, etc.) |
| `reporter_return` | A reporter fleet returned to Sol |
| `alien_exhausted` | Alien empire has been sufficiently depleted |
| `game_over` | The game has ended |

> **Note:** `combat_silent` and `alien_spawn` events are internal-only and never included in API responses.

---

### `GET /api/events` (SSE stream)

Opens a persistent Server-Sent Events connection. The server pushes events as they mature (`arrivalYear ≤ gameYear`) without polling.

**Headers set by server:**
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

**On connect** the server immediately sends a `connected` event carrying the full current state (same shape as `GET /api/state` but as SSE data):

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
Sent when an event's `arrivalYear` passes the current `gameYear`.
```
event: game_event
data: {
  "id":          "evt-42",
  "arrivalYear": 45.200,
  "systemId":    "proxima-centauri",
  "type":        "combat_occurred",
  "description": "Human forces repelled alien attack. 3 alien units and 2 human units lost.",
  "details":     {"humanLosses":{"interceptor":2},"alienLosses":{"escort":3},"humanWon":true,"alienWon":false,"draw":false}
}
```

#### `system_update`
Sent alongside each `game_event`, carrying the updated known state of the affected system.
```
event: system_update
data: {
  "systemId":        "proxima-centauri",
  "knownStatus":     "human",
  "knownAsOfYear":   40.100,
  "knownEconLevel":  3,
  "knownLocalUnits": {"orbital_defense": 2},
  "knownFleets":     []
}
```

#### `game_over`
Sent when a victory or defeat condition is reached.
```
event: game_over
data: {"winner":"human","reason":"Alien forces exhausted. Earth and 68% of systems retained."}
```

**Reconnection:** The browser `EventSource` API reconnects automatically on disconnect. On reconnection the client should call `GET /api/state` to re-sync any events missed during the gap, since the SSE stream does not replay past events.

---

### `POST /api/command`

Dispatches a player command from Sol. The command is queued with an execution year of `gameYear + dist(Sol, system) / 0.8` (commands targeting Sol itself execute immediately at `gameYear`).

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
| `type` | string | ✓ | `"construct"` or `"move"` |
| `systemId` | string | ✓ | Target system (must be known human-held or Sol) |
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
- System is known to be alien-held
- Insufficient accumulated wealth
- Economic level too low for requested weapon type
- Fleet not found or already in transit

> **Note:** The 400 response reflects validation against the *known* state at the time the command is issued. The ground-truth execution at arrival time may still fail (e.g., the system was captured while the command was in transit). Such late failures are recorded as `command_failed` events and propagated back to Sol.

---

### `POST /api/pause`

Pauses or unpauses the game clock and simulation. A `clock_sync` SSE event is broadcast to all connected clients immediately.

**Request body**

```json
{"paused": true}
```

**Response (200)**

```json
{"ok": true}
```

---

## Weapon Types Reference

| ID | Display Name | Cost | Min Level | Attack | Vulnerability | Mobile | Reports | Comm |
|----|--------------|-----:|----------:|-------:|-------------:|--------|---------|------|
| `orbital_defense` | Orbital Defense | 1 | 1 | low | high | No | No | No |
| `interceptor` | Interceptor | 2 | 1 | medium | medium | No | No | No |
| `reporter` | Reporter | 4 | 1 | none | medium | Yes | Yes | No |
| `escort` | Escort | 8 | 2 | medium | medium | Yes | No | No |
| `battleship` | Battleship | 32 | 3 | high | low | Yes | No | No |
| `comm_laser` | Comm Laser | 64 | 4 | none | high | No | No | Yes |

- **Mobile** — fleet-capable; can be ordered to other systems.
- **Reports** — when present at combat start, flees at 0.8c and carries the combat result back to Sol (`arrivalYear = eventYear + dist / 0.8`).
- **Comm** — when present, all events in the system are immediately reported to Sol at light speed (`arrivalYear = eventYear + dist`). Comm lasers report alien arrival even if subsequently destroyed in the same combat.

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

Sol is always level 5. Non-Sol human systems start at a Gaussian-distributed level (mean 2.5, σ 1.0, clamped to [1, 5]). Economic level rises by 1 per 100 in-game years without combat; any combat reduces level by 1 and resets the clock.

---

## Time Scale

| Parameter | Value |
|-----------|-------|
| In-game years per real second | 10/180 ≈ 0.0556 |
| Tick interval | 100 ms real = 0.00556 in-game years |
| Fleet / command speed | 0.8c (0.8 LY / in-game year) |
| Comm laser report speed | 1.0c (1.0 LY / in-game year) |
