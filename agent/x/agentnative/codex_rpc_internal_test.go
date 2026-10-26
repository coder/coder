package agentnative

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
	"github.com/coder/websocket"

	"github.com/coder/coder/v2/testutil"
)

func TestCodexRPCWireOrderingAndRefusal(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	clock := quartz.NewMock(t)
	var observed []codexMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ws, err := websocket.Accept(w, request, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var input codexMessage
		if json.Unmarshal(data, &input) != nil {
			return
		}
		for _, message := range []any{
			map[string]any{"id": "approval", "method": "item/tool/requestUserInput", "params": map[string]any{"threadId": "thread", "turnId": "turn"}},
			map[string]any{"method": "turn/started", "params": map[string]any{"threadId": "thread"}},
			map[string]any{"id": input.ID, "error": map[string]any{"code": -32600, "message": "goals feature is disabled"}},
		} {
			data, _ := json.Marshal(message)
			if ws.Write(ctx, websocket.MessageText, data) != nil {
				return
			}
		}
		_, _, _ = ws.Read(ctx)
	}))
	defer server.Close()
	ws, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	rpc := newCodexWebsocketRPC(ws, clock, func(message codexMessage) { observed = append(observed, message) }, func() {})
	defer rpc.close()
	var output map[string]any
	err = rpc.call(ctx, "thread/goal/get", map[string]any{"threadId": "thread"}, &output)
	var failure *codexRPCError
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.rejected)
	require.True(t, failure.goalsDisabled)
	require.Len(t, observed, 2)
	require.Equal(t, "item/tool/requestUserInput", observed[0].Method)
	require.Equal(t, "turn/started", observed[1].Method)
}

func TestCodexRPCTimeoutDisconnectsWithoutRetry(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	clock := quartz.NewMock(t)
	requestRead := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ws, err := websocket.Accept(w, request, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		if _, _, err := ws.Read(ctx); err != nil {
			return
		}
		close(requestRead)
		_, _, _ = ws.Read(ctx)
	}))
	defer server.Close()
	ws, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	disconnected := make(chan struct{})
	rpc := newCodexWebsocketRPC(ws, clock, func(codexMessage) {}, func() { close(disconnected) })
	defer rpc.close()
	trap := clock.Trap().AfterFunc("codex-request")
	defer trap.Close()
	done := make(chan error, 1)
	go func() {
		var result any
		done <- rpc.call(ctx, "turn/start", map[string]any{"threadId": "thread"}, &result)
	}()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	select {
	case <-requestRead:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	clock.Advance(10 * time.Second).MustWait(ctx)
	select {
	case err := <-done:
		var failure *codexRPCError
		require.ErrorAs(t, err, &failure)
		require.False(t, failure.rejected)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-disconnected:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestCodexRPCRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"binary", "both-result-and-error", "missing-params", "bad-error-code"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ctx := codexTestContext(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				ws, err := websocket.Accept(w, request, nil)
				if err != nil {
					return
				}
				defer ws.CloseNow()
				_, data, err := ws.Read(ctx)
				if err != nil {
					return
				}
				var call codexMessage
				if json.Unmarshal(data, &call) != nil {
					return
				}
				kind := websocket.MessageText
				var response any
				switch scenario {
				case "binary":
					kind = websocket.MessageBinary
					response = map[string]any{"id": call.ID, "result": map[string]any{}}
				case "both-result-and-error":
					response = map[string]any{"id": call.ID, "result": map[string]any{}, "error": map[string]any{"code": -1}}
				case "missing-params":
					response = map[string]any{"method": "turn/started"}
				case "bad-error-code":
					response = map[string]any{"id": call.ID, "error": map[string]any{"code": "secret-diagnostic"}}
				}
				data, _ = json.Marshal(response)
				_ = ws.Write(ctx, kind, data)
				_, _, _ = ws.Read(ctx)
			}))
			defer server.Close()
			ws, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if response != nil && response.Body != nil {
				defer response.Body.Close()
			}
			require.NoError(t, err)
			rpc := newCodexWebsocketRPC(ws, quartz.NewMock(t), func(codexMessage) {}, func() {})
			defer rpc.close()
			var result any
			err = rpc.call(ctx, "thread/read", map[string]any{}, &result)
			var failure *codexRPCError
			require.ErrorAs(t, err, &failure)
			require.False(t, failure.rejected)
			require.NotContains(t, err.Error(), "secret-diagnostic")
		})
	}
}

// The helper forwards bytes only. This exercises the real HTTP/WebSocket
// upgrade over process pipes without requiring an installed Codex CLI.
//
//nolint:paralleltest,revive // Subprocess mode owns stdio and must exit before the test runner prints to it.
func TestCodexProxyHelperProcess(t *testing.T) {
	if os.Getenv("CODEX_TEST_PROXY") != "1" {
		t.Parallel()
		return
	}
	socket := os.Args[len(os.Args)-1]
	conn, err := net.Dial("unix", socket)
	if err != nil {
		os.Exit(2)
	}
	go func() { _, _ = io.Copy(conn, os.Stdin); _ = conn.Close() }()
	_, _ = io.Copy(os.Stdout, conn)
	_ = conn.Close()
	os.Exit(0)
}

func TestCodexNativeProxyOverUnixSocket(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	// Unix socket paths have a small platform limit; avoid the long test name.
	dir, err := os.MkdirTemp("", "codex-proxy-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "codex.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	var mu sync.Mutex
	var methods []string
	server := &http.Server{ReadHeaderTimeout: testutil.WaitLong, Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ws, err := websocket.Accept(w, request, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			var message codexMessage
			if json.Unmarshal(data, &message) != nil {
				return
			}
			mu.Lock()
			methods = append(methods, message.Method)
			mu.Unlock()
			if message.Method == "initialized" {
				continue
			}
			result := map[string]any{"userAgent": "codex/0.164.0", "platformFamily": "unix", "codexHome": dir}
			data, _ = json.Marshal(map[string]any{"id": message.ID, "result": result})
			if ws.Write(ctx, websocket.MessageText, data) != nil {
				return
			}
		}
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	host := &codexTestHost{sessions: map[string]Session{}, events: map[string][]Event{}}
	var proxy *exec.Cmd
	c, err := NewCodex(BackendOptions{StateDir: dir, Host: host, Environment: append(os.Environ(), "CODEX_TEST_PROXY=1"), Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
		require.Equal(t, "codex", name)
		require.Equal(t, []string{"app-server", "proxy", "--sock", socket}, args)
		// #nosec G204 -- The test reexecutes its own binary with an isolated local socket.
		proxy = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexProxyHelperProcess$", "--", socket)
		return proxy
	}})
	require.NoError(t, err)
	defer c.Close()
	// This fixture exercises the byte proxy transport. Ownership is covered by
	// the process lifecycle tests and the installed CLI smoke test.
	c.dial = c.dialProxy
	c.lifecycle.Lock()
	connection, err := c.ensure(ctx, false)
	c.lifecycle.Unlock()
	require.NoError(t, err)
	require.NotNil(t, connection)
	require.NoError(t, c.Close())
	// The HTTP server still accepts connections after the disposable proxy exits.
	probe, err := net.Dial("unix", socket)
	require.NoError(t, err)
	require.NoError(t, probe.Close())
	require.NotNil(t, proxy)
	mu.Lock()
	require.Contains(t, methods, "initialize")
	mu.Unlock()
}
