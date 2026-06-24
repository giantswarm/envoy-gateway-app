package basicauth

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/clustertest/v5/pkg/logger"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cr "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The microservices-demo-app chart creates one Gateway per endpoint
	// namespace: gateway-{i} in loadtesting-{i} for i in [0, PUBLIC_ENDPOINTS),
	// and each boutique HTTPRoute attaches to its own local gateway. Envoy
	// Gateway requires a SecurityPolicy to live in the same namespace as the
	// Gateway it targets, so enforceEnvoyBasicAuth provisions one policy +
	// htpasswd Secret per namespace, looping over publicEndpoints().
	envoyGatewayNamespacePrefix = "loadtesting-"
	envoyGatewayNamePrefix      = "gateway-"

	// envoyHtpasswdSecret holds the htpasswd file consumed by the Envoy
	// SecurityPolicy basicAuth (Secret key ".htpasswd").
	envoyHtpasswdSecret = "boutique-basic-auth-htpasswd"

	// nginxAuthSecret holds the htpasswd file consumed by ingress-nginx
	// (Secret key "auth"). Created in each namespace that owns a boutique
	// nginx Ingress.
	nginxAuthSecret = "boutique-basic-auth"

	// nginxHostPrefix is the host the chart assigns to the boutique nginx
	// Ingresses (from ingress.host in buildMicroservicesDemoAppValues). The
	// Ingress objects are named frontend-nginx-{i}, so matching on the rule host
	// — which we control — is the stable way to find them.
	nginxHostPrefix = "nginx-onlineboutique"

	// Kong runs as a Gateway API implementation (its own GatewayClass), exactly
	// like Envoy — not via Ingress. The chart's kong path is a single HTTPRoute
	// "kong-{frontend.name}" in the loadtesting namespace. (The chart also
	// renders frontend-kong-{i} Ingresses, but kong-app runs Gateway-API-only
	// with ingressClass: none and the kong DNSEndpoint only publishes the single
	// HTTPRoute host, so those Ingresses are neither routable nor reconciled.)
	kongRouteNamespace = "loadtesting"
	kongRouteName      = "kong-frontend"

	// Kong basic-auth wiring: a basic-auth KongPlugin is attached to the boutique
	// kong HTTPRoute via the konghq.com/plugins annotation, and a consumer +
	// credential secret holds the single valid identity.
	kongNamespace      = "kong"
	kongCredentialName = "boutique-basic-auth-cred"
	kongConsumerName   = "boutique-consumer"
	kongPluginName     = "boutique-basic-auth"

	// kongIngressClass must match kong-app's ingressController.ingressClass
	// (set to "none" in kongAppValues for Gateway-API-only operation).
	kongIngressClass = "none"
)

// httpRouteGVK is the Gateway API HTTPRoute kind, used to fetch and annotate
// the chart-created kong HTTPRoute.
var httpRouteGVK = schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}

func basicAuthUser() string     { return envOrDefault("BASIC_AUTH_USER", "testuser") }
func basicAuthPassword() string { return envOrDefault("BASIC_AUTH_PASSWORD", "testpassword") }

// publicEndpoints is the number of boutique endpoint namespaces the chart
// creates (httproute.namespaces.number). Must match the PUBLIC_ENDPOINTS used
// in buildMicroservicesDemoAppValues so every gateway-{i}/loadtesting-{i} gets
// a SecurityPolicy.
func publicEndpoints() int {
	n, err := strconv.Atoi(envOrDefault("PUBLIC_ENDPOINTS", "10"))
	if err != nil || n < 1 {
		return 10
	}
	return n
}

// htpasswdLine renders a single htpasswd entry using the {SHA} scheme, which
// both ingress-nginx (auth_basic) and Envoy Gateway basicAuth accept and which
// needs no external dependency to compute.
func htpasswdLine(user, password string) string {
	sum := sha1.Sum([]byte(password))
	return fmt.Sprintf("%s:{SHA}%s", user, base64.StdEncoding.EncodeToString(sum[:]))
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

// enforceEnvoyBasicAuth provisions, for each boutique endpoint namespace, a
// SecurityPolicy targeting that namespace's Gateway plus the htpasswd Secret it
// references. The chart creates gateway-{i}/loadtesting-{i} for every endpoint,
// and the k6 scenario hits all of them, so a single policy on gateway-0 would
// leave the other endpoints unauthenticated.
func enforceEnvoyBasicAuth() {
	htpasswd := []byte(htpasswdLine(basicAuthUser(), basicAuthPassword()) + "\n")

	for i := 0; i < publicEndpoints(); i++ {
		namespace := fmt.Sprintf("%s%d", envoyGatewayNamespacePrefix, i)
		gateway := fmt.Sprintf("%s%d", envoyGatewayNamePrefix, i)

		By(fmt.Sprintf("Creating the Envoy basic-auth htpasswd Secret in %s", namespace))
		applySecret(namespace, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: envoyHtpasswdSecret},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{".htpasswd": htpasswd},
		})

		By(fmt.Sprintf("Applying the SecurityPolicy targeting %s/%s", namespace, gateway))
		sp := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1",
			"kind":       "SecurityPolicy",
			"metadata": map[string]any{
				"name":      "boutique-basic-auth",
				"namespace": namespace,
			},
			"spec": map[string]any{
				"targetRefs": []any{
					map[string]any{
						"group": "gateway.networking.k8s.io",
						"kind":  "HTTPRoute",
						"name":  fmt.Sprintf("frontend-%d", i),
					},
				},
				"basicAuth": map[string]any{
					"users": map[string]any{"name": envoyHtpasswdSecret},
				},
			},
		}}
		sp.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "SecurityPolicy"})
		applyUnstructured(sp)
	}
}

// boutiqueIngresses returns every Ingress whose rule host starts with
// hostPrefix. The chart names the Ingress objects frontend-{nginx,kong}-{i},
// but assigns the host from the ingress.host / kong.host values we set, so the
// host is the reliable selector.
func boutiqueIngresses(hostPrefix string) []*networkingv1.Ingress {
	wc := wcClient()
	list := &networkingv1.IngressList{}
	err := wc.List(state.GetContext(), list)
	Expect(err).NotTo(HaveOccurred(), "failed to list ingresses")

	var matched []*networkingv1.Ingress
	for i := range list.Items {
		ing := &list.Items[i]
		for _, rule := range ing.Spec.Rules {
			if strings.HasPrefix(rule.Host, hostPrefix) {
				matched = append(matched, ing)
				break
			}
		}
	}
	return matched
}

// enforceNginxBasicAuth annotates every boutique nginx Ingress with the
// ingress-nginx basic-auth annotations and provisions the htpasswd Secret in
// each Ingress's namespace.
func enforceNginxBasicAuth() {
	wc := wcClient()
	ctx := state.GetContext()

	ingresses := boutiqueIngresses(nginxHostPrefix)
	Expect(ingresses).NotTo(BeEmpty(), "found no nginx boutique ingresses (host %q) to protect with basic auth", nginxHostPrefix)

	htpasswd := htpasswdLine(basicAuthUser(), basicAuthPassword()) + "\n"
	secretCreated := map[string]bool{}

	for _, ing := range ingresses {
		if !secretCreated[ing.Namespace] {
			applySecret(ing.Namespace, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: nginxAuthSecret},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"auth": []byte(htpasswd)},
			})
			secretCreated[ing.Namespace] = true
		}

		if ing.Annotations == nil {
			ing.Annotations = map[string]string{}
		}
		ing.Annotations["nginx.ingress.kubernetes.io/auth-type"] = "basic"
		ing.Annotations["nginx.ingress.kubernetes.io/auth-secret"] = nginxAuthSecret
		ing.Annotations["nginx.ingress.kubernetes.io/auth-realm"] = "Authentication Required"
		err := wc.Update(ctx, ing)
		Expect(err).NotTo(HaveOccurred(), "failed to annotate ingress %s/%s", ing.Namespace, ing.Name)
		logger.Log("Annotated nginx ingress %s/%s with basic auth", ing.Namespace, ing.Name)
	}
}

// enforceKongBasicAuth attaches a basic-auth KongPlugin to the boutique kong
// HTTPRoute (the Gateway API route the scenario hits) via the konghq.com/plugins
// annotation, plus the consumer and credential that make exactly one identity
// valid. Kong runs as a Gateway API implementation here, so enforcement targets
// the HTTPRoute, mirroring how the Envoy side targets its HTTPRoutes.
func enforceKongBasicAuth() {
	By("Creating the Kong basic-auth credential Secret")
	applySecret(kongNamespace, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   kongCredentialName,
			Labels: map[string]string{"konghq.com/credential": "basic-auth"},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": basicAuthUser(),
			"password": basicAuthPassword(),
		},
	})

	By("Creating the KongConsumer")
	consumer := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "configuration.konghq.com/v1",
		"kind":       "KongConsumer",
		"metadata": map[string]any{
			"name":      kongConsumerName,
			"namespace": kongNamespace,
			"annotations": map[string]any{
				"kubernetes.io/ingress.class": kongIngressClass,
			},
		},
		"username":    kongConsumerName,
		"credentials": []any{kongCredentialName},
	}}
	consumer.SetGroupVersionKind(schema.GroupVersionKind{Group: "configuration.konghq.com", Version: "v1", Kind: "KongConsumer"})
	applyUnstructured(consumer)

	By(fmt.Sprintf("Applying the basic-auth KongPlugin in %s", kongRouteNamespace))
	// The KongPlugin must live in the same namespace as the HTTPRoute that
	// references it via konghq.com/plugins.
	plugin := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "configuration.konghq.com/v1",
		"kind":       "KongPlugin",
		"metadata": map[string]any{
			"name":      kongPluginName,
			"namespace": kongRouteNamespace,
		},
		"plugin": "basic-auth",
		"config": map[string]any{"hide_credentials": true},
	}}
	plugin.SetGroupVersionKind(schema.GroupVersionKind{Group: "configuration.konghq.com", Version: "v1", Kind: "KongPlugin"})
	applyUnstructured(plugin)

	By(fmt.Sprintf("Annotating the kong HTTPRoute %s/%s with the basic-auth plugin", kongRouteNamespace, kongRouteName))
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
		Should(Succeed(), "failed to annotate kong HTTPRoute %s/%s with the basic-auth plugin", kongRouteNamespace, kongRouteName)

	logger.Log("Attached basic-auth KongPlugin to kong HTTPRoute %s/%s", kongRouteNamespace, kongRouteName)
}

// expectEndpointRequiresAuth verifies the gateway enforces basic auth: an
// unauthenticated request is rejected with 401, while a request carrying the
// provisioned credentials reaches the boutique frontend (200). Both checks
// poll to absorb policy-propagation delay.
func expectEndpointRequiresAuth(endpoint string) {
	httpClient := newHttpClientWithProxy(endpoint)
	user := basicAuthUser()
	pass := basicAuthPassword()

	By(fmt.Sprintf("expecting %s to reject unauthenticated requests with 401", endpoint))
	Eventually(func() (int, error) {
		resp, err := httpClient.Get(endpoint)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		logger.Log("Unauthenticated GET %s -> %d", endpoint, resp.StatusCode)
		return resp.StatusCode, nil
	}).
		WithTimeout(10 * time.Minute).
		WithPolling(10 * time.Second).
		Should(Equal(http.StatusUnauthorized))

	By(fmt.Sprintf("expecting %s to serve traffic with valid credentials", endpoint))
	Eventually(func() (string, error) {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.SetBasicAuth(user, pass)
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
