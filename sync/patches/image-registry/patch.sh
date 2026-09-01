#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

repo_dir=$(git rev-parse --show-toplevel) ; readonly repo_dir
script_dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd ) ; readonly script_dir

cd "${repo_dir}"

readonly script_dir_rel=".${script_dir#"${repo_dir}"}"

# Upstream's `eg.image` fallback hardcodes `docker.io/envoyproxy/gateway:{{ .Chart.Version }}`.
# Our chart version is our own (it does not track appVersion), so that tag never exists, and the
# fallback ignores `global.imageRegistry` entirely. Point it at the gsoci mirror instead.
# `git apply` fails the sync if upstream reworks the helper, so the fix cannot be lost silently.
set -x
git apply "${script_dir_rel}/000-image-fallback.patch"

{ set +x; } 2>/dev/null
