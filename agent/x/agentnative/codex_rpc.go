package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

type codexRPC interface {
	call(context.Context, string, any, any) error
	notify(context.Context, string) error
	close() error
}

type codexRPCError struct {
	rejected      bool
	goalsDisabled bool
}

func (e *codexRPCError) Error() string {
	if e.rejected {
		return "native Codex request was rejected; inspect the native terminal"
	}
	return "native Codex RPC failed; outcome is unknown"
}

type codexRPCResponse struct {
	result json.RawMessage
	err    error
}
type codexWebsocketRPC struct {
	ws           *websocket.Conn
	clock        quartz.Clock
	onMessage    func(codexMessage)
	onDisconnect func()
	mu           sync.Mutex
	pending      map[string]chan codexRPCResponse
	done         chan struct{}
	once         sync.Once
}

// codexTimeout uses the same injectable clock for handshake, request, and
// readiness budgets. Canceling a waiter never implies canceling native work.
func codexTimeout(ctx context.Context, clock quartz.Clock, duration time.Duration, tag string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := clock.AfterFunc(duration, cancel, tag)
	return ctx, func() { timer.Stop(); cancel() }
}

type codexBoundRPC struct {
	codexRPC
	peer codexSocketPeer
}

func (c *Codex) dialRPC(ctx context.Context, onMessage func(codexMessage), onDisconnect func()) (codexRPC, error) {
	before, err := c.boundSocket(ctx)
	if err != nil {
		return nil, err
	}
	connection, err := c.dialProxy(ctx, onMessage, onDisconnect)
	if err != nil {
		return nil, err
	}
	after, err := c.boundSocket(ctx)
	if err != nil || before != after {
		_ = connection.close()
		return nil, errCodexOwnership
	}
	return &codexBoundRPC{codexRPC: connection, peer: after}, nil
}

func (c *Codex) dialProxy(ctx context.Context, onMessage func(codexMessage), onDisconnect func()) (codexRPC, error) {
	// This is the native byte proxy, not another app-server engine. Its lifetime
	// is independent of both the request and the dedicated native server.
	cmd, err := c.command(context.Background(), "app-server", "proxy", "--sock", filepath.Join(c.options.StateDir, "codex.sock"))
	if err != nil {
		return nil, err
	}
	cmd.Env = append([]string(nil), c.options.Environment...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, xerrors.New("create codex proxy input")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, xerrors.New("create codex proxy output")
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, xerrors.New("start codex byte proxy")
	}
	pipe := &codexPipeConn{reader: stdout, writer: stdin, stop: func() { _ = cmd.Process.Kill() }}
	go func() { _ = cmd.Wait(); _ = pipe.Close() }()
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) { return pipe, nil }}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return xerrors.New("codex proxy redirect is forbidden") }}
	ctx, cancel := codexTimeout(ctx, c.clock, 10*time.Second, "codex-handshake")
	defer cancel()
	ws, response, err := websocket.Dial(ctx, "ws://localhost/", &websocket.DialOptions{HTTPClient: client, CompressionMode: websocket.CompressionDisabled})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	transport.CloseIdleConnections()
	if err != nil {
		_ = pipe.Close()
		return nil, xerrors.New("connect codex byte proxy")
	}
	return newCodexWebsocketRPC(ws, c.clock, onMessage, onDisconnect), nil
}

func newCodexWebsocketRPC(ws *websocket.Conn, clock quartz.Clock, onMessage func(codexMessage), onDisconnect func()) *codexWebsocketRPC {
	ws.SetReadLimit(codexBufferLimit)
	r := &codexWebsocketRPC{ws: ws, clock: clock, onMessage: onMessage, onDisconnect: onDisconnect, pending: make(map[string]chan codexRPCResponse), done: make(chan struct{})}
	go r.read()
	return r
}

func (r *codexWebsocketRPC) fail() {
	r.once.Do(func() {
		close(r.done)
		_ = r.ws.CloseNow()
		r.onDisconnect()
	})
}

func (r *codexWebsocketRPC) close() error { r.fail(); return nil }

func codexRPCID(raw json.RawMessage) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return codexSafeID(text)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return false
	}
	value, err := strconv.ParseFloat(number.String(), 64)
	return err == nil && math.Abs(value) <= 9007199254740991 && math.Trunc(value) == value
}

func codexObject(raw json.RawMessage) bool { return len(raw) > 0 && raw[0] == '{' && json.Valid(raw) }

func (r *codexWebsocketRPC) read() {
	defer r.fail()
	for {
		kind, data, err := r.ws.Read(context.Background())
		if err != nil || kind != websocket.MessageText || !utf8.Valid(data) {
			return
		}
		var message codexMessage
		if !codexObject(data) || json.Unmarshal(data, &message) != nil {
			return
		}
		if message.Method != "" {
			if !codexSafeID(message.Method) || !codexObject(message.Params) || len(message.Result) != 0 || len(message.Error) != 0 {
				return
			}
			if len(message.ID) != 0 && !codexRPCID(message.ID) {
				return
			}
			r.onMessage(message)
			continue
		}
		if !codexRPCID(message.ID) || (len(message.Result) == 0) == (len(message.Error) == 0) {
			return
		}
		var failure struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		}
		if len(message.Error) != 0 && (!codexObject(message.Error) || json.Unmarshal(message.Error, &failure) != nil || failure.Code == nil) {
			return
		}
		r.mu.Lock()
		pending := r.pending[string(message.ID)]
		r.mu.Unlock()
		if pending == nil {
			continue
		}
		response := codexRPCResponse{result: message.Result}
		if failure.Code != nil {
			response.err = &codexRPCError{rejected: true, goalsDisabled: *failure.Code == -32600 && failure.Message == "goals feature is disabled"}
		}
		select {
		case pending <- response:
		default:
		}
	}
}

func (r *codexWebsocketRPC) call(ctx context.Context, method string, params, result any) error {
	switch method {
	case "initialize", "thread/start", "thread/read", "thread/resume", "thread/queue/list", "thread/goal/get", "turn/start", "turn/interrupt":
	default:
		return xerrors.Errorf("codex method %q: %w", method, ErrUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id := uuid.NewString()
	key, _ := json.Marshal(id)
	data, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil || len(data) > codexBufferLimit {
		return &codexRPCError{rejected: true}
	}
	pending := make(chan codexRPCResponse, 1)
	r.mu.Lock()
	if len(r.pending) >= 32 {
		r.mu.Unlock()
		return &codexRPCError{rejected: true}
	}
	r.pending[string(key)] = pending
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, string(key)); r.mu.Unlock() }()
	ctx, cancel := codexTimeout(ctx, r.clock, 10*time.Second, "codex-request")
	defer cancel()
	select {
	case <-r.done:
		return &codexRPCError{}
	default:
	}
	if err := r.ws.Write(ctx, websocket.MessageText, data); err != nil {
		r.fail()
		return &codexRPCError{}
	}
	select {
	case <-ctx.Done():
		r.fail()
		return &codexRPCError{}
	case <-r.done:
		return &codexRPCError{}
	case response := <-pending:
		if response.err != nil {
			var failure *codexRPCError
			if method != "thread/goal/get" && errors.As(response.err, &failure) {
				failure.goalsDisabled = false
			}
			return response.err
		}
		if !codexObject(response.result) || json.Unmarshal(response.result, result) != nil {
			r.fail()
			return &codexRPCError{}
		}
		return nil
	}
}

func (r *codexWebsocketRPC) notify(ctx context.Context, method string) error {
	if method != "initialized" {
		return ErrUnsupported
	}
	ctx, cancel := codexTimeout(ctx, r.clock, 10*time.Second, "codex-notify")
	defer cancel()
	if err := r.ws.Write(ctx, websocket.MessageText, []byte(`{"method":"initialized"}`)); err != nil {
		r.fail()
		return &codexRPCError{}
	}
	return nil
}

// The HTTP upgrade runs over the native proxy's stdin/stdout. Context
// cancellation closes the pipes; pipe descriptors do not support net deadlines.
type codexPipeConn struct {
	reader io.ReadCloser
	writer io.WriteCloser
	stop   func()
	once   sync.Once
}

func (p *codexPipeConn) Read(b []byte) (int, error)  { return p.reader.Read(b) }
func (p *codexPipeConn) Write(b []byte) (int, error) { return p.writer.Write(b) }
func (p *codexPipeConn) Close() error {
	p.once.Do(func() { _ = p.reader.Close(); _ = p.writer.Close(); p.stop() })
	return nil
}
func (*codexPipeConn) LocalAddr() net.Addr              { return &net.UnixAddr{Name: "codex-proxy", Net: "unix"} }
func (*codexPipeConn) RemoteAddr() net.Addr             { return &net.UnixAddr{Name: "codex-server", Net: "unix"} }
func (*codexPipeConn) SetDeadline(time.Time) error      { return nil }
func (*codexPipeConn) SetReadDeadline(time.Time) error  { return nil }
func (*codexPipeConn) SetWriteDeadline(time.Time) error { return nil }
