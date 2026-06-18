package keyauth

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/clustertest/v5/pkg/logger"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cr "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The microservices-demo-app chart creates one HTTPRoute per endpoint
	// namespace: frontend-{i} in loadtesting-{i} for i in [0, PUBLIC_ENDPOINTS).
	// Envoy Gateway requires a SecurityPolicy to live in the same namespace as
	// the HTTPRoute it targets, so enforceEnvoyKeyAuth provisions one policy +
	// API key Secret per namespace, looping over publicEndpoints().
	envoyRouteNamespacePrefix = "loadtesting-"
	envoyRouteNamePrefix      = "frontend-"

	// envoyAPIKeySecret holds the API keys consumed by the Envoy SecurityPolicy
	// apiKeyAuth: each Secret data entry maps a client id -> API key value.
	envoyAPIKeySecret = "boutique-key-auth-keys"

	// apiKeyHeader is the header the gateways extract the API key from. Matches
	// the upstream key-auth use case.
	apiKeyHeader = "x-api-key"

	// apiKeyClientID is the (arbitrary) client identifier the API key is filed
	// under in the Envoy apiKeyAuth Secret.
	apiKeyClientID = "boutique-client"

	// Kong runs as a Gateway API implementation (its own GatewayClass), exactly
	// like Envoy — not via Ingress. The chart's kong path is a single HTTPRoute
	// "kong-{frontend.name}" in the loadtesting namespace serving the single
	// host kong-onlineboutique.loadtesting.<base>. (The chart also renders
	// frontend-kong-{i} Ingresses, but kong-app runs Gateway-API-only with
	// ingressClass: none and the kong DNSEndpoint only publishes the single
	// HTTPRoute host, so those Ingresses are neither routable nor reconciled.)
	kongRouteNamespace = "loadtesting"
	kongRouteName      = "kong-frontend"

	// Kong key-auth wiring: a key-auth KongPlugin is attached to the boutique
	// kong HTTPRoute via the konghq.com/plugins annotation, and a consumer +
	// credential secret holds the single valid API key.
	kongNamespace      = "kong"
	kongCredentialName = "boutique-key-auth-cred"
	kongConsumerName   = "boutique-consumer"
	kongPluginName     = "boutique-key-auth"
)

// httpRouteGVK is the Gateway API HTTPRoute kind, used to fetch and annotate
// the chart-created kong HTTPRoute.
var httpRouteGVK = schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}

func apiKey() string { return envOrDefault("API_KEY", "k6-perf-test-api-key-9f3a2b") }

// publicEndpoints is the number of boutique endpoint namespaces the chart
// creates (httproute.namespaces.number). Must match the PUBLIC_ENDPOINTS used
// in buildMicroservicesDemoAppValues so every frontend-{i}/loadtesting-{i} gets
// a SecurityPolicy.
func publicEndpoints() int {
	n, err := strconv.Atoi(envOrDefault("PUBLIC_ENDPOINTS", "10"))
	if err != nil || n < 1 {
		return 10
	}
	return n
}

func wcClient() cr.Client {
	wc, err := state.GetFramework().WC(state.GetCluster().Name)
	Expect(err).NotTo(HaveOccurred())
	return wc
}

// applySecret writes a Secret into namespace ns, replacing any stale copy from
// a previous run.
func applySecret(ns string, secret *corev1.Secret) {
	wc := wcClient()
	ctx := state.GetContext()
	secret.Namespace = ns
	_ = wc.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secret.Name, Namespace: ns}})
	err := wc.Create(ctx, secret)
	Expect(err).NotTo(HaveOccurred(), "failed to create secret %s/%s", ns, secret.Name)
}

// applyUnstructured creates obj, or updates it in place if it already exists.
func applyUnstructured(obj *unstructured.Unstructured) {
	wc := wcClient()
	ctx := state.GetContext()

	existing := obj.DeepCopy()
	err := wc.Get(ctx, cr.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if errors.IsNotFound(err) {
		err = wc.Create(ctx, obj)
		Expect(err).NotTo(HaveOccurred(), "failed to create %s %s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())
		return
	}
	Expect(err).NotTo(HaveOccurred(), "failed to get %s %s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())

	obj.SetResourceVersion(existing.GetResourceVersion())
	err = wc.Update(ctx, obj)
	Expect(err).NotTo(HaveOccurred(), "failed to update %s %s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())
}

// enforceEnvoyKeyAuth provisions, for each boutique endpoint namespace, a
// SecurityPolicy with apiKeyAuth targeting that namespace's HTTPRoute plus the
// API key Secret it references. The chart creates frontend-{i}/loadtesting-{i}
// for every endpoint and the k6 scenario hits all of them, so a single policy
// would leave the other endpoints unauthenticated.
func enforceEnvoyKeyAuth() {
	for i := 0; i < publicEndpoints(); i++ {
		namespace := fmt.Sprintf("%s%d", envoyRouteNamespacePrefix, i)
		route := fmt.Sprintf("%s%d", envoyRouteNamePrefix, i)

		By(fmt.Sprintf("Creating the Envoy API key Secret in %s", namespace))
		applySecret(namespace, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: envoyAPIKeySecret},
			Type:       corev1.SecretTypeOpaque,
			// data entry: client id -> API key value.
			StringData: map[string]string{apiKeyClientID: apiKey()},
		})

		By(fmt.Sprintf("Applying the apiKeyAuth SecurityPolicy targeting %s/%s", namespace, route))
		sp := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1",
			"kind":       "SecurityPolicy",
			"metadata": map[string]any{
				"name":      "boutique-key-auth",
				"namespace": namespace,
			},
			"spec": map[string]any{
				"targetRefs": []any{
					map[string]any{
						"group": "gateway.networking.k8s.io",
						"kind":  "HTTPRoute",
						"name":  route,
					},
				},
				"apiKeyAuth": map[string]any{
					"credentialRefs": []any{
						map[string]any{"group": "", "kind": "Secret", "name": envoyAPIKeySecret},
					},
					"extractFrom": []any{
						map[string]any{"headers": []any{apiKeyHeader}},
					},
				},
			},
		}}
		sp.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "SecurityPolicy"})
		applyUnstructured(sp)
	}
}

// enforceKongKeyAuth attaches a key-auth KongPlugin to the boutique kong
// HTTPRoute (the Gateway API route the scenario hits) via the konghq.com/plugins
// annotation, plus the consumer and credential that make exactly one API key
// valid. Kong runs as a Gateway API implementation here, so enforcement targets
// the HTTPRoute, mirroring how the Envoy side targets its HTTPRoutes.
func enforceKongKeyAuth() {
	By("Creating the Kong key-auth credential Secret")
	applySecret(kongNamespace, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   kongCredentialName,
			Labels: map[string]string{"konghq.com/credential": "key-auth"},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"key": apiKey()},
	})

	By("Creating the KongConsumer")
	consumer := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "configuration.konghq.com/v1",
		"kind":       "KongConsumer",
		"metadata": map[string]any{
			"name":      kongConsumerName,
			"namespace": kongNamespace,
			"annotations": map[string]any{
				"kubernetes.io/ingress.class": "kong",
			},
		},
		"username":    kongConsumerName,
		"credentials": []any{kongCredentialName},
	}}
	consumer.SetGroupVersionKind(schema.GroupVersionKind{Group: "configuration.konghq.com", Version: "v1", Kind: "KongConsumer"})
	applyUnstructured(consumer)

	By(fmt.Sprintf("Applying the key-auth KongPlugin in %s", kongRouteNamespace))
	// key_names is set to the x-api-key header used by the scenario (Kong
	// defaults to apikey). The KongPlugin must live in the same namespace as the
	// HTTPRoute that references it via konghq.com/plugins.
	plugin := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "configuration.konghq.com/v1",
		"kind":       "KongPlugin",
		"metadata": map[string]any{
			"name":      kongPluginName,
			"namespace": kongRouteNamespace,
		},
		"plugin": "key-auth",
		"config": map[string]any{
			"key_names":        []any{apiKeyHeader},
			"hide_credentials": true,
		},
	}}
	plugin.SetGroupVersionKind(schema.GroupVersionKind{Group: "configuration.konghq.com", Version: "v1", Kind: "KongPlugin"})
	applyUnstructured(plugin)

	By(fmt.Sprintf("Annotating the kong HTTPRoute %s/%s with the key-auth plugin", kongRouteNamespace, kongRouteName))
	annotateKongRoute()
}

// annotateKongRoute adds the konghq.com/plugins annotation to the chart-created
// kong HTTPRoute, re-fetching on each attempt so a concurrent reconcile (KIC /
// external-dns) can't lose the update to a resourceVersion conflict.
func annotateKongRoute() {
	wc := wcClient()
	ctx := state.GetContext()

	Eventually(func() error {
		route := &unstructured.Unstructured{}
		route.SetGroupVersionKind(httpRouteGVK)
		if err := wc.Get(ctx, cr.ObjectKey{Name: kongRouteName, Namespace: kongRouteNamespace}, route); err != nil {
			return err
		}
		annotations := route.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["konghq.com/plugins"] = kongPluginName
		route.SetAnnotations(annotations)
		return wc.Update(ctx, route)
	}).
		WithTimeout(5*time.Minute).
		WithPolling(10*time.Second).
		Should(Succeed(), "failed to annotate kong HTTPRoute %s/%s with the key-auth plugin", kongRouteNamespace, kongRouteName)

	logger.Log("Attached key-auth KongPlugin to kong HTTPRoute %s/%s", kongRouteNamespace, kongRouteName)
}

// expectEndpointRequiresKey verifies the gateway enforces API key auth: a
// request with no key is rejected with 401, while a request carrying the
// provisioned x-api-key reaches the boutique frontend (200). Both checks poll
// to absorb policy-propagation delay.
func expectEndpointRequiresKey(endpoint string) {
	httpClient := newHttpClientWithProxy(endpoint)
	key := apiKey()

	By(fmt.Sprintf("expecting %s to reject requests with no API key with 401", endpoint))
	Eventually(func() (int, error) {
		resp, err := httpClient.Get(endpoint)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		logger.Log("Keyless GET %s -> %d", endpoint, resp.StatusCode)
		return resp.StatusCode, nil
	}).
		WithTimeout(10 * time.Minute).
		WithPolling(10 * time.Second).
		Should(Equal(http.StatusUnauthorized))

	By(fmt.Sprintf("expecting %s to serve traffic with a valid API key", endpoint))
	Eventually(func() (string, error) {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set(apiKeyHeader, key)
		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			logger.Log("Authenticated GET %s -> %d (want 200)", endpoint, resp.StatusCode)
			_, _ = io.Copy(io.Discard, resp.Body)
			return "", nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}).
		WithTimeout(10 * time.Minute).
		WithPolling(10 * time.Second).
		Should(ContainSubstring("Online Boutique"))
}
