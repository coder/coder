package httpmw_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw/loggermock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
)

func TestAPIKeyResource(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{})
	origin, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	const (
		reports  = "https://coder.example.com/reports"
		jobs     = "https://coder.example.com/jobs"
		metadata = "https://discovery.example.com/reports"
	)

	for _, tc := range []struct {
		name              string
		audience          string
		resourceURI       string
		delegatedResource string
		wrongKey          bool
		invalidSecret     bool
		missingToken      bool
		queryToken        bool
		status            int
	}{
		{name: "DefaultRoot", audience: origin.String(), status: http.StatusNoContent},
		{name: "DefaultRejectsPathAudience", audience: reports, status: http.StatusForbidden},
		{name: "ReportsResource", audience: reports, resourceURI: reports, status: http.StatusNoContent},
		{name: "JobsResource", audience: jobs, resourceURI: jobs, status: http.StatusNoContent},
		{name: "NormalizedResource", audience: "https://CODER.example.com:443/reports/", resourceURI: reports, status: http.StatusNoContent},
		{name: "ResourceRejectsRoot", audience: origin.String(), resourceURI: reports, status: http.StatusForbidden},
		{name: "ResourceRejectsOtherResource", audience: jobs, resourceURI: reports, status: http.StatusForbidden},
		{name: "ResourceRejectsOtherDeployment", audience: "https://other.example.com/reports", resourceURI: reports, status: http.StatusForbidden},
		{name: "ReportsDelegation", audience: reports, delegatedResource: reports, status: http.StatusNoContent},
		{name: "JobsDelegation", audience: jobs, delegatedResource: jobs, status: http.StatusNoContent},
		{name: "DelegationOverridesDestinationResource", audience: reports, resourceURI: jobs, delegatedResource: reports, status: http.StatusNoContent},
		{name: "DelegationRejectsDestinationAudience", audience: jobs, resourceURI: jobs, delegatedResource: reports, status: http.StatusForbidden},
		{name: "DelegationRejectsRoot", audience: origin.String(), delegatedResource: reports, status: http.StatusForbidden},
		{name: "DelegationRejectsOtherResource", audience: jobs, delegatedResource: reports, status: http.StatusForbidden},
		{name: "DelegationRejectsOtherDeployment", audience: "https://other.example.com/reports", delegatedResource: reports, status: http.StatusForbidden},
		{name: "DelegationRejectsSubstitutedKey", audience: reports, delegatedResource: reports, wrongKey: true, status: http.StatusUnauthorized},
		{name: "DelegationRejectsInvalidSecret", audience: reports, delegatedResource: reports, invalidSecret: true, status: http.StatusUnauthorized},
		{name: "DelegationRequiresCredential", audience: reports, delegatedResource: reports, missingToken: true, status: http.StatusUnauthorized},
		{name: "DelegationRejectsQueryToken", audience: reports, delegatedResource: reports, queryToken: true, status: http.StatusUnauthorized},
		{name: "ResourceDiscoveryWithoutCredential", resourceURI: reports, missingToken: true, status: http.StatusUnauthorized},
		{name: "UnboundLegacyResource", resourceURI: reports, status: http.StatusNoContent},
		{name: "UnboundLegacyDelegation", delegatedResource: reports, status: http.StatusNoContent},
		{name: "UnboundLegacyRejectsSubstitutedKey", delegatedResource: reports, wrongKey: true, status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, token := dbgen.APIKey(t, db, database.APIKey{
				UserID: user.ID, LoginType: database.LoginTypeOAuth2ProviderApp,
				Scopes: database.APIKeyScopes{database.ApiKeyScopeUserRead},
			})
			dbgen.OAuth2ProviderAppToken(t, db, database.OAuth2ProviderAppToken{
				AppID: app.ID, UserID: user.ID, APIKeyID: key.ID, HashPrefix: []byte(key.ID),
				Audience: sql.NullString{String: tc.audience, Valid: tc.audience != ""},
			})
			delegatedID := key.ID
			if tc.wrongKey {
				otherKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
				delegatedID = otherKey.ID
			}
			if tc.invalidSecret {
				token = key.ID + "-" + strings.Repeat("0", 22)
			}

			for _, precheck := range []bool{false, true} {
				t.Run(fmt.Sprintf("Precheck=%t", precheck), func(t *testing.T) {
					t.Parallel()
					// Request-controlled resource and delegation hints must have no
					// effect, whether or not server-owned delegation is present.
					req := httptest.NewRequest(http.MethodGet, "https://untrusted.example.com/api/v2/users/me?resource="+url.QueryEscape(reports), nil)
					req.Header.Set("X-Coder-API-Key-Delegation", key.ID)
					req.Header.Set("X-Coder-MCP-Delegation", "true")
					req.Header.Set("X-Forwarded-Uri", "/reports")
					req.Header.Set("X-Forwarded-Host", "coder.example.com")
					req.Header.Set("Referer", reports)
					if tc.queryToken {
						q := req.URL.Query()
						q.Set("access_token", token)
						req.URL.RawQuery = q.Encode()
					} else if !tc.missingToken {
						req.Header.Set("Authorization", "Bearer "+token)
					}
					if tc.delegatedResource != "" {
						req = req.WithContext(httpmw.WithAPIKeyDelegation(req.Context(), tc.delegatedResource, delegatedID))
					}
					originalURL := req.URL.String()
					handler := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
						DB: db, AccessURL: origin, Logger: testutil.Logger(t),
						ResourceURI: tc.resourceURI, ResourceMetadataURL: metadata,
					})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, originalURL, r.URL.String())
						assert.Equal(t, key.ID, httpmw.APIKey(r).ID)
						actor := httpmw.UserAuthorization(r.Context())
						assert.Equal(t, user.ID.String(), actor.ID)
						assert.Equal(t, key.ScopeSet(), actor.Scope)
						w.WriteHeader(http.StatusNoContent)
					}))
					if precheck {
						handler = httpmw.PrecheckAPIKey(httpmw.ValidateAPIKeyConfig{DB: db, Logger: testutil.Logger(t)})(handler)
					}
					rw := httptest.NewRecorder()
					handler.ServeHTTP(rw, req)
					require.Equal(t, tc.status, rw.Code)
					if tc.wrongKey {
						var response codersdk.Response
						require.NoError(t, json.NewDecoder(rw.Body).Decode(&response))
						require.Equal(t, "Delegated requests must use the credential that authenticated the originating request.", response.Message)
						require.NotContains(t, response.Message, key.ID)
						require.NotContains(t, response.Message, delegatedID)
						require.NotContains(t, rw.Header().Get("WWW-Authenticate"), "expired")
					}
					require.Equal(t, originalURL, req.URL.String())
					if tc.status != http.StatusNoContent {
						require.Contains(t, rw.Header().Get("WWW-Authenticate"), `resource_metadata="`+metadata+`"`)
					}
				})
			}
		})
	}
}

func randomAPIKeyParts() (id string, secret string, hashedSecret []byte) {
	id, _ = cryptorand.String(10)
	secret, hashedSecret, _ = apikey.GenerateSecret(22)
	return id, secret, hashedSecret
}

func TestAPIKey(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	// assertActorOk asserts all the properties of the user auth are ok.
	assertActorOk := func(t *testing.T, r *http.Request) {
		t.Helper()

		actor, ok := dbauthz.ActorFromContext(r.Context())
		assert.True(t, ok, "dbauthz actor ok")
		if ok {
			_, err := actor.Roles.Expand()
			assert.NoError(t, err, "actor roles ok")

			_, err = actor.Scope.Expand()
			assert.NoError(t, err, "actor scope ok")

			err = actor.RegoValueOk()
			assert.NoError(t, err, "actor rego ok")
		}

		auth, ok := httpmw.UserAuthorizationOptional(r.Context())
		assert.True(t, ok, "httpmw auth ok")
		if ok {
			_, err := auth.Roles.Expand()
			assert.NoError(t, err, "auth roles ok")

			_, err = auth.Scope.Expand()
			assert.NoError(t, err, "auth scope ok")

			err = auth.RegoValueOk()
			assert.NoError(t, err, "auth rego ok")
		}
	}

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	t.Run("NoCookie", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NoCookieRedirects", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: true,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		location, err := res.Location()
		require.NoError(t, err)
		require.NotEmpty(t, location.Query().Get("message"))
		require.Equal(t, http.StatusSeeOther, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, "test-wow-hello")

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidIDLength", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, "test-wow")

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecretLength", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, "testtestid-wow")

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			id, secret, _ = randomAPIKeyParts()
			r             = httptest.NewRequest("GET", "/", nil)
			rw            = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, fmt.Sprintf("%s-%s", id, secret))

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("GetAPIKeyByIDInternalError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		id, secret, _ := randomAPIKeyParts()
		r := httptest.NewRequest("GET", "/", nil)
		rw := httptest.NewRecorder()
		r.Header.Set(codersdk.SessionTokenHeader, fmt.Sprintf("%s-%s", id, secret))

		db.EXPECT().GetAPIKeyByID(gomock.Any(), id).Return(database.APIKey{}, xerrors.New("db unavailable"))

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusInternalServerError, res.StatusCode)

		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.NotEqual(t, httpmw.SignedOutErrorMessage, resp.Message)
		require.Contains(t, resp.Detail, "Internal error fetching API key by id")
	})

	t.Run("UserLinkNotFound", func(t *testing.T) {
		t.Parallel()
		var (
			r    = httptest.NewRequest("GET", "/", nil)
			rw   = httptest.NewRecorder()
			user = dbgen.User(t, db, database.User{
				LoginType: database.LoginTypeGithub,
			})
			// Intentionally not inserting any user link
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LoginType: user.LoginType,
			})
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, resp.Message, httpmw.SignedOutErrorMessage)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			r    = httptest.NewRequest("GET", "/", nil)
			rw   = httptest.NewRecorder()
			user = dbgen.User(t, db, database.User{})

			// Use a different secret so they don't match!
			hashed   = sha256.Sum256([]byte("differentsecret"))
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:       user.ID,
				HashedSecret: hashed[:],
			})
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: time.Now().Add(time.Hour * -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		var apiRes codersdk.Response
		dec := json.NewDecoder(res.Body)
		_ = dec.Decode(&apiRes)
		require.True(t, strings.HasPrefix(apiRes.Detail, "API key expired"))
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			assertActorOk(t, r)
			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidWithScope", func(t *testing.T) {
		t.Parallel()
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect},
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: token,
		})

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			apiKey := httpmw.APIKey(r)
			assert.Equal(t, database.ApiKeyScopeCoderApplicationConnect, apiKey.Scopes[0])
			assertActorOk(t, r)

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "it worked!",
			})
		})).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("QueryParameter", func(t *testing.T) {
		t.Parallel()
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		q := r.URL.Query()
		q.Add(codersdk.SessionTokenCookie, token)
		r.URL.RawQuery = q.Encode()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.APIKey(r)
			assertActorOk(t, r)

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("ValidUpdateLastUsed", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.NotEqual(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidUpdateExpiry", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().Add(time.Minute),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.NotEqual(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("TokenNoExpiryRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().Add(time.Minute),
				LoginType: database.LoginTypeToken,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		// Programmatic tokens honor a fixed lifetime, so the expiry must not be
		// extended on use even though it is within the refresh window.
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("NoRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                          db,
			RedirectToLogin:             false,
			DisableSessionExpiryRefresh: true,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.NotEqual(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("OAuthNotExpired", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeGithub,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeGithub,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("APIKeyExpiredOAuthExpired", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, -1),
				LoginType: database.LoginTypeOIDC,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:      user.ID,
				LoginType:   database.LoginTypeOIDC,
				OAuthExpiry: dbtime.Now().AddDate(0, 0, -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		// Include a valid oauth token for refreshing. If this token is invalid,
		// it is difficult to tell an auth failure from an expired api key, or
		// an expired oauth key.
		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtime.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				OIDC: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("APIKeyExpiredOAuthNotExpired", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, -1),
				LoginType: database.LoginTypeOIDC,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtime.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				OIDC: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("OAuthRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeGithub,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:            user.ID,
				LoginType:         database.LoginTypeGithub,
				OAuthRefreshToken: "hello",
				OAuthExpiry:       dbtime.Now().AddDate(0, 0, -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtestutil.NowInDefaultTimezone().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				Github: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		// Note that OAuth expiry is independent of APIKey expiry, so an OIDC refresh DOES NOT affect the expiry of the
		// APIKey
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)

		gotLink, err := db.GetUserLinkByUserIDLoginType(r.Context(), database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeGithub,
		})
		require.NoError(t, err)
		require.Equal(t, gotLink.OAuthRefreshToken, "moo")
	})

	t.Run("OAuthExpiredNoRefresh", func(t *testing.T) {
		t.Parallel()
		var (
			ctx               = testutil.Context(t, testutil.WaitShort)
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeGithub,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		_, err := db.InsertUserLink(ctx, database.InsertUserLinkParams{
			UserID:           user.ID,
			LoginType:        database.LoginTypeGithub,
			OAuthExpiry:      dbtime.Now().AddDate(0, 0, -1),
			OAuthAccessToken: "letmein",
		})
		require.NoError(t, err)

		r.Header.Set(codersdk.SessionTokenHeader, token)

		oauthToken := &oauth2.Token{
			AccessToken:  "wow",
			RefreshToken: "moo",
			Expiry:       dbtime.Now().AddDate(0, 0, 1),
		}
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			OAuth2Configs: &httpmw.OAuth2Configs{
				Github: &testutil.OAuth2Config{
					Token: oauthToken,
				},
			},
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("RemoteIPUpdates", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now().AddDate(0, 0, -1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.RemoteAddr = "1.1.1.1"
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, "1.1.1.1", gotAPIKey.IPAddress.IPNet.IP.String())
	})

	t.Run("RedirectToLogin", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: true,
		})(successHandler).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusSeeOther, res.StatusCode)
		u, err := res.Location()
		require.NoError(t, err)
		require.Equal(t, "/login", u.Path)
	})

	t.Run("Optional", func(t *testing.T) {
		t.Parallel()
		var (
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			count   atomic.Int64
			handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				count.Add(1)

				apiKey, ok := httpmw.APIKeyOptional(r)
				assert.False(t, ok)
				assert.Zero(t, apiKey)

				rw.WriteHeader(http.StatusOK)
			})
		)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
			Optional:        true,
		})(handler).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, 1, count.Load())
	})

	t.Run("Tokens", func(t *testing.T) {
		t.Parallel()
		var (
			user              = dbgen.User(t, db, database.User{})
			sentAPIKey, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeToken,
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), sentAPIKey.ID)
		require.NoError(t, err)

		require.Equal(t, sentAPIKey.LastUsed, gotAPIKey.LastUsed)
		require.Equal(t, sentAPIKey.ExpiresAt, gotAPIKey.ExpiresAt)
		require.Equal(t, sentAPIKey.LoginType, gotAPIKey.LoginType)
	})

	t.Run("MissingConfig", func(t *testing.T) {
		t.Parallel()
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				LastUsed:  dbtime.Now(),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
				LoginType: database.LoginTypeOIDC,
			})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:            user.ID,
				LoginType:         database.LoginTypeOIDC,
				OAuthRefreshToken: "random",
				// expired
				OAuthExpiry: time.Now().Add(time.Hour * -1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusInternalServerError, res.StatusCode)
		out, _ := io.ReadAll(res.Body)
		require.Contains(t, string(out), "Unable to refresh")
	})

	t.Run("CustomRoles", func(t *testing.T) {
		t.Parallel()
		var (
			org        = dbgen.Organization(t, db, database.Organization{})
			customRole = dbgen.CustomRole(t, db, database.CustomRole{
				Name:           "custom-role",
				OrgPermissions: []database.CustomRolePermission{},
				OrganizationID: uuid.NullUUID{
					UUID:  org.ID,
					Valid: true,
				},
			})
			user = dbgen.User(t, db, database.User{
				RBACRoles: []string{},
			})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
				Roles: []string{
					rbac.RoleOrgAdmin(),
					customRole.Name,
				},
			})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			assertActorOk(t, r)

			auth := httpmw.UserAuthorization(r.Context())

			roles, err := auth.Roles.Expand()
			assert.NoError(t, err, "expand user roles")
			// Assert built in org role
			assert.True(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == rbac.RoleOrgAdmin() && role.Identifier.OrganizationID == org.ID
			}), "org admin role")
			// Assert custom role
			assert.True(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == customRole.Name && role.Identifier.OrganizationID == org.ID
			}), "custom org role")

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	// There is no sql foreign key constraint to require all assigned roles
	// still exist in the database. We need to handle deleted roles.
	t.Run("RoleNotExists", func(t *testing.T) {
		t.Parallel()
		var (
			roleNotExistsName = "role-not-exists"
			org               = dbgen.Organization(t, db, database.Organization{})
			user              = dbgen.User(t, db, database.User{
				RBACRoles: []string{
					// Also provide an org not exists. In practice this makes no sense
					// to store org roles in the user table, but there is no org to
					// store it in. So just throw this here for even more unexpected
					// behavior handling!
					rbac.RoleIdentifier{Name: roleNotExistsName, OrganizationID: uuid.New()}.String(),
				},
			})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
				Roles: []string{
					rbac.RoleOrgAdmin(),
					roleNotExistsName,
				},
			})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			assertActorOk(t, r)
			auth := httpmw.UserAuthorization(r.Context())

			roles, err := auth.Roles.Expand()
			assert.NoError(t, err, "expand user roles")
			// Assert built in org role
			assert.True(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == rbac.RoleOrgAdmin() && role.Identifier.OrganizationID == org.ID
			}), "org admin role")

			// Assert the role-not-exists is not returned
			assert.False(t, slices.ContainsFunc(roles, func(role rbac.Role) bool {
				return role.Identifier.Name == roleNotExistsName
			}), "role should not exist")

			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
				Message: "It worked!",
			})
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("LogsAPIKeyID", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name           string
			expired        bool
			expectedStatus int
		}{
			{
				name:           "OnSuccess",
				expired:        false,
				expectedStatus: http.StatusOK,
			},
			{
				name:           "OnFailure",
				expired:        true,
				expectedStatus: http.StatusUnauthorized,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var (
					user   = dbgen.User(t, db, database.User{})
					expiry = dbtime.Now().AddDate(0, 0, 1)
				)
				if tc.expired {
					expiry = dbtime.Now().AddDate(0, 0, -1)
				}
				sentAPIKey, token := dbgen.APIKey(t, db, database.APIKey{
					UserID:    user.ID,
					ExpiresAt: expiry,
				})

				var (
					ctrl       = gomock.NewController(t)
					mockLogger = loggermock.NewMockRequestLogger(ctrl)
					r          = httptest.NewRequest("GET", "/", nil)
					rw         = httptest.NewRecorder()
				)
				r.Header.Set(codersdk.SessionTokenHeader, token)

				// Expect WithAuthContext to be called (from dbauthz.As).
				mockLogger.EXPECT().WithAuthContext(gomock.Any()).AnyTimes()
				// Expect WithFields to be called with api_key_id field regardless of success/failure.
				mockLogger.EXPECT().WithFields(
					slog.F("api_key_id", sentAPIKey.ID),
				).Times(1)

				// Add the mock logger to the context.
				ctx := loggermw.WithRequestLogger(r.Context(), mockLogger)
				r = r.WithContext(ctx)

				httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
					DB:              db,
					RedirectToLogin: false,
				})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					if tc.expired {
						t.Error("handler should not be called on auth failure")
					}
					httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
						Message: "It worked!",
					})
				})).ServeHTTP(rw, r)

				res := rw.Result()
				defer res.Body.Close()
				require.Equal(t, tc.expectedStatus, res.StatusCode)
			})
		}
	})
}
