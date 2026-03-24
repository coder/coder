//go:build !slim

package cli

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIBridgeProxyDaemon(coderAPI *coderd.API) (*aibridgeproxyd.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridgeproxy daemon")

	logger := coderAPI.Logger.Named("aibridgeproxyd")

	reg := prometheus.WrapRegistererWithPrefix("coder_aibridgeproxyd_", coderAPI.PrometheusRegistry)
	metrics := aibridgeproxyd.NewMetrics(reg)

	srv, err := aibridgeproxyd.New(ctx, logger, aibridgeproxyd.Options{
		ListenAddr:          coderAPI.DeploymentValues.AI.BridgeProxyConfig.ListenAddr.String(),
		TLSCertFile:         coderAPI.DeploymentValues.AI.BridgeProxyConfig.TLSCertFile.String(),
		TLSKeyFile:          coderAPI.DeploymentValues.AI.BridgeProxyConfig.TLSKeyFile.String(),
		CoderAccessURL:      coderAPI.AccessURL.String(),
		MITMCertFile:        coderAPI.DeploymentValues.AI.BridgeProxyConfig.MITMCertFile.String(),
		MITMKeyFile:         coderAPI.DeploymentValues.AI.BridgeProxyConfig.MITMKeyFile.String(),
		DomainAllowlist:     coderAPI.DeploymentValues.AI.BridgeProxyConfig.DomainAllowlist.Value(),
		UpstreamProxy:       coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxy.String(),
		UpstreamProxyCA:     coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxyCA.String(),
		AllowedPrivateCIDRs: coderAPI.DeploymentValues.AI.BridgeProxyConfig.AllowedPrivateCIDRs.Value(),
		Metrics:             metrics,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to start in-memory aibridgeproxy daemon: %w", err)
	}

	return srv, nil
}
