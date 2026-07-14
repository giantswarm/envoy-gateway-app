# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.8.0] - 2026-07-14

### Added

- Add `basicauth` performance test suite.
- Add `keyauth` performance test suite.
- Add `mobilelatency` performance test suite.
- Add `perf-report` repo-specific claude skill.

### Changed

- Set `giantswarm-critical` priorityClass on the Envoy Gateway control plane pods.
- Add `circuitBreaker` config field in bundle values for envoy performance test suites to better sustain high request load.
- Configure Envoy Gateway to be HA by default.
- Move apps' versions in dependencies_test files from each performance test suite into a single file used in each of those.
- Update Envoy Gateway to [v1.8.2](https://gateway.envoyproxy.io/news/releases/notes/v1.8.2) (data plane Envoy bumped to v1.38.3).

### Fixed

- Fix kong performance tests in `basic` and `basicauth` suites.

## [1.7.3] - 2026-06-09

### Added

- Import load testing framework from microservices-demo app.

### Fixed

- Correct the ports path and format in the Envoy Gateway ingress `CiliumNetworkPolicy`. The previous empty `toPorts` entry put the endpoint into default-deny mode, silently dropping all xDS connections from new proxy pods.

## [1.7.2] - 2026-06-08

### Changed

- Update Envoy Gateway to [v1.8.1](https://gateway.envoyproxy.io/news/releases/notes/v1.8.1) (data plane Envoy bumped to v1.38.1).

## [1.7.1] - 2026-05-28

### Changed

- Envoy Gateway CRDs are no longer installed as Helm-managed resources. Instead, a dedicated Docker image (`gsoci.azurecr.io/giantswarm/envoy-gateway-crds`) is built and a pre-install/pre-upgrade hook Job applies the CRDs via `kubectl apply --server-side`. This avoids CRD ownership conflicts and allows safe upgrades.

## [1.7.0] - 2026-05-22

### Changed

- Update Envoy Gateway to [v1.8.0](https://gateway.envoyproxy.io/news/releases/notes/v1.8.0).

## [1.6.2] - 2026-05-07

### Changed

- Update Envoy Gateway to [v1.7.2](https://gateway.envoyproxy.io/news/releases/notes/v1.7.2).
- Grant infra-manager `get/list/watch` on Secrets when `GatewayNamespace` deploy mode is used with explicit watch namespaces.

## [1.6.1] - 2026-04-01

### Changed

- Update Envoy Gateway to [v1.7.1](https://gateway.envoyproxy.io/news/releases/notes/v1.7.1).

## [1.6.0] - 2026-02-23

### Changed

- Update Envoy Gateway to [v1.7.0](https://gateway.envoyproxy.io/news/releases/notes/v1.7.0).

## [1.5.0] - 2026-02-10

### Added

- Add convenience values switch for enabling [Envoy Gateway Backend extension](https://gateway.envoyproxy.io/docs/tasks/traffic/backend/). Disabled by default.
- Add optional Kyverno policies for restricting usage of Backend resources with problematic targets like localhost, cloud metadata endpoints, envoy admin ports or dynamic resolver usage.
  See `values.yaml` file for more information.

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

[Unreleased]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.8.0...HEAD
[1.8.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.7.3...v1.8.0
[1.7.3]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.7.2...v1.7.3
[1.7.2]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.7.1...v1.7.2
[1.7.1]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.7.0...v1.7.1
[1.7.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.6.2...v1.7.0
[1.6.2]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.6.1...v1.6.2
[1.6.1]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.6.0...v1.6.1
[1.6.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.4.1...v1.5.0
[1.4.1]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.3.0...v1.0.0
[0.3.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/giantswarm/envoy-gateway-app/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/giantswarm/envoy-gateway-app/releases/tag/v0.1.0
