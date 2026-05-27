//go:build !slim

package cli

import (
	"context"
	"io"
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

// aiBridgeProxyDaemon bundles the proxy server and its pubsub
// subscription so both are torn down by a single Close call.
type aiBridgeProxyDaemon struct {
	server      *aibridgeproxyd.Server
	unsubscribe func()
}

func (d *aiBridgeProxyDaemon) Close() error {
	if d.unsubscribe != nil {
		d.unsubscribe()
	}
	return d.server.Close()
}

// newAIBridgeProxyDaemon starts the enterprise aibridge proxy daemon,
// subscribes to ai_providers changes so the proxy's routing snapshot
// tracks the database, and registers the HTTP handler on the API.
// The returned io.Closer tears down both the subscription and server.
func newAIBridgeProxyDaemon(coderAPI *coderd.API) (io.Closer, error) {
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
		return nil, xerrors.Errorf("failed to start in-memory aibridgeproxy daemon: %w", err)
	}

	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, coderAPI.Pubsub, srv, logger.Named("provider-reload"))
	if err != nil {
		logger.Warn(ctx, "subscribe aibridgeproxyd to ai providers change channel", slog.Error(err))
		unsubscribe = func() {}
	}

	// Register the handler so coderd can serve the proxy endpoints.
	coderAPI.RegisterInMemoryAIBridgeProxydHTTPHandler(srv.Handler())

	return &aiBridgeProxyDaemon{
		server:      srv,
		unsubscribe: unsubscribe,
	}, nil
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
