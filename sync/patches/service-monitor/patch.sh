#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

repo_dir=$(git rev-parse --show-toplevel) ; readonly repo_dir
script_dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd ) ; readonly script_dir

cd "${repo_dir}"

readonly script_dir_rel=".${script_dir#"${repo_dir}"}"

set -x
cp "${script_dir_rel}/envoy-gateway-servicemonitor.yaml" ./helm/envoy-gateway/templates/envoy-gateway-servicemonitor.yaml

{ set +x; } 2>/dev/null
