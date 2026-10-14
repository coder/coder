//go:build !slim

// Package aibridgedtest provides helpers for starting an in-process
// aibridged daemon in tests.
package aibridgedtest

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// StartTestAIBridgeDaemon wires an in-process aibridged daemon onto the
// supplied API through [cli.NewAIBridgeDaemon], the same constructor
// cli/server.go uses. Tests that create AI provider rows with BaseURL pointing
// at fake upstream HTTP servers (e.g. chattest.NewOpenAI) will have their
// requests proxied through the real aibridged stack as they would in
// production, under the API's own deployment configuration.
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
	StartTestAIBridgeDaemonWithPubsub(ctx, t, api, metrics, api.Pubsub)
}

// StartTestAIBridgeDaemonWithPubsub is StartTestAIBridgeDaemon with an
// explicit pubsub for the provider-reload subscription. A pubsub that is
// disconnected from api.Pubsub cuts the daemon off from provider change
// events, leaving the initial synchronous load as its only route source.
func StartTestAIBridgeDaemonWithPubsub(
	ctx context.Context,
	t testing.TB,
	api *coderd.API,
	metrics *aibridged.Metrics,
	ps pubsub.Pubsub,
) {
	t.Helper()

	srv, unsubscribe, err := cli.NewAIBridgeDaemon(ctx, cli.AIBridgeDaemonOptions{
		API:             api,
		Config:          api.DeploymentValues.AI.BridgeConfig,
		Registerer:      prometheus.NewRegistry(),
		ProviderMetrics: metrics,
		Pubsub:          ps,
	})
	if err != nil {
		t.Fatalf("create aibridged server: %v", err)
	}
	t.Cleanup(unsubscribe)
	t.Cleanup(func() { _ = srv.Close() })

	api.RegisterInMemoryAIBridgedHTTPHandler(srv)
}
