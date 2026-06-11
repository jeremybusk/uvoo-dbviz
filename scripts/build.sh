#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoo-sqvizerver}"
OUT="${OUT:-$ROOT/bin/$APP_NAME}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need go
need npm

cd "$ROOT/web"
if [ -f package-lock.json ]; then
  npm ci
else
  npm install
fi
npm run build

cd "$ROOT"
mkdir -p "$(dirname "$OUT")"
BUILD_VERSION="${BUILD_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
GOCACHE="${GOCACHE:-/tmp/go-build-cache}" CGO_ENABLED="${CGO_ENABLED:-0}" go build -trimpath -ldflags="-s -w -X uvoo-sqviz/internal/build.Version=$BUILD_VERSION -X uvoo-sqviz/internal/build.Commit=$BUILD_COMMIT -X uvoo-sqviz/internal/build.Date=$BUILD_DATE" -o "$OUT" ./cmd/uvoo-sqvizerver
echo "built $OUT"
