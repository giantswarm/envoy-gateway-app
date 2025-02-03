# Sync chart from upstream

We keep this repository aligned with the upstream Helm Chart, plus a few modifications that are either Giant Swarm specific or to be contributed.

For this, [vendir](https://github.com/carvel-dev/vendir) and a series of [patches](https://github.com/giantswarm/envoy-gateway-app/tree/main/sync/patches) are being used.

**To sync the chart from upstream install `vendir` and execute `./sync/sync.sh` in the root of the repository.**

## Sync from upstream

Scenario: Upstream released a new chart version and you want to update our chart.

- In `vendir.yml`: Change the `directories[0].contents[0].helmChart.ref` field to the new Helm Chart version.
- Run `./sync/sync.sh`.
- Fix syncing errors by changing the patches in `sync/patches`.
- In case of container image version changes:
  - Update the `tag` fields in `sync/patches/values/values.yaml`.
  - Update the `appVersion` field in `helm/kong-app/Chart.yaml`.
- Commit your changes in directories `sync`, `diffs` and `helm`.

## Changes to default `values.yaml`

- Update any value you want as default in the `sync/patches/values/values.yaml` file.
- Update the schema in the same directory.
- Run `sync/sync.sh`.

## General chart changes

Changes to chart templates or helpers follow the approach of maintaining patches or scripts that can be applied to the upstream chart.

Each change should have its own patch directory in `sync/patches` and have an executable `patch.sh` script.

The `patch.sh` script is being called by `sync/sync.sh` and contains code to transform the upstream chart in `vendor/gateway-helm` into the desired version in `helm/envoy-gateway`.

# Patches

### image-registry

- Adapt image templating to use the `image.registry` value.
- Set `image.registry` as the `gsoci.azurecr.io`.
- Use `name` instead of `repository` as image name.

### team-label

- Include team label in `eg.labels` template function.

### values

- Add GS values
- Add values.schema.json.
- Set resources requests and limits for certgen Job.

TODO:
- Generate values.schema.json in sync.sh
- Discuss with upstream to include as default values.

### pss-comply

- Add `readOnlyRootFilesystem=true` to container SecurityContext.
- Add `seccompProfile.type=RuntimeDefault` to SecurityContext.
- Drop ALL capabilities.

TODO: Push to upstream as default or make it configurable through values.

### disable-gateway-api-crds

Remove the bundled Gateway API CRD file. We're installing these separately as part of the [gateway-api-bundle](https://github.com/giantswarm/gateway-api-bundle)
