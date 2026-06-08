#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOWED_GO="${ALLOWED_GO:-Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC,BlueOak-1.0.0}"
ALLOWED_NPM="${ALLOWED_NPM:-Apache-2.0;MIT;BSD-2-Clause;BSD-3-Clause;ISC;BlueOak-1.0.0;0BSD;(MIT OR CC0-1.0)}"
ALLOWED_LOCK_PATTERN="${ALLOWED_LOCK_PATTERN:-^(Apache-2.0|MIT|BSD-2-Clause|BSD-3-Clause|ISC|BlueOak-1.0.0|0BSD|\\(MIT OR CC0-1.0\\))$}"
status=0

echo "checking Go module licenses"
if command -v go-licenses >/dev/null 2>&1; then
  go-licenses check "$ROOT/..." --allowed_licenses="$ALLOWED_GO" || status=1
else
  echo "skipping Go license scan: install with 'go install github.com/google/go-licenses@latest'" >&2
fi

echo "checking npm package licenses"
if command -v jq >/dev/null 2>&1 && [ -f "$ROOT/web/package-lock.json" ]; then
  lock_issues="$(
    jq -r --arg pattern "$ALLOWED_LOCK_PATTERN" '
      .packages
      | to_entries[]
      | select(.value.dev != true)
      | select(.value.license and (.value.license | test($pattern) | not))
      | "\(.key)\t\(.value.version // "")\t\(.value.license)"
    ' "$ROOT/web/package-lock.json"
  )"
  if [ -n "$lock_issues" ]; then
    echo "package-lock.json contains licenses outside the allow-list:" >&2
    printf '%s\n' "$lock_issues" >&2
    status=1
  fi
else
  echo "skipping package-lock license check: install jq and run web-install" >&2
fi

if [ -x "$ROOT/web/node_modules/.bin/license-checker-rseidelsohn" ]; then
  (cd "$ROOT/web" && ./node_modules/.bin/license-checker-rseidelsohn --production --excludePrivatePackages --summary --onlyAllow "$ALLOWED_NPM") || status=1
else
  echo "skipping npm license scanner: run 'make web-install'" >&2
fi

exit "$status"

