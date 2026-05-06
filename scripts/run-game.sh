#!/usr/bin/env bash
# run-game.sh — build both binaries and launch server then bot. (FR-69)
# Run from the repo root. Requires nearest.csv, planets.csv,
# alien-nearest.csv, alien-planets.csv in cwd (or set env vars).
# Either process can be killed independently for debugging.
set -euo pipefail

# Build (idempotent).
echo "==> Building spacegame..."
go build -o ./spacegame ./srv/cmd/spacegame

echo "==> Building spacebot..."
go build -o ./spacebot  ./srv/cmd/spacebot

# Start server in background; kill it when this script exits.
echo "==> Starting spacegame server..."
./spacegame &
SERVER_PID=$!
trap 'echo "==> Stopping server (pid $SERVER_PID)..."; kill "$SERVER_PID" 2>/dev/null || true' EXIT

# Wait until the server is reachable (up to 5 seconds).
echo "==> Waiting for server..."
for i in {1..50}; do
    if curl -fsS http://127.0.0.1:8080/api/stars >/dev/null 2>&1; then
        echo "==> Server is up."
        break
    fi
    sleep 0.1
done

if ! curl -fsS http://127.0.0.1:8080/api/stars >/dev/null 2>&1; then
    echo "ERROR: server did not become reachable after 5 seconds." >&2
    exit 1
fi

# Start bot (foreground; script exits when bot exits, trap kills server).
echo "==> Starting spacebot (alien player)..."
./spacebot --server http://127.0.0.1:8080
