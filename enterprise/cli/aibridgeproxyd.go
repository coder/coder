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
		ListenAddr:      coderAPI.DeploymentValues.AI.BridgeProxyConfig.ListenAddr.String(),
		CoderAccessURL:  coderAPI.AccessURL.String(),
		CertFile:        coderAPI.DeploymentValues.AI.BridgeProxyConfig.CertFile.String(),
		KeyFile:         coderAPI.DeploymentValues.AI.BridgeProxyConfig.KeyFile.String(),
		DomainAllowlist: coderAPI.DeploymentValues.AI.BridgeProxyConfig.DomainAllowlist.Value(),
		UpstreamProxy:   coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxy.String(),
		UpstreamProxyCA: coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxyCA.String(),
		Metrics:         metrics,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to start in-memory aibridgeproxy daemon: %w", err)
	}

	return srv, nil
}
