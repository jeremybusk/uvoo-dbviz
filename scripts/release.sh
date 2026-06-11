#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoo-sqviz}"
REMOTE="${REMOTE:-origin}"

if [ -z "${VERSION:-}" ]; then
  echo "usage: VERSION=v0.1.0 make release" >&2
  exit 2
fi

TAG="$VERSION"
case "$TAG" in
  v*) ;;
  *) TAG="v$TAG" ;;
esac

case "$TAG" in
  *[!A-Za-z0-9._-]*)
    echo "invalid release tag: $TAG" >&2
    exit 2
    ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need git
need gh
need helm

cd "$ROOT"

if [ -n "$(git status --porcelain)" ]; then
  echo "working tree is dirty; commit or stash changes before releasing" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  echo "tag already exists locally: $TAG" >&2
  exit 1
fi

set +e
git ls-remote --exit-code --tags "$REMOTE" "refs/tags/$TAG" >/dev/null
remote_tag_status=$?
set -e
if [ "$remote_tag_status" -eq 0 ]; then
  echo "tag already exists on $REMOTE: $TAG" >&2
  exit 1
elif [ "$remote_tag_status" -ne 2 ]; then
  echo "could not check tags on remote $REMOTE" >&2
  exit 1
fi

gh auth status >/dev/null

if [ "${SKIP_VERIFY:-false}" != "true" ]; then
  make test
  make helm-lint
fi

rm -rf "$ROOT/dist"
VERSION="$TAG" make package
VERSION="$TAG" make helm-package

mapfile -t assets < <(find "$ROOT/dist" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.tgz' -o -name 'checksums.txt' \) | sort)
if [ "${#assets[@]}" -eq 0 ]; then
  echo "no release assets were created" >&2
  exit 1
fi

echo "creating and pushing tag $TAG"
git tag -a "$TAG" -m "$APP_NAME $TAG"
git push "$REMOTE" "$TAG"

echo "creating GitHub release $TAG"
gh release create "$TAG" "${assets[@]}" \
  --title "$APP_NAME $TAG" \
  --notes "Release $TAG"

echo "created release $TAG"
