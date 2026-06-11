#!/bin/bash
set -eu

BUILD_VERSION="${BUILD_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

docker build \
  --build-arg SQVIZ_BUILD_VERSION="$BUILD_VERSION" \
  --build-arg SQVIZ_BUILD_COMMIT="$BUILD_COMMIT" \
  --build-arg SQVIZ_BUILD_DATE="$BUILD_DATE" \
  -t ghcr.io/jeremybusk/uvoo-sqviz:latest .
docker push ghcr.io/jeremybusk/uvoo-sqviz:latest
kubectl -n sqviz rollout restart deployment/sqviz-uvoo-sqviz
kubectl -n sqviz rollout status deployment/sqviz-uvoo-sqviz

# If you need to log in first:
# docker login ghcr.io
