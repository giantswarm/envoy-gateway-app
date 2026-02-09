#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

repo_dir=$(git rev-parse --show-toplevel) ; readonly repo_dir

cd "${repo_dir}"

set -x
APP_VERSION=$(yq e '.directories[] | select(.path == "vendor").contents[] | select(.path == "gateway-helm").helmChart.version' vendir.yml)

yq -i e ".appVersion |= \"${APP_VERSION#v}\"" ./helm/envoy-gateway/Chart.yaml

{ set +x; } 2>/dev/null
