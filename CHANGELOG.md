# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2025-02-05

### Changed

- Upgrade envoy-gateway to [v1.3.0](https://github.com/envoyproxy/gateway/releases/tag/v1.3.0).

### Removed

- Remove namespace from values
- Remove the bundled Gateway API CRD file. We're installing these separately as part of the [gateway-api-bundle](https://github.com/giantswarm/gateway-api-bundle)

## [0.1.0] - 2024-12-19

- changed: `app.giantswarm.io` label group was changed to `application.giantswarm.io`
- Sync with upstream 1.2.1 helm chart
- Add team cabbage annotation and label
- Add values.schema.json
- Adapt the namespace to our taste
- Set requests and limits for certgen Job
- Improve security for PSS compliance

[Unreleased]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/giantswarm/envoy-gateway-app/releases/tag/v0.1.0
