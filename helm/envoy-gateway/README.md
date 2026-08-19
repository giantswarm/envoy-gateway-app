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
| backend.enabled | bool | `false` | Enable Backend extension API (disabled by default for security) |
| certgen | object | `{"job":{"affinity":{},"annotations":{},"args":[],"nodeSelector":{},"pod":{"annotations":{},"labels":{},"securityContext":{"fsGroup":65532,"runAsGroup":65532,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}},"resources":{"limits":{"memory":"500Mi"},"requests":{"cpu":"50m","memory":"100Mi"}},"securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"privileged":false,"readOnlyRootFilesystem":true,"runAsGroup":65532,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}},"tolerations":[],"ttlSecondsAfterFinished":30},"rbac":{"annotations":{},"labels":{}}}` | Certgen is used to generate the certificates required by EnvoyGateway. If you want to construct a custom certificate, you can generate a custom certificate through Cert-Manager before installing EnvoyGateway. Certgen will not overwrite the custom certificate. Please do not manually modify `values.yaml` to disable certgen, it may cause EnvoyGateway OIDC,OAuth2,etc. to not work as expected. |
| ciliumNetworkPolicy.controlPlaneAllowWorld | bool | `false` | Allow envoy-gateway control plane pods to communicate with the outside world. This can be required in certain cases with SecurityPolicies trying to contact external providers for additional OIDC or JWT configuration. |
| commonLabels | object | `{}` | Labels to apply to all resources |
| config.envoyGateway | object | `{"extensionApis":{},"gateway":{"controllerName":"gateway.envoyproxy.io/gatewayclass-controller"},"logging":{"level":{"default":"info"}},"provider":{"type":"Kubernetes"}}` | EnvoyGateway configuration. Visit https://gateway.envoyproxy.io/docs/api/extension_types/#envoygateway to view all options. |
| crds.image.registry | string | `"gsoci.azurecr.io"` |  |
| crds.image.repository | string | `"giantswarm/envoy-gateway-crds"` |  |
| createNamespace | bool | `false` |  |
| deployment.annotations | object | `{}` |  |
| deployment.envoyGateway.extraEnv | list | `[]` | Additional environment variables for the envoy-gateway container. |
| deployment.envoyGateway.image.repository | string | `""` |  |
| deployment.envoyGateway.image.tag | string | `""` |  |
| deployment.envoyGateway.imagePullPolicy | string | `""` |  |
| deployment.envoyGateway.imagePullSecrets | list | `[]` |  |
| deployment.envoyGateway.livenessProbe.httpGet.path | string | `"/healthz"` |  |
| deployment.envoyGateway.livenessProbe.httpGet.port | int | `8081` |  |
| deployment.envoyGateway.livenessProbe.periodSeconds | int | `20` |  |
| deployment.envoyGateway.livenessProbe.successThreshold | int | `1` |  |
| deployment.envoyGateway.livenessProbe.timeoutSeconds | int | `1` |  |
| deployment.envoyGateway.readinessProbe.httpGet.path | string | `"/readyz"` |  |
| deployment.envoyGateway.readinessProbe.httpGet.port | int | `8081` |  |
| deployment.envoyGateway.readinessProbe.periodSeconds | int | `10` |  |
| deployment.envoyGateway.readinessProbe.successThreshold | int | `1` |  |
| deployment.envoyGateway.readinessProbe.timeoutSeconds | int | `1` |  |
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
| deployment.envoyGateway.startupProbe.failureThreshold | int | `30` |  |
| deployment.envoyGateway.startupProbe.httpGet.path | string | `"/healthz"` |  |
| deployment.envoyGateway.startupProbe.httpGet.port | int | `8081` |  |
| deployment.envoyGateway.startupProbe.periodSeconds | int | `1` |  |
| deployment.envoyGateway.startupProbe.successThreshold | int | `1` |  |
| deployment.envoyGateway.startupProbe.timeoutSeconds | int | `1` |  |
| deployment.envoyGateway.strategy | object | `{}` | Volume source for the Wasm module cache mounted at /var/lib/eg/wasm. Defaults to an emptyDir when left empty. Example: persist the Wasm module cache across controller restarts by backing it with a PersistentVolumeClaim:   wasmCacheVolume:     persistentVolumeClaim:       claimName: envoy-gateway-wasm-cache |
| deployment.envoyGateway.wasmCacheVolume | object | `{}` |  |
| deployment.pod.affinity | object | `{}` |  |
| deployment.pod.annotations."karpenter.sh/do-not-disrupt" | string | `"true"` |  |
| deployment.pod.annotations."prometheus.io/port" | string | `"19001"` |  |
| deployment.pod.annotations."prometheus.io/scrape" | string | `"true"` |  |
| deployment.pod.extraVolumeMounts | list | `[]` |  |
| deployment.pod.extraVolumes | list | `[]` |  |
| deployment.pod.labels | object | `{}` |  |
| deployment.pod.nodeSelector | object | `{}` |  |
| deployment.pod.securityContext.fsGroup | int | `65532` |  |
| deployment.pod.securityContext.runAsGroup | int | `65532` |  |
| deployment.pod.securityContext.runAsNonRoot | bool | `true` |  |
| deployment.pod.securityContext.runAsUser | int | `65532` |  |
| deployment.pod.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| deployment.pod.tolerations | list | `[]` |  |
| deployment.pod.topologySpreadConstraints[0].labelSelector.matchLabels."app.kubernetes.io/name" | string | `"envoy-gateway"` |  |
| deployment.pod.topologySpreadConstraints[0].maxSkew | int | `1` |  |
| deployment.pod.topologySpreadConstraints[0].topologyKey | string | `"kubernetes.io/hostname"` |  |
| deployment.pod.topologySpreadConstraints[0].whenUnsatisfiable | string | `"ScheduleAnyway"` |  |
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
| deployment.priorityClassName | string | `"giantswarm-critical"` |  |
| deployment.replicas | int | `2` |  |
| global.image | object | `{"registry":"gsoci.azurecr.io"}` | Global override for image registry |
| global.imagePullSecrets | list | `[]` | Global override for image pull secrets |
| global.images.envoyGateway.image | string | `"gsoci.azurecr.io/giantswarm/envoyproxy-gateway:v1.9.0"` | Full image for the Envoy Gateway control plane Deployment installed by this chart. |
| global.images.envoyGateway.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the Envoy Gateway control plane Deployment. Default behavior: latest images will be Always else IfNotPresent. |
| global.images.envoyGateway.pullSecrets | list | `[]` | Pull secrets for the Envoy Gateway control plane Deployment. |
| global.images.envoyProxy.image | string | `"gsoci.azurecr.io/giantswarm/envoy:distroless-v1.39.0"` | Full image for the managed Envoy Proxy data plane. This updates the generated `envoyProxy` config and does not change the `envoy-gateway` control plane Deployment image. If not specified, the default image built into `envoy-gateway` is used. |
| global.images.envoyProxy.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the managed Envoy Proxy data plane. Default behavior: IfNotPresent. |
| global.images.envoyProxy.pullSecrets | list | `[]` | Pull secrets for the managed Envoy Proxy data plane. |
| global.images.ratelimit.image | string | `"gsoci.azurecr.io/giantswarm/envoyproxy-ratelimit:17b1956c"` |  |
| global.images.ratelimit.pullPolicy | string | `"IfNotPresent"` |  |
| global.images.ratelimit.pullSecrets | list | `[]` |  |
| hpa.behavior | object | `{}` |  |
| hpa.enabled | bool | `true` |  |
| hpa.maxReplicas | int | `5` |  |
| hpa.metrics[0].resource.name | string | `"cpu"` |  |
| hpa.metrics[0].resource.target.averageUtilization | int | `80` |  |
| hpa.metrics[0].resource.target.type | string | `"Utilization"` |  |
| hpa.metrics[0].type | string | `"Resource"` |  |
| hpa.minReplicas | int | `2` |  |
| kubernetesClusterDomain | string | `"cluster.local"` |  |
| kyvernoPolicies.backend.allowedDynamicResolverNamespaces | list | `[]` | Restrict DynamicResolver type to specific namespaces (empty = deny all) |
| kyvernoPolicies.backend.denyAdminPort | bool | `true` | Block access to Envoy admin port (19000) |
| kyvernoPolicies.backend.denyMetadataService | bool | `true` | Block access to cloud metadata service (169.254.169.254) |
| kyvernoPolicies.backend.enabled | bool | `true` | Enable Kyverno policies to restrict Backend resource creation |
| kyvernoPolicies.backend.validationFailureAction | string | `"Enforce"` | Validation failure action: Enforce (block) or Audit (warn only) |
| namespaceOverride | string | `""` | Override the namespace for resources deployed by the chart. Defaults to the release namespace. |
| podDisruptionBudget.minAvailable | int | `1` |  |
| podDisruptionBudget.unhealthyPodEvictionPolicy | string | `"AlwaysAllow"` |  |
| service.annotations | object | `{}` |  |
| service.trafficDistribution | string | `""` |  |
| service.type | string | `"ClusterIP"` | Service type. Can be set to LoadBalancer with specific IP, e.g.: type: LoadBalancer loadBalancerIP: 10.236.90.20 |
| serviceType | string | `"managed"` |  |
| topologyInjector.annotations | object | `{}` |  |
| topologyInjector.enabled | bool | `true` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)

