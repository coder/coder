package agentegress

import (
	"strings"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// ProxyEnv returns the environment variables that point workspace processes
// at the local egress proxy. Both upper and lower case spellings are set
// because tooling is inconsistent about which it honors. NO_PROXY contains
// only exemption hostnames because it has no protocol or port semantics. It
// applies only to the explicit proxy path; transparent capture remains scoped
// by the full protocol and port exemptions.
func ProxyEnv(listenAddr string, cfg agentsdk.EgressConfig) map[string]string {
	proxyURL := "http://" + listenAddr
	noProxy := noProxyList(cfg.ControlPlaneHosts)
	return map[string]string{
		"HTTP_PROXY":  proxyURL,
		"HTTPS_PROXY": proxyURL,
		"ALL_PROXY":   proxyURL,
		"http_proxy":  proxyURL,
		"https_proxy": proxyURL,
		"all_proxy":   proxyURL,
		"NO_PROXY":    noProxy,
		"no_proxy":    noProxy,
	}
}

func noProxyList(controlPlaneHosts []string) string {
	entries := []string{"localhost", "127.0.0.1", "::1"}
	seen := map[string]struct{}{}
	for _, e := range entries {
		seen[e] = struct{}{}
	}
	for _, value := range controlPlaneHosts {
		exemption, err := ParseExemption(value)
		if err != nil {
			continue
		}
		host := exemption.Host
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		entries = append(entries, host)
	}
	return strings.Join(entries, ",")
}
