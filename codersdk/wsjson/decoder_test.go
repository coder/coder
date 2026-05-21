package wsjson_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestDecoder_CloseAfterServerClose(t *testing.T) {
	t.Parallel()

	// Server sends one message, then closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = conn.CloseRead(r.Context())
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`"hello"`))
		if err != nil {
			return
		}
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}))
	defer srv.Close()

	ctx := testutil.Context(t, testutil.WaitShort)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	// nolint: bodyclose
	conn, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)

	logger := slogtest.Make(t, nil)
	dec := wsjson.NewDecoder[string](conn, websocket.MessageText, logger)
	ch := dec.Chan()

	// Read the message with a context timeout.
	msg := testutil.RequireReceive(ctx, t, ch)
	require.Equal(t, "hello", msg)

	// Wait for the channel to close (server closed the connection).
	select {
	case <-ctx.Done():
		t.Fatal("timed out waiting for channel close")
	case _, ok := <-ch:
		require.False(t, ok, "channel should be closed")
	}

	// Close returns the result of the first close attempt. After a
	// server-initiated close the Chan goroutine typically wins and closes
	// with StatusGoingAway. The returned error may be nil (close frame
	// sent successfully) or a network error (TCP already torn down).
	err = dec.Close()
	// Calling Close again must return the same stored result.
	err2 := dec.Close()
	require.Equal(t, err, err2, "subsequent Close calls must return the same error")
}

func TestDecoder_ConcurrentClose(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = conn.CloseRead(r.Context())
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx := testutil.Context(t, testutil.WaitShort)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	// nolint: bodyclose
	conn, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)

	logger := slogtest.Make(t, nil)
	dec := wsjson.NewDecoder[string](conn, websocket.MessageText, logger)

	// Launch multiple goroutines calling Close simultaneously.
	// Under -race this validates the sync.Once protects conn.Close.
	const goroutines = 10
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			errs[i] = dec.Close()
		}()
	}
	wg.Wait()

	// All goroutines must observe the same error value.
	for i := 1; i < goroutines; i++ {
		require.Equal(t, errs[0], errs[i], "all concurrent Close calls must return the same error")
	}
}

func TestDecoder_CloseWithoutChan(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Keep connection open until client closes.
		_ = conn.CloseRead(r.Context())
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx := testutil.Context(t, testutil.WaitShort)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	// nolint: bodyclose
	conn, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)

	logger := slogtest.Make(t, nil)
	dec := wsjson.NewDecoder[string](conn, websocket.MessageText, logger)

	// Close without ever calling Chan.
	err = dec.Close()
	require.NoError(t, err)

	// Second close should also be safe.
	err = dec.Close()
	require.NoError(t, err)
}
