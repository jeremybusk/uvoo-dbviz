#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoo-sqvizerver}"
PROJECT_NAME="${PROJECT_NAME:-uvoo-sqviz}"
DIST="${DIST:-$ROOT/dist}"
PLATFORMS="${PLATFORMS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64}"

if [ -z "${VERSION:-}" ]; then
  VERSION="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
fi

case "$VERSION" in
  v*) PACKAGE_VERSION="${VERSION#v}" ;;
  *) PACKAGE_VERSION="$VERSION" ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}

need go
need npm
need tar

cd "$ROOT/web"
if [ -f package-lock.json ]; then
  npm ci
else
  npm install
fi
npm run build

cd "$ROOT"
rm -rf "$DIST/package"
mkdir -p "$DIST/package" "$DIST"
rm -f "$DIST/checksums.txt.tmp"

for platform in $PLATFORMS; do
  os="${platform%/*}"
  arch="${platform#*/}"
  ext=""
  if [ "$os" = "windows" ]; then
    ext=".exe"
  fi

  package_name="${PROJECT_NAME}_${PACKAGE_VERSION}_${os}_${arch}"
  stage="$DIST/package/$package_name"
  binary="$stage/bin/$APP_NAME$ext"

  rm -rf "$stage"
  mkdir -p "$stage/bin"

  echo "building $package_name"
  GOCACHE="${GOCACHE:-/tmp/go-build-cache}" CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags="-s -w" -o "$binary" ./cmd/uvoo-sqvizerver

  cp -a README.md LICENSE .env.example docker-compose.yml "$stage/"
  cp -a docs migrations deploy "$stage/"
  mkdir -p "$stage/web"
  cp -a web/dist "$stage/web/dist"

  tarball="$DIST/$package_name.tar.gz"
  tar -C "$DIST/package" -czf "$tarball" "$package_name"
  sha256_file "$tarball" >> "$DIST/checksums.txt.tmp"
done

sort "$DIST/checksums.txt.tmp" > "$DIST/checksums.txt"
rm -f "$DIST/checksums.txt.tmp"
echo "wrote release assets to $DIST"
