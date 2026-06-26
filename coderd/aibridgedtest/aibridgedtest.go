//go:build !slim

// Package aibridgedtest provides helpers for starting an in-process
// aibridged daemon in tests.
package aibridgedtest

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// StartTestAIBridgeDaemon wires an in-process aibridged daemon onto the
// supplied API, mirroring what cli/server.go does in production. Tests that
// create AI provider rows with BaseURL pointing at fake upstream HTTP servers
// (e.g. chattest.NewOpenAI) will have their requests proxied through the real
// aibridged stack as they would in production.
//
// metrics is the registry the daemon reports provider reload events to.
// The caller owns the metrics instance and can assert on it after the daemon
// runs. Use [aibridged.NewMetrics] to create one, or nil for a throwaway.
func StartTestAIBridgeDaemon(
	ctx context.Context,
	t testing.TB,
	api *coderd.API,
	metrics *aibridged.Metrics,
) {
	t.Helper()

	logger := api.Logger.Named("aibridged").Leveled(slog.LevelDebug)
	cfg := api.DeploymentValues.AI.BridgeConfig
	tracer := otel.Tracer("aibridge-test")

	providers, _, err := cli.BuildProviders(ctx, api.Database, cfg, logger, nil)
	if err != nil {
		t.Fatalf("build providers: %v", err)
	}

	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool"), nil, tracer)
	if err != nil {
		t.Fatalf("create bridge pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	if metrics == nil {
		metrics = aibridged.NewMetrics(prometheus.NewRegistry())
	}
	reloader := &testPoolReloader{pool: pool, db: api.Database, cfg: cfg, logger: logger.Named("reloader"), metrics: metrics}
	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, api.Pubsub, reloader, logger.Named("subscriber"))
	if err != nil {
		t.Fatalf("subscribe provider reload: %v", err)
	}
	t.Cleanup(unsubscribe)

	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return api.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	if err != nil {
		t.Fatalf("create aibridged server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	api.RegisterInMemoryAIBridgedHTTPHandler(srv)
}

type testPoolReloader struct {
	pool    *aibridged.CachedBridgePool
	db      database.Store
	cfg     codersdk.AIBridgeConfig
	logger  slog.Logger
	metrics *aibridged.Metrics
}

func (r *testPoolReloader) Reload(ctx context.Context) error {
	// Stamp the attempt before building providers so the gap between
	// attempt and success timestamps reveals a mid-reload hang.
	r.metrics.RecordReloadAttempt()
	providers, outcomes, err := cli.BuildProviders(ctx, r.db, r.cfg, r.logger, nil)
	if err != nil {
		return err
	}
	r.pool.ReplaceProviders(providers)
	r.metrics.RecordReloadSuccess(outcomes)
	return nil
}
