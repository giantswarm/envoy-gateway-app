# envoy-gateway

The Helm chart for Envoy Gateway

**Homepage:** <https://github.com/giantswarm/envoy-gateway-app>

## Source Code

* <https://github.com/envoyproxy/gateway>

## Usage

[Helm](https://helm.sh) must be installed to use the charts.
Please refer to Helm's [documentation](https://helm.sh/docs) to get started.

### Install from DockerHub

Once Helm has been set up correctly, install the chart from dockerhub:

``` shell
    helm install eg oci://docker.io/envoyproxy/gateway-helm --version v0.0.0-latest -n envoy-gateway-system --create-namespace
```
You can find all helm chart release in [Dockerhub](https://hub.docker.com/r/envoyproxy/gateway-helm/tags)

### Install from Source Code

You can also install the helm chart from the source code:

To install the eg chart along with Gateway API CRDs and Envoy Gateway CRDs:

``` shell
    make kube-deploy TAG=latest
```

### Skip install CRDs

You can install the eg chart along without Gateway API CRDs and Envoy Gateway CRDs, make sure CRDs exist in Cluster first if you want to skip to install them, otherwise EG may fail to start:

``` shell
    helm install eg --create-namespace oci://docker.io/envoyproxy/gateway-helm --version v0.0.0-latest -n envoy-gateway-system --skip-crds
```

To uninstall the chart:

``` shell
    helm uninstall eg -n envoy-gateway-system
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| certgen.job.affinity | object | `{}` |  |
| certgen.job.annotations | object | `{}` |  |
| certgen.job.args | list | `[]` |  |
| certgen.job.nodeSelector | object | `{}` |  |
| certgen.job.pod.annotations | object | `{}` |  |
| certgen.job.pod.labels | object | `{}` |  |
| certgen.job.resources.limits.memory | string | `"500Mi"` |  |
| certgen.job.resources.requests.cpu | string | `"50m"` |  |
| certgen.job.resources.requests.memory | string | `"100Mi"` |  |
| certgen.job.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| certgen.job.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| certgen.job.securityContext.privileged | bool | `false` |  |
| certgen.job.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| certgen.job.securityContext.runAsGroup | int | `65532` |  |
| certgen.job.securityContext.runAsNonRoot | bool | `true` |  |
| certgen.job.securityContext.runAsUser | int | `65532` |  |
| certgen.job.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| certgen.job.tolerations | list | `[]` |  |
| certgen.job.ttlSecondsAfterFinished | int | `30` |  |
| certgen.rbac.annotations | object | `{}` |  |
| certgen.rbac.labels | object | `{}` |  |
| config.envoyGateway | object | `{"extensionApis":{},"gateway":{"controllerName":"gateway.envoyproxy.io/gatewayclass-controller"},"logging":{"level":{"default":"info"}},"provider":{"type":"Kubernetes"}}` | EnvoyGateway configuration. Visit https://gateway.envoyproxy.io/docs/api/extension_types/#envoygateway to view all options. |
| createNamespace | bool | `false` |  |
| deployment.envoyGateway.image.repository | string | `""` |  |
| deployment.envoyGateway.image.tag | string | `""` |  |
| deployment.envoyGateway.imagePullPolicy | string | `""` |  |
| deployment.envoyGateway.imagePullSecrets | list | `[]` |  |
| deployment.envoyGateway.resources.limits.memory | string | `"1024Mi"` |  |
| deployment.envoyGateway.resources.requests.cpu | string | `"100m"` |  |
| deployment.envoyGateway.resources.requests.memory | string | `"256Mi"` |  |
| deployment.envoyGateway.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| deployment.envoyGateway.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| deployment.envoyGateway.securityContext.privileged | bool | `false` |  |
| deployment.envoyGateway.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| deployment.envoyGateway.securityContext.runAsGroup | int | `65532` |  |
| deployment.envoyGateway.securityContext.runAsNonRoot | bool | `true` |  |
| deployment.envoyGateway.securityContext.runAsUser | int | `65532` |  |
| deployment.envoyGateway.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| deployment.pod.affinity | object | `{}` |  |
| deployment.pod.annotations."prometheus.io/port" | string | `"19001"` |  |
| deployment.pod.annotations."prometheus.io/scrape" | string | `"true"` |  |
| deployment.pod.labels | object | `{}` |  |
| deployment.pod.nodeSelector | object | `{}` |  |
| deployment.pod.tolerations | list | `[]` |  |
| deployment.pod.topologySpreadConstraints | list | `[]` |  |
| deployment.ports[0].name | string | `"grpc"` |  |
| deployment.ports[0].port | int | `18000` |  |
| deployment.ports[0].targetPort | int | `18000` |  |
| deployment.ports[1].name | string | `"ratelimit"` |  |
| deployment.ports[1].port | int | `18001` |  |
| deployment.ports[1].targetPort | int | `18001` |  |
| deployment.ports[2].name | string | `"wasm"` |  |
| deployment.ports[2].port | int | `18002` |  |
| deployment.ports[2].targetPort | int | `18002` |  |
| deployment.ports[3].name | string | `"metrics"` |  |
| deployment.ports[3].port | int | `19001` |  |
| deployment.ports[3].targetPort | int | `19001` |  |
| deployment.priorityClassName | string | `nil` |  |
| deployment.replicas | int | `1` |  |
| global.image | object | `{"registry":"gsoci.azurecr.io"}` | Global override for image registry |
| global.imagePullSecrets | list | `[]` | Global override for image pull secrets |
| global.images.envoyGateway.image | string | `"gsoci.azurecr.io/giantswarm/envoyproxy-gateway:v1.5.6"` |  |
| global.images.envoyGateway.pullPolicy | string | `"IfNotPresent"` |  |
| global.images.envoyGateway.pullSecrets | list | `[]` |  |
| global.images.ratelimit.image | string | `"gsoci.azurecr.io/giantswarm/envoyproxy-ratelimit:e74a664a"` |  |
| global.images.ratelimit.pullPolicy | string | `"IfNotPresent"` |  |
| global.images.ratelimit.pullSecrets | list | `[]` |  |
| hpa.behavior | object | `{}` |  |
| hpa.enabled | bool | `false` |  |
| hpa.maxReplicas | int | `1` |  |
| hpa.metrics | list | `[]` |  |
| hpa.minReplicas | int | `1` |  |
| kubernetesClusterDomain | string | `"cluster.local"` |  |
| podDisruptionBudget.minAvailable | int | `0` |  |
| service.annotations | object | `{}` |  |
| service.trafficDistribution | string | `""` |  |
| serviceType | string | `"managed"` |  |
| topologyInjector.annotations | object | `{}` |  |
| topologyInjector.enabled | bool | `true` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)

