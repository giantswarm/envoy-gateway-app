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

