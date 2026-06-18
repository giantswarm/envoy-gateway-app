package keyauth

import (
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/apptest-framework/v5/pkg/suite"
	"github.com/giantswarm/clustertest/v5/pkg/application"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	"github.com/giantswarm/clustertest/v5/pkg/wait"
)

const (
	isUpgrade = false

	proxyControllerKong = "kong"
)

// microservicesDemoAppValuesTmpl mirrors
// envoy-loadtesting/wc-deployment/values/microservices-demo.yaml. ${BASE}
// stands in for the source file's ${WC}.${BASE_DOMAIN} (the test framework
// already hands us the concatenated FQDN as baseDomain).
const microservicesDemoAppValuesTmpl = `
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

adService:
  resources:
    requests:
      cpu: 200m
      memory: 180Mi
    limits:
      cpu: 300m
      memory: 300Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

cartService:
  resources:
    requests:
      cpu: 200m
      memory: 128Mi
    limits:
      cpu: 300m
      memory: 256Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

checkoutService:
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

currencyService:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

emailService:
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

frontend:
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

loadGenerator:
  resources:
    requests:
      cpu: 300m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi

paymentService:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

productCatalogService:
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

recommendationService:
  resources:
    requests:
      cpu: 100m
      memory: 220Mi
    limits:
      cpu: 200m
      memory: 450Mi
  hpa:
    enabled: true
    minReplicas: ${HPA_MIN_REPLICAS}
    maxReplicas: ${HPA_MAX_REPLICAS}
    targetCPUUtilizationPercentage: 80

shippingService:
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
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
	// This suite only compares against Kong (see resolveProxyController), so the
	// nginx ingress branch of the chart stays disabled.
	vars := map[string]string{
		"INGRESS_NGINX_ENABLED": "false",
		"KONG_ENABLED":          "true",
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
				awsLBApp *application.Application
				kongApp  *application.Application
			)

			It("should create the loadtesting namespace", FlakeAttempts(3), func() {
				createWorkloadClusterNamespace("loadtesting")
			})

			It("should install aws-load-balancer-controller", FlakeAttempts(3), func() {
				mcName := state.GetFramework().MC().GetClusterName()
				clusterName := state.GetCluster().Name
				awsLBApp = deployDependency("aws-lb-controller-bundle", fmt.Sprintf(awsLBControllerBundleValues, mcName, clusterName, clusterName))
			})

			It("should wait for aws-load-balancer-controller to be ready", FlakeAttempts(3), func() {
				waitForDependency(awsLBApp)
			})

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
		}).
		Tests(func() {
			var (
				microservicesDemoApp *application.Application
				envoyUrl             string
				kongUrl              string
			)
			BeforeEach(func() {
				envoyUrl = fmt.Sprintf("https://onlineboutique.loadtesting-0.%s", getWorkloadClusterBaseDomain())
				// Kong runs as a Gateway API implementation: the chart exposes a
				// single HTTPRoute host (no per-endpoint fan-out like Envoy).
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
				namespaces := []string{"aws-load-balancer-controller", "envoy-gateway-system", "kong"}
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

			It("should have ready LoadBalancer services on the workload cluster", func() {
				namespaces := []string{"envoy-gateway-system", "kong"}
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
					{Namespace: "loadtesting", Name: "frontend-kong-wildcard"},
				}

				Eventually(func() (bool, error) {
					return allCertificatesReady(expected)
				}).
					WithTimeout(20 * time.Minute).
					WithPolling(5 * time.Second).
					Should(BeTrue())
			})
			It("should serve traffic from envoy gateway", func() {
				DeferCleanup(func() {
					if CurrentSpecReport().Failed() {
						AbortSuite("envoy gateway failed to serve traffic, aborting remaining tests")
					}
				})
				expectEndpointServesTraffic(envoyUrl)
			})
			It("should serve traffic from kong", func() {
				DeferCleanup(func() {
					if CurrentSpecReport().Failed() {
						AbortSuite("kong failed to serve traffic, aborting remaining tests")
					}
				})
				expectEndpointServesTraffic(kongUrl)
			})

			// With the unauthenticated baseline confirmed above, enforce API key
			// auth at each gateway and verify it before load testing the
			// authenticated path. The k6 key-auth scenario then asserts
			// valid->200, wrong->401, missing->401 under load.
			It("should enforce api key auth on the envoy gateway", FlakeAttempts(3), func() {
				enforceEnvoyKeyAuth()
			})
			It("should enforce api key auth on kong", FlakeAttempts(3), func() {
				enforceKongKeyAuth()
			})
			It("should require an api key on envoy gateway", func() {
				DeferCleanup(func() {
					if CurrentSpecReport().Failed() {
						AbortSuite("envoy gateway api key auth not enforced, aborting remaining tests")
					}
				})
				expectEndpointRequiresKey(envoyUrl)
			})
			It("should require an api key on kong", func() {
				DeferCleanup(func() {
					if CurrentSpecReport().Failed() {
						AbortSuite("kong api key auth not enforced, aborting remaining tests")
					}
				})
				expectEndpointRequiresKey(kongUrl)
			})
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
					WithTimeout(120 * time.Minute).
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
		Run(t, "Key Auth Performance Test")
}
