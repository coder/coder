package wsjson_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Read the message.
	msg := <-ch
	require.Equal(t, "hello", msg)

	// Wait for the channel to close (server closed the connection).
	_, ok := <-ch
	require.False(t, ok, "channel should be closed")

	// Close returns the result of the first close (from the Chan goroutine).
	// The Chan goroutine may have closed the websocket successfully, or the
	// server may have already torn down the connection, making the close a
	// no-op. Either way, calling Close() must not panic and must be safe.
	_ = dec.Close()
}

func TestDecoder_CloseReturnsError(t *testing.T) {
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

	// Close the raw websocket directly, bypassing the decoder's sync.Once.
	// This simulates the connection being torn down externally.
	_ = conn.Close(websocket.StatusGoingAway, "")

	// dec.Close() must surface the error from conn.Close(), not swallow it.
	err = dec.Close()
	require.Error(t, err)

	// Calling Close again must return the same stored error.
	err2 := dec.Close()
	require.Equal(t, err, err2, "subsequent Close calls must return the same error")
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
