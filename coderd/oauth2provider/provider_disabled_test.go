package oauth2provider_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// gatedRoutes lists every route that CODER_OAUTH2_PROVIDER_ENABLE controls.
var gatedRoutes = []struct {
	method string
	path   string
}{
	{http.MethodGet, "/.well-known/oauth-authorization-server"},
	{http.MethodGet, "/.well-known/oauth-protected-resource"},
	{http.MethodGet, "/oauth2/authorize"},
	{http.MethodPost, "/oauth2/tokens"},
	{http.MethodPost, "/oauth2/revoke"},
	{http.MethodPost, "/oauth2/register"},
	{http.MethodGet, "/api/v2/oauth2-provider/apps"},
	{http.MethodPost, "/api/experimental/mcp/http"},
}

func newProviderClient(t *testing.T, enabled bool, experiments ...codersdk.Experiment) *codersdk.Client {
	t.Helper()
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.OAuth2.Provider.Enable = serpent.Bool(enabled)
			for _, exp := range experiments {
				dv.Experiments = append(dv.Experiments, string(exp))
			}
		}),
	})
	_ = coderdtest.CreateFirstUser(t, client)
	return client
}

func TestOAuth2ProviderDisabled(t *testing.T) {
	t.Parallel()

	t.Run("GatedRoutesReturn404", func(t *testing.T) {
		t.Parallel()
		client := newProviderClient(t, false, codersdk.ExperimentMCPServerHTTP)
		ctx := testutil.Context(t, testutil.WaitLong)

		for _, route := range gatedRoutes {
			res, err := client.Request(ctx, route.method, route.path, nil)
			require.NoError(t, err)
			body, err := io.ReadAll(res.Body)
			_ = res.Body.Close()
			require.NoError(t, err)
			require.Equal(t, http.StatusNotFound, res.StatusCode, "%s %s", route.method, route.path)
			// Same body as an unregistered path, so a disabled provider cannot
			// be told apart from a missing one.
			require.JSONEq(t, `{"message":"Route not found."}`, string(body), "%s %s", route.method, route.path)
		}
	})

	t.Run("GatedRoutesReachableWhenEnabled", func(t *testing.T) {
		t.Parallel()
		client := newProviderClient(t, true, codersdk.ExperimentMCPServerHTTP)
		ctx := testutil.Context(t, testutil.WaitLong)

		for _, route := range gatedRoutes {
			res, err := client.Request(ctx, route.method, route.path, nil)
			require.NoError(t, err)
			_ = res.Body.Close()
			require.NotEqual(t, http.StatusNotFound, res.StatusCode, "%s %s", route.method, route.path)
		}
	})

	t.Run("SettingsReachableWhenDisabled", func(t *testing.T) {
		t.Parallel()
		client := newProviderClient(t, false)
		ctx := testutil.Context(t, testutil.WaitLong)

		settings, err := client.OAuth2ProviderSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings.DynamicClientRegistrationEnabled)
		require.False(t, *settings.DynamicClientRegistrationEnabled)

		settings, err = client.PutOAuth2ProviderSettings(ctx, codersdk.OAuth2ProviderSettings{
			DynamicClientRegistrationEnabled: new(true),
		})
		require.NoError(t, err)
		require.NotNil(t, settings.DynamicClientRegistrationEnabled)
		require.True(t, *settings.DynamicClientRegistrationEnabled)
	})

	t.Run("MCPExperimentStillRequired", func(t *testing.T) {
		t.Parallel()
		client := newProviderClient(t, true)
		ctx := testutil.Context(t, testutil.WaitLong)

		res, err := client.Request(ctx, http.MethodPost, "/api/experimental/mcp/http", nil)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, res.StatusCode)
		require.Contains(t, string(body), string(codersdk.ExperimentMCPServerHTTP))
	})
}
