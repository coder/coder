package coderd_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestDebugHealth(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			calls        = atomic.Int64{}
			ctx, cancel  = context.WithTimeout(context.Background(), testutil.WaitShort)
			sessionToken string
			client       = coderdtest.New(t, &coderdtest.Options{
				HealthcheckFunc: func(_ context.Context, apiKey string) *codersdk.HealthcheckReport {
					calls.Add(1)
					assert.Equal(t, sessionToken, apiKey)
					return &codersdk.HealthcheckReport{
						Time: time.Now(),
					}
				},
				HealthcheckRefresh: time.Hour, // Avoid flakes.
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		sessionToken = client.SessionToken()
		for i := 0; i < 10; i++ {
			res, err := client.Request(ctx, "GET", "/api/v2/debug/health", nil)
			require.NoError(t, err)
			_, _ = io.ReadAll(res.Body)
			res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
		}
		// The healthcheck should only have been called once.
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("Forced", func(t *testing.T) {
		t.Parallel()

		var (
			calls        = atomic.Int64{}
			ctx, cancel  = context.WithTimeout(context.Background(), testutil.WaitShort)
			sessionToken string
			client       = coderdtest.New(t, &coderdtest.Options{
				HealthcheckFunc: func(_ context.Context, apiKey string) *codersdk.HealthcheckReport {
					calls.Add(1)
					assert.Equal(t, sessionToken, apiKey)
					return &codersdk.HealthcheckReport{
						Time: time.Now(),
					}
				},
				HealthcheckRefresh: time.Hour, // Avoid flakes.
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		sessionToken = client.SessionToken()
		for i := 0; i < 10; i++ {
			res, err := client.Request(ctx, "GET", "/api/v2/debug/health?force=true", nil)
			require.NoError(t, err)
			_, _ = io.ReadAll(res.Body)
			res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
		}
		// The healthcheck func should have been called each time.
		require.EqualValues(t, 10, calls.Load())
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		var (
			// Need to ignore errors due to ctx timeout
			logger      = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			client      = coderdtest.New(t, &coderdtest.Options{
				Logger:             &logger,
				HealthcheckTimeout: time.Microsecond,
				HealthcheckFunc: func(context.Context, string) *codersdk.HealthcheckReport {
					t := time.NewTimer(time.Second)
					defer t.Stop()

					select {
					case <-ctx.Done():
						return &codersdk.HealthcheckReport{}
					case <-t.C:
						return &codersdk.HealthcheckReport{}
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

	t.Run("Refresh", func(t *testing.T) {
		t.Parallel()

		var (
			calls       = make(chan struct{})
			callsDone   = make(chan struct{})
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckRefresh: time.Microsecond,
				HealthcheckFunc: func(context.Context, string) *codersdk.HealthcheckReport {
					calls <- struct{}{}
					return &codersdk.HealthcheckReport{}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)

		defer cancel()

		go func() {
			defer close(callsDone)
			<-calls
			<-time.After(testutil.IntervalFast)
			<-calls
		}()

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

		select {
		case <-callsDone:
		case <-ctx.Done():
			t.Fatal("timed out waiting for calls to finish")
		}
	})

	t.Run("Deduplicated", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			calls       int
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckRefresh: time.Hour,
				HealthcheckTimeout: time.Hour,
				HealthcheckFunc: func(context.Context, string) *codersdk.HealthcheckReport {
					calls++
					return &codersdk.HealthcheckReport{
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
				HealthcheckFunc: func(_ context.Context, apiKey string) *codersdk.HealthcheckReport {
					assert.Equal(t, sessionToken, apiKey)
					return &codersdk.HealthcheckReport{
						Time:    time.Now(),
						Healthy: true,
						DERP:    codersdk.DERPHealthReport{Healthy: true},
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

func TestHealthSettings(t *testing.T) {
	t.Parallel()

	t.Run("InitialState", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		adminClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, adminClient)

		// when
		settings, err := adminClient.HealthSettings(ctx)
		require.NoError(t, err)

		// then
		require.Equal(t, codersdk.HealthSettings{DismissedHealthchecks: []codersdk.HealthSection{}}, settings)
	})

	t.Run("DismissSection", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		adminClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, adminClient)

		expected := codersdk.HealthSettings{
			DismissedHealthchecks: []codersdk.HealthSection{codersdk.HealthSectionDERP, codersdk.HealthSectionWebsocket},
		}

		// when: dismiss "derp" and "websocket"
		err := adminClient.PutHealthSettings(ctx, expected)
		require.NoError(t, err)

		// then
		settings, err := adminClient.HealthSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, settings)

		// then
		res, err := adminClient.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		bs, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		defer res.Body.Close()
		var hc codersdk.HealthcheckReport
		require.NoError(t, json.Unmarshal(bs, &hc))
		require.True(t, hc.DERP.Dismissed)
		require.True(t, hc.Websocket.Dismissed)
	})

	t.Run("UnDismissSection", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		adminClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, adminClient)

		initial := codersdk.HealthSettings{
			DismissedHealthchecks: []codersdk.HealthSection{codersdk.HealthSectionDERP, codersdk.HealthSectionWebsocket},
		}

		err := adminClient.PutHealthSettings(ctx, initial)
		require.NoError(t, err)

		expected := codersdk.HealthSettings{
			DismissedHealthchecks: []codersdk.HealthSection{codersdk.HealthSectionDERP},
		}

		// when: undismiss "websocket"
		err = adminClient.PutHealthSettings(ctx, expected)
		require.NoError(t, err)

		// then
		settings, err := adminClient.HealthSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, settings)

		// then
		res, err := adminClient.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		bs, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		defer res.Body.Close()
		var hc codersdk.HealthcheckReport
		require.NoError(t, json.Unmarshal(bs, &hc))
		require.True(t, hc.DERP.Dismissed)
		require.False(t, hc.Websocket.Dismissed)
	})

	t.Run("NotModified", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		adminClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, adminClient)

		expected := codersdk.HealthSettings{
			DismissedHealthchecks: []codersdk.HealthSection{codersdk.HealthSectionDERP, codersdk.HealthSectionWebsocket},
		}

		err := adminClient.PutHealthSettings(ctx, expected)
		require.NoError(t, err)

		// when
		err = adminClient.PutHealthSettings(ctx, expected)

		// then
		require.Error(t, err)
		require.Contains(t, err.Error(), "health settings not modified")
	})
}

func TestDebugWebsocket(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
	})
}
