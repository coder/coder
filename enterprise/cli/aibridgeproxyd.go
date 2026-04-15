//go:build !slim

package cli

import (
	"context"
	"net/url"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIBridgeProxyDaemon(coderAPI *coderd.API, providers []aibridge.Provider) (*aibridgeproxyd.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridgeproxy daemon")

	logger := coderAPI.Logger.Named("aibridgeproxyd")

	domains, providerFromHost := domainsFromProviders(providers)

	reg := prometheus.WrapRegistererWithPrefix("coder_aibridgeproxyd_", coderAPI.PrometheusRegistry)
	metrics := aibridgeproxyd.NewMetrics(reg)

	srv, err := aibridgeproxyd.New(ctx, logger, aibridgeproxyd.Options{
		ListenAddr:               coderAPI.DeploymentValues.AI.BridgeProxyConfig.ListenAddr.String(),
		TLSCertFile:              coderAPI.DeploymentValues.AI.BridgeProxyConfig.TLSCertFile.String(),
		TLSKeyFile:               coderAPI.DeploymentValues.AI.BridgeProxyConfig.TLSKeyFile.String(),
		CoderAccessURL:           coderAPI.AccessURL.String(),
		MITMCertFile:             coderAPI.DeploymentValues.AI.BridgeProxyConfig.MITMCertFile.String(),
		MITMKeyFile:              coderAPI.DeploymentValues.AI.BridgeProxyConfig.MITMKeyFile.String(),
		DomainAllowlist:          domains,
		AIBridgeProviderFromHost: providerFromHost,
		UpstreamProxy:            coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxy.String(),
		UpstreamProxyCA:          coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxyCA.String(),
		AllowedPrivateCIDRs:      coderAPI.DeploymentValues.AI.BridgeProxyConfig.AllowedPrivateCIDRs.Value(),
		Metrics:                  metrics,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to start in-memory aibridgeproxy daemon: %w", err)
	}

	return srv, nil
}

// domainsFromProviders extracts distinct hostnames from providers' base
// URLs and builds a host-to-provider-name mapping function. The returned
// domain list is suitable for use as DomainAllowlist and the mapping
// function is suitable for use as AIBridgeProviderFromHost.
func domainsFromProviders(providers []aibridge.Provider) ([]string, func(string) string) {
	hostToProvider := make(map[string]string, len(providers))
	var domains []string
	for _, p := range providers {
		raw := p.BaseURL()
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if _, exists := hostToProvider[host]; exists {
			// First provider wins; duplicates are expected when
			// multiple providers share a base URL host (e.g. two
			// OpenAI providers using the same proxy).
			continue
		}
		hostToProvider[host] = p.Name()
		domains = append(domains, host)
	}

	return domains, func(host string) string {
		return hostToProvider[strings.ToLower(host)]
	}
}
