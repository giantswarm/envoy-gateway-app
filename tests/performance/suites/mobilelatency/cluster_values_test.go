package mobilelatency

import (
	"fmt"
	"os"
)

// clusterValuesFile is the workload-cluster values file the apptest framework
// reads verbatim at cluster standup (clusterbuilder.LoadOrBuildCluster reads
// "./test_data/cluster_values.yaml" relative to the suite working directory). It
// applies no templating, so we materialize the file at package init based on the
// selected proxy controller.
const clusterValuesFile = "test_data/cluster_values.yaml"

// NOTE: do NOT add global.apps.certManager.values...config.enableGatewayAPI
// here. It turns on cert-manager's ExperimentalGatewayAPISupport, and the
// controller then exits 1 with "the Gateway API CRDs do not seem to be present,
// but ExperimentalGatewayAPISupport is set to true" — those CRDs only arrive
// with gateway-api-bundle, which this suite installs *after* BeforeSuite's
// HelmRelease gate. The resulting deadlock (crashlooping cert-manager ->
// aws-pod-identity-webhook's Certificate never issued -> its TLS secret never
// created -> its HelmRelease never Ready) blew the 900s gate and killed the
// runs on 2026-09-01 and 2026-09-02. Every Certificate in this suite is an
// explicit cert-manager.io/v1 resource solved via DNS-01, so cert-manager
// needs no Gateway API support at all.
// baseClusterValues is the default (nginx) workload-cluster configuration. It
// must stay byte-identical to the checked-in test_data/cluster_values.yaml so a
// default run rewrites it to the same content (no-op, no git churn).
const baseClusterValues = `global:
  connectivity:
    certManager:
      useDnsChallenges: true
  nodePools:
    def00:
      minSize: 5
      maxSize: 5
`

// kongClusterValues additionally points the cluster's wildcard DNS CNAME at the
// gateway. Kong serves the boutique purely via Gateway API; without this the
// DNS-01 ACME challenge for the kong wildcard certificate self-loops and fails.
const kongClusterValues = `global:
  connectivity:
    dns:
      wildcardCnameTarget: gateway
    certManager:
      useDnsChallenges: true
  nodePools:
    def00:
      minSize: 5
      maxSize: 5
`

// init materializes test_data/cluster_values.yaml before the apptest framework
// reads it: the Kong variant adds global.connectivity.dns.wildcardCnameTarget,
// while the default variant restores the base so a prior Kong run cannot leak
// the override into a subsequent nginx run.
func init() {
	content := baseClusterValues
	if proxyController == proxyControllerKong {
		content = kongClusterValues
	}
	if err := os.WriteFile(clusterValuesFile, []byte(content), 0o600); err != nil {
		panic(fmt.Sprintf("failed to materialize %s: %v", clusterValuesFile, err))
	}
}
