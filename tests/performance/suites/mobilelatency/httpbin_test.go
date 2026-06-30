package mobilelatency

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	cr "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// httpbin is deployed into the boutique's namespace so HTTPRoutes/Ingresses
	// can reference it without cross-namespace ReferenceGrants.
	httpbinNamespace = "loadtesting"
	httpbinName      = "httpbin"
	httpbinPort      = 8080

	// delayPath is overlaid (as a more specific PathPrefix) on the existing
	// boutique hostnames, so go-httpbin's /delay/{seconds} is reachable without
	// minting new DNS records or certificates.
	delayPath = "/delay"

	// The boutique creates, per endpoint i, a ListenerSet listeners-{i} in
	// loadtesting-{i} (parented to the giantswarm-default Gateway in
	// envoy-gateway-system) holding the *.loadtesting-{i} hostname. httpbin
	// HTTPRoutes attach to these ListenerSets, exactly like the boutique's
	// frontend-{i} routes, so per-namespace hostnames bind correctly.
	envoyRouteNamespacePrefix = "loadtesting-"
	envoyListenerSetPrefix    = "listeners-"
)

// publicEndpoints is the number of boutique endpoint namespaces / ListenerSets
// the chart creates (httproute.namespaces.number). Must match the
// PUBLIC_ENDPOINTS used in buildMicroservicesDemoAppValues.
func publicEndpoints() int {
	n, err := strconv.Atoi(envOrDefault("PUBLIC_ENDPOINTS", "10"))
	if err != nil || n < 1 {
		return 10
	}
	return n
}

func httpbinImage() string {
	return envOrDefault("HTTPBIN_IMAGE", "gsoci.azurecr.io/giantswarm/go-httpbin:2.23.1")
}

func boolPtr(b bool) *bool    { return &b }
func int32Ptr(i int32) *int32 { return &i }
func int64Ptr(i int64) *int64 { return &i }

func wcClient() cr.Client {
	wc, err := state.GetFramework().WC(state.GetCluster().Name)
	Expect(err).NotTo(HaveOccurred())
	return wc
}

// createOrUpdate creates obj, or updates it in place (preserving resourceVersion)
// if it already exists, so the step is safe to retry.
func createOrUpdate(obj cr.Object) {
	wc := wcClient()
	ctx := state.GetContext()
	err := wc.Create(ctx, obj)
	if err == nil {
		return
	}
	if !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred(), "failed to create %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
	}
	existing := obj.DeepCopyObject().(cr.Object)
	err = wc.Get(ctx, cr.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	Expect(err).NotTo(HaveOccurred())
	obj.SetResourceVersion(existing.GetResourceVersion())
	// A Service's clusterIP(s) are immutable; carry them over from the existing
	// object so an update on retry isn't rejected for blanking them.
	if svc, ok := obj.(*corev1.Service); ok {
		cur := existing.(*corev1.Service)
		svc.Spec.ClusterIP = cur.Spec.ClusterIP
		svc.Spec.ClusterIPs = cur.Spec.ClusterIPs
	}
	err = wc.Update(ctx, obj)
	Expect(err).NotTo(HaveOccurred(), "failed to update %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
}

// applyHTTPRoute creates or updates a Gateway API HTTPRoute given as unstructured.
func applyHTTPRoute(obj *unstructured.Unstructured) {
	wc := wcClient()
	ctx := state.GetContext()
	existing := obj.DeepCopy()
	err := wc.Get(ctx, cr.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if errors.IsNotFound(err) {
		Expect(wc.Create(ctx, obj)).To(Succeed(), "failed to create HTTPRoute %s/%s", obj.GetNamespace(), obj.GetName())
		return
	}
	Expect(err).NotTo(HaveOccurred())
	obj.SetResourceVersion(existing.GetResourceVersion())
	Expect(wc.Update(ctx, obj)).To(Succeed(), "failed to update HTTPRoute %s/%s", obj.GetNamespace(), obj.GetName())
}

// deployHttpbin deploys go-httpbin (Deployment + Service) into the loadtesting
// namespace and waits for it to become ready. -max-duration is raised above the
// 10s default so the 12s "over_10s" band is served faithfully.
func deployHttpbin() {
	labels := map[string]string{"app": httpbinName}

	By("Creating the go-httpbin Deployment")
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: httpbinName, Namespace: httpbinNamespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   boolPtr(true),
						RunAsUser:      int64Ptr(1000),
						RunAsGroup:     int64Ptr(1000),
						FSGroup:        int64Ptr(1000),
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:  httpbinName,
						Image: httpbinImage(),
						Args:  []string{fmt.Sprintf("-port=%d", httpbinPort), "-max-duration=60s"},
						Ports: []corev1.ContainerPort{{ContainerPort: httpbinPort}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
								Path: "/status/200",
								Port: intstr.FromInt(httpbinPort),
							}},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             boolPtr(true),
							RunAsUser:                int64Ptr(1000),
							RunAsGroup:               int64Ptr(1000),
							AllowPrivilegeEscalation: boolPtr(false),
							ReadOnlyRootFilesystem:   boolPtr(true),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					}},
				},
			},
		},
	}
	createOrUpdate(dep)

	By("Creating the go-httpbin Service")
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: httpbinName, Namespace: httpbinNamespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       httpbinPort,
				TargetPort: intstr.FromInt(httpbinPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	createOrUpdate(svc)

	By("Waiting for go-httpbin to be ready")
	Eventually(httpbinReady).
		WithTimeout(10 * time.Minute).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}

func httpbinReady() (bool, error) {
	dep := &appsv1.Deployment{}
	err := wcClient().Get(state.GetContext(), cr.ObjectKey{Name: httpbinName, Namespace: httpbinNamespace}, dep)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	return dep.Status.ReadyReplicas >= desired, nil
}

// routeHttpbinThroughGateways overlays the /delay path of the existing boutique
// hostnames onto go-httpbin: an HTTPRoute on the Envoy gateway always, and an
// Ingress (nginx) or HTTPRoute (kong) for the selected reverse proxy. The more
// specific /delay PathPrefix wins over the boutique frontend's "/" route, so the
// boutique keeps serving everything else.
func routeHttpbinThroughGateways() {
	baseDomain := getWorkloadClusterBaseDomain()

	By("Routing /delay to go-httpbin on every Envoy endpoint")
	// One HTTPRoute per endpoint namespace, attached to that namespace's
	// ListenerSet (listeners-{i}, itself parented to giantswarm-default), exactly
	// like the boutique's frontend-{i} routes. The /delay prefix is overlaid on
	// the existing onlineboutique.loadtesting-{i} hostname (reusing DNS + TLS).
	// The backendRef to httpbin (in loadtesting) is permitted by the boutique's
	// allow-from-frontend ReferenceGrant.
	for i := 0; i < publicEndpoints(); i++ {
		ns := fmt.Sprintf("%s%d", envoyRouteNamespacePrefix, i)
		applyHTTPRoute(httpbinHTTPRoute(
			fmt.Sprintf("httpbin-envoy-%d", i),
			ns,
			"ListenerSet", fmt.Sprintf("%s%d", envoyListenerSetPrefix, i), ns,
			fmt.Sprintf("onlineboutique.loadtesting-%d.%s", i, baseDomain),
		))
	}

	switch proxyController {
	case proxyControllerNginx:
		By("Routing /delay to go-httpbin on ingress-nginx")
		createOrUpdate(httpbinNginxIngress(fmt.Sprintf("nginx-onlineboutique-0.loadtesting.%s", baseDomain)))
	case proxyControllerKong:
		By("Routing /delay to go-httpbin on the kong gateway")
		applyHTTPRoute(httpbinHTTPRoute(
			"httpbin-kong",
			httpbinNamespace,
			"Gateway", "kong", "loadtesting",
			fmt.Sprintf("kong-onlineboutique.loadtesting.%s", baseDomain),
		))
	}
}

// httpbinHTTPRoute builds an HTTPRoute (in routeNamespace) attaching to the given
// parent (a Gateway or ListenerSet) and forwarding the /delay prefix to the
// httpbin Service.
func httpbinHTTPRoute(name, routeNamespace, parentKind, parentName, parentNamespace, hostname string) *unstructured.Unstructured {
	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]any{
			"name":      name,
			"namespace": routeNamespace,
		},
		"spec": map[string]any{
			"parentRefs": []any{
				map[string]any{
					"group":     "gateway.networking.k8s.io",
					"kind":      parentKind,
					"name":      parentName,
					"namespace": parentNamespace,
				},
			},
			"hostnames": []any{hostname},
			"rules": []any{
				map[string]any{
					"matches": []any{
						map[string]any{
							"path": map[string]any{"type": "PathPrefix", "value": delayPath},
						},
					},
					"backendRefs": []any{
						map[string]any{
							"group":     "",
							"kind":      "Service",
							"name":      httpbinName,
							"namespace": httpbinNamespace,
							"port":      int64(httpbinPort),
						},
					},
				},
			},
		},
	}}
	route.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"})
	return route
}

// httpbinNginxIngress builds an Ingress that forwards the /delay prefix of the
// boutique nginx host to go-httpbin, reusing the wildcard TLS secret.
func httpbinNginxIngress(host string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	ingressClass := "nginx"
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "httpbin-nginx", Namespace: httpbinNamespace},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClass,
			TLS: []networkingv1.IngressTLS{{
				Hosts:      []string{host},
				SecretName: "nginx-ingress-wildcard-tls",
			}},
			Rules: []networkingv1.IngressRule{{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     delayPath,
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: httpbinName,
									Port: networkingv1.ServiceBackendPort{Number: httpbinPort},
								},
							},
						}},
					},
				},
			}},
		},
	}
}

// expectDelayServesTraffic verifies go-httpbin is reachable through the gateway
// at the /delay endpoint, polling to absorb route-propagation delay.
func expectDelayServesTraffic(baseURL string) {
	httpClient := newHttpClientWithProxy(baseURL)
	url := fmt.Sprintf("%s/delay/0", baseURL)

	By(fmt.Sprintf("expecting %s to return 200", url))
	Eventually(func() (int, error) {
		resp, err := httpClient.Get(url)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		logger.Log("GET %s -> %d", url, resp.StatusCode)
		return resp.StatusCode, nil
	}).
		WithTimeout(15 * time.Minute).
		WithPolling(10 * time.Second).
		Should(Equal(http.StatusOK))
}
