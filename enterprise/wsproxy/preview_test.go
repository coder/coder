package wsproxy_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// A workspace proxy decrypts a sign-in that coderd minted for one of its
// preview hosts with the keys it already fetches, and sets the session
// cookie on that host only.
func TestWorkspaceProxyPreviewSignIn(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
	client, closer, api, first := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
			AppHostname:      "*.primary.test.coder.com",
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureWorkspaceProxy: 1},
		},
	})
	t.Cleanup(func() { _ = closer.Close() })
	proxy := coderdenttest.NewWorkspaceProxyReplica(t, api, client, &coderdenttest.ProxyOptions{
		Name:        "sydney",
		AppHostname: "*.sydney.test.coder.com",
	})
	region, err := client.WorkspaceProxyByName(ctx, "sydney")
	require.NoError(t, err)
	owner, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	ws := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		Name:           "site",
		OwnerID:        owner.ID,
		OrganizationID: first.OrganizationID,
	}).WithAgent().Do()

	res, err := codersdk.NewExperimentalClient(client).PreviewSignIn(ctx, codersdk.PreviewSignInRequest{
		WorkspaceID: ws.Workspace.ID,
		AgentName:   "dev",
		PreviewSlug: "preview--8080",
		RegionID:    region.ID,
	})
	require.NoError(t, err)
	previewURL, err := url.Parse(res.URL)
	require.NoError(t, err)
	require.Equal(t, "preview--8080--dev--site--"+owner.Username+".sydney.test.coder.com:"+proxy.ServerURL.Port(), previewURL.Host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.ServerURL.String()+previewURL.RequestURI(), nil)
	require.NoError(t, err)
	req.Host = previewURL.Host
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	signedIn, err := noRedirect.Do(req)
	require.NoError(t, err)
	_ = signedIn.Body.Close()

	require.Equal(t, http.StatusSeeOther, signedIn.StatusCode)
	require.Equal(t, "/", signedIn.Header.Get("Location"))
	require.Contains(t, signedIn.Header.Get("Content-Security-Policy"), "frame-ancestors 'self' "+api.AccessURL.Scheme+"://"+api.AccessURL.Host)
	cookies := signedIn.Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, codersdk.PreviewSessionTokenCookie, cookies[0].Name)
	require.Empty(t, cookies[0].Domain)
	require.NotEmpty(t, cookies[0].Value)
}
