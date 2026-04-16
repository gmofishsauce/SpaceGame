# SpaceGame

A limited-information real-time strategy game set in an interstellar war. A human player defends a sphere of colonized star systems against alien incursions, hampered by light-speed communication delays. The code is authoritative; specs in `OLD_SPECS/` are historical only.

## Architecture

**Go server** (`srv/`) — authoritative game engine + HTTP API on `127.0.0.1:8080`.
- Entry point: `srv/cmd/spacegame/main.go`
- Game engine: `srv/internal/game/` — engine loop, combat, economy, bot AI, event system
- HTTP layer: `srv/internal/server/` — REST handlers + SSE streaming
- The engine holds a mutex-protected `GameState`; the engine is the sole writer, HTTP handlers take read locks

**JavaScript SPA** (`web/src/`) — Three.js star map + sidebar UI, no framework.
- `main.js` boots the app; `starmap.js` renders the 3D map; `sidebar.js` and `ui.js` handle the control panel; `api.js` talks to the server; `state.js` is the client-side state store
- Built with Vite; output goes to `web/dist/` which is `//go:embed`-ed into the Go binary (`web/embed.go`)

**API** — documented in `server_api.md`:
- `GET /api/stars` — static star catalogue
- `GET /api/state` — full player-visible game snapshot
- `GET /api/events` — SSE stream (clock_sync, game_event, system_update, game_over)
- `POST /api/command` — issue construct/move commands
- `POST /api/pause` — pause/unpause

**Data files** — `nearest.csv` and `planets.csv` in repo root, loaded at startup.

## Build & Run

```bash
# Build frontend (requires npm; commit web/dist/ so others skip this)
scripts/build-frontend.sh

# Build server (no npm needed if web/dist/ is committed)
go build -o spacegame srv/cmd/spacegame/main.go

# Run from repo root (needs nearest.csv and planets.csv in cwd)
./spacegame
# Then visit http://localhost:8080
```

For frontend dev, run `cd web && npm run dev` for Vite's dev server (proxied or separate).

After any frontend source change, run `scripts/build-frontend.sh` and commit the updated `web/dist/` alongside the source changes.

## Key Game Concepts

- **Limited information**: player only sees events that have propagated to Sol at light speed (or 0.8c via reporters). The `Known*` fields on `StarSystem` track this.
- **Commands travel at 0.8c**: orders issued from Sol arrive after `distance / 0.8` game years. They can fail on arrival if the situation changed.
- **Weapon types** (ascending cost): orbital_defense, interceptor, reporter, escort, battleship, comm_laser. Only mobile types (reporter, escort, battleship) can form fleets.
- **Economy**: each system accumulates wealth per tick based on its econ level (1-5). Sol is always level 5. Levels grow over time without combat.
- **Bot**: alien opponent implemented in `srv/internal/game/bot.go`.
- **Time scale**: ~0.056 game years per real second; 100ms tick interval.

## Testing

```bash
go test ./srv/...
```

## Directories

- `proto/` — early Three.js prototype (historical)
- `tools/gendata/` — CSV data generation tool
- `OLD_SPECS/` — original vision/requirements docs (not authoritative)
