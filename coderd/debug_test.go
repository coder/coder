package coderd_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/testutil"
)

func TestDebugHealth(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel  = context.WithTimeout(context.Background(), testutil.WaitShort)
			sessionToken string
			client       = coderdtest.New(t, &coderdtest.Options{
				HealthcheckFunc: func(_ context.Context, apiKey string) *healthcheck.Report {
					assert.Equal(t, sessionToken, apiKey)
					return &healthcheck.Report{}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		sessionToken = client.SessionToken()
		res, err := client.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckTimeout: time.Microsecond,
				HealthcheckFunc: func(context.Context, string) *healthcheck.Report {
					t := time.NewTimer(time.Second)
					defer t.Stop()

					select {
					case <-ctx.Done():
						return &healthcheck.Report{}
					case <-t.C:
						return &healthcheck.Report{}
					}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		res, err := client.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Deduplicated", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			calls       int
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckRefresh: time.Hour,
				HealthcheckTimeout: time.Hour,
				HealthcheckFunc: func(context.Context, string) *healthcheck.Report {
					calls++
					return &healthcheck.Report{
						Time: time.Now(),
					}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		res, err := client.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)

		require.Equal(t, http.StatusOK, res.StatusCode)

		res, err = client.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, 1, calls)
	})

	t.Run("Text", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel  = context.WithTimeout(context.Background(), testutil.WaitShort)
			sessionToken string
			client       = coderdtest.New(t, &coderdtest.Options{
				HealthcheckFunc: func(_ context.Context, apiKey string) *healthcheck.Report {
					assert.Equal(t, sessionToken, apiKey)
					return &healthcheck.Report{
						Time:    time.Now(),
						Healthy: true,
						DERP:    derphealth.Report{Healthy: true},
					}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		sessionToken = client.SessionToken()
		res, err := client.Request(ctx, "GET", "/api/v2/debug/health?format=text", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		resB, _ := io.ReadAll(res.Body)
		require.Equal(t, http.StatusOK, res.StatusCode)

		resStr := string(resB)
		assert.Contains(t, resStr, "healthy: true")
		assert.Contains(t, resStr, "derp: true")
		assert.Contains(t, resStr, "access_url: false")
		assert.Contains(t, resStr, "websocket: false")
		assert.Contains(t, resStr, "database: false")
	})
}

func TestDebugWebsocket(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
	})
}
