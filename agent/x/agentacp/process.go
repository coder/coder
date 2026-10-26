package agentacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/quartz"
)

const (
	diagnosticHalfSize = 32 * 1024
	stdinWriteTimeout  = 30 * time.Second
)

type diagnosticBuffer struct {
	mu  sync.Mutex
	buf *agentproc.HeadTailBuffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf == nil {
		b.buf = agentproc.NewHeadTailBufferSized(diagnosticHalfSize, diagnosticHalfSize)
	}
	return b.buf.Write(p)
}

func (b *diagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf == nil {
		return ""
	}
	// Output also truncates long lines, which may hide diagnostic details.
	output := b.buf.Bytes()
	if omitted := b.buf.TotalWritten() - len(output); omitted > 0 {
		return fmt.Sprintf("%s\n... [omitted %d bytes] ...\n%s", output[:diagnosticHalfSize], omitted, output[diagnosticHalfSize:])
	}
	return string(output)
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

type launchOptions struct {
	clock   quartz.Clock
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
	p := &process{cmd: cmd, cancel: cancel, done: make(chan struct{}), clock: opts.clock}
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
		cmd.Cancel() //nolint:errcheck // The process group may already be gone.
		close(p.done)
	}()
	return p, nil
}

func (p *process) close() {
	p.once.Do(func() {
		p.cancel()
		_ = p.stdin.Close()
		_ = p.stdout.Close()
		<-p.done
	})
}

// Write observes request admission at the ACP transport boundary. The FSM
// holds its mutex until this write finishes before accepting another operation.
func (p *process) Write(raw []byte) (int, error) {
	// Closing the transport bounds the actual write, including calls whose
	// contexts the ACP SDK only checks after writing the request.
	timedOut := make(chan struct{})
	timer := p.clock.AfterFunc(stdinWriteTimeout, func() {
		defer close(timedOut)
		p.cancel()
		_ = p.stdin.Close()
		_ = p.stdout.Close()
	}, "acp-write")
	n, err := p.stdin.Write(raw)
	// Stop before admission processing, which can wait on writeMu. If the
	// timeout already fired, wait for transport closure before reporting it.
	if !timer.Stop() {
		<-timedOut
		err = errors.Join(err, context.DeadlineExceeded)
	}
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

// watchNextWrite registers a one-shot notification for the next write of an ACP
// message with the given method. The channel receives the write error, or nil
// on success. Only one watch may be pending at a time.
func (p *process) watchNextWrite(method string) <-chan error {
	p.writeMu.Lock()
	p.requestMethod = method
	defer p.writeMu.Unlock()
	p.requestWritten = make(chan error, 1)
	return p.requestWritten
}
