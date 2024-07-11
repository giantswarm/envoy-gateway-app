#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd ) ; readonly dir
cd "${dir}/.."

# Stage 1 sync
set -x
vendir sync
{ set +x; } 2>/dev/null

# Patches
# ./sync/patches/values/patch.sh
# ./sync/patches/image_registries/patch.sh

for patch in ./sync/patches/*.patch; do
    set +e
    set -x
    git apply $patch
    { set +x; } 2>/dev/null
    set -e
done


# Store diffs
rm -f ./diffs/*
for f in $(git --no-pager diff --no-exit-code --no-color --no-index vendor/gateway-helm helm/envoy-gateway --name-only) ; do
        set +e
        set -x
        git --no-pager diff --no-exit-code --no-color --no-index "vendor/gateway-helm/${f#"helm/envoy-gateway/"}" "${f}" \
                > "./diffs/${f//\//__}.patch" # ${f//\//__} replaces all "/" with "__"
        ret=$?
        { set +x; } 2>/dev/null
        set -e
        if [ $ret -ne 0 ] && [ $ret -ne 1 ] ; then
                exit $ret
        fi
done
