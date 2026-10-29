package coderd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// BenchmarkMCPDelegatedTransport isolates transport overhead with a trivial
// handler, excluding MCP, authentication, database work, and request creation.
func BenchmarkMCPDelegatedTransport(b *testing.B) {
	origin := &url.URL{Scheme: "https", Host: "coder.example.com"}
	req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, origin.String()+"/api/v2/users/me", nil)
	require.NoError(b, err)
	logger := testutil.Logger(b)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	b.Run("Reuse", func(b *testing.B) {
		transport, closeTransport := newMCPDelegatedTransport(logger, handler, origin, "benchmark-key")
		defer closeTransport()
		benchmarkMCPNoContent(b, transport, req)
		b.ReportAllocs()
		for b.Loop() {
			benchmarkMCPNoContent(b, transport, req)
		}
	})
	b.Run("SetupTeardown", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, closeTransport := newMCPDelegatedTransport(logger, handler, origin, "benchmark-key")
			closeTransport()
		}
	})
	b.Run("SetupFirstCallTeardown", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			func() {
				transport, closeTransport := newMCPDelegatedTransport(logger, handler, origin, "benchmark-key")
				defer closeTransport()
				benchmarkMCPNoContent(b, transport, req)
			}()
		}
	})
	b.Run("BufferedRecorderBaseline", func(b *testing.B) {
		// This direct buffered call is not production-equivalent: it has no
		// streaming, hijacking, HTTP serialization, or delegation isolation.
		b.ReportAllocs()
		for b.Loop() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			resp := recorder.Result()
			if err := resp.Body.Close(); err != nil {
				b.Fatal(err)
			}
			if resp.StatusCode != http.StatusNoContent {
				b.Fatalf("unexpected status: %d", resp.StatusCode)
			}
		}
	})
}

func benchmarkMCPNoContent(b *testing.B, transport http.RoundTripper, req *http.Request) {
	resp, err := transport.RoundTrip(req)
	if err != nil {
		b.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		b.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

// BenchmarkMCPDelegatedTransportActiveStreams measures one cohort per iteration,
// not per-request latency. Run standalone with -run '^$' -benchtime=1x so global
// goroutine counts are not obscured by other tests. Each stream owns a transport,
// modeling separate outer MCP requests rather than one shared connection pool.
func BenchmarkMCPDelegatedTransportActiveStreams(b *testing.B) {
	for b.Loop() {
		benchmarkMCPActiveStreamCohort(b)
	}
}

func benchmarkMCPActiveStreamCohort(b *testing.B) {
	const streams = 1024
	ctx, cancel := context.WithTimeout(b.Context(), testutil.WaitLong)
	defer cancel()
	origin := &url.URL{Scheme: "https", Host: "coder.example.com"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.String()+"/api/v2/logs", nil)
	require.NoError(b, err)
	logger := testutil.Logger(b)
	baseline := runtime.NumGoroutine()
	completed := make(chan struct{}, streams)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { completed <- struct{}{} }()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ready\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	closers := make([]func(), 0, streams)
	cleanup := sync.OnceFunc(func() {
		for _, closeTransport := range closers {
			closeTransport()
		}
	})
	defer cleanup()
	for range streams {
		transport, closeTransport := newMCPDelegatedTransport(logger, handler, origin, "benchmark-key")
		closers = append(closers, closeTransport)
		//nolint:bodyclose // Bodies are closed with their transports in cleanup.
		resp, err := transport.RoundTrip(req)
		require.NoError(b, err)
		closers[len(closers)-1] = func() {
			closeTransport()
			_ = resp.Body.Close()
		}
		require.Equal(b, http.StatusOK, resp.StatusCode)
		var first [len("data: ready\n\n")]byte
		_, err = io.ReadFull(resp.Body, first[:])
		require.NoError(b, err)
	}
	active := runtime.NumGoroutine()
	cleanup()
	for range streams {
		testutil.TryReceive(ctx, b, completed)
	}
	// Connection goroutines can exit after Server.Close returns. Report rather
	// than assert the process-wide baseline, which unrelated work can change.
	for runtime.NumGoroutine() > baseline && ctx.Err() == nil {
		runtime.Gosched()
	}
	b.ReportMetric(float64(streams), "streams")
	b.ReportMetric(float64(baseline), "goroutines-baseline")
	b.ReportMetric(float64(active), "goroutines-active")
	b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines-cleaned")
}
