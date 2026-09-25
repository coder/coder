//go:build !slim

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// TestNewAIBridgeDaemonBackend asserts the embedded daemon reaches its mode
// selection from coderAPI.Experiments. The remaining mode matrix is shared with
// the standalone E2E test and covered by coderd/aibridged.
func TestNewAIBridgeDaemonBackend(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		experiment bool
		proxy      bool
	}{
		{name: "Interception"},
		{name: "Proxy", experiment: true, proxy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dv := coderdtest.DeploymentValues(t)
			if tc.experiment {
				dv.Experiments = append(dv.Experiments, string(codersdk.ExperimentAIGatewayReverseProxy))
			}
			client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: dv})
			firstUser := coderdtest.CreateFirstUser(t, client)
			srv, unsubscribe, err := NewAIBridgeDaemon(t.Context(), AIBridgeDaemonOptions{
				API:        api,
				Config:     dv.AI.BridgeConfig,
				Registerer: prometheus.NewRegistry(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, srv.Close()) })
			t.Cleanup(unsubscribe)
			handler, err := srv.GetRequestHandler(testutil.Context(t, testutil.WaitLong), aibridged.Request{InitiatorID: firstUser.UserID, SessionKey: client.SessionToken()})
			require.NoError(t, err)
			_, isBridge := handler.(*aibridge.RequestBridge)
			require.Equal(t, !tc.proxy, isBridge, "interception serves from a RequestBridge")
			if tc.proxy {
				// A published router answers unregistered routes with 404;
				// the not-ready handler would answer 503.
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil))
				require.Equal(t, http.StatusNotFound, rec.Code)
			}
		})
	}
}

// TestNewAIBridgeDaemonOwnsKeyPoolStateCollector guards the daemon's ownership
// of the key pool collector. Anything else registering one alongside it fails
// MustRegister, which takes coderd's whole startup down.
func TestNewAIBridgeDaemonOwnsKeyPoolStateCollector(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	_, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: dv})

	reg := prometheus.NewRegistry()
	srv, unsubscribe, err := NewAIBridgeDaemon(t.Context(), AIBridgeDaemonOptions{
		API:        api,
		Config:     dv.AI.BridgeConfig,
		Registerer: reg,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })
	t.Cleanup(unsubscribe)

	// The collector reports nothing until providers load, so ownership is
	// asserted by registration rather than by gathering.
	err = reg.Register(keypool.NewStateCollector(func() []*keypool.Pool { return nil }))
	require.Error(t, err, "the daemon should already have registered the key pool state collector")
	require.Contains(t, err.Error(), "duplicate metrics collector registration")
}

// TestNewAIBridgeDaemonAppliesRecordPolicy asserts the deployment's record
// policy reaches the daemon. The policy is only observable on the pool the
// daemon builds for itself, so a daemon constructed from configuration that
// never reaches that pool fails here.
func TestNewAIBridgeDaemonAppliesRecordPolicy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(*codersdk.AIBridgeConfig)
	}{
		{
			name:   "Defaults",
			mutate: func(*codersdk.AIBridgeConfig) {},
		},
		{
			name: "ContentRecordingDisabled",
			mutate: func(cfg *codersdk.AIBridgeConfig) {
				cfg.DisableContentRecording = serpent.Bool(true)
			},
		},
		{
			name: "GatewayEmitsStructuredLogs",
			mutate: func(cfg *codersdk.AIBridgeConfig) {
				cfg.StructuredLogging = serpent.Bool(true)
				cfg.StructuredLoggingSource = string(codersdk.AIStructuredLoggingSourceGateway)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dv := coderdtest.DeploymentValues(t)
			tc.mutate(&dv.AI.BridgeConfig)
			_, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: dv})

			srv, unsubscribe, err := NewAIBridgeDaemon(t.Context(), AIBridgeDaemonOptions{
				API:        api,
				Config:     dv.AI.BridgeConfig,
				Registerer: prometheus.NewRegistry(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, srv.Close()) })
			t.Cleanup(unsubscribe)

			require.Equal(t, aibridged.PoolOptionsFromConfig(dv.AI.BridgeConfig), srv.PoolOptions())
		})
	}
}
