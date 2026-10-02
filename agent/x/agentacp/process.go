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

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
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

type process struct {
	writeMu       sync.Mutex
	promptWritten chan struct{}
	conn          *acp.ClientSideConnection
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	cancel        context.CancelFunc
	done          chan struct{}
	err           error // Read only after done closes.
	once          sync.Once
	stderr        diagnosticBuffer
}

func (m *Manager) launch(ctx context.Context, directory string, cfg harnessConfig, handler acp.Client) (*process, error) {
	ctx, cancel := context.WithCancel(ctx)
	name, args := m.envInfo.ModifyCommand("sh", "-c", cfg.command)
	cmd := m.execer.CommandContext(ctx, name, args...)
	cmd.Dir = directory
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
	cmd.WaitDelay = 2 * time.Second
	p := &process{cmd: cmd, cancel: cancel, done: make(chan struct{})}
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
	go func() { p.err = cmd.Wait(); close(p.done) }()
	return p, nil
}

func (p *process) close() {
	p.once.Do(func() { p.cancel(); _ = p.stdin.Close(); _ = p.stdout.Close(); <-p.done })
}

// Write observes prompt admission at the ACP transport boundary. The session's
// admission lock ensures cancel and steering cannot overtake the first prompt.
func (p *process) Write(raw []byte) (int, error) {
	n, err := p.stdin.Write(raw)
	var message struct {
		Method string `json:"method"`
	}
	if err == nil && json.Unmarshal(raw, &message) == nil && message.Method == "session/prompt" {
		p.writeMu.Lock()
		if p.promptWritten != nil {
			close(p.promptWritten)
			p.promptWritten = nil
		}
		p.writeMu.Unlock()
	}
	return n, err
}

func (p *process) expectPrompt() <-chan struct{} {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.promptWritten = make(chan struct{})
	return p.promptWritten
}

func (p *process) failure(err error, cfg harnessConfig) error {
	p.stderr.mu.Lock()
	diagnostic := strings.TrimSpace(p.stderr.buf.String())
	p.stderr.mu.Unlock()
	diagnostic = strings.ReplaceAll(diagnostic, cfg.command, "[harness command]")
	if diagnostic == "" {
		return xerrors.Errorf("harness failed: %w", err)
	}
	return xerrors.Errorf("harness failed: %w: %s", err, diagnostic)
}
