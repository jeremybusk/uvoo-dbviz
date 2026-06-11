#!/bin/bash
set -eu

docker build -t ghcr.io/jeremybusk/uvoo-sqviz:latest .
docker push ghcr.io/jeremybusk/uvoo-sqviz:latest
kubectl -n sqviz rollout restart deployment/sqviz-uvoo-sqviz
kubectl -n sqviz rollout status deployment/sqviz-uvoo-sqviz

# If you need to log in first:
# docker login ghcr.io
