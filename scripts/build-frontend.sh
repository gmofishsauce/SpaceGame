#!/usr/bin/env bash
# build-frontend.sh — rebuild the Vite/JS frontend and place the output in
# web/dist/ so that `go build` embeds it via //go:embed dist.
#
# Run this whenever you change files under web/src (or web/*.{html,ts,js}).
# After running, commit the updated web/dist/ so others can build the Go
# server without needing npm.
#
# Usage: scripts/build-frontend.sh [--no-install]
#   --no-install  skip `npm install` (faster if node_modules is up to date)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB_DIR="$REPO_ROOT/web"

cd "$WEB_DIR"

if [[ "${1:-}" != "--no-install" ]]; then
    echo "==> npm install"
    npm install
fi

echo "==> npm run build"
npm run build

echo "==> Done. web/dist/ is up to date."
echo "    Commit web/dist/ to keep the embedded assets in sync:"
echo "    git add web/dist && git commit -m 'chore: rebuild frontend assets'"
