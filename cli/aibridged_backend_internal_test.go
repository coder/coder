//go:build !slim

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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
			dbgen.AIProviderWithOptionalKey(t, api.Database, database.AIProvider{
				Type:    database.AIProviderTypeOpenai,
				Name:    "openai",
				Enabled: true,
				BaseUrl: "http://upstream.test",
			}, "key")
			srv, unsubscribe, err := newAIBridgeDaemon(api, dv.AI.BridgeConfig, prometheus.NewRegistry(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, srv.Close()) })
			t.Cleanup(unsubscribe)
			handler, err := srv.GetRequestHandler(testutil.Context(t, testutil.WaitLong), aibridged.Request{InitiatorID: firstUser.UserID, SessionKey: client.SessionToken()})
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{`)))
			if tc.proxy {
				// The placeholder proxy has no enabled-provider routes; the
				// not-ready handler would answer 503.
				require.Equal(t, http.StatusNotFound, rec.Code)
				return
			}
			require.Equal(t, http.StatusInternalServerError, rec.Code,
				"interception must handle the known route and reject malformed JSON")
		})
	}
}
