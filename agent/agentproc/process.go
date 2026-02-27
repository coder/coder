package agentproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
)

// Process represents a running or completed process.
type Process struct {
	mu         sync.Mutex
	id         string
	command    string
	workDir    string
	background bool
	cmd        *exec.Cmd
	buf        *HeadTailBuffer
	running    bool
	exitCode   *int
	startedAt  time.Time
	exitedAt   *time.Time
	done       chan struct{} // closed when process exits
}

// Info returns a snapshot of the process state.
func (p *Process) Info() ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	info := ProcessInfo{
		ID:         p.id,
		Command:    p.command,
		WorkDir:    p.workDir,
		Background: p.background,
		Running:    p.running,
		ExitCode:   p.exitCode,
		StartedAt:  p.startedAt.Unix(),
	}
	if p.exitedAt != nil {
		unix := p.exitedAt.Unix()
		info.ExitedAt = &unix
	}
	return info
}

// Output returns the truncated output from the process buffer
// along with optional truncation metadata.
func (p *Process) Output() (string, *TruncationInfo) {
	return p.buf.Output()
}

// Wait blocks until the process exits or the context is
// canceled.
func (p *Process) Wait(ctx context.Context) error {
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Manager tracks processes spawned by the agent.
type Manager struct {
	mu     sync.Mutex
	logger slog.Logger
	execer agentexec.Execer
	procs  map[string]*Process
}

// NewManager creates a new process manager.
func NewManager(logger slog.Logger, execer agentexec.Execer) *Manager {
	return &Manager{
		logger: logger,
		execer: execer,
		procs:  make(map[string]*Process),
	}
}

// Start spawns a new process. For both foreground and
// background processes, it returns immediately. The caller
// can poll via Output(). Background processes use
// context.Background() so they survive the HTTP request.
func (m *Manager) Start(ctx context.Context, req StartProcessRequest) (*Process, error) {
	id := uuid.New().String()

	// Background processes must not be tied to the HTTP
	// request context, otherwise they die when the
	// request completes.
	cmdCtx := ctx
	if req.Background {
		cmdCtx = context.Background()
	}

	cmd := m.execer.CommandContext(cmdCtx, "sh", "-c", req.Command)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	cmd.Stdin = nil

	buf := NewHeadTailBuffer()
	cmd.Stdout = buf
	cmd.Stderr = buf

	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, xerrors.Errorf("start process: %w", err)
	}

	proc := &Process{
		id:         id,
		command:    req.Command,
		workDir:    req.WorkDir,
		background: req.Background,
		cmd:        cmd,
		buf:        buf,
		running:    true,
		startedAt:  time.Now(),
		done:       make(chan struct{}),
	}

	m.mu.Lock()
	m.procs[id] = proc
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		now := time.Now()

		proc.mu.Lock()
		proc.running = false
		proc.exitedAt = &now
		code := 0
		if err != nil {
			// Extract the exit code from the error.
			var exitErr *exec.ExitError
			if xerrors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			} else {
				// Unknown error; use -1 as a sentinel.
				code = -1
				m.logger.Warn(
					context.Background(),
					"process wait returned non-exit error",
					slog.F("id", id),
					slog.Error(err),
				)
			}
		}
		proc.exitCode = &code
		proc.mu.Unlock()

		close(proc.done)
	}()

	return proc, nil
}

// Get returns a process by ID.
func (m *Manager) Get(id string) (*Process, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.procs[id]
	return proc, ok
}

// List returns info about all tracked processes.
func (m *Manager) List() []ProcessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	infos := make([]ProcessInfo, 0, len(m.procs))
	for _, proc := range m.procs {
		infos = append(infos, proc.Info())
	}
	return infos
}

// Signal sends a signal to a running process.
func (m *Manager) Signal(id string, signal string) error {
	m.mu.Lock()
	proc, ok := m.procs[id]
	m.mu.Unlock()

	if !ok {
		return xerrors.Errorf("process %q not found", id)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.running {
		return xerrors.Errorf(
			"process %q is not running", id,
		)
	}

	switch signal {
	case "kill":
		if err := proc.cmd.Process.Kill(); err != nil {
			return xerrors.Errorf("kill process: %w", err)
		}
	case "terminate":
		//nolint:revive // syscall.SIGTERM is portable enough
		// for our supported platforms.
		if err := proc.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return xerrors.Errorf("terminate process: %w", err)
		}
	default:
		return xerrors.Errorf("unsupported signal %q", signal)
	}

	return nil
}
