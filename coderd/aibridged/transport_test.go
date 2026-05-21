package aibridged_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

func TestTransportFactory_TransportFor(t *testing.T) {
	t.Parallel()

	t.Run("CoderAgentReturnsTransport", func(t *testing.T) {
		t.Parallel()
		f := aibridged.NewTransportFactory(http.NotFoundHandler(), nil)
		rt, err := f.TransportFor(uuid.New(), true)
		require.NoError(t, err)
		require.NotNil(t, rt)
	})

	t.Run("NonCoderAgentFallsThrough", func(t *testing.T) {
		t.Parallel()
		f := aibridged.NewTransportFactory(http.NotFoundHandler(), nil)
		rt, err := f.TransportFor(uuid.New(), false)
		require.NoError(t, err)
		require.Nil(t, rt)
	})

	t.Run("NilHandlerErrors", func(t *testing.T) {
		t.Parallel()
		f := aibridged.NewTransportFactory(nil, nil)
		_, err := f.TransportFor(uuid.New(), true)
		require.Error(t, err)
	})
}

func TestInMemoryRoundTripper_PassesHeadersAndStatus(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	rt, err := aibridged.NewTransportFactory(handler, nil).TransportFor(uuid.New(), true)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/v1/test", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusTeapot, resp.StatusCode)
	require.Equal(t, "yes", resp.Header.Get("X-Custom"))
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, string(body))
}

// Verify that response chunks become readable on the client side before the
// handler has finished writing. This is the property SSE/NDJSON streaming
// depends on.
func TestInMemoryRoundTripper_Streams(t *testing.T) {
	t.Parallel()

	const chunks = 4
	released := make([]chan struct{}, chunks)
	for i := range released {
		released[i] = make(chan struct{})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "ResponseWriter must implement http.Flusher")
		for i := 0; i < chunks; i++ {
			<-released[i]
			_, err := fmt.Fprintf(w, "data: chunk-%d\n\n", i)
			require.NoError(t, err)
			flusher.Flush()
		}
	})

	rt, err := aibridged.NewTransportFactory(handler, nil).TransportFor(uuid.New(), true)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/stream", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	br := bufio.NewReader(resp.Body)
	for i := 0; i < chunks; i++ {
		close(released[i])
		dataLine, err := br.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("data: chunk-%d\n", i), dataLine)
		// Consume blank-line separator.
		_, err = br.ReadString('\n')
		require.NoError(t, err)
	}
}

// Canceling the request context must surface as a body-read error, matching
// real-network behavior, and the handler must observe the cancellation
// through its own request context.
func TestInMemoryRoundTripper_CancelCloses(t *testing.T) {
	t.Parallel()

	handlerCtxObserved := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		close(handlerCtxObserved)
	})

	rt, err := aibridged.NewTransportFactory(handler, nil).TransportFor(uuid.New(), true)
	require.NoError(t, err)

	parentCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(parentCtx)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/stream", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	cancel()
	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)

	select {
	case <-handlerCtxObserved:
	case <-parentCtx.Done():
		t.Fatal("handler did not observe context cancellation")
	}
}

// Many independent in-flight requests on a shared handler must not interfere.
func TestInMemoryRoundTripper_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	rt, err := aibridged.NewTransportFactory(handler, nil).TransportFor(uuid.New(), true)
	require.NoError(t, err)

	const n = 16
	errs := make(chan error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			payload := fmt.Sprintf("payload-%d", i)
			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/echo", strings.NewReader(payload))
			if err != nil {
				errs <- err
				return
			}
			resp, err := rt.RoundTrip(req)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				errs <- err
				return
			}
			if string(got) != payload {
				errs <- xerrors.Errorf("payload mismatch: want %q got %q", payload, string(got))
				return
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// A handler that returns without writing must not block RoundTrip; the caller
// gets a zero-length 200 OK.
func TestInMemoryRoundTripper_HandlerReturnsWithoutWriting(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	rt, err := aibridged.NewTransportFactory(handler, nil).TransportFor(uuid.New(), true)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/noop", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Empty(t, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
