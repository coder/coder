package immortalstreams_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/immortalstreams"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestImmortalStreamsHandler_CreateStream(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, nil)

		// Start a test server
		listener, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		defer listener.Close()

		port := listener.Addr().(*net.TCPAddr).Port

		// Accept connections in the background
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				go func() {
					defer conn.Close()
					_, _ = io.Copy(io.Discard, conn)
				}()
			}
		}()

		// Create handler
		dialer := &handlerTestDialer{}
		manager := immortalstreams.New(logger, dialer)
		defer manager.Close()

		handler := immortalstreams.NewHandler(logger, manager)
		router := chi.NewRouter()
		router.Mount("/api/v0/immortal-stream", handler.Routes())

		// Create request
		req := codersdk.CreateImmortalStreamRequest{
			TCPPort: port,
		}
		body, err := json.Marshal(req)
		require.NoError(t, err)

		// Make request
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v0/immortal-stream", bytes.NewReader(body))
		r = r.WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(w, r)

		// Check response
		assert.Equal(t, http.StatusCreated, w.Code)

		var stream codersdk.ImmortalStream
		err = json.Unmarshal(w.Body.Bytes(), &stream)
		require.NoError(t, err)

		assert.NotEmpty(t, stream.ID)
		assert.NotEmpty(t, stream.Name) // Name is generated randomly
		assert.Equal(t, port, stream.TCPPort)
		assert.False(t, stream.CreatedAt.IsZero())
		assert.False(t, stream.LastConnectionAt.IsZero())
		assert.Nil(t, stream.LastDisconnectionAt)
	})

	t.Run("ConnectionRefused", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, nil)

		// Create handler
		dialer := &handlerTestDialer{}
		manager := immortalstreams.New(logger, dialer)
		defer manager.Close()

		handler := immortalstreams.NewHandler(logger, manager)
		router := chi.NewRouter()
		router.Mount("/api/v0/immortal-stream", handler.Routes())

		// Create request with port that won't connect
		req := codersdk.CreateImmortalStreamRequest{
			TCPPort: 65535,
		}
		body, err := json.Marshal(req)
		require.NoError(t, err)

		// Make request
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v0/immortal-stream", bytes.NewReader(body))
		r = r.WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(w, r)

		// Check response
		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp codersdk.Response
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "The connection was refused.", resp.Message)
	})
}

func TestImmortalStreamsHandler_ListStreams(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	// Create handler
	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	handler := immortalstreams.NewHandler(logger, manager)
	router := chi.NewRouter()
	router.Mount("/api/v0/immortal-stream", handler.Routes())

	// Initially empty
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v0/immortal-stream", nil)
	r = r.WithContext(ctx)

	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var streams []codersdk.ImmortalStream
	err = json.Unmarshal(w.Body.Bytes(), &streams)
	require.NoError(t, err)
	assert.Empty(t, streams)

	// Create some streams
	stream1, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	stream2, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// List again
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/api/v0/immortal-stream", nil)
	r = r.WithContext(ctx)

	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &streams)
	require.NoError(t, err)
	assert.Len(t, streams, 2)

	// Check that both streams are in the list
	foundIDs := make(map[uuid.UUID]bool)
	for _, s := range streams {
		foundIDs[s.ID] = true
	}
	assert.True(t, foundIDs[stream1.ID])
	assert.True(t, foundIDs[stream2.ID])
}

func TestImmortalStreamsHandler_GetStream(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	// Create handler
	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	handler := immortalstreams.NewHandler(logger, manager)
	router := chi.NewRouter()
	router.Mount("/api/v0/immortal-stream", handler.Routes())

	// Create a stream
	stream, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// Get the stream
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/v0/immortal-stream/%s", stream.ID), nil)
	r = r.WithContext(ctx)

	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var gotStream codersdk.ImmortalStream
	err = json.Unmarshal(w.Body.Bytes(), &gotStream)
	require.NoError(t, err)

	assert.Equal(t, stream.ID, gotStream.ID)
	assert.Equal(t, stream.Name, gotStream.Name)
	assert.Equal(t, stream.TCPPort, gotStream.TCPPort)

	// Get non-existent stream
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", fmt.Sprintf("/api/v0/immortal-stream/%s", uuid.New()), nil)
	r = r.WithContext(ctx)

	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestImmortalStreamsHandler_DeleteStream(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	// Create handler
	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()

	handler := immortalstreams.NewHandler(logger, manager)
	router := chi.NewRouter()
	router.Mount("/api/v0/immortal-stream", handler.Routes())

	// Create a stream
	stream, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// Delete the stream
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v0/immortal-stream/%s", stream.ID), nil)
	r = r.WithContext(ctx)

	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify it's deleted
	_, ok := manager.GetStream(stream.ID)
	assert.False(t, ok)

	// Delete non-existent stream
	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", fmt.Sprintf("/api/v0/immortal-stream/%s", uuid.New()), nil)
	r = r.WithContext(ctx)

	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestImmortalStreamsHandler_RawUpgrade(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Start a test server providing echo on accepted connections
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	// Create handler and server
	dialer := &testDialer{}
	manager := immortalstreams.New(logger, dialer)
	defer manager.Close()
	handler := immortalstreams.NewHandler(logger, manager)
	server := httptest.NewServer(handler.Routes())
	defer server.Close()

	// Create a stream
	stream, err := manager.CreateStream(ctx, port)
	require.NoError(t, err)

	// Dial server and send raw HTTP/1.1 Upgrade request
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	c, err := net.Dial("tcp", u.Host)
	require.NoError(t, err)
	defer c.Close()

	req := fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: %s\r\nUpgrade: %s\r\nConnection: Upgrade\r\n%s: 0\r\n\r\n",
		stream.ID,
		u.Host,
		codersdk.UpgradeImmortalStream,
		codersdk.HeaderImmortalStreamSequenceNum,
	)
	_, err = c.Write([]byte(req))
	require.NoError(t, err)

	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(status, "HTTP/1.1 101 ") || strings.HasPrefix(status, "HTTP/1.0 101 "))

	// Read headers until blank line and find sequence header
	seenSeq := false
	for {
		line, rerr := br.ReadString('\n')
		require.NoError(t, rerr)
		if line == "\r\n" {
			break
		}
		if i := strings.IndexByte(line, ':'); i > 0 {
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(strings.TrimSuffix(line[i+1:], "\r\n"))
			if strings.EqualFold(k, codersdk.HeaderImmortalStreamSequenceNum) {
				_, _ = strconv.ParseUint(v, 10, 64)
				seenSeq = true
			}
		}
	}
	assert.True(t, seenSeq)

	// Echo round-trip over upgraded connection
	payload := []byte("hello world")
	_, err = c.Write(payload)
	require.NoError(t, err)
	buf := make([]byte, len(payload))
	_, err = io.ReadFull(br, buf)
	require.NoError(t, err)
	assert.Equal(t, payload, buf)
}

// Test helpers

type handlerTestDialer struct{}

func (*handlerTestDialer) DialContext(_ context.Context, address string) (net.Conn, error) {
	return net.Dial("tcp", address)
}
