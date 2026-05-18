package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGetAppHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		accessURL   string
		appHostname string
		expected    string
	}{
		{
			name:        "OK",
			accessURL:   "https://test.coder.com",
			appHostname: "*.test.coder.com",
			expected:    "*.test.coder.com",
		},
		{
			name:        "None",
			accessURL:   "https://test.coder.com",
			appHostname: "",
			expected:    "",
		},
		{
			name:        "OKWithPort",
			accessURL:   "https://test.coder.com:8443",
			appHostname: "*.test.coder.com",
			expected:    "*.test.coder.com:8443",
		},
		{
			name:        "OKWithSuffix",
			accessURL:   "https://test.coder.com:8443",
			appHostname: "*--suffix.test.coder.com",
			expected:    "*--suffix.test.coder.com:8443",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			accessURL, err := url.Parse(c.accessURL)
			require.NoError(t, err)

			client := coderdtest.New(t, &coderdtest.Options{
				AccessURL:   accessURL,
				AppHostname: c.appHostname,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Should not leak to unauthenticated users.
			host, err := client.AppHost(ctx)
			require.Error(t, err)
			require.Equal(t, "", host.Host)

			_ = coderdtest.CreateFirstUser(t, client)
			host, err = client.AppHost(ctx)
			require.NoError(t, err)
			require.Equal(t, c.expected, host.Host)
		})
	}
}

func TestWorkspaceApplicationAuth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		accessURL        string
		appHostname      string
		proxyURL         string
		proxyAppHostname string

		redirectURI    string
		expectRedirect string
	}{
		{
			name:             "OK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://something.test.coder.com",
			expectRedirect:   "https://something.test.coder.com",
		},
		{
			name:             "ProxyPathOK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://proxy.test.coder.com/path",
			expectRedirect:   "https://proxy.test.coder.com/path",
		},
		{
			name:             "ProxySubdomainOK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://something.proxy.test.coder.com/path?yeah=true",
			expectRedirect:   "https://something.proxy.test.coder.com/path?yeah=true",
		},
		{
			name:             "ProxySubdomainSuffixOK",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*--suffix.proxy.test.coder.com",
			redirectURI:      "https://something--suffix.proxy.test.coder.com/",
			expectRedirect:   "https://something--suffix.proxy.test.coder.com/",
		},
		{
			name:             "NormalizeSchemePrimaryAppHostname",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "http://x.test.coder.com",
			expectRedirect:   "https://x.test.coder.com",
		},
		{
			name:             "NormalizeSchemeProxyAppHostname",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "http://x.proxy.test.coder.com",
			expectRedirect:   "https://x.proxy.test.coder.com",
		},
		{
			name:             "NoneError",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "",
			expectRedirect:   "",
		},
		{
			name:             "PrimaryAccessURLError",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://test.coder.com/",
			expectRedirect:   "",
		},
		{
			name:             "OtherError",
			accessURL:        "https://test.coder.com",
			appHostname:      "*.test.coder.com",
			proxyURL:         "https://proxy.test.coder.com",
			proxyAppHostname: "*.proxy.test.coder.com",
			redirectURI:      "https://example.com/",
			expectRedirect:   "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			logger := testutil.Logger(t)
			accessURL, err := url.Parse(c.accessURL)
			require.NoError(t, err)

			db, ps := dbtestutil.NewDB(t)
			fetcher := &cryptokeys.DBFetcher{
				DB: db,
			}

			kc, err := cryptokeys.NewEncryptionCache(ctx, logger, fetcher, codersdk.CryptoKeyFeatureWorkspaceAppsAPIKey)
			require.NoError(t, err)

			clock := quartz.NewMock(t)

			client := coderdtest.New(t, &coderdtest.Options{
				AccessURL:             accessURL,
				AppHostname:           c.appHostname,
				Database:              db,
				Pubsub:                ps,
				APIKeyEncryptionCache: kc,
				Clock:                 clock,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			// Disable redirects.
			client.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}

			_, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{
				Url:              c.proxyURL,
				WildcardHostname: c.proxyAppHostname,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			resp, err := client.Request(ctx, http.MethodGet, "/api/v2/applications/auth-redirect", nil, func(req *http.Request) {
				q := req.URL.Query()
				q.Set("redirect_uri", c.redirectURI)
				req.URL.RawQuery = q.Encode()
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusSeeOther {
				err = codersdk.ReadBodyAsError(resp)
				if c.expectRedirect == "" {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				return
			}
			if c.expectRedirect == "" {
				t.Fatal("expected a failure but got a success")
			}

			loc, err := resp.Location()
			require.NoError(t, err)
			q := loc.Query()

			// Verify the API key is set.
			encryptedAPIKey := loc.Query().Get(workspaceapps.SubdomainProxyAPIKeyParam)
			require.NotEmpty(t, encryptedAPIKey, "no API key was set in the query parameters")

			// Strip the API key from the actual redirect URI and compare.
			q.Del(workspaceapps.SubdomainProxyAPIKeyParam)
			loc.RawQuery = q.Encode()
			require.Equal(t, c.expectRedirect, loc.String())

			var token workspaceapps.EncryptedAPIKeyPayload
			err = jwtutils.Decrypt(ctx, kc, encryptedAPIKey, &token, jwtutils.WithDecryptExpected(jwt.Expected{
				Time:        clock.Now(),
				AnyAudience: jwt.Audience{"wsproxy"},
				Issuer:      "coderd",
			}))
			require.NoError(t, err)
			require.Equal(t, jwt.NewNumericDate(clock.Now().Add(time.Minute)), token.Expiry)
			require.Equal(t, jwt.NewNumericDate(clock.Now().Add(-time.Minute)), token.NotBefore)
		})
	}
}

// TestWorkspaceAppsProxy_DLPDenied exercises the DLP enforcement gate in
// workspaceapps.Server.proxyWorkspaceApp: a coder_app slug that is not in
// the policy's AllowedApplications list must be blocked with the
// "Blocked by workspace policy" static error page. The sibling
// PortForwardingAccess gate runs in the same code path with isPort=true,
// which the request resolver only sets for subdomain access; subdomain
// coverage is deferred to stage 2 (needs wildcard-host setup).
func TestWorkspaceAppsProxy_DLPDenied(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitShort)

	agentToken, err := uuid.Parse(r.AgentToken)
	require.NoError(t, err)
	ao, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agentToken)
	require.NoError(t, err)
	build, err := db.GetWorkspaceBuildByID(dbauthz.AsSystemRestricted(ctx), ao.WorkspaceBuild.ID)
	require.NoError(t, err)
	workspace, err := db.GetWorkspaceByID(dbauthz.AsSystemRestricted(ctx), build.WorkspaceID)
	require.NoError(t, err)
	owner, err := db.GetUserByID(dbauthz.AsSystemRestricted(ctx), workspace.OwnerID)
	require.NoError(t, err)

	// Attach an app the policy will not allow.
	const blockedSlug = "blocked-app"
	dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
		AgentID:      ao.WorkspaceAgent.ID,
		Slug:         blockedSlug,
		DisplayName:  "Blocked",
		SharingLevel: database.AppSharingLevelOwner,
		Url:          sql.NullString{String: "http://127.0.0.1:8080", Valid: true},
	})

	policy, err := db.InsertTemplateVersionDLPPolicy(dbauthz.AsProvisionerd(ctx), database.InsertTemplateVersionDLPPolicyParams{
		ID:                   uuid.New(),
		TemplateVersionID:    build.TemplateVersionID,
		Name:                 "strict",
		SshAccess:            true,
		WebTerminalAccess:    true,
		PortForwardingAccess: true,
		AllowedApplications:  []string{"some-other-app"},
		CreatedAt:            dbtime.Now(),
	})
	require.NoError(t, err)

	dbgen.SetWorkspaceAgentDLPPolicy(t, db, ao.WorkspaceAgent.ID, policy.ID)

	// The path-app handler rejects offline agents before the DLP gate. Mark
	// the agent connected so the request reaches the gate.
	require.NoError(t, db.UpdateWorkspaceAgentConnectionByID(
		dbauthz.AsSystemRestricted(ctx),
		database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:                     ao.WorkspaceAgent.ID,
			FirstConnectedAt:       sql.NullTime{Time: dbtime.Now(), Valid: true},
			LastConnectedAt:        sql.NullTime{Time: dbtime.Now(), Valid: true},
			DisconnectedAt:         sql.NullTime{},
			LastConnectedReplicaID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			UpdatedAt:              dbtime.Now(),
		},
	))

	// Request the blocked app via the path-app endpoint. The DLP gate
	// returns the static "Blocked by workspace policy" HTML page.
	appPath := fmt.Sprintf("/@%s/%s.%s/apps/%s/", owner.Username, workspace.Name, ao.WorkspaceAgent.Name, blockedSlug)
	//nolint: bodyclose // closed below
	resp, err := client.Request(ctx, http.MethodGet, appPath, nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "Blocked by workspace policy")
	require.Contains(t, string(body), blockedSlug)
	require.Contains(t, string(body), "is not permitted by the workspace policy")
	require.Contains(t, string(body), "is not permitted by the workspace policy")
}
