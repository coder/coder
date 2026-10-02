package agentacp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/quartz"
)

type diagnosticBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if remaining := 64*1024 - b.buf.Len(); remaining > 0 {
		_, _ = b.buf.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}

func (b *diagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type process struct {
	writeMu       sync.Mutex
	promptWritten chan error
	clock         quartz.Clock
	conn          *acp.ClientSideConnection
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	cancel        context.CancelFunc
	done          chan struct{}
	updatesDone   <-chan struct{}
	err           error // Read only after done closes.
	once          sync.Once
	stderr        diagnosticBuffer
}

func (m *Manager) launch(ctx context.Context, cfg harnessConfig, handler acp.Client) (*process, error) {
	ctx, cancel := context.WithCancel(ctx)
	name, args := m.envInfo.ModifyCommand("sh", "-c", cfg.command)
	cmd := m.execer.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.directory
	env := m.envInfo.Environ()
	if m.updateEnv != nil {
		var err error
		env, err = m.updateEnv(env)
		if err != nil {
			cancel()
			return nil, xerrors.Errorf("prepare harness environment: %w", err)
		}
	}
	cmd.Env = env
	configureProcess(cmd)
	cmd.Cancel = sync.OnceValue(cmd.Cancel)
	cmd.WaitDelay = 2 * time.Second
	p := &process{cmd: cmd, cancel: cancel, done: make(chan struct{}), clock: m.clock}
	var err error
	p.stdin, err = cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	p.stdout, err = cmd.StdoutPipe()
	if err != nil {
		_ = p.stdin.Close()
		cancel()
		return nil, err
	}
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		_ = p.stdin.Close()
		_ = p.stdout.Close()
		cancel()
		return nil, xerrors.Errorf("start harness: %w", err)
	}
	p.conn, p.updatesDone = newClientConnection(handler, p, p.stdout)
	go func() {
		p.err = cmd.Wait()
		// The shell may exit before its descendants. CommandContext stops
		// watching cancellation after Wait, so terminate the group explicitly.
		_ = cmd.Cancel()
		close(p.done)
	}()
	return p, nil
}

func (p *process) close() {
	p.once.Do(func() { p.cancel(); _ = p.stdin.Close(); _ = p.stdout.Close(); <-p.done })
}

// Write observes prompt admission at the ACP transport boundary. The session's
// admission lock ensures cancel and steering cannot overtake the first prompt.
func (p *process) Write(raw []byte) (int, error) {
	// Closing the transport bounds the actual write, including calls whose
	// contexts the ACP SDK only checks after writing the request.
	timer := p.clock.AfterFunc(setupTimeout, func() {
		p.cancel()
		_ = p.stdin.Close()
		_ = p.stdout.Close()
	}, "acp-write")
	defer timer.Stop()
	n, err := p.stdin.Write(raw)
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	var message struct {
		Method string `json:"method"`
	}
	if json.Unmarshal(raw, &message) == nil && message.Method == "session/prompt" {
		p.writeMu.Lock()
		if p.promptWritten != nil {
			p.promptWritten <- err
			p.promptWritten = nil
		}
		p.writeMu.Unlock()
	}
	return n, err
}

func (p *process) expectPrompt() <-chan error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.promptWritten = make(chan error, 1)
	return p.promptWritten
}

// The SDK signals disconnect before draining notifications and exposes no
// drain hook. A local notification appended after input ends runs after all
// earlier notifications. Its random method name keeps it private to this
// connection. Read errors end input too, including pipes closed during cleanup.
func newClientConnection(handler acp.Client, input io.Writer, output io.Reader) (*acp.ClientSideConnection, <-chan struct{}) {
	method := "_coder/drain/" + uuid.NewString()
	client := &notificationClient{Client: handler, method: method, done: make(chan struct{})}
	marker := "\n{\"jsonrpc\":\"2.0\",\"method\":\"" + method + "\",\"params\":{}}\n"
	reader := io.MultiReader(endReader{output}, strings.NewReader(marker))
	return acp.NewClientSideConnection(client, input, reader), client.done
}

type endReader struct{ io.Reader }

func (r endReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil {
		err = io.EOF
	}
	return n, err
}

type notificationClient struct {
	acp.Client
	method string
	done   chan struct{}
	once   sync.Once
}

func (c *notificationClient) HandleExtensionMethod(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if method == c.method {
		c.once.Do(func() { close(c.done) })
		return nil, nil //nolint:nilnil // Drain notifications have no response payload.
	}
	if handler, ok := c.Client.(acp.ExtensionMethodHandler); ok {
		return handler.HandleExtensionMethod(ctx, method, params)
	}
	return nil, acp.NewMethodNotFound(method)
}
