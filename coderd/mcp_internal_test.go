package coderd

import (
	"context"
	"encoding/json"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// dbauthzTestStore wraps the test database with the same dbauthz layer
// used in production (coderd.go:370). Without it the test would not
// catch RBAC failures from the chatd subject; with it the test fails
// loudly if the elevation in OIDCAccessToken is removed or weakened.
func dbauthzTestStore(t *testing.T, db database.Store) database.Store {
	t.Helper()

	authz := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var tacs dbauthz.AccessControlStore = fakeAccessControlStore{}
	acs.Store(&tacs)
	return dbauthz.New(db, authz, testutil.Logger(t), acs)
}

// fakeAccessControlStore mirrors coderdtest.FakeAccessControlStore but is
// inlined here to avoid an import cycle (coderdtest imports coderd).
type fakeAccessControlStore struct{}

func (fakeAccessControlStore) GetTemplateAccessControl(t database.Template) dbauthz.TemplateAccessControl {
	return dbauthz.TemplateAccessControl{
		RequireActiveVersion: t.RequireActiveVersion,
	}
}

func (fakeAccessControlStore) SetTemplateAccessControl(context.Context, database.Store, uuid.UUID, dbauthz.TemplateAccessControl) error {
	panic("not implemented")
}

func TestShouldRefreshOIDCToken(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	cases := []struct {
		name string
		link database.UserLink
		want bool
	}{
		{
			name: "NoRefreshToken",
			link: database.UserLink{OAuthExpiry: now.Add(-time.Hour)},
		},
		{
			name: "ZeroExpiry",
			link: database.UserLink{OAuthRefreshToken: "refresh"},
		},
		{
			name: "Expired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-time.Hour),
			},
			want: true,
		},
		{
			name: "Fresh",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(time.Hour),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := shouldRefreshOIDCToken(tc.link)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestOIDCMCPTokenSource(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	t.Run("NilConfig", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		require.Nil(t, newOIDCMCPTokenSource(db, nil, logger))
	})

	t.Run("NoLink", func(t *testing.T) {
		// When the user has no OIDC link the source returns ("", nil)
		// rather than an error so the caller can fall through to
		// "no Authorization header".
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})

		src := newOIDCMCPTokenSource(store, &testutil.OAuth2Config{}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, tok)
	})

	t.Run("FreshToken", func(t *testing.T) {
		// A non-expired token is returned as-is; no refresh is performed.
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:            user.ID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  "fresh",
			OAuthRefreshToken: "refresh",
			OAuthExpiry:       dbtime.Now().Add(time.Hour),
		})

		src := newOIDCMCPTokenSource(store, &testutil.OAuth2Config{
			Token: &oauth2.Token{AccessToken: "should-not-be-used"},
		}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, "fresh", tok)
	})

	t.Run("RefreshExpired", func(t *testing.T) {
		// An expired token triggers a refresh; the new token is
		// persisted via UpdateUserLink. This exercises the dbauthz
		// elevation: chatd lacks ResourceSystem.Read and
		// ResourceUser.UpdatePersonal so a non-elevated context
		// would fail both reads and writes.
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:            user.ID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  "stale",
			OAuthRefreshToken: "refresh",
			OAuthExpiry:       dbtime.Now().Add(-time.Hour),
		})

		src := newOIDCMCPTokenSource(store, &testutil.OAuth2Config{
			Token: &oauth2.Token{
				AccessToken:  "fresh",
				RefreshToken: "new-refresh",
				Expiry:       dbtime.Now().Add(time.Hour),
			},
		}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, "fresh", tok)

		// Verify the refresh was persisted via UpdateUserLink.
		got, err := db.GetUserLinkByUserIDLoginType(
			dbauthz.AsSystemRestricted(context.Background()),
			database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
			},
		)
		require.NoError(t, err)
		require.Equal(t, "fresh", got.OAuthAccessToken)
		require.Equal(t, "new-refresh", got.OAuthRefreshToken)
	})

	t.Run("RefreshFailureReturnsEmpty", func(t *testing.T) {
		// A refresh attempt that fails (e.g. invalid client config)
		// must not surface an error to the caller; per the
		// UserOIDCTokenSource contract this is treated as "no
		// Authorization header".
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:            user.ID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  "stale",
			OAuthRefreshToken: "refresh",
			OAuthExpiry:       dbtime.Now().Add(-time.Hour),
		})

		// An empty oauth2.Config triggers a refresh failure
		// because it has no token endpoint to call.
		src := newOIDCMCPTokenSource(store, &oauth2.Config{}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, tok)
	})
}

func TestProtectedResourceMetadataResourceUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    resourceIdentifiers
		wantErr bool
	}{
		{
			name:  "String",
			input: `{"resource": "https://gitlab.example.com/api/v4/mcp", "authorization_servers": ["https://gitlab.example.com"]}`,
			want:  resourceIdentifiers{"https://gitlab.example.com/api/v4/mcp"},
		},
		{
			// GitLab's official MCP server returns an array of
			// resources despite RFC 9728 defining a string.
			name:  "Array",
			input: `{"resource": ["https://gitlab.example.com/api/v4/mcp", "https://gitlab.example.com/api/v4/orbit/mcp"], "authorization_servers": ["https://gitlab.example.com"]}`,
			want: resourceIdentifiers{
				"https://gitlab.example.com/api/v4/mcp",
				"https://gitlab.example.com/api/v4/orbit/mcp",
			},
		},
		{
			name:  "Absent",
			input: `{"authorization_servers": ["https://gitlab.example.com"]}`,
			want:  nil,
		},
		{
			name:    "InvalidType",
			input:   `{"resource": 42, "authorization_servers": ["https://gitlab.example.com"]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var meta protectedResourceMetadata
			err := json.Unmarshal([]byte(tt.input), &meta)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, meta.Resource)
		})
	}
}

func TestMCPOAuth2ResourceIndicator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		serverURL  string
		advertised []string
		want       string
	}{
		{
			name:       "ExactMatch",
			serverURL:  "https://mcp.agent.dagster.cloud/mcp",
			advertised: []string{"https://mcp.agent.dagster.cloud/mcp"},
			want:       "https://mcp.agent.dagster.cloud/mcp",
		},
		{
			name:       "OriginCoversPath",
			serverURL:  "https://mcp.example.com/v1/mcp",
			advertised: []string{"https://mcp.example.com"},
			want:       "https://mcp.example.com",
		},
		{
			name:       "CaseInsensitiveOriginKeptVerbatim",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"HTTPS://MCP.Example.com/mcp"},
			want:       "HTTPS://MCP.Example.com/mcp",
		},
		{
			name:       "PathCoversSubpath",
			serverURL:  "https://mcp.example.com/mcp/x",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "TrailingSlashResourceDoesNotCoverBarePath",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"https://mcp.example.com/mcp/"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "EmptyPathEqualsSlash",
			serverURL:  "https://mcp.example.com",
			advertised: []string{"https://mcp.example.com/"},
			want:       "https://mcp.example.com/",
		},
		{
			name:       "PathPrefixIsNotSegment",
			serverURL:  "https://mcp.example.com/mcp2",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp2",
		},
		{
			name:       "OtherOriginFallsBack",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"https://other.example.com/mcp", "http://mcp.example.com/mcp", "https://mcp.example.com:8443/mcp"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:      "MostSpecificWins",
			serverURL: "https://mcp.example.com/a/b/c",
			advertised: []string{
				"https://mcp.example.com/",
				"https://mcp.example.com/a/b",
				"https://mcp.example.com/a",
			},
			want: "https://mcp.example.com/a/b",
		},
		{
			name:      "QueryMustMatch",
			serverURL: "https://mcp.example.com/mcp?tenant=b",
			advertised: []string{
				"https://mcp.example.com/mcp?tenant=a",
				"https://mcp.example.com/mcp?tenant=b",
			},
			want: "https://mcp.example.com/mcp?tenant=b",
		},
		{
			name:       "MatchingQueryBeatsNoQuery",
			serverURL:  "https://mcp.example.com/mcp?tenant=b",
			advertised: []string{"https://mcp.example.com/mcp", "https://mcp.example.com/mcp?tenant=b"},
			want:       "https://mcp.example.com/mcp?tenant=b",
		},
		{
			name:       "NoQueryCoversQuery",
			serverURL:  "https://mcp.example.com/mcp?tenant=a",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "EmptyQueryIsNotOtherQuery",
			serverURL:  "https://mcp.example.com/mcp?tenant=b",
			advertised: []string{"https://mcp.example.com/mcp?"},
			want:       "https://mcp.example.com/mcp?tenant=b",
		},
		{
			name:       "EmptyQueryIsNotNoQuery",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"https://mcp.example.com/mcp?"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "ExplicitDefaultPortOnServer",
			serverURL:  "https://mcp.example.com:443/mcp",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "ExplicitDefaultPortOnResource",
			serverURL:  "http://mcp.example.com/mcp",
			advertised: []string{"http://mcp.example.com:80/mcp"},
			want:       "http://mcp.example.com:80/mcp",
		},
		{
			name:       "PortLeadingZerosAreIgnored",
			serverURL:  "https://mcp.example.com:0443/mcp",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "EscapedSlashIsNotSegmentBoundary",
			serverURL:  "https://mcp.example.com/mcp%2Ftenant",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp%2Ftenant",
		},
		{
			name:       "EscapeHexCaseIsEquivalent",
			serverURL:  "https://mcp.example.com/mcp%2ftenant",
			advertised: []string{"https://mcp.example.com/mcp%2Ftenant"},
			want:       "https://mcp.example.com/mcp%2Ftenant",
		},
		{
			name:       "EscapedUnreservedIsEquivalent",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"https://mcp.example.com/%6Dcp"},
			want:       "https://mcp.example.com/%6Dcp",
		},
		{
			name:       "EscapedQueryIsEquivalent",
			serverURL:  "https://mcp.example.com/mcp?tenant=%61",
			advertised: []string{"https://mcp.example.com/mcp?tenant=a"},
			want:       "https://mcp.example.com/mcp?tenant=a",
		},
		{
			name:       "DotSegmentsAreRemoved",
			serverURL:  "https://mcp.example.com/a/../mcp",
			advertised: []string{"https://mcp.example.com/mcp"},
			want:       "https://mcp.example.com/mcp",
		},
		{
			name:       "EscapedDotSegmentsAreRemoved",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"https://mcp.example.com/a/%2E%2E/mcp"},
			want:       "https://mcp.example.com/a/%2E%2E/mcp",
		},
		{
			name:      "InvalidFallbackIsEmpty",
			serverURL: "https://mcp.example.com/a[b]",
			want:      "",
		},
		{
			name:       "IPLiteralHost",
			serverURL:  "https://[::1]:8443/mcp",
			advertised: []string{"https://[::1]:8443/"},
			want:       "https://[::1]:8443/",
		},
		{
			name:       "EquivalentIPv6Literal",
			serverURL:  "https://[0:0:0:0:0:0:0:1]/mcp",
			advertised: []string{"https://[::1]/mcp"},
			want:       "https://[::1]/mcp",
		},
		{
			name:      "NoEntriesFallsBack",
			serverURL: "https://mcp.example.com/mcp",
			want:      "https://mcp.example.com/mcp",
		},
		{
			name:      "FallbackIsCanonical",
			serverURL: "HTTPS://user:pass@MCP.Example.COM:8443/Path/MCP?tenant=Acme#frag",
			want:      "https://mcp.example.com:8443/Path/MCP?tenant=Acme",
		},
		{
			name:       "InvalidEntriesIgnored",
			serverURL:  "https://mcp.example.com/mcp",
			advertised: []string{"https://mcp.example.com/#frag", "/mcp", "", "  "},
			want:       "https://mcp.example.com/mcp",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			serverURL, err := url.Parse(tc.serverURL)
			require.NoError(t, err)
			require.Equal(t, tc.want, mcpOAuth2ResourceIndicator(serverURL, tc.advertised))
		})
	}
}

func TestPlanMCPServerConfigWrite(t *testing.T) {
	t.Parallel()

	const (
		serverURL = "https://mcp.example.com/mcp"
		otherURL  = "https://other.example.com/mcp"
		manual    = string(codersdk.MCPServerOAuth2ClientRegistrationManual)
		dynamic   = string(codersdk.MCPServerOAuth2ClientRegistrationDynamic)
	)
	noAuth := database.MCPServerConfig{
		Url:                      serverURL,
		AuthType:                 "none",
		OAuth2ClientRegistration: manual,
		CustomHeaders:            "{}",
	}
	manualOAuth2 := noAuth
	manualOAuth2.AuthType = "oauth2"
	manualOAuth2.OAuth2ClientID = "manual-client"
	manualOAuth2.OAuth2AuthURL = "https://auth.example.com/authorize"
	manualOAuth2.OAuth2TokenURL = "https://auth.example.com/token"
	dynamicOAuth2 := manualOAuth2
	dynamicOAuth2.OAuth2ClientID = "dynamic-client"
	dynamicOAuth2.OAuth2ClientRegistration = dynamic
	// A row that an update saved as oauth2 without a client before
	// updates could register one.
	brokenOAuth2 := noAuth
	brokenOAuth2.AuthType = "oauth2"

	cases := []struct {
		name            string
		prev            database.MCPServerConfig
		req             codersdk.UpdateMCPServerConfigRequest
		wantMode        string
		wantRegisterFor string
		wantErr         string
	}{
		{
			name: "CreateWithClientIsManual",
			req: mcpServerConfigChangeFromCreate(codersdk.CreateMCPServerConfigRequest{
				URL:            serverURL,
				AuthType:       "oauth2",
				OAuth2ClientID: "manual-client",
				OAuth2AuthURL:  "https://auth.example.com/authorize",
				OAuth2TokenURL: "https://auth.example.com/token",
			}),
			wantMode: manual,
		},
		{
			name: "CreateWithoutClientRegisters",
			req: mcpServerConfigChangeFromCreate(codersdk.CreateMCPServerConfigRequest{
				URL:      serverURL,
				AuthType: "oauth2",
			}),
			wantMode:        dynamic,
			wantRegisterFor: serverURL,
		},
		{
			name:            "NoneToOAuth2Registers",
			prev:            noAuth,
			req:             codersdk.UpdateMCPServerConfigRequest{AuthType: ptr.Ref("oauth2"), OAuth2ClientID: ptr.Ref("")},
			wantMode:        dynamic,
			wantRegisterFor: serverURL,
		},
		{
			name:            "BrokenOAuth2Registers",
			prev:            brokenOAuth2,
			req:             codersdk.UpdateMCPServerConfigRequest{URL: ptr.Ref(serverURL)},
			wantMode:        dynamic,
			wantRegisterFor: serverURL,
		},
		{
			name:     "EnabledOnlySkipsAuth",
			prev:     brokenOAuth2,
			req:      codersdk.UpdateMCPServerConfigRequest{Enabled: ptr.Ref(false)},
			wantMode: manual,
		},
		{
			name:            "ManualToDynamicRegisters",
			prev:            manualOAuth2,
			req:             codersdk.UpdateMCPServerConfigRequest{OAuth2ClientRegistration: ptr.Ref(codersdk.MCPServerOAuth2ClientRegistrationDynamic)},
			wantMode:        dynamic,
			wantRegisterFor: serverURL,
		},
		{
			name:     "ManualURLChangeKeepsClient",
			prev:     manualOAuth2,
			req:      codersdk.UpdateMCPServerConfigRequest{URL: ptr.Ref(otherURL)},
			wantMode: manual,
		},
		{
			name:            "DynamicURLChangeRegisters",
			prev:            dynamicOAuth2,
			req:             codersdk.UpdateMCPServerConfigRequest{URL: ptr.Ref(otherURL)},
			wantMode:        dynamic,
			wantRegisterFor: otherURL,
		},
		{
			name: "DynamicRepeatedClientKeepsClient",
			prev: dynamicOAuth2,
			req: codersdk.UpdateMCPServerConfigRequest{
				URL:            ptr.Ref(serverURL),
				OAuth2ClientID: ptr.Ref(dynamicOAuth2.OAuth2ClientID),
				OAuth2AuthURL:  ptr.Ref(dynamicOAuth2.OAuth2AuthURL),
				OAuth2TokenURL: ptr.Ref(dynamicOAuth2.OAuth2TokenURL),
			},
			wantMode: dynamic,
		},
		{
			name:     "DynamicToManualKeepsClient",
			prev:     dynamicOAuth2,
			req:      codersdk.UpdateMCPServerConfigRequest{OAuth2ClientRegistration: ptr.Ref(codersdk.MCPServerOAuth2ClientRegistrationManual)},
			wantMode: manual,
		},
		{
			name:    "PartialManualClient",
			prev:    noAuth,
			req:     codersdk.UpdateMCPServerConfigRequest{AuthType: ptr.Ref("oauth2"), OAuth2ClientID: ptr.Ref("manual-client")},
			wantErr: "automatic discovery",
		},
		{
			name:    "DynamicRejectsClientChange",
			prev:    dynamicOAuth2,
			req:     codersdk.UpdateMCPServerConfigRequest{OAuth2ClientID: ptr.Ref("other-client")},
			wantErr: "Set oauth2_client_registration to manual",
		},
		{
			name:    "APIKeyWithoutValue",
			prev:    noAuth,
			req:     codersdk.UpdateMCPServerConfigRequest{AuthType: ptr.Ref("api_key"), APIKeyHeader: ptr.Ref("X-Api-Key")},
			wantErr: "requires api_key_header and api_key_value",
		},
		{
			name:    "CustomHeadersWithoutHeader",
			prev:    noAuth,
			req:     codersdk.UpdateMCPServerConfigRequest{AuthType: ptr.Ref("custom_headers")},
			wantErr: "requires at least one custom header",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w, err := planMCPServerConfigWrite(tc.prev, tc.req, "")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantMode, w.next.OAuth2ClientRegistration)
			require.Equal(t, tc.wantRegisterFor, w.registerFor)
		})
	}
}

func TestMCPServerConfigWriteWithRegistration(t *testing.T) {
	t.Parallel()

	const (
		serverURL = "https://mcp.example.com/mcp"
		otherURL  = "https://other.example.com/mcp"
		clientID  = "dynamic-client"
	)
	next := database.MCPServerConfig{
		Url:                      serverURL,
		AuthType:                 "oauth2",
		OAuth2ClientRegistration: string(codersdk.MCPServerOAuth2ClientRegistrationDynamic),
	}
	registrationFor := func(u string) *mcpOAuth2Discovery {
		return &mcpOAuth2Discovery{
			serverURL: u,
			clientID:  clientID,
			authURL:   "https://auth.example.com/authorize",
			tokenURL:  "https://auth.example.com/token",
		}
	}

	cases := []struct {
		name         string
		registerFor  string
		registration *mcpOAuth2Discovery
		wantClientID string
		wantConflict bool
	}{
		{
			name: "NoRegistration",
		},
		{
			name:         "MatchingRegistration",
			registerFor:  serverURL,
			registration: registrationFor(serverURL),
			wantClientID: clientID,
		},
		{
			name:         "MissingRegistration",
			registerFor:  serverURL,
			wantConflict: true,
		},
		{
			name:         "RegistrationForOtherURL",
			registerFor:  serverURL,
			registration: registrationFor(otherURL),
			wantConflict: true,
		},
		{
			name:         "UnneededRegistration",
			registration: registrationFor(serverURL),
			wantConflict: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := mcpServerConfigWrite{next: next, registerFor: tc.registerFor}
			got, err := w.withRegistration(database.MCPServerConfig{}, codersdk.UpdateMCPServerConfigRequest{}, tc.registration)
			if tc.wantConflict {
				require.ErrorIs(t, err, errMCPConfigChangedDuringRegistration)
				return
			}
			require.NoError(t, err)
			require.Equal(t, serverURL, got.Url)
			require.Equal(t, tc.wantClientID, got.OAuth2ClientID)
		})
	}
}
