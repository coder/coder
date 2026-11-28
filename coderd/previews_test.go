package coderd_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestPreviewSignIn(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
	dv.HTTPCookies.EnableHostPrefix = true
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		AccessURL:        &url.URL{Scheme: "http", Host: "dashboard.test.coder.com"},
		AppHostname:      "*.apps.test.coder.com",
		DeploymentValues: dv,
	})
	first := coderdtest.CreateFirstUser(t, client)
	owner, err := client.User(testutil.Context(t, testutil.WaitShort), codersdk.Me)
	require.NoError(t, err)
	siteAdmin, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner())
	ws := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		Name:           "site",
		OwnerID:        owner.ID,
		OrganizationID: first.OrganizationID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].DisplayApps = &proto.DisplayApps{PortForwardingHelper: true}
		return agents
	}).Do()
	_ = agenttest.New(t, client.URL, ws.AgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, ws.Workspace.ID).Wait()

	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte("preview ok"))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	signIn := func(c *codersdk.Client, req codersdk.PreviewSignInRequest) (codersdk.PreviewSignInResponse, error) {
		return codersdk.NewExperimentalClient(c).PreviewSignIn(testutil.Context(t, testutil.WaitShort), req)
	}
	valid := codersdk.PreviewSignInRequest{
		WorkspaceID: ws.Workspace.ID,
		AgentName:   "dev",
		PreviewSlug: "preview--" + upstreamURL.Port(),
		URLPath:     "/docs/?page=2",
	}

	t.Run("SignsInOnlyThePreviewHost", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		res, err := signIn(client, valid)
		require.NoError(t, err)
		host := valid.PreviewSlug + "--dev--" + ws.Workspace.Name + "--" + owner.Username + ".apps.test.coder.com"
		require.Equal(t, "http://"+host, res.Origin)
		require.Equal(t, "allow-scripts allow-same-origin allow-forms allow-modals allow-downloads", res.Sandbox)
		previewURL, err := url.Parse(res.URL)
		require.NoError(t, err)
		require.Equal(t, host, previewURL.Host)
		require.Equal(t, "/docs/", previewURL.Path)
		require.True(t, strings.HasPrefix(previewURL.RawQuery, "page=2&coder_preview_sign_in="), previewURL.RawQuery)

		// The preview host trades the token for a cookie on itself only.
		get := func(requestURI string, cookies ...*http.Cookie) (int, http.Header, string) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+requestURI, nil)
			require.NoError(t, err)
			req.Host = host
			req.Header.Set("Sec-Fetch-Dest", "iframe")
			for _, cookie := range cookies {
				req.AddCookie(cookie)
			}
			noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			res, err := noRedirect.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			return res.StatusCode, res.Header, string(body)
		}
		status, header, _ := get(previewURL.RequestURI())
		require.Equal(t, http.StatusSeeOther, status)
		require.Equal(t, "/docs/?page=2", header.Get("Location"))
		cookies := (&http.Response{Header: header}).Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, codersdk.PreviewSessionTokenCookie, cookies[0].Name)
		require.Empty(t, cookies[0].Domain)
		require.True(t, cookies[0].HttpOnly)

		// The cookie holds an application_connect key of the owner who
		// signed in.
		keyID, _, _ := strings.Cut(cookies[0].Value, "-")
		//nolint:gocritic // The test reads the key row the endpoint created.
		key, err := api.Database.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
		require.NoError(t, err)
		require.Equal(t, owner.ID, key.UserID)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect}, key.Scopes)

		// The cookie opens the preview through the real server, whose
		// connection logging reads the status under the preview policy.
		status, header, body := get(header.Get("Location"), cookies[0])
		require.Equal(t, http.StatusOK, status, body)
		require.Equal(t, "preview ok", body)
		require.Contains(t, header.Get("Content-Security-Policy"), "frame-ancestors 'self' "+strings.TrimSuffix(api.AccessURL.String(), "/"))
	})

	// The dashboard names the primary region by the deployment ID.
	t.Run("DeploymentRegion", func(t *testing.T) {
		t.Parallel()

		req := valid
		req.RegionID = uuid.MustParse(api.DeploymentID)
		res, err := signIn(client, req)
		require.NoError(t, err)
		require.Equal(t, "http://"+valid.PreviewSlug+"--dev--"+ws.Workspace.Name+"--"+owner.Username+".apps.test.coder.com", res.Origin)
	})

	t.Run("Refused", func(t *testing.T) {
		t.Parallel()

		// A workspace name may be 32 characters, which puts its preview
		// hostname label over the DNS limit.
		longName := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
			Name:           strings.Repeat("w", 32),
			OwnerID:        owner.ID,
			OrganizationID: first.OrganizationID,
		}).WithAgent().Do()
		for _, tc := range []struct {
			name       string
			client     *codersdk.Client
			mutate     func(*codersdk.PreviewSignInRequest)
			wantStatus int
		}{
			// A site owner can connect to the workspace's apps, but a preview
			// opens only for the workspace owner.
			{name: "NotWorkspaceOwner", client: siteAdmin, wantStatus: http.StatusNotFound},
			{name: "UnknownWorkspace", mutate: func(r *codersdk.PreviewSignInRequest) { r.WorkspaceID = uuid.New() }, wantStatus: http.StatusNotFound},
			{name: "AppSlug", mutate: func(r *codersdk.PreviewSignInRequest) { r.PreviewSlug = "code-server" }, wantStatus: http.StatusBadRequest},
			{name: "UnknownAgent", mutate: func(r *codersdk.PreviewSignInRequest) { r.AgentName = "other" }, wantStatus: http.StatusBadRequest},
			{
				name: "UnknownAgentForFile",
				mutate: func(r *codersdk.PreviewSignInRequest) {
					r.PreviewSlug = "preview--0123abcd"
					r.AgentName = "other"
				},
				wantStatus: http.StatusBadRequest,
			},
			{name: "UnknownRegion", mutate: func(r *codersdk.PreviewSignInRequest) { r.RegionID = uuid.New() }, wantStatus: http.StatusBadRequest},
			{name: "URLPathBadEscape", mutate: func(r *codersdk.PreviewSignInRequest) { r.URLPath = "/%zz" }, wantStatus: http.StatusBadRequest},
			{name: "URLPathSignInParam", mutate: func(r *codersdk.PreviewSignInRequest) { r.URLPath = "/?coder_preview_sign_in=x" }, wantStatus: http.StatusBadRequest},
			{
				name: "HostnameTooLong",
				mutate: func(r *codersdk.PreviewSignInRequest) {
					r.WorkspaceID = longName.Workspace.ID
					r.PreviewSlug = "preview--0123abcd"
				},
				wantStatus: http.StatusBadRequest,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				req := valid
				if tc.mutate != nil {
					tc.mutate(&req)
				}
				c := client
				if tc.client != nil {
					c = tc.client
				}
				// The address endpoint refuses what sign-in refuses.
				experimental := codersdk.NewExperimentalClient(c)
				for name, call := range map[string]func(context.Context, codersdk.PreviewSignInRequest) (codersdk.PreviewSignInResponse, error){
					"Address": experimental.PreviewAddress,
					"SignIn":  experimental.PreviewSignIn,
				} {
					_, err := call(testutil.Context(t, testutil.WaitShort), req)
					var sdkErr *codersdk.Error
					require.ErrorAs(t, err, &sdkErr, name)
					require.Equal(t, tc.wantStatus, sdkErr.StatusCode(), name)
				}
			})
		}
	})

	// Text after a ":" in a preview host would pass for a port in a
	// host:port split, so a user could mint a sign-in whose host starts
	// with another user's preview host. Short names keep the label under
	// the DNS limit, so only the name checks can refuse it, before any key
	// exists.
	t.Run("AgentNameWithHostSyntax", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		otherClient, other := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			r.Username = "atk"
		})
		otherWS := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
			Name:           "w",
			OwnerID:        other.ID,
			OrganizationID: first.OrganizationID,
		}).WithAgent().Do()
		for _, agentName := range []string{"dev--site--testuser.a.co:1", "dev--site--testuser.a.co"} {
			_, err := signIn(otherClient, codersdk.PreviewSignInRequest{
				WorkspaceID: otherWS.Workspace.ID,
				AgentName:   agentName,
				PreviewSlug: "preview--0123abcd",
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, agentName)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), agentName)
		}

		//nolint:gocritic // The test reads the key rows the endpoint would create.
		keys, err := api.Database.GetAPIKeysByUserID(dbauthz.AsSystemRestricted(ctx), database.GetAPIKeysByUserIDParams{
			LoginType: database.LoginTypePassword,
			UserID:    other.ID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, keys, "the user's own session key")
		for _, key := range keys {
			require.NotContains(t, key.Scopes, database.ApiKeyScopeCoderApplicationConnect)
		}
	})
}

// A region serves previews only with a wildcard hostname on the same site as
// the dashboard and with --host-prefix-cookie. Both endpoints refuse any other
// region without minting a key, and a failure looking a region up is a server
// error. For a region that serves previews, the address endpoint returns the
// URL that sign-in would, without the token, and mints no key.
func TestPreviewRegions(t *testing.T) {
	t.Parallel()

	const apps = "*.apps.test.coder.com"
	const regionRefused = "This region does not serve previews."
	const hostPrefixOff = "Previews require --host-prefix-cookie. An administrator can enable it."
	proxy := func(proxyURL, wildcardHostname string) func(*testing.T, *failingProxyLookupStore) uuid.UUID {
		return func(t *testing.T, db *failingProxyLookupStore) uuid.UUID {
			p, _ := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{Url: proxyURL, WildcardHostname: wildcardHostname})
			return p.ID
		}
	}
	for _, tc := range []struct {
		name          string
		appHostname   string
		hostPrefixOff bool
		region        func(*testing.T, *failingProxyLookupStore) uuid.UUID
		wantStatus    int
		wantMessage   string
		wantDetail    string
		// wantOrigin has "*" where the preview's subdomain goes.
		wantOrigin string
	}{
		{name: "Primary", appHostname: apps, wantStatus: http.StatusOK, wantOrigin: "https://*.apps.test.coder.com:8443"},
		{name: "PrimaryNoWildcard", wantStatus: http.StatusBadRequest, wantMessage: regionRefused, wantDetail: "the deployment has no wildcard app hostname"},
		{name: "PrimaryCrossSite", appHostname: "*.apps.example.net", wantStatus: http.StatusBadRequest, wantMessage: "Previews need a wildcard access URL on the same site as the dashboard."},
		{name: "PrimaryNoHostPrefixCookie", appHostname: apps, hostPrefixOff: true, wantStatus: http.StatusBadRequest, wantMessage: hostPrefixOff},
		{name: "Proxy", appHostname: apps, region: proxy("https://sydney.test.coder.com", "*.sydney.test.coder.com"), wantStatus: http.StatusOK, wantOrigin: "https://*.sydney.test.coder.com"},
		{name: "ProxyNoHostPrefixCookie", appHostname: apps, hostPrefixOff: true, region: proxy("https://sydney.test.coder.com", "*.sydney.test.coder.com"), wantStatus: http.StatusBadRequest, wantMessage: hostPrefixOff},
		{name: "ProxyCrossSite", appHostname: apps, region: proxy("https://sydney.example.net", "*.sydney.example.net"), wantStatus: http.StatusBadRequest, wantMessage: "Previews need a workspace proxy on the same site as the dashboard."},
		{name: "ProxyNoWildcard", appHostname: apps, region: proxy("https://plain.test.coder.com", ""), wantStatus: http.StatusBadRequest, wantMessage: regionRefused, wantDetail: "has no wildcard app hostname"},
		{
			name:        "ProxyDeleted",
			appHostname: apps,
			region: func(t *testing.T, db *failingProxyLookupStore) uuid.UUID {
				id := proxy("https://deleted.test.coder.com", "*.deleted.test.coder.com")(t, db)
				require.NoError(t, db.UpdateWorkspaceProxyDeleted(testutil.Context(t, testutil.WaitShort), database.UpdateWorkspaceProxyDeletedParams{ID: id, Deleted: true}))
				return id
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: regionRefused,
			wantDetail:  "has no wildcard app hostname",
		},
		{name: "UnknownProxy", appHostname: apps, region: func(*testing.T, *failingProxyLookupStore) uuid.UUID { return uuid.New() }, wantStatus: http.StatusBadRequest, wantMessage: regionRefused, wantDetail: "no workspace proxy has ID"},
		{name: "ProxyLookupFails", appHostname: apps, region: func(_ *testing.T, db *failingProxyLookupStore) uuid.UUID { return db.id }, wantStatus: http.StatusInternalServerError, wantMessage: "An internal server error occurred."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, ps := dbtestutil.NewDB(t)
			store := &failingProxyLookupStore{Store: db, id: uuid.New()}
			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
			dv.HTTPCookies.EnableHostPrefix = !tc.hostPrefixOff
			// The dashboard's :8443 port makes the Primary and Proxy rows fail
			// if the same-site check compares ports, and the Primary row expects
			// the port in its origin.
			client := coderdtest.New(t, &coderdtest.Options{
				AccessURL:        &url.URL{Scheme: "https", Host: "dashboard.test.coder.com:8443"},
				AppHostname:      tc.appHostname,
				DeploymentValues: dv,
				Database:         store,
				Pubsub:           ps,
			})
			first := coderdtest.CreateFirstUser(t, client)
			// A member, not an admin, so the proxy rows prove members can read
			// workspace proxies.
			memberClient, member := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
			ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				Name:           "site",
				OwnerID:        member.ID,
				OrganizationID: first.OrganizationID,
			}).WithAgent().Do()
			req := codersdk.PreviewSignInRequest{
				WorkspaceID: ws.Workspace.ID,
				AgentName:   "dev",
				PreviewSlug: "preview--0123abcd",
				URLPath:     "/docs/?page=2",
			}
			if tc.region != nil {
				req.RegionID = tc.region(t, store)
			}

			experimental := codersdk.NewExperimentalClient(memberClient)
			address, addressErr := experimental.PreviewAddress(ctx, req)
			require.Equal(t, 0, appConnectKeys(ctx, t, db, member.ID))
			signIn, signInErr := experimental.PreviewSignIn(ctx, req)
			if tc.wantStatus == http.StatusOK {
				require.NoError(t, addressErr)
				require.NoError(t, signInErr)
				require.Equal(t, 1, appConnectKeys(ctx, t, db, member.ID))
				require.Equal(t, strings.Replace(tc.wantOrigin, "*", "preview--0123abcd--dev--site--"+member.Username, 1), address.Origin)
				require.Equal(t, address.Origin, signIn.Origin)
				require.Equal(t, address.Sandbox, signIn.Sandbox)
				require.Equal(t, address.Origin+"/docs/?page=2", address.URL)
				require.True(t, strings.HasPrefix(signIn.URL, address.URL+"&coder_preview_sign_in="), signIn.URL)
				return
			}
			for name, err := range map[string]error{"Address": addressErr, "SignIn": signInErr} {
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr, name)
				require.Equal(t, tc.wantStatus, sdkErr.StatusCode(), name)
				require.Equal(t, tc.wantMessage, sdkErr.Message, name)
				require.Contains(t, sdkErr.Detail, tc.wantDetail, name)
			}
			require.Equal(t, 0, appConnectKeys(ctx, t, db, member.ID))
		})
	}
}

// failingProxyLookupStore fails the workspace proxy lookup of one ID, as a
// broken database connection would.
type failingProxyLookupStore struct {
	database.Store
	id uuid.UUID
}

func (s *failingProxyLookupStore) GetWorkspaceProxyByID(ctx context.Context, id uuid.UUID) (database.WorkspaceProxy, error) {
	if id == s.id {
		return database.WorkspaceProxy{}, xerrors.New("connection reset by peer")
	}
	return s.Store.GetWorkspaceProxyByID(ctx, id)
}

// Like the app auth redirect, sign-in mints an application_connect key only
// for a caller whose own key could act with it.
func TestPreviewSignInScopedToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
	dv.HTTPCookies.EnableHostPrefix = true
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		AccessURL:        &url.URL{Scheme: "https", Host: "dashboard.test.coder.com"},
		AppHostname:      "*.apps.test.coder.com",
		DeploymentValues: dv,
	})
	first := coderdtest.CreateFirstUser(t, client)
	ws := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		Name:           "site",
		OwnerID:        first.UserID,
		OrganizationID: first.OrganizationID,
	}).WithAgent().Do()

	allow := []codersdk.APIAllowListTarget{
		codersdk.AllowResourceTarget(codersdk.ResourceWorkspace, ws.Workspace.ID),
		codersdk.AllowTypeTarget(codersdk.ResourceApiKey),
	}
	signIn := func(regionID uuid.UUID, scopes ...codersdk.APIKeyScope) error {
		token, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{Scopes: scopes, AllowList: allow})
		require.NoError(t, err)
		scoped := codersdk.New(client.URL, codersdk.WithSessionToken(token.Key))
		_, err = codersdk.NewExperimentalClient(scoped).PreviewSignIn(ctx, codersdk.PreviewSignInRequest{
			WorkspaceID: ws.Workspace.ID,
			AgentName:   "dev",
			PreviewSlug: "preview--0123abcd",
			RegionID:    regionID,
		})
		return err
	}

	err := signIn(uuid.Nil, codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeApiKeyCreate)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	require.Equal(t, 0, appConnectKeys(ctx, t, api.Database, first.UserID))

	require.NoError(t, signIn(uuid.Nil, codersdk.APIKeyScopeCoderWorkspacesAccess, codersdk.APIKeyScopeApiKeyCreate))
	require.Equal(t, 1, appConnectKeys(ctx, t, api.Database, first.UserID))

	// No public low-level scope grants workspace_proxy:read, and the allow list has no proxy entry,
	// so the token signs in to a proxy region only because the lookup needs neither.
	proxy, _ := dbgen.WorkspaceProxy(t, api.Database, database.WorkspaceProxy{Url: "https://sydney.test.coder.com", WildcardHostname: "*.sydney.test.coder.com"})
	require.NoError(t, signIn(proxy.ID, codersdk.APIKeyScopeCoderWorkspacesAccess, codersdk.APIKeyScopeApiKeyCreate))
	require.Equal(t, 2, appConnectKeys(ctx, t, api.Database, first.UserID))
}

// appConnectKeys counts userID's application_connect keys, which only preview
// sign-in mints in these tests.
func appConnectKeys(ctx context.Context, t *testing.T, db database.Store, userID uuid.UUID) int {
	t.Helper()

	count := 0
	// A key minted for a token caller keeps the token's login type.
	for _, loginType := range []database.LoginType{database.LoginTypePassword, database.LoginTypeToken} {
		//nolint:gocritic // The test reads the key rows the endpoint creates.
		keys, err := db.GetAPIKeysByUserID(dbauthz.AsSystemRestricted(ctx), database.GetAPIKeysByUserIDParams{
			LoginType: loginType,
			UserID:    userID,
		})
		require.NoError(t, err)
		for _, key := range keys {
			if key.Scopes.Has(database.ApiKeyScopeCoderApplicationConnect) {
				count++
			}
		}
	}
	return count
}

func TestPreviewSignInWithoutExperiment(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{AppHostname: "*.apps.test.coder.com"})
	first := coderdtest.CreateFirstUser(t, client)
	ws := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		OwnerID:        first.UserID,
		OrganizationID: first.OrganizationID,
	}).WithAgent().Do()

	_, err := codersdk.NewExperimentalClient(client).PreviewSignIn(testutil.Context(t, testutil.WaitShort), codersdk.PreviewSignInRequest{
		WorkspaceID: ws.Workspace.ID,
		AgentName:   "dev",
		PreviewSlug: "preview--0123abcd",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
}
