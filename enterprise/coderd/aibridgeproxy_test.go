package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestAIBridgeProxy(t *testing.T) {
	t.Parallel()

	t.Run("DisabledReturns404", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		// Proxy is disabled by default, so we don't need to set it explicitly.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)

		// Make a request to the proxy CA cert endpoint.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+"/api/v2/aibridge/proxy/ca-cert.pem", nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				// No aibridge feature.
				Features: license.Features{},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)

		// Make a request to the proxy CA cert endpoint.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+"/api/v2/aibridge/proxy/ca-cert.pem", nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}
