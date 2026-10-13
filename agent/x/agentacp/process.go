package agentacp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"sync"
	"time"

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
	writeMu        sync.Mutex
	requestWritten chan error
	requestMethod  string
	clock          quartz.Clock
	conn           *acp.ClientSideConnection
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	cancel         context.CancelFunc
	done           chan struct{}
	err            error // Read only after done closes.
	once           sync.Once
	stderr         diagnosticBuffer
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
	p.conn = acp.NewClientSideConnection(handler, p, p.stdout)
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

// Write observes request admission at the ACP transport boundary. The session
// loop waits for this write before accepting another operation.
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
	if json.Unmarshal(raw, &message) == nil && message.Method != "" {
		p.writeMu.Lock()
		if p.requestWritten != nil && message.Method == p.requestMethod {
			p.requestWritten <- err
			p.requestWritten = nil
		}
		p.writeMu.Unlock()
	}
	return n, err
}

func (p *process) expectWrite(method string) <-chan error {
	p.writeMu.Lock()
	p.requestMethod = method
	defer p.writeMu.Unlock()
	p.requestWritten = make(chan error, 1)
	return p.requestWritten
}
