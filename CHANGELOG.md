# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Update Envoy Gateway to [v1.6.3](https://gateway.envoyproxy.io/news/releases/notes/v1.6.3).

## [1.4.1] - 2026-01-19

### Changed

- Add values switch to allow egress traffic to outside world of envoy-gateway control plane pods. This is required in certain cases where SecurityPolicies need to obtain additional configuration from OIDC or JWT providers.

## [1.4.0] - 2026-01-12

### Changed

- Update Envoy Gateway to [v1.6.1](https://gateway.envoyproxy.io/news/releases/notes/v1.6.1).
- Update Chart.yaml to use updated `io.giantswarm.application.team` annotation

## [1.3.0] - 2026-01-09

### Added

- Add PodLogs resource for log collection

### Changed

- Update Envoy Gateway to [v1.5.6](https://gateway.envoyproxy.io/news/releases/notes/v1.5.6).

## [1.2.0] - 2025-11-11

### Changed

- Apply CRDs as templates with the keep annotation.

## [1.1.0] - 2025-11-05

### Changed

- Update Envoy Gateway to [v1.5.4](https://gateway.envoyproxy.io/news/releases/notes/v1.5.4).

## [1.0.0] - 2025-10-29

### Changed

- Update Envoy Gateway to [v1.4.4](https://gateway.envoyproxy.io/news/releases/notes/v1.4.4).
- Sync with upstream Helm chart.
- Refactor Image patch, now using the `globa.image.registry` value.

### Removed

- Drop PSS patch as now the chart is compliant.

## [0.3.0] - 2025-03-05

### Added

- Add PodMonitor scraping envoy-gateway controller.

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

[Unreleased]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.4.1...HEAD
[1.4.1]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.3.0...v1.0.0
[0.3.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/giantswarm/envoy-gateway-app/releases/tag/v0.1.0
