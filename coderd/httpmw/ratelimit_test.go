package httpmw_test

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func randRemoteAddr() string {
	var b [4]byte
	// nolint:gosec
	_, _ = rand.Read(b[:])
	// nolint:gosec
	return fmt.Sprintf("%s:%v", net.IP(b[:]).String(), rand.Int31()%(1<<16))
}

func TestRateLimit(t *testing.T) {
	t.Parallel()
	t.Run("NoUserSucceeds", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.RateLimit(1, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.Equal(t, i != 0, resp.StatusCode == http.StatusTooManyRequests)
		}
	})

	t.Run("RandomIPs", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		// Because these are random IPs, the limit should never be hit!
		rtr.Use(httpmw.RateLimit(1, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			req.RemoteAddr = randRemoteAddr()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.False(t, resp.StatusCode == http.StatusTooManyRequests)
		}
	})

	t.Run("RegularUser", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		u := dbgen.User(t, db, database.User{})
		_, key := dbgen.APIKey(t, db, database.APIKey{UserID: u.ID})

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:       db,
			Optional: false,
		}))

		rtr.Use(httpmw.RateLimit(1, time.Second))
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

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(codersdk.SessionTokenHeader, key)
			rec := httptest.NewRecorder()
			// Assert we're not using IP address.
			req.RemoteAddr = randRemoteAddr()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.Equal(t, i != 0, resp.StatusCode == http.StatusTooManyRequests)
		}
	})

	t.Run("OwnerBypass", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()

		u := dbgen.User(t, db, database.User{
			RBACRoles: []string{rbac.RoleOwner()},
		})
		_, key := dbgen.APIKey(t, db, database.APIKey{UserID: u.ID})

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:       db,
			Optional: false,
		}))

		rtr.Use(httpmw.RateLimit(1, time.Second))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(codersdk.SessionTokenHeader, key)
			req.Header.Set(codersdk.BypassRatelimitHeader, "true")
			rec := httptest.NewRecorder()
			// Assert we're not using IP address.
			req.RemoteAddr = randRemoteAddr()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.False(t, resp.StatusCode == http.StatusTooManyRequests)
		}
	})
}
