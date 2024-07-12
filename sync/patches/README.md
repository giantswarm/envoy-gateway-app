# Patches

### 000-image-registry.patch

- Adapt image templating to use the `image.registry` value.
- Set `image.registry` as the `gsoci.azurecr.io`.
- Use `name` instead of `repository` as image name.

### 001-team-label.patch

- Include team label in `eg.labels` template function.

### 002-values-schema.patch

- Add values.schema.json.

TODO: Generate that in sync.sh

### 003-certgen-resources.patch

- Set resources requests and limits for certgen Job.

TODO: Discuss with upstream to include as default values.

### 004-pss-comply.patch

- Add `readOnlyRootFilesystem=true` to container SecurityContext.
- Add `seccompProfile.type=RuntimeDefault` to SecurityContext.
- Drop ALL capabilities.

TODO: Push to upstream as default or make it configurable through values.

