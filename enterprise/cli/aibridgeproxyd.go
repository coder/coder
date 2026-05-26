//go:build !slim

package cli

import (
	"context"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/enterprise/coderd"
)

// newAIBridgeProxyDaemon starts the enterprise aibridge proxy daemon
// and subscribes to ai_providers changes so the proxy's routing
// snapshot tracks the database. The returned unsubscribe function
// must be invoked alongside Server.Close on shutdown.
func newAIBridgeProxyDaemon(coderAPI *coderd.API) (*aibridgeproxyd.Server, func(), error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridgeproxy daemon")

	logger := coderAPI.Logger.Named("aibridgeproxyd")

	reg := prometheus.WrapRegistererWithPrefix("coder_aibridgeproxyd_", coderAPI.PrometheusRegistry)
	metrics := aibridgeproxyd.NewMetrics(reg)

	var newDumper func(provider, requestID string) aibridgeproxyd.RoundTripDumper
	if dumpDir := coderAPI.DeploymentValues.AI.BridgeProxyConfig.APIDumpDir.String(); dumpDir != "" {
		newDumper = func(provider, requestID string) aibridgeproxyd.RoundTripDumper {
			return apidump.NewDumper(filepath.Join(dumpDir, provider, requestID), logger)
		}
	}

	srv, err := aibridgeproxyd.New(ctx, logger, aibridgeproxyd.Options{
		ListenAddr:          coderAPI.DeploymentValues.AI.BridgeProxyConfig.ListenAddr.String(),
		TLSCertFile:         coderAPI.DeploymentValues.AI.BridgeProxyConfig.TLSCertFile.String(),
		TLSKeyFile:          coderAPI.DeploymentValues.AI.BridgeProxyConfig.TLSKeyFile.String(),
		CoderAccessURL:      coderAPI.AccessURL.String(),
		MITMCertFile:        coderAPI.DeploymentValues.AI.BridgeProxyConfig.MITMCertFile.String(),
		MITMKeyFile:         coderAPI.DeploymentValues.AI.BridgeProxyConfig.MITMKeyFile.String(),
		UpstreamProxy:       coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxy.String(),
		UpstreamProxyCA:     coderAPI.DeploymentValues.AI.BridgeProxyConfig.UpstreamProxyCA.String(),
		AllowedPrivateCIDRs: coderAPI.DeploymentValues.AI.BridgeProxyConfig.AllowedPrivateCIDRs.Value(),
		NewDumper:           newDumper,
		Metrics:             metrics,
		RefreshProviders:    refreshProxyProviders(coderAPI.Database),
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to start in-memory aibridgeproxy daemon: %w", err)
	}

	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, coderAPI.Pubsub, srv, logger.Named("provider-reload"))
	if err != nil {
		logger.Warn(ctx, "subscribe aibridgeproxyd to ai providers change channel", slog.Error(err))
		unsubscribe = func() {}
	}

	return srv, unsubscribe, nil
}

func refreshProxyProviders(db database.Store) aibridgeproxyd.RefreshProvidersFunc {
	return func(ctx context.Context) ([]aibridgeproxyd.ProviderRoute, error) {
		//nolint:gocritic // AsAIProviderMetadataReader is the correct subject for routing-only access.
		rows, err := db.GetAIProviders(dbauthz.AsAIProviderMetadataReader(ctx), database.GetAIProvidersParams{
			IncludeDisabled: false,
		})
		if err != nil {
			return nil, xerrors.Errorf("load ai providers: %w", err)
		}
		out := make([]aibridgeproxyd.ProviderRoute, 0, len(rows))
		for _, row := range rows {
			out = append(out, aibridgeproxyd.ProviderRoute{Name: row.Name, BaseURL: row.BaseUrl})
		}
		return out, nil
	}
}
