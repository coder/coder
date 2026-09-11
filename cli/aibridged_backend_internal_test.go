//go:build !slim

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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
			srv, unsubscribe, err := newAIBridgeDaemon(api, dv.AI.BridgeConfig, prometheus.NewRegistry(), nil)
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
