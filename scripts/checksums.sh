#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${DIST:-$ROOT/dist}"

cd "$DIST"

mapfile -t assets < <(find . -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.tgz' \) | sort)
if [ "${#assets[@]}" -eq 0 ]; then
  echo "no release archives found in $DIST" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${assets[@]}" | sort > checksums.txt
else
  shasum -a 256 "${assets[@]}" | sort > checksums.txt
fi
