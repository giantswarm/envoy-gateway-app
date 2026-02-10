#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

repo_dir=$(git rev-parse --show-toplevel) ; readonly repo_dir

cd "${repo_dir}"

target_file="./helm/envoy-gateway/templates/_helpers.tpl"

# Create a temporary file with the modified content
# We need to replace:
#   {{- with .Values.config.envoyGateway.extensionApis }}
#   extensionApis:
#     {{- toYaml . | nindent 2 }}
#   {{- end }}
# With the backend-aware version

set -x

# Use awk to perform the multi-line replacement
awk '
/^\{\{- with \.Values\.config\.envoyGateway\.extensionApis \}\}$/ {
    print "{{- $extensionApis := .Values.config.envoyGateway.extensionApis | default dict -}}"
    print "{{- if .Values.backend.enabled -}}"
    print "{{-   $extensionApis = merge (dict \"enableBackend\" true) $extensionApis -}}"
    print "{{- end -}}"
    print "{{- with $extensionApis }}"
    print "{{- if gt (len .) 0 }}"
    getline  # skip "extensionApis:"
    print "extensionApis:"
    getline  # skip "  {{- toYaml . | nindent 2 }}"
    print $0
    getline  # skip "{{- end }}"
    print "{{- end }}"
    print "{{- end }}"
    next
}
{ print }
' "$target_file" > "${target_file}.tmp" && mv "${target_file}.tmp" "$target_file"

{ set +x; } 2>/dev/null
