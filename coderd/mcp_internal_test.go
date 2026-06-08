package coderd

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestIsSlackUserTokenAuthURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		authURL string
		want    bool
	}{
		{"slack user-token endpoint", "https://slack.com/oauth/v2_user/authorize", true},
		{"slack bot-token endpoint", "https://slack.com/oauth/v2/authorize", false},
		{"slack legacy endpoint", "https://slack.com/oauth/authorize", false},
		{"other provider", "https://github.com/login/oauth/authorize", false},
		{"empty", "", false},
		{"not a url", "://bogus", false},
		{"lookalike host", "https://notslack.com/oauth/v2_user/authorize", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isSlackUserTokenAuthURL(tc.authURL))
		})
	}
}

func TestExchangeSlackUserToken(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		var gotForm string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			gotForm = r.Form.Encode()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"ok": true,
				"authed_user": {
					"id": "U123",
					"access_token": "xoxp-user-token",
					"token_type": "Bearer"
				}
			}`))
		}))
		defer srv.Close()

		cfg := &oauth2.Config{
			ClientID:     "cid",
			ClientSecret: "secret",
			Endpoint:     oauth2.Endpoint{TokenURL: srv.URL},
			RedirectURL:  "https://coder.example.com/callback",
		}
		tok, err := exchangeSlackUserToken(context.Background(), srv.Client(), cfg, "the-code", "the-verifier")
		require.NoError(t, err)
		require.Equal(t, "xoxp-user-token", tok.AccessToken)
		require.Equal(t, "Bearer", tok.TokenType)
		// The user token (not a bot token) must be the one stored.
		require.Contains(t, gotForm, "code=the-code")
		require.Contains(t, gotForm, "code_verifier=the-verifier")
		require.Contains(t, gotForm, "grant_type=authorization_code")
	})

	t.Run("SlackErrorOnHTTP200", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Slack signals failures with HTTP 200 + ok:false.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": false, "error": "invalid_code"}`))
		}))
		defer srv.Close()

		cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL}}
		_, err := exchangeSlackUserToken(context.Background(), srv.Client(), cfg, "bad", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_code")
	})

	t.Run("MissingUserToken", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// ok:true but no authed_user.access_token (e.g. only a bot token).
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": true, "access_token": "xoxb-bot-only"}`))
		}))
		defer srv.Close()

		cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL}}
		_, err := exchangeSlackUserToken(context.Background(), srv.Client(), cfg, "code", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "no user access token")
	})
}
