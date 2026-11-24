package wsproxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisionersdk/proto"
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
	dv.HTTPCookies.EnableHostPrefix = true
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
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].DisplayApps = &proto.DisplayApps{PortForwardingHelper: true}
		return agents
	}).Do()

	// The test dashboard's host is an IP address, which no proxy shares a
	// site with, so sign-in refuses the region. The proxy's half of a
	// sign-in gets a token minted as sign-in mints it.
	_, err = codersdk.NewExperimentalClient(client).PreviewSignIn(ctx, codersdk.PreviewSignInRequest{
		WorkspaceID: ws.Workspace.ID,
		AgentName:   "dev",
		PreviewSlug: "preview--8080",
		RegionID:    region.ID,
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

	host := "preview--8080--dev--site--" + owner.Username + ".sydney.test.coder.com:" + proxy.ServerURL.Port()
	payload := workspaceapps.PreviewSignInPayload{Host: host, APIKey: client.SessionToken()}
	payload.Fill(time.Now())
	token, err := jwtutils.Encrypt(ctx, api.AppEncryptionKeyCache, payload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.ServerURL.String()+"/?"+previewhost.SignInParam+"="+url.QueryEscape(token), nil)
	require.NoError(t, err)
	req.Host = host
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

// Through a workspace proxy, coderd's Issue decides who opens a preview: only
// the workspace owner, and only with the experiment and --host-prefix-cookie
// on. Every response the proxy sends carries the preview policy, including
// coderd's signed-out page, whose script asks the dashboard to sign in.
func TestWorkspaceProxyPreview(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Add("Set-Cookie", "host=1; Path=/")
		rw.Header().Add("Set-Cookie", "wide=1; Path=/; Domain=test.coder.com")
		_, _ = rw.Write([]byte("preview ok"))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	type request struct {
		name       string
		asOther    bool
		signedOut  bool
		wantStatus int
	}
	for _, tc := range []struct {
		name       string
		experiment bool
		hostPrefix bool
		requests   []request
	}{
		{
			name:       "Enabled",
			experiment: true,
			hostPrefix: true,
			requests: []request{
				{name: "Owner", wantStatus: http.StatusOK},
				{name: "NotOwner", asOther: true, wantStatus: http.StatusNotFound},
				{name: "SignedOut", signedOut: true, wantStatus: http.StatusUnauthorized},
			},
		},
		{name: "WithoutExperiment", hostPrefix: true, requests: []request{{name: "Owner", wantStatus: http.StatusNotFound}}},
		{name: "WithoutHostPrefixCookie", experiment: true, requests: []request{{name: "Owner", wantStatus: http.StatusNotFound}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			dv := coderdtest.DeploymentValues(t)
			if tc.experiment {
				dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
			}
			dv.HTTPCookies.EnableHostPrefix = tc.hostPrefix
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
			owner, err := client.User(ctx, codersdk.Me)
			require.NoError(t, err)
			otherClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner())
			ws := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
				Name:           "site",
				OwnerID:        owner.ID,
				OrganizationID: first.OrganizationID,
			}).WithAgent().Do()
			_ = agenttest.New(t, client.URL, ws.AgentToken)
			coderdtest.NewWorkspaceAgentWaiter(t, client, ws.Workspace.ID).Wait()
			host := fmt.Sprintf("preview--%s--dev--site--%s.sydney.test.coder.com:%s", upstreamURL.Port(), owner.Username, proxy.ServerURL.Port())

			for _, rc := range tc.requests {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.ServerURL.String()+"/", nil)
				require.NoError(t, err)
				req.Host = host
				req.Header.Set("Sec-Fetch-Dest", "iframe")
				session := client.SessionToken()
				if rc.asOther {
					session = otherClient.SessionToken()
				}
				if !rc.signedOut {
					req.AddCookie(&http.Cookie{Name: codersdk.PreviewSessionTokenCookie, Value: session})
				}
				res, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				body, err := io.ReadAll(res.Body)
				_ = res.Body.Close()
				require.NoError(t, err)

				require.Equal(t, rc.wantStatus, res.StatusCode, "%s: %s", rc.name, body)
				require.Len(t, res.Header.Values("Content-Security-Policy"), 1, rc.name)
				switch rc.wantStatus {
				case http.StatusOK:
					require.Equal(t, "preview ok", string(body))
					// The proxy's own signed token cookie is host-only too.
					require.Contains(t, res.Header.Values("Set-Cookie"), "host=1; Path=/")
					for _, line := range res.Header.Values("Set-Cookie") {
						require.NotContains(t, strings.ToLower(line), "domain=", line)
					}
				case http.StatusUnauthorized:
					dashboardOrigin := strings.TrimSuffix(api.AccessURL.String(), "/")
					require.Contains(t, string(body), `window.top.postMessage("coder-preview-sign-in-needed", "`+dashboardOrigin+`")`)
				}
			}
		})
	}
}
