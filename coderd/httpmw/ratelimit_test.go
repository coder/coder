package httpmw_test

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func randRemoteAddr() string {
	var b [4]byte
	// nolint:gosec
	rand.Read(b[:])
	// nolint:gosec
	return fmt.Sprintf("%s:%v", net.IP(b[:]).String(), rand.Int31()%(1<<16))
}

func TestRateLimit(t *testing.T) {
	t.Parallel()
	t.Run("NoUserSucceeds", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.RateLimit(5, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		require.Eventually(t, func() bool {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusTooManyRequests
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("RandomIPs", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.RateLimit(5, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		require.Never(t, func() bool {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			req.RemoteAddr = randRemoteAddr()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusTooManyRequests
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("RegularUser", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		db := databasefake.New()
		gen := databasefake.NewGenerator(t, db)
		u := gen.User(ctx, database.User{})
		_, key := gen.APIKey(ctx, database.APIKey{UserID: u.ID})

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:       db,
			Optional: false,
		}))

		rtr.Use(httpmw.RateLimit(5, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		// Bypass must fail
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set(codersdk.SessionTokenHeader, key)
		req.Header.Set(codersdk.BypassRatelimitHeader, "true")
		rec := httptest.NewRecorder()
		// Assert we're not using IP address.
		req.RemoteAddr = randRemoteAddr()
		rtr.ServeHTTP(rec, req)
		resp := rec.Result()
		defer resp.Body.Close()
		require.Equal(t, http.StatusPreconditionRequired, resp.StatusCode)

		require.Eventually(t, func() bool {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(codersdk.SessionTokenHeader, key)
			rec := httptest.NewRecorder()
			// Assert we're not using IP address.
			req.RemoteAddr = randRemoteAddr()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusTooManyRequests
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("OwnerBypass", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		db := databasefake.New()

		gen := databasefake.NewGenerator(t, db)
		u := gen.User(ctx, database.User{
			RBACRoles: []string{rbac.RoleOwner()},
		})
		_, key := gen.APIKey(ctx, database.APIKey{UserID: u.ID})

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
			DB:       db,
			Optional: false,
		}))

		rtr.Use(httpmw.RateLimit(5, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		require.Never(t, func() bool {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(codersdk.SessionTokenHeader, key)
			req.Header.Set(codersdk.BypassRatelimitHeader, "true")
			rec := httptest.NewRecorder()
			// Assert we're not using IP address.
			req.RemoteAddr = randRemoteAddr()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusTooManyRequests
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}
