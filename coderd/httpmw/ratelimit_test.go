package httpmw_test

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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

		db, _ := dbtestutil.NewDB(t)
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

		db, _ := dbtestutil.NewDB(t)

		u := dbgen.User(t, db, database.User{
			RBACRoles: []string{codersdk.RoleOwner},
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

func TestRateLimitByAuthToken(t *testing.T) {
	t.Parallel()

	t.Run("LimitsByAuthHeader", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			headerName string
			headerVal  string
		}{
			{
				name:       "BearerToken",
				headerName: "Authorization",
				headerVal:  "Bearer test-token-123",
			},
			{
				name:       "XApiKey",
				headerName: "X-Api-Key",
				headerVal:  "test-api-key-456",
			},
			{
				name:       "NoToken",
				headerName: "",
				headerVal:  "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				rtr := chi.NewRouter()
				rtr.Use(httpmw.RateLimitByAuthToken(2, time.Hour))
				rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusOK)
				})

				// Same token (or IP if no token) should be rate limited after 2 requests.
				for i := 0; i < 5; i++ {
					req := httptest.NewRequest("GET", "/", nil)
					if tt.headerName != "" {
						req.Header.Set(tt.headerName, tt.headerVal)
					}
					rec := httptest.NewRecorder()
					rtr.ServeHTTP(rec, req)
					resp := rec.Result()
					_ = resp.Body.Close()
					if i < 2 {
						require.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i)
					} else {
						require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "request %d should be rate limited", i)
						// Verify Retry-After header is set.
						require.NotEmpty(t, resp.Header.Get("Retry-After"), "Retry-After header should be set")
					}
				}
			})
		}
	})

	t.Run("DifferentTokensNotLimited", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.RateLimitByAuthToken(1, time.Hour))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		// Different tokens should not be rate limited against each other.
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer token-%d", i))
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i)
		}
	})

	t.Run("DisabledWhenZero", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.RateLimitByAuthToken(0, time.Hour))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		// Should not be rate limited when limit is 0.
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer same-token")
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})
}

func TestConcurrencyLimit(t *testing.T) {
	t.Parallel()

	t.Run("LimitsConcurrentRequests", func(t *testing.T) {
		t.Parallel()

		const maxConcurrency = 2
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ConcurrencyLimit(maxConcurrency, "Test"))

		// Use a WaitGroup as a barrier to ensure all requests are in the handler
		// before any of them proceed.
		var handlersReady sync.WaitGroup
		handlersReady.Add(maxConcurrency)
		releaseHandler := make(chan struct{})

		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			handlersReady.Done()
			// Wait until released.
			<-releaseHandler
			rw.WriteHeader(http.StatusOK)
		})

		server := httptest.NewServer(rtr)
		defer server.Close()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Start maxConcurrency requests that will block.
		// We use channels to collect errors instead of require in goroutines.
		type result struct {
			statusCode int
			err        error
		}
		results := make(chan result, maxConcurrency)

		var wg sync.WaitGroup
		for i := 0; i < maxConcurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/", nil)
				if err != nil {
					results <- result{err: err}
					return
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					results <- result{err: err}
					return
				}
				defer resp.Body.Close()
				results <- result{statusCode: resp.StatusCode}
			}()
		}

		// Wait for all requests to enter the handler with a timeout.
		handlersReadyCh := make(chan struct{})
		go func() {
			handlersReady.Wait()
			close(handlersReadyCh)
		}()
		select {
		case <-handlersReadyCh:
		case <-ctx.Done():
			t.Fatal("timed out waiting for handlers to be ready")
		}

		// Next request should be rejected since we're at capacity.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		// Release all blocked requests.
		close(releaseHandler)
		wg.Wait()
		close(results)

		// Check all goroutine results.
		for res := range results {
			require.NoError(t, res.err)
			require.Equal(t, http.StatusOK, res.statusCode)
		}
	})

	t.Run("DisabledWhenZero", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ConcurrencyLimit(0, "Test"))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		// Should not be limited when maxConcurrency is 0.
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})
}
