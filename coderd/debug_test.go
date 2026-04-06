package coderd_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
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
				HealthcheckFunc: func(_ context.Context, apiKey string, _ *healthcheck.Progress) *healthsdk.HealthcheckReport {
					calls.Add(1)
					assert.Equal(t, sessionToken, apiKey)
					return &healthsdk.HealthcheckReport{
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
				HealthcheckFunc: func(_ context.Context, apiKey string, _ *healthcheck.Progress) *healthsdk.HealthcheckReport {
					calls.Add(1)
					assert.Equal(t, sessionToken, apiKey)
					return &healthsdk.HealthcheckReport{
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
			done        = make(chan struct{})
			client      = coderdtest.New(t, &coderdtest.Options{
				Logger:             &logger,
				HealthcheckTimeout: time.Second,
				HealthcheckFunc: func(_ context.Context, _ string, progress *healthcheck.Progress) *healthsdk.HealthcheckReport {
					progress.Start("test")
					<-done
					return &healthsdk.HealthcheckReport{}
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
		)
		defer cancel()

		res, err := client.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		close(done)
		bs, err := io.ReadAll(res.Body)
		require.NoError(t, err, "reading body")
		require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
		var sdkResp codersdk.Response
		require.NoError(t, json.Unmarshal(bs, &sdkResp), "unmarshaling sdk response")
		require.Equal(t, "Healthcheck timed out.", sdkResp.Message)
		require.Contains(t, sdkResp.Detail, "Still running: test (elapsed:")
	})

	t.Run("Refresh", func(t *testing.T) {
		t.Parallel()

		var (
			calls       = make(chan struct{})
			callsDone   = make(chan struct{})
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			client      = coderdtest.New(t, &coderdtest.Options{
				HealthcheckRefresh: time.Microsecond,
				HealthcheckFunc: func(context.Context, string, *healthcheck.Progress) *healthsdk.HealthcheckReport {
					calls <- struct{}{}
					return &healthsdk.HealthcheckReport{}
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
				HealthcheckFunc: func(context.Context, string, *healthcheck.Progress) *healthsdk.HealthcheckReport {
					calls++
					return &healthsdk.HealthcheckReport{
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
				HealthcheckFunc: func(_ context.Context, apiKey string, _ *healthcheck.Progress) *healthsdk.HealthcheckReport {
					assert.Equal(t, sessionToken, apiKey)
					return &healthsdk.HealthcheckReport{
						Time:    time.Now(),
						Healthy: true,
						DERP:    healthsdk.DERPHealthReport{Healthy: true},
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
		settings, err := healthsdk.New(adminClient).HealthSettings(ctx)
		require.NoError(t, err)

		// then
		require.Equal(t, healthsdk.HealthSettings{DismissedHealthchecks: []healthsdk.HealthSection{}}, settings)
	})

	t.Run("DismissSection", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		adminClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, adminClient)

		expected := healthsdk.HealthSettings{
			DismissedHealthchecks: []healthsdk.HealthSection{healthsdk.HealthSectionDERP, healthsdk.HealthSectionWebsocket},
		}

		// when: dismiss "derp" and "websocket"
		err := healthsdk.New(adminClient).PutHealthSettings(ctx, expected)
		require.NoError(t, err)

		// then
		settings, err := healthsdk.New(adminClient).HealthSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, settings)

		// then
		res, err := adminClient.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		bs, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		defer res.Body.Close()
		var hc healthsdk.HealthcheckReport
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

		initial := healthsdk.HealthSettings{
			DismissedHealthchecks: []healthsdk.HealthSection{healthsdk.HealthSectionDERP, healthsdk.HealthSectionWebsocket},
		}

		err := healthsdk.New(adminClient).PutHealthSettings(ctx, initial)
		require.NoError(t, err)

		expected := healthsdk.HealthSettings{
			DismissedHealthchecks: []healthsdk.HealthSection{healthsdk.HealthSectionDERP},
		}

		// when: undismiss "websocket"
		err = healthsdk.New(adminClient).PutHealthSettings(ctx, expected)
		require.NoError(t, err)

		// then
		settings, err := healthsdk.New(adminClient).HealthSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, settings)

		// then
		res, err := adminClient.Request(ctx, "GET", "/api/v2/debug/health", nil)
		require.NoError(t, err)
		bs, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		defer res.Body.Close()
		var hc healthsdk.HealthcheckReport
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

		expected := healthsdk.HealthSettings{
			DismissedHealthchecks: []healthsdk.HealthSection{healthsdk.HealthSectionDERP, healthsdk.HealthSectionWebsocket},
		}

		err := healthsdk.New(adminClient).PutHealthSettings(ctx, expected)
		require.NoError(t, err)

		// when
		err = healthsdk.New(adminClient).PutHealthSettings(ctx, expected)

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

// noopProfileCollector avoids calling process-global runtime functions
// (CPU profiler, tracer) so that tests can run in parallel safely.
type noopProfileCollector struct{}

func (noopProfileCollector) StartCPUProfile(io.Writer) (func(), error) { return func() {}, nil }
func (noopProfileCollector) StartTrace(io.Writer) (func(), error)      { return func() {}, nil }
func (noopProfileCollector) LookupProfile(string, io.Writer) error     { return nil }
func (noopProfileCollector) SetBlockProfileRate(int)                   {}
func (noopProfileCollector) SetMutexProfileFraction(int) int           { return 0 }

// Compile-time check.
var _ coderd.ProfileCollector = noopProfileCollector{}

// blockingProfileCollector blocks in StartCPUProfile until unblocked,
// allowing deterministic testing of the concurrency guard.
type blockingProfileCollector struct {
	noopProfileCollector
	started chan struct{} // closed when StartCPUProfile is entered
	block   chan struct{} // StartCPUProfile blocks until this is closed
}

func (b *blockingProfileCollector) StartCPUProfile(io.Writer) (func(), error) {
	close(b.started)
	<-b.block
	return func() {}, nil
}

func newTestAPI(t *testing.T) (*codersdk.Client, io.Closer, *coderd.API) {
	t.Helper()
	client, closer, api := coderdtest.NewWithAPI(t, nil)
	api.ProfileCollector = noopProfileCollector{}
	return client, closer, api
}

func TestDebugCollectProfile(t *testing.T) {
	t.Parallel()

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		client, closer, api := newTestAPI(t)
		defer closer.Close()
		_ = coderdtest.CreateFirstUser(t, client)

		asserter := coderdtest.AssertRBAC(t, api, client)

		body, err := client.DebugCollectProfile(ctx, codersdk.DebugProfileOptions{
			// Use a very short duration so the test finishes quickly.
			// The noop collector means no real profiling occurs.
			Duration: 100 * time.Millisecond,
		})
		require.NoError(t, err)
		defer body.Close()

		data, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NotEmpty(t, data, "archive should not be empty")

		// Verify that the response is a valid tar.gz archive containing
		// the expected profile files.
		files := extractTarGzFiles(t, data)
		require.Contains(t, files, "cpu.prof")
		require.Contains(t, files, "heap.prof")
		require.Contains(t, files, "allocs.prof")
		require.Contains(t, files, "block.prof")
		require.Contains(t, files, "mutex.prof")
		require.Contains(t, files, "goroutine.prof")

		// Verify the endpoint checks the correct RBAC permission.
		asserter.AssertChecked(t, policy.ActionRead, rbac.ResourceDebugInfo)
	})

	t.Run("CustomProfiles", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		client, closer, _ := newTestAPI(t)
		defer closer.Close()
		_ = coderdtest.CreateFirstUser(t, client)

		body, err := client.DebugCollectProfile(ctx, codersdk.DebugProfileOptions{
			Duration: 100 * time.Millisecond,
			Profiles: []string{"heap", "goroutine"},
		})
		require.NoError(t, err)
		defer body.Close()

		data, err := io.ReadAll(body)
		require.NoError(t, err)

		files := extractTarGzFiles(t, data)
		require.Contains(t, files, "heap.prof")
		require.Contains(t, files, "goroutine.prof")
		// Should NOT contain profiles we didn't ask for.
		require.NotContains(t, files, "cpu.prof")
		require.NotContains(t, files, "allocs.prof")
	})

	t.Run("WithTraceAndCPU", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		client, closer, _ := newTestAPI(t)
		defer closer.Close()
		_ = coderdtest.CreateFirstUser(t, client)

		body, err := client.DebugCollectProfile(ctx, codersdk.DebugProfileOptions{
			Duration: 100 * time.Millisecond,
			Profiles: []string{"cpu", "trace"},
		})
		require.NoError(t, err)
		defer body.Close()

		data, err := io.ReadAll(body)
		require.NoError(t, err)

		files := extractTarGzFiles(t, data)
		require.Contains(t, files, "cpu.prof")
		require.Contains(t, files, "trace.out")
	})

	t.Run("DurationTooLong", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(ctx, "POST", "/api/v2/debug/profile?duration=5m", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("InvalidDuration", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(ctx, "POST", "/api/v2/debug/profile?duration=notaduration", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("InvalidProfile", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(ctx, "POST", "/api/v2/debug/profile?profiles=nonexistent", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Create a non-admin user.
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		res, err := memberClient.Request(ctx, "POST", "/api/v2/debug/profile", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusForbidden, res.StatusCode)
	})

	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		blocker := &blockingProfileCollector{
			started: make(chan struct{}),
			block:   make(chan struct{}),
		}

		client, closer, api := coderdtest.NewWithAPI(t, nil)
		defer closer.Close()
		api.ProfileCollector = blocker
		_ = coderdtest.CreateFirstUser(t, client)

		// Start a profile collection that will block inside
		// StartCPUProfile until we explicitly unblock it.
		done := make(chan struct{})
		go func() {
			defer close(done)
			body, err := client.DebugCollectProfile(ctx, codersdk.DebugProfileOptions{
				Duration: 1 * time.Second,
			})
			if err == nil {
				body.Close()
			}
		}()

		// Wait deterministically for the first request to enter the
		// collector — no time.Sleep needed.
		testutil.TryReceive(ctx, t, blocker.started)

		// The second request should get 409 Conflict.
		res, err := client.Request(ctx, "POST", "/api/v2/debug/profile?duration=1s", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusConflict, res.StatusCode)

		// Unblock the first request and wait for it to finish.
		close(blocker.block)
		testutil.TryReceive(ctx, t, done)
	})
}

// extractTarGzFiles extracts file names from a tar.gz archive.
func extractTarGzFiles(t *testing.T, data []byte) map[string]bool {
	t.Helper()

	gr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer gr.Close()

	tr := tar.NewReader(gr)
	files := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		files[hdr.Name] = true
	}
	return files
}
