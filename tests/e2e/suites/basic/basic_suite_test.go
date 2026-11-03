package basic

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/client"
	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/apptest-framework/v5/pkg/suite"
	"github.com/giantswarm/clustertest/v5/pkg/wait"
)

const (
	isUpgrade = false
)

func TestBasic(t *testing.T) {
	suite.New().
		InAppBundle("gateway-api-bundle").
		WithInstallNamespace("envoy-gateway-system").
		WithIsUpgrade(isUpgrade).
		WithValuesFile("./values.yaml").
		WithBundleValuesFile("./bundle_values.yaml").
		Tests(func() {
			It("should have created a bundle application", func() {
				Expect(state.GetBundleApplication()).ToNot(BeNil())
				Expect(state.GetBundleApplication().AppName).ToNot(Equal(state.GetApplication().AppName))
			})

			It("should have deployed the bundle app", func() {
				Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), state.GetBundleApplication().InstallName, state.GetBundleApplication().InstallNamespace)).
					WithTimeout(30 * time.Second).
					WithPolling(50 * time.Millisecond).
					Should(BeTrue())
			})

			// gateway-api-bundle v2 renders its children as Flux HelmReleases instead of App CRs.
			It("should have deployed the test app", func() {
				Eventually(func() (bool, error) {
					return client.IsHelmReleaseReady(state.GetContext(), state.GetApplication().InstallName, state.GetApplication().Organization.GetNamespace())
				}).
					WithTimeout(15 * time.Minute).
					WithPolling(5 * time.Second).
					Should(BeTrue())
			})

			It("should have deployed the test app with correct version", func() {
				Eventually(func() (bool, error) {
					return client.IsHelmReleaseVersion(state.GetContext(), state.GetApplication().InstallName, state.GetApplication().Organization.GetNamespace(), state.GetApplication().Version)
				}).
					WithTimeout(5 * time.Minute).
					WithPolling(5 * time.Second).
					Should(BeTrue())
			})
		}).
		Run(t, "Envoy Gateway Test")
}
