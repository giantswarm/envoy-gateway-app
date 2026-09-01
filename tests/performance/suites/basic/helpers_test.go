package basic

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/clustertest/v5/pkg/application"
	"github.com/giantswarm/clustertest/v5/pkg/logger"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createWorkloadClusterNamespace(name string) {
	wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
	Expect(err).NotTo(HaveOccurred())

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	Eventually(func() error {
		err := wcClient.Create(state.GetContext(), ns)
		if err == nil || errors.IsAlreadyExists(err) {
			return nil
		}
		logger.Log("Create namespace %s failed, will retry: %v", name, err)
		return err
	}).
		WithTimeout(5 * time.Minute).
		WithPolling(5 * time.Second).
		Should(Succeed())
}

func getWorkloadClusterBaseDomain() string {
	values := &application.ClusterValues{}
	err := state.GetFramework().MC().GetHelmValues(state.GetCluster().Name, state.GetCluster().GetNamespace(), values)
	Expect(err).NotTo(HaveOccurred())

	if values.BaseDomain == "" {
		Fail("baseDomain field missing from cluster helm values")
	}

	return fmt.Sprintf("%s.%s", state.GetCluster().Name, values.BaseDomain)
}

func newHttpClientWithProxy(endpoint string) *http.Client {
	httpProxy := os.Getenv("HTTP_PROXY")
	noProxy := os.Getenv("NO_PROXY")
	useProxy := httpProxy != "" && !endpointMatchesNoProxy(endpoint, noProxy)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Resolver: &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						if useProxy {
							u, err := url.Parse(httpProxy)
							if err != nil {
								logger.Log("Error parsing HTTP_PROXY as a URL %s", httpProxy)
							} else {
								if addr == u.Host {
									// always use coredns for proxy address resolution.
									var d net.Dialer
									return d.Dial(network, address)
								}
							}
						}
						d := net.Dialer{
							Timeout: time.Millisecond * time.Duration(10000),
						}
						return d.DialContext(ctx, "udp", "8.8.4.4:53")
					},
				},
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}

	if useProxy {
		logger.Log("Detected need to use PROXY as HTTP_PROXY env var was set to %s", httpProxy)
		transport.Proxy = http.ProxyFromEnvironment
	} else if httpProxy != "" {
		logger.Log("Skipping HTTP_PROXY %s for endpoint %s because it matches NO_PROXY=%s", httpProxy, endpoint, noProxy)
	}

	httpClient := &http.Client{
		Transport: transport,
	}
	return httpClient
}

func endpointMatchesNoProxy(endpoint, noProxy string) bool {
	if noProxy == "" {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	for entry := range strings.SplitSeq(noProxy, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		entry = strings.TrimPrefix(entry, ".")
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}

// patchDeploymentResources overwrites the resource requests/limits of a single
// container in an already-deployed Deployment on the workload cluster, then
// waits for the resulting rollout to finish. Used for chart values that a
// dependency app doesn't expose (see the redis-cart step), so the load-test
// sizing can stay in this repo instead of forking the upstream chart.
//
// Helm won't undo this on its own — app-operator only re-renders on an App CR
// change — but a chart upgrade or a HelmRelease drift correction would, so this
// runs after the app is deployed and before traffic starts.
func patchDeploymentResources(namespace, name, container string, requests, limits corev1.ResourceList) {
	wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("patching resources of %s/%s container %q", namespace, name, container))
	Eventually(func() error {
		dep := &appsv1.Deployment{}
		if err := wcClient.Get(state.GetContext(), client.ObjectKey{Namespace: namespace, Name: name}, dep); err != nil {
			return err
		}

		idx := -1
		for i := range dep.Spec.Template.Spec.Containers {
			if dep.Spec.Template.Spec.Containers[i].Name == container {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("container %q not found in deployment %s/%s", container, namespace, name)
		}

		dep.Spec.Template.Spec.Containers[idx].Resources = corev1.ResourceRequirements{
			Requests: requests,
			Limits:   limits,
		}

		// Conflicts are expected here — the deployment controller writes
		// status concurrently — so re-read and retry rather than failing.
		return wcClient.Update(state.GetContext(), dep)
	}).
		WithTimeout(5 * time.Minute).
		WithPolling(10 * time.Second).
		Should(Succeed())

	By(fmt.Sprintf("waiting for %s/%s to roll out", namespace, name))
	Eventually(func() (bool, error) {
		dep := &appsv1.Deployment{}
		if err := wcClient.Get(state.GetContext(), client.ObjectKey{Namespace: namespace, Name: name}, dep); err != nil {
			return false, err
		}
		if dep.Status.ObservedGeneration < dep.Generation {
			logger.Log("Deployment %s/%s rollout not observed yet", namespace, name)
			return false, nil
		}
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		ready := dep.Status.UpdatedReplicas == desired &&
			dep.Status.ReadyReplicas == desired &&
			dep.Status.UnavailableReplicas == 0
		if !ready {
			logger.Log("Deployment %s/%s rolling out: %d/%d updated, %d/%d ready",
				namespace, name, dep.Status.UpdatedReplicas, desired, dep.Status.ReadyReplicas, desired)
		}
		return ready, nil
	}).
		WithTimeout(10 * time.Minute).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}

func deploymentReadyInNamespace(namespace string) (bool, error) {
	wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
	if err != nil {
		return false, err
	}

	logger.Log("Checking for ready deployments in namespace %s", namespace)
	deployments := &appsv1.DeploymentList{}
	err = wcClient.List(state.GetContext(), deployments, client.InNamespace(namespace))
	if err != nil {
		return false, err
	}

	for i := range deployments.Items {
		dep := &deployments.Items[i]
		if dep.Spec.Replicas == nil {
			continue
		}
		desired := *dep.Spec.Replicas
		if desired > 0 && dep.Status.ReadyReplicas == desired && dep.Status.AvailableReplicas == desired {
			logger.Log("Deployment %s/%s is ready (%d/%d replicas)", namespace, dep.Name, dep.Status.ReadyReplicas, desired)
			return true, nil
		}
	}

	logger.Log("No ready deployment found in namespace %s", namespace)
	return false, nil
}

func loadBalancerServiceReadyInNamespace(namespace string) (bool, error) {
	wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
	if err != nil {
		return false, err
	}

	logger.Log("Checking for ready LoadBalancer services in namespace %s", namespace)
	services := &corev1.ServiceList{}
	err = wcClient.List(state.GetContext(), services, client.InNamespace(namespace))
	if err != nil {
		return false, err
	}

	for i := range services.Items {
		svc := &services.Items[i]
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if len(svc.Status.LoadBalancer.Ingress) > 0 &&
				(svc.Status.LoadBalancer.Ingress[0].Hostname != "" || svc.Status.LoadBalancer.Ingress[0].IP != "") {
				logger.Log("LoadBalancer service %s/%s is ready with address: %s%s",
					namespace, svc.Name, svc.Status.LoadBalancer.Ingress[0].Hostname, svc.Status.LoadBalancer.Ingress[0].IP)
				return true, nil
			}
		}
	}

	logger.Log("No ready LoadBalancer service found in namespace %s", namespace)
	return false, nil
}

func allCertificatesReady(expected []types.NamespacedName) (bool, error) {
	wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
	if err != nil {
		return false, err
	}

	logger.Log("Listing all certificates on the workload cluster")
	certs := &cmv1.CertificateList{}
	if err := wcClient.List(state.GetContext(), certs); err != nil {
		return false, err
	}

	present := make(map[types.NamespacedName]struct{}, len(certs.Items))
	for i := range certs.Items {
		cert := &certs.Items[i]
		present[types.NamespacedName{Namespace: cert.Namespace, Name: cert.Name}] = struct{}{}
	}
	for _, key := range expected {
		if _, ok := present[key]; !ok {
			logger.Log("Expected certificate %s not found yet", key)
			return false, nil
		}
	}

	for i := range certs.Items {
		cert := &certs.Items[i]
		ready := false
		for _, condition := range cert.Status.Conditions {
			if condition.Type == cmv1.CertificateConditionReady && condition.Status == cmmeta.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			logger.Log("Certificate %s/%s not ready yet", cert.Namespace, cert.Name)
			return false, nil
		}
	}

	logger.Log("All %d certificates are ready", len(certs.Items))
	return true, nil
}

func crdExists(name string) (bool, error) {
	wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
	if err != nil {
		return false, err
	}

	logger.Log("Checking if CRD %s exists", name)
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	err = wcClient.Get(state.GetContext(), client.ObjectKey{Name: name}, crd)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Log("CRD %s not found yet", name)
			return false, nil
		}
		return false, err
	}

	logger.Log("CRD %s exists", name)
	return true, nil
}

func expectEndpointServesTraffic(endpoint string) {
	httpClient := newHttpClientWithProxy(endpoint)
	Eventually(func() (string, error) {
		logger.Log("Trying to get a successful response from %s", endpoint)
		resp, err := httpClient.Get(endpoint)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Log("Was expecting status code '%d' but actually got '%d'", http.StatusOK, resp.StatusCode)
			return "", err
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Log("Was not expecting the response body to be empty")
			return "", err
		}

		return string(bodyBytes), nil
	}).
		WithTimeout(15 * time.Minute).
		WithPolling(5 * time.Second).
		Should(ContainSubstring("Online Boutique"))
}
