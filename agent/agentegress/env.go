package agentegress

import (
	"net"
	"strings"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// ProxyEnv returns the environment variables that point workspace processes
// at the local egress proxy. Both upper and lower case spellings are set
// because tooling is inconsistent about which it honors. Control plane
// hosts are excluded so the workspace keeps talking to coderd directly.
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
	for _, hostport := range controlPlaneHosts {
		host := stripPort(hostport)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		entries = append(entries, host)
	}
	return strings.Join(entries, ",")
}

// stripPort removes a trailing :port from host[:port], tolerating bracketed
// and bare IPv6 literals.
func stripPort(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return strings.Trim(hostport, "[]")
}
