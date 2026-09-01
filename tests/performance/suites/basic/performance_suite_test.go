package basic

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/apptest-framework/v5/pkg/suite"
	"github.com/giantswarm/clustertest/v5/pkg/application"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	"github.com/giantswarm/clustertest/v5/pkg/wait"
)

const (
	isUpgrade = false

	proxyControllerNginx = "nginx"
	proxyControllerKong  = "kong"

	// proxyControllerEnvVar selects which ingress controller is deployed
	// alongside Envoy Gateway.
	proxyControllerEnvVar = "PROXY_CONTROLLER"
)

// proxyController is the ingress controller that this suite will install,
// resolved once at package init from the PROXY_CONTROLLER env var.
// Default: nginx.
var proxyController = resolveProxyController()

func resolveProxyController() string {
	loadConfigEnv()
	v := strings.ToLower(strings.TrimSpace(os.Getenv(proxyControllerEnvVar)))
	switch v {
	case "":
		return proxyControllerNginx
	case proxyControllerNginx, proxyControllerKong:
		return v
	default:
		panic(fmt.Sprintf("%s must be %q or %q (got: %q)", proxyControllerEnvVar, proxyControllerNginx, proxyControllerKong, v))
	}
}

// microservicesDemoAppValuesTmpl mirrors
// envoy-loadtesting/wc-deployment/values/microservices-demo.yaml. ${BASE}
// stands in for the source file's ${WC}.${BASE_DOMAIN} (the test framework
// already hands us the concatenated FQDN as baseDomain).
const microservicesDemoAppValuesTmpl = `
images:
  tag: v0.10.5

ingress:
  enabled: ${INGRESS_NGINX_ENABLED}
  number: ${PUBLIC_ENDPOINTS}
  base: ${BASE}
  host: nginx-onlineboutique

kong:
  enabled: ${KONG_ENABLED}
  number: ${PUBLIC_ENDPOINTS}
  base: ${BASE}
  host: kong-onlineboutique
  ingressCname: kong-ingress.${BASE}

httproute:
  enabled: true
  base: ${BASE}
  hostname: onlineboutique
  namespaces:
    create: true
    number: ${PUBLIC_ENDPOINTS}

# Per-pod sizing for a PEAK_HTTP_RPS=5000 run. Every PUBLIC_ENDPOINTS hostname
# fans into the single frontend Service in this namespace, so the whole budget
# lands on one set of deployments and the only lever left (HPA maxReplicas is
# pinned by HPA_MAX_REPLICAS) is vertical size. Requests are kept well below
# limits on purpose: the HPA targets 80% of *requests*, so a modest request
# makes it scale out early while the limit leaves burst headroom for the
# fan-out spikes. Rough budget at 20 replicas: ~68 CPU of requests, which is
# why cluster_values_test.go provisions m5.4xlarge nodes.
adService:
  # Java/JVM, one GetAds per product page. Needs heap headroom above requests.
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1500m
      memory: 1536Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

cartService:
  # .NET, one GetCart on every single page render — same rate as the frontend.
  resources:
    requests:
      cpu: 400m
      memory: 256Mi
    limits:
      cpu: 1500m
      memory: 1Gi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

checkoutService:
  # Go, only on the checkout flow (~1/32 of requests) but fans out to payment,
  # shipping, email, cart and productcatalog per order.
  resources:
    requests:
      cpu: 200m
      memory: 128Mi
    limits:
      cpu: 1
      memory: 512Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

currencyService:
  # Node.js and the worst amplifier in the stack: the frontend calls Convert
  # once per product per render, so ~10x the HTTP rate (~35-45k gRPC calls/s at
  # peak). JS is single-threaded, so extra CPU beyond ~1 core only buys libuv
  # and gRPC thread headroom — this service is the first thing to watch.
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: 1500m
      memory: 1Gi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

emailService:
  # Python, checkout flow only.
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
    limits:
      cpu: 1
      memory: 512Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

frontend:
  # Go, terminates every request from both proxies and renders the templates.
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: 2
      memory: 1Gi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

loadGenerator:
  # create defaults to false — k6 is the only load source. Sized only so an
  # accidental enable doesn't land a BestEffort pod on a saturated node.
  resources:
    requests:
      cpu: 300m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi

paymentService:
  # Node.js, checkout flow only.
  resources:
    requests:
      cpu: 200m
      memory: 128Mi
    limits:
      cpu: 1
      memory: 512Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

productCatalogService:
  # Go, one ListProducts or GetProduct on every render.
  resources:
    requests:
      cpu: 300m
      memory: 256Mi
    limits:
      cpu: 1
      memory: 1Gi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

recommendationService:
  # Python, on product pages only (~60% of requests) but the slowest runtime
  # per call in the stack, and it re-reads the catalog on each request.
  resources:
    requests:
      cpu: 400m
      memory: 512Mi
    limits:
      cpu: 1500m
      memory: 1Gi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

shippingService:
  # Go, cart and checkout flows.
  resources:
    requests:
      cpu: 200m
      memory: 128Mi
    limits:
      cpu: 1
      memory: 512Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80
`

// buildMicroservicesDemoAppValues returns the values overlay applied to the
// microservices-demo-app dependency. Mirrors
// envoy-loadtesting/wc-deployment/values/microservices-demo.yaml; the
// PUBLIC_ENDPOINTS / HPA_MIN_REPLICAS / HPA_MAX_REPLICAS knobs are read via
// envOrDefault so config.env (loaded by loadConfigEnv) supplies the same
// defaults as the manual pipeline. Only the chosen ingress controller branch
// is enabled.
func buildMicroservicesDemoAppValues(baseDomain string) string {
	ingressEnabled := "false"
	kongEnabled := "false"
	switch proxyController {
	case proxyControllerNginx:
		ingressEnabled = "true"
	case proxyControllerKong:
		kongEnabled = "true"
	}
	vars := map[string]string{
		"INGRESS_NGINX_ENABLED": ingressEnabled,
		"KONG_ENABLED":          kongEnabled,
		"BASE":                  baseDomain,
		"PUBLIC_ENDPOINTS":      envOrDefault("PUBLIC_ENDPOINTS", "10"),
		"HPA_MIN_REPLICAS":      envOrDefault("HPA_MIN_REPLICAS", "1"),
		"HPA_MAX_REPLICAS":      envOrDefault("HPA_MAX_REPLICAS", "20"),
	}
	return os.Expand(microservicesDemoAppValuesTmpl, func(key string) string {
		return vars[key]
	})
}

func TestPerformance(t *testing.T) {
	suite.New().
		// envoy-gateway is the SUT; the framework installs it via the
		// gateway-api-bundle so the gateway-api CRDs and the default
		// Gateway/HTTPRoute config come up at the same time. Bundle-level
		// values (ListenerSet, listeners, TLS issuer) live in
		// bundle_values.yaml.
		InAppBundle("gateway-api-bundle").
		WithInstallNamespace("envoy-gateway-system").
		WithIsUpgrade(isUpgrade).
		WithValuesFile("./values.yaml").
		WithBundleValuesFile("./bundle_values.yaml").
		AfterClusterReady(func() {
			var (
				awsLBApp        *application.Application
				ingressNginxApp *application.Application
				kongApp         *application.Application
			)

			It("should create the loadtesting namespace", FlakeAttempts(3), func() {
				createWorkloadClusterNamespace("loadtesting")
			})

			It("should install aws-load-balancer-controller", FlakeAttempts(3), func() {
				mcName := state.GetFramework().MC().GetClusterName()
				clusterName := state.GetCluster().Name
				awsLBApp = deployDependency("aws-lb-controller-bundle", fmt.Sprintf(awsLBControllerBundleValues, mcName, clusterName, clusterName))
			})

			if proxyController == proxyControllerNginx {
				It("should install ingress-nginx", FlakeAttempts(3), func() {
					ingressNginxApp = deployDependency("ingress-nginx", ingressNginxValues)
				})
			}

			It("should wait for aws-load-balancer-controller to be ready", FlakeAttempts(3), func() {
				waitForDependency(awsLBApp)
			})

			if proxyController == proxyControllerNginx {
				It("should wait for ingress-nginx to be ready", FlakeAttempts(3), func() {
					waitForDependency(ingressNginxApp)
				})
			}

			if proxyController == proxyControllerKong {
				It("should install kong-app", FlakeAttempts(3), func() {
					baseDomain := getWorkloadClusterBaseDomain()
					kongApp = deployDependency("kong-app", fmt.Sprintf(kongAppValues, baseDomain), "kong")
					waitForDependency(kongApp)
				})

				It("should configure kong prometheus plugin", FlakeAttempts(3), func() {
					By("Waiting for KongClusterPlugin CRD to be registered")
					Eventually(func() (bool, error) {
						return crdExists("kongclusterplugins.configuration.konghq.com")
					}).
						WithTimeout(5 * time.Minute).
						WithPolling(10 * time.Second).
						Should(BeTrue())

					By("Adding extraObjects config to kong-app via spec.extraConfigs")
					clusterName := state.GetCluster().Name
					addExtraConfigToApp(
						fmt.Sprintf("%s-kong-app", clusterName),
						fmt.Sprintf("%s-kong-extra-objects", clusterName),
						kongExtraObjectsValues,
					)
				})
			}
		}).
		Tests(func() {
			var (
				microservicesDemoApp *application.Application
				nginxUrl             string
				envoyUrl             string
				kongUrl              string
			)
			BeforeEach(func() {
				nginxUrl = fmt.Sprintf("https://nginx-onlineboutique-0.loadtesting.%s", getWorkloadClusterBaseDomain())
				envoyUrl = fmt.Sprintf("https://onlineboutique.loadtesting-0.%s", getWorkloadClusterBaseDomain())
				// Kong runs as a Gateway API implementation: the chart exposes a
				// single HTTPRoute host (no per-endpoint fan-out like Envoy/nginx).
				kongUrl = fmt.Sprintf("https://kong-onlineboutique.loadtesting.%s", getWorkloadClusterBaseDomain())
			})

			It("should have deployed envoy-gateway via the gateway-api-bundle", func() {
				bundleApp := state.GetBundleApplication()
				Expect(bundleApp).NotTo(BeNil())

				Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), bundleApp.InstallName, bundleApp.GetNamespace())).
					WithTimeout(15 * time.Minute).
					WithPolling(5 * time.Second).
					Should(BeTrue())

				Eventually(func() (bool, error) {
					done, err := wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), state.GetApplication().InstallName, state.GetApplication().Organization.GetNamespace())()
					if err != nil {
						if errors.IsNotFound(err) {
							logger.Log("App '%s/%s' doesn't exist yet", state.GetApplication().Organization.GetNamespace(), state.GetApplication().InstallName)
							return false, nil
						}
						return false, err
					}
					return done, nil
				}).
					WithTimeout(15 * time.Minute).
					WithPolling(5 * time.Second).
					Should(BeTrue())
			})

			It("should have gateway api CRDs registered", func() {
				for _, crd := range []string{
					"gateways.gateway.networking.k8s.io",
					"httproutes.gateway.networking.k8s.io",
					"listenersets.gateway.networking.k8s.io",
				} {
					Eventually(func() (bool, error) {
						return crdExists(crd)
					}).
						WithTimeout(5 * time.Minute).
						WithPolling(10 * time.Second).
						Should(BeTrue())
				}
			})

			It("should have ready dependency deployments on the workload cluster", func() {
				namespaces := []string{"aws-load-balancer-controller", "envoy-gateway-system"}
				if proxyController == proxyControllerKong {
					namespaces = append(namespaces, "kong")
				}
				for _, ns := range namespaces {
					Eventually(func() (bool, error) {
						return deploymentReadyInNamespace(ns)
					}).
						WithTimeout(10 * time.Minute).
						WithPolling(5 * time.Second).
						Should(BeTrue())
				}
			})

			It("should install and wait for microservices-demo-app", func() {
				baseDomain := getWorkloadClusterBaseDomain()
				microservicesDemoApp = deployDependency("microservices-demo-app", buildMicroservicesDemoAppValues(baseDomain), "loadtesting")
				waitForDependency(microservicesDemoApp)
			})

			It("should raise redis-cart resources for the load test", func() {
				// The microservices-demo-app chart hard-codes redis-cart at
				// 125m CPU / 256Mi with no values override and no HPA (see
				// templates/cart-service/cartservice.yaml). Every page render
				// does a GetCart, so at PEAK_HTTP_RPS a 125m limit throttles
				// the whole boutique and the Envoy-vs-proxy delta disappears
				// behind cart latency. Patch it in place instead of forking
				// the chart.
				//
				// Vertical only: redis is single-threaded, so it cannot use
				// more than ~1 core, and extra replicas would shard carts
				// across pods with per-pod emptyDir storage — a checkout would
				// then find an empty cart and fail the "order is complete"
				// check.
				patchDeploymentResources("loadtesting", "redis-cart", "redis",
					corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				)
			})

			It("should have ready LoadBalancer services on the workload cluster", func() {
				namespaces := []string{"envoy-gateway-system"}
				switch proxyController {
				case proxyControllerNginx:
					namespaces = append(namespaces, "default")
				case proxyControllerKong:
					namespaces = append(namespaces, "kong")
				}
				for _, ns := range namespaces {
					Eventually(func() (bool, error) {
						return loadBalancerServiceReadyInNamespace(ns)
					}).
						WithTimeout(10 * time.Minute).
						WithPolling(10 * time.Second).
						Should(BeTrue())
				}
			})

			It("should have ready certificates on the workload cluster", func() {
				expected := []types.NamespacedName{
					{Namespace: "loadtesting-0", Name: "gateway-0-https"},
				}
				switch proxyController {
				case proxyControllerNginx:
					expected = append(expected, types.NamespacedName{Namespace: "loadtesting", Name: "frontend-nginx-wildcard"})
				case proxyControllerKong:
					expected = append(expected, types.NamespacedName{Namespace: "loadtesting", Name: "frontend-kong-wildcard"})
				}

				Eventually(func() (bool, error) {
					return allCertificatesReady(expected)
				}).
					WithTimeout(20 * time.Minute).
					WithPolling(5 * time.Second).
					Should(BeTrue())
			})
			if proxyController == proxyControllerNginx {
				It("should serve traffic from ingress-nginx", func() {
					DeferCleanup(func() {
						if CurrentSpecReport().Failed() {
							AbortSuite("ingress-nginx failed to serve traffic, aborting remaining tests")
						}
					})
					expectEndpointServesTraffic(nginxUrl)
				})
			}
			It("should serve traffic from envoy gateway", func() {
				DeferCleanup(func() {
					if CurrentSpecReport().Failed() {
						AbortSuite("envoy gateway failed to serve traffic, aborting remaining tests")
					}
				})
				expectEndpointServesTraffic(envoyUrl)
			})
			if proxyController == proxyControllerKong {
				It("should serve traffic from kong", func() {
					DeferCleanup(func() {
						if CurrentSpecReport().Failed() {
							AbortSuite("kong failed to serve traffic, aborting remaining tests")
						}
					})
					expectEndpointServesTraffic(kongUrl)
				})
			}
			It("should run k6 load tests successfully", func() {
				k6Namespace := getK6Namespace()
				baseDomain := getWorkloadClusterBaseDomain()
				testRunName := fmt.Sprintf("e2e-load-test-%s", state.GetCluster().Name)
				configMapName := fmt.Sprintf("e2e-load-test-scenario-%s", state.GetCluster().Name)
				testID := envOrDefault("K6_TEST_ID", testRunName)

				// Clean up any stale resources from a previous interrupted run
				cleanupK6Resources(testRunName, configMapName, k6Namespace)

				if prometheusEnabled() {
					By("Mirroring alloy-metrics credentials into the k6 namespace")
					mirrorPrometheusCredentials(k6Namespace)
				}

				By("Creating test scenario ConfigMap on the MC")
				cm := buildScenarioConfigMap(configMapName, k6Namespace)
				err := state.GetFramework().MC().Create(state.GetContext(), cm)
				Expect(err).NotTo(HaveOccurred())

				By("Creating TestRun on the MC")
				testRun := buildTestRunUnstructured(testRunName, k6Namespace, configMapName, baseDomain, testID)
				err = state.GetFramework().MC().Create(state.GetContext(), testRun)
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for TestRun to complete")
				var lastStage string
				Eventually(func() (string, error) {
					stage, err := getTestRunStage(testRunName, k6Namespace)
					if err != nil {
						return "", err
					}
					if stage != "" && stage != testRunGone {
						lastStage = stage
					}
					return stage, nil
				}).
					// SCENARIO_DURATION_SECONDS runs once per controller with
					// WAIT_BETWEEN_SCENARIOS in between: at 1h each that is
					// ~2h05 of wall clock, so the gate needs room beyond it.
					WithTimeout(180 * time.Minute).
					WithPolling(30 * time.Second).
					Should(BeElementOf("finished", "error", testRunGone))

				By("Asserting TestRun succeeded")
				assertTestRunSuccess(testRunName, k6Namespace, lastStage)

				By("Cleaning up k6 resources")
				cleanupK6Resources(testRunName, configMapName, k6Namespace)
			})
		}).
		AfterSuite(func() {
			k6Namespace := getK6Namespace()
			testRunName := fmt.Sprintf("e2e-load-test-%s", state.GetCluster().Name)
			configMapName := fmt.Sprintf("e2e-load-test-scenario-%s", state.GetCluster().Name)
			cleanupK6Resources(testRunName, configMapName, k6Namespace)
		}).
		Run(t, "Performance Test")
}
