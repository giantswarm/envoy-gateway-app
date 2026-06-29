package deps

// Versions maps a dependency app name to the chart/app version each suite
// installs via deployDependency. Suites look it up by name, so a suite that
// doesn't deploy a given dependency (e.g. the kong-only keyauth suite never
// installs ingress-nginx) simply never reads that entry — extra entries are
// harmless.
//
// The gateway-api-bundle is installed by apptest-framework via InAppBundle and
// is intentionally not listed here.
var Versions = map[string]string{
	"aws-lb-controller-bundle": "5.2.0",
	"ingress-nginx":            "4.3.3",
	"kong-app":                 "5.2.2",
	"microservices-demo-app":   "0.8.1",
}
