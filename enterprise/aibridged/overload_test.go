package aibridged_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/enterprise/aibridged"
	"github.com/coder/coder/v2/testutil"
)

func TestOverloadProtection_ConcurrencyLimit(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	t.Run("allows_requests_within_limit", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			MaxConcurrency: 5,
		}, logger)

		var handlerCalls atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		wrapped := op.WrapHandler(handler)

		// Make 5 requests in sequence - all should succeed.
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}

		assert.Equal(t, int32(5), handlerCalls.Load())
	})

	t.Run("rejects_requests_over_limit", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			MaxConcurrency: 2,
		}, logger)

		// Create a handler that blocks until we release it.
		blocked := make(chan struct{})
		var handlerCalls atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalls.Add(1)
			<-blocked
			w.WriteHeader(http.StatusOK)
		})

		wrapped := op.WrapHandler(handler)

		// Start 2 requests that will block.
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				wrapped.ServeHTTP(rec, req)
			}()
		}

		// Wait for the handlers to be called.
		require.Eventually(t, func() bool {
			return handlerCalls.Load() == 2
		}, testutil.WaitShort, testutil.IntervalFast)

		// Make a third request - it should be rejected.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

		// Verify current concurrency is 2.
		assert.Equal(t, int64(2), op.CurrentConcurrency())

		// Unblock the handlers.
		close(blocked)
		wg.Wait()

		// Verify concurrency is back to 0.
		assert.Equal(t, int64(0), op.CurrentConcurrency())
	})

	t.Run("disabled_when_zero", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			MaxConcurrency: 0, // Disabled.
		}, logger)

		assert.Nil(t, op.ConcurrencyMiddleware())
	})
}

func TestOverloadProtection_RateLimit(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	t.Run("allows_requests_within_limit", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			RateLimit:  5,
			RateWindow: time.Minute,
		}, logger)

		var handlerCalls atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		wrapped := op.WrapHandler(handler)

		// Make 5 requests - all should succeed.
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}

		assert.Equal(t, int32(5), handlerCalls.Load())
	})

	t.Run("rejects_requests_over_limit", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			RateLimit:  2,
			RateWindow: time.Minute,
		}, logger)

		var handlerCalls atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		wrapped := op.WrapHandler(handler)

		// Make 3 requests - first 2 should succeed, 3rd should be rate limited.
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			if i < 2 {
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				assert.Equal(t, http.StatusTooManyRequests, rec.Code)
			}
		}

		assert.Equal(t, int32(2), handlerCalls.Load())
	})

	t.Run("disabled_when_zero", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			RateLimit: 0, // Disabled.
		}, logger)

		assert.Nil(t, op.RateLimitMiddleware())
	})
}

func TestOverloadProtection_Combined(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	t.Run("both_limits_applied", func(t *testing.T) {
		t.Parallel()

		op := aibridged.NewOverloadProtection(aibridged.OverloadConfig{
			MaxConcurrency: 10,
			RateLimit:      3,
			RateWindow:     time.Minute,
		}, logger)

		var handlerCalls atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		wrapped := op.WrapHandler(handler)

		// Make 4 requests - first 3 should succeed, 4th should be rate limited.
		for i := 0; i < 4; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			if i < 3 {
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				assert.Equal(t, http.StatusTooManyRequests, rec.Code)
			}
		}

		assert.Equal(t, int32(3), handlerCalls.Load())
	})
}
