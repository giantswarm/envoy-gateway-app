package mobilelatency

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/clustertest/v5/pkg/application"
	"github.com/giantswarm/clustertest/v5/pkg/wait"

	"performance/internal/deps"
)

const awsLBControllerBundleValues = `
managementCluster:
  name: %s
  namespace: org-giantswarm

clusterName: %s
clusterID: %s

provider: aws

global:
  podSecurityStandards:
    enforced: true

enableServiceMutatorWebhook: false
`

const ingressNginxValues = `
controller:
  ingressClassResource:
    enabled: true
  service:
    externalDNS:
      enabled: true
  extraArgs:
    update-status: "true"
`

func deployDependency(depName, depValues string, installNs ...string) *application.Application {
	By(fmt.Sprintf("deploying %s", depName))

	org := state.GetCluster().Organization

	isBundle := strings.Contains(depName, "bundle")
	installNamespace := org.GetNamespace()
	if !isBundle {
		installNamespace = "default"
	}
	if len(installNs) > 0 && installNs[0] != "" {
		installNamespace = installNs[0]
	}

	version, ok := deps.Versions[depName]
	if !ok {
		Fail(fmt.Sprintf("no version pin defined for dependency %q — add it to performance/internal/deps", depName))
	}

	clusterName := state.GetCluster().Name
	app := application.New(fmt.Sprintf("%s-%s", clusterName, depName), depName).
		WithCatalog("giantswarm").
		WithOrganization(*org).
		WithVersion(version).
		WithClusterName(clusterName).
		WithInCluster(isBundle).
		WithInstallNamespace(installNamespace).
		MustWithValues(depValues, nil)

	err := state.GetFramework().MC().DeployApp(state.GetContext(), *app)
	Expect(err).NotTo(HaveOccurred())

	return app
}

func waitForDependency(app *application.Application) {
	By(fmt.Sprintf("waiting for %s to be deployed", app.InstallName))

	org := state.GetCluster().Organization
	Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), app.InstallName, org.GetNamespace())).
		WithTimeout(10 * time.Minute).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}
