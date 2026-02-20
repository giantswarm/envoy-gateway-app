# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Giant Swarm Helm chart wrapper for [Envoy Gateway](https://gateway.envoyproxy.io/), maintained by the `cabbage` team. It syncs from upstream using **vendir** and applies custom patches for Giant Swarm's infrastructure requirements.

## Common Commands

### Chart Development

```bash
# Sync from upstream and apply all patches
./sync/sync.sh
```

### Updating from Upstream

1. Check the latest upstream release: `gh release list --repo envoyproxy/gateway --limit 10`
2. Create a branch: `git checkout -b upgrade-vVERSION`
3. Edit `vendir.yml` to update the chart version
4. Run `./sync/sync.sh` — this fetches upstream and applies all patches in `sync/patches/`
5. Fix any patch conflicts
6. Compare `sync/patches/values/values.yaml` against `vendor/gateway-helm/values.yaml` to identify new or removed upstream fields that should be reflected in our patch. Also update the `global.images.envoyGateway.image` tag to the new version.
7. Re-run `./sync/sync.sh` if the values patch was updated
8. Update `CHANGELOG.md`: add a new version entry under `[Unreleased]` and update the comparison links at the bottom. The app version follows the upstream minor version (e.g. upstream v1.7.0 → app v1.6.0).
9. Commit changes in `vendir.yml`, `vendir.lock.yml`, `sync/patches/`, `helm/`, `diffs/`, and `CHANGELOG.md`

## Architecture

### Upstream Sync Pattern

The core pattern is: **vendir + ordered patches**.

- `vendir.yml` — pins the upstream Envoy Gateway Helm chart version (fetched as OCI artifact from `docker.io/envoyproxy/gateway`)
- `vendor/` — contains the raw upstream chart after `vendir sync`
- `sync/sync.sh` — orchestrates the full sync: runs vendir, applies patches, generates diffs
- `sync/patches/` — Git patch files applied in this order:
  1. `image-registry` — switches image registry to `gsoci.azurecr.io`
  2. `team-label` — adds `app.giantswarm.io/team: cabbage` labels
  3. `backend` — safe enable/disable of the Backend extension API
  4. `kyverno-policies` — restricts Backend API (blocks localhost, metadata service, admin port)
  5. `values` — Giant Swarm defaults and `values.schema.json`
  6. `network-policies` — Cilium/Calico network policies
  7. `monitoring` — PodMonitor for metrics
  8. `crds` — removes CRDs (installed separately via `gateway-api-bundle`)
  9. `chart_yaml` — updates `appVersion` from vendir lock

The actual chart lives in `helm/envoy-gateway/`. Never edit files there directly that are managed by patches — edit the patch source in `sync/patches/` and re-run `sync.sh`.

### Key Design Decisions

- **Gateway API CRDs are excluded** from this chart; they are installed via the `gateway-api-bundle` dependency app
- **Envoy Gateway CRDs are included** in this chart, installed as templates in /helm/envoy-gateway/templates/crds
- **Image registry**: all images use `gsoci.azurecr.io` instead of upstream registries
- **Kyverno policies** (enabled by default) restrict the `Backend` resource to prevent access to cluster-internal services
- **Network policies** support both Cilium (`CiliumNetworkPolicy`) and Kubernetes (`NetworkPolicy`) via values toggle

### E2E Tests

Tests live in `tests/e2e/` and use the Giant Swarm `apptest-framework` with Ginkgo v2.

```bash
cd tests/e2e
# Tests require a running CAPA Kubernetes cluster
# See config.yaml for cluster provider settings
```

The basic test suite (`tests/e2e/suites/basic/`) verifies:
- The `gateway-api-bundle` dependency app is deployed
- The `envoy-gateway` app deploys to the `envoy-gateway-system` namespace
- The correct app version is running (15-minute timeout, 5s polling)

### Release Process

- Releases are triggered by pushing a git tag matching `/^v.*/`
- CircleCI (`architect/push-to-app-catalog`) packages and pushes to `giantswarm-catalog` and `giantswarm-test-catalog`
- Update `CHANGELOG.md` (Keep a Changelog format) before tagging

## Important Files

| File | Purpose |
|------|---------|
| `vendir.yml` | Pins upstream chart version |
| `sync/sync.sh` | Sync + patch entrypoint |
| `sync/patches/` | All Giant Swarm customizations as git patches |
| `helm/envoy-gateway/values.yaml` | Chart configuration (generated via patches) |
| `helm/envoy-gateway/values.schema.json` | JSON schema for values validation |
| `sync/readme.gotmpl` | Template for auto-generated `helm/envoy-gateway/README.md` |
| `.circleci/config.yml` | Release pipeline |
| `.kube-linter.yaml` | Kube-linter config (excludes cpu-requirements, liveness/readiness port checks) |
