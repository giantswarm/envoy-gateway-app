#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

repo_dir=$(git rev-parse --show-toplevel) ; readonly repo_dir
script_dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd ) ; readonly script_dir

cd "${repo_dir}"

readonly script_dir_rel=".${script_dir#"${repo_dir}"}"

set -x
rm -f ./helm/envoy-gateway/charts/crds/crds/gatewayapi-crds.yaml
mkdir -p ./crds
cp ./helm/envoy-gateway/charts/crds/crds/generated/*.yaml ./crds/
rm -rf ./helm/envoy-gateway/charts/crds

mkdir -p ./helm/envoy-gateway/templates/crds
cp "${script_dir_rel}/crds-serviceaccount.yaml" ./helm/envoy-gateway/templates/crds/serviceaccount.yaml
cp "${script_dir_rel}/crds-rbac.yaml" ./helm/envoy-gateway/templates/crds/rbac.yaml
cp "${script_dir_rel}/crds-job.yaml" ./helm/envoy-gateway/templates/crds/job.yaml

{ set +x; } 2>/dev/null
