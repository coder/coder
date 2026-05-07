package httpmw_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// trustedLoopbackCIDR is the CIDR used by the tests to mark
// httptest.NewRequest's default 192.0.2.1 RemoteAddr as a trusted
// origin.
const trustedLoopbackCIDR = "192.0.2.0/24"

func parseCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	return network
}

func mustExternalAuthConfig(t *testing.T, enabled bool, origins ...string) httpmw.ExternalAuthHeaderConfig {
	t.Helper()
	cfg, err := httpmw.ParseExternalAuthHeaderConfig(enabled, origins)
	require.NoError(t, err)
	return cfg
}

// successHandlerWritingActor mirrors successHandler in apikey_test
// but additionally writes the dbauthz actor's friendly name (the
// asserted user's username) so tests can confirm impersonation.
func successHandlerWritingActor(t *testing.T) http.HandlerFunc {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		actor, ok := dbauthz.ActorFromContext(r.Context())
		require.True(t, ok, "expected dbauthz actor")
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: actor.FriendlyName,
		})
	})
}

func TestExternalAuthHeader(t *testing.T) {
	t.Parallel()

	t.Run("Username", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{Username: "alice"})

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=alice")
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, user.Username, resp.Message)
	})

	t.Run("Email", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{Username: "bob", Email: "bob@example.com"})

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic UserEmail=bob@example.com")
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, user.Username, resp.Message)
	})

	t.Run("UntrustedOriginIgnored", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbgen.User(t, db, database.User{Username: "alice"})

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=alice")
		// Explicitly set an origin that is NOT in the trusted CIDR.
		r.RemoteAddr = "203.0.113.5:1234"
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		// The header was ignored, so the request fell through to the
		// normal session-token flow with no token, producing a 401.
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("DisabledIgnoresHeader", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbgen.User(t, db, database.User{Username: "alice"})

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=alice")
		rw := httptest.NewRecorder()

		// Enabled=false: feature off, header ignored, falls through
		// to no-token 401.
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB: db,
			ExternalAuthHeader: httpmw.ExternalAuthHeaderConfig{
				Enabled: false,
				TrustedOrigins: []*net.IPNet{
					parseCIDR(t, trustedLoopbackCIDR),
				},
			},
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("EmptyTrustedOriginsSilentlyDisables", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbgen.User(t, db, database.User{Username: "alice"})

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=alice")
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true /* no origins */),
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("UnknownUserHardFail", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=ghost")
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
			Optional:           true, // Hard error must surface even on optional routes.
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("MalformedHeaderHardFail", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		cases := []string{
			"Basic",                 // No fields at all.
			"Basic GarbageOnlyKey",  // Field without =.
			"Bearer Username=alice", // Wrong scheme.
			"Basic TokenName=tok",   // No identity field.
		}
		for _, header := range cases {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set(httpmw.ExternalAuthHeaderName, header)
			rw := httptest.NewRecorder()

			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:                 db,
				ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
			})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

			res := rw.Result()
			res.Body.Close()
			assert.NotEqual(t, http.StatusOK, res.StatusCode, "header %q should be rejected", header)
		}
	})

	t.Run("DeletedUserNotFound", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{Username: "ghost"})

		// Soft-delete the user. GetUserByEmailOrUsername filters
		// out deleted=true rows so the lookup will fail just like
		// for a missing user.
		ctx := dbauthz.AsSystemRestricted(context.Background())
		err := db.UpdateUserDeletedByID(ctx, user.ID)
		require.NoError(t, err)

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=ghost")
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NoDBWritesForSyntheticKey", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{Username: "alice"})

		// Snapshot how many keys exist before the request.
		ctx := dbauthz.AsSystemRestricted(context.Background())
		beforeKeys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
			LoginType: database.LoginTypeNone,
			UserID:    user.ID,
		})
		require.NoError(t, err)

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(httpmw.ExternalAuthHeaderName, "Basic Username=alice")
		rw := httptest.NewRecorder()

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:                 db,
			ExternalAuthHeader: mustExternalAuthConfig(t, true, trustedLoopbackCIDR),
		})(successHandlerWritingActor(t)).ServeHTTP(rw, r)

		res := rw.Result()
		res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		afterKeys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
			LoginType: database.LoginTypeNone,
			UserID:    user.ID,
		})
		require.NoError(t, err)
		require.Len(t, afterKeys, len(beforeKeys),
			"synthesized api key must not be persisted")
	})
}

func TestParseExternalAuthHeaderConfig(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		cfg, err := httpmw.ParseExternalAuthHeaderConfig(true, []string{"127.0.0.0/8", "10.0.0.0/8"})
		require.NoError(t, err)
		require.True(t, cfg.Enabled)
		require.Len(t, cfg.TrustedOrigins, 2)
	})

	t.Run("InvalidCIDR", func(t *testing.T) {
		t.Parallel()
		_, err := httpmw.ParseExternalAuthHeaderConfig(true, []string{"not-a-cidr"})
		require.Error(t, err)
	})

	t.Run("EmptyEntriesSkipped", func(t *testing.T) {
		t.Parallel()
		cfg, err := httpmw.ParseExternalAuthHeaderConfig(true, []string{"", "127.0.0.0/8", "  "})
		require.NoError(t, err)
		require.Len(t, cfg.TrustedOrigins, 1)
	})
}
