package agentacp

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
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
	conn   *acp.ClientSideConnection
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cancel context.CancelFunc
	done   chan struct{}
	err    error // Read only after done closes.
	once   sync.Once
	stderr diagnosticBuffer
}

type launchOptions struct {
	execer  agentexec.Execer
	envInfo usershell.EnvInfoer
	env     []string
}

func launch(ctx context.Context, cfg harnessConfig, directory string, handler acp.Client, opts launchOptions) (*process, error) {
	ctx, cancel := context.WithCancel(ctx)
	name, args := opts.envInfo.ModifyCommand("sh", "-c", cfg.command)
	cmd := opts.execer.CommandContext(ctx, name, args...)
	cmd.Dir = directory
	cmd.Env = opts.env
	configureProcess(cmd)
	cmd.Cancel = sync.OnceValue(cmd.Cancel)
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
	p.conn = acp.NewClientSideConnection(handler, p.stdin, p.stdout)
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
