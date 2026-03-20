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
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

var (
	errProcessNotFound   = xerrors.New("process not found")
	errProcessNotRunning = xerrors.New("process is not running")

	// exitedProcessReapAge is how long an exited process is
	// kept before being automatically removed from the map.
	exitedProcessReapAge = 5 * time.Minute
)

// process represents a running or completed process.
type process struct {
	mu         sync.Mutex
	id         string
	command    string
	workDir    string
	background bool
	chatID     string
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	buf        *HeadTailBuffer
	running    bool
	exitCode   *int
	startedAt  int64
	exitedAt   *int64
	done       chan struct{} // closed when process exits
}

// info returns a snapshot of the process state.
func (p *process) info() workspacesdk.ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	return workspacesdk.ProcessInfo{
		ID:         p.id,
		Command:    p.command,
		WorkDir:    p.workDir,
		Background: p.background,
		Running:    p.running,
		ExitCode:   p.exitCode,
		StartedAt:  p.startedAt,
		ExitedAt:   p.exitedAt,
	}
}

// output returns the truncated output from the process buffer
// along with optional truncation metadata.
func (p *process) output() (string, *workspacesdk.ProcessTruncation) {
	return p.buf.Output()
}

// manager tracks processes spawned by the agent.
type manager struct {
	mu         sync.Mutex
	logger     slog.Logger
	execer     agentexec.Execer
	clock      quartz.Clock
	procs      map[string]*process
	closed     bool
	updateEnv  func(current []string) (updated []string, err error)
	workingDir func() string
}

// newManager creates a new process manager.
func newManager(logger slog.Logger, execer agentexec.Execer, updateEnv func(current []string) (updated []string, err error), workingDir func() string) *manager {
	return &manager{
		logger:     logger,
		execer:     execer,
		clock:      quartz.NewReal(),
		procs:      make(map[string]*process),
		updateEnv:  updateEnv,
		workingDir: workingDir,
	}
}

// start spawns a new process. Both foreground and background
// processes use a long-lived context so the process survives
// the HTTP request lifecycle. The background flag only affects
// client-side polling behavior.
func (m *manager) start(req workspacesdk.StartProcessRequest, chatID string) (*process, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, xerrors.New("manager is closed")
	}
	m.mu.Unlock()

	id := uuid.New().String()

	// Use a cancellable context so Close() can terminate
	// all processes. context.Background() is the parent so
	// the process is not tied to any HTTP request.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.execer.CommandContext(ctx, "sh", "-c", req.Command)
	cmd.Dir = m.resolveWorkDir(req.WorkDir)
	cmd.Stdin = nil
	cmd.SysProcAttr = procSysProcAttr()

	// WaitDelay ensures cmd.Wait returns promptly after
	// the process is killed, even if child processes are
	// still holding the stdout/stderr pipes open.
	cmd.WaitDelay = 5 * time.Second

	buf := NewHeadTailBuffer()
	cmd.Stdout = buf
	cmd.Stderr = buf

	// Build the process environment. If the manager has an
	// updateEnv hook (provided by the agent), use it to get the
	// full agent environment including GIT_ASKPASS, CODER_* vars,
	// etc. Otherwise fall back to the current process env.
	baseEnv := os.Environ()
	if m.updateEnv != nil {
		updated, err := m.updateEnv(baseEnv)
		if err != nil {
			m.logger.Warn(
				context.Background(),
				"failed to update command environment, falling back to os env",
				slog.Error(err),
			)
		} else {
			baseEnv = updated
		}
	}

	// Always set cmd.Env explicitly so that req.Env overrides
	// are applied on top of the full agent environment.
	cmd.Env = baseEnv
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, xerrors.Errorf("start process: %w", err)
	}

	now := m.clock.Now().Unix()
	proc := &process{
		id:         id,
		command:    req.Command,
		workDir:    cmd.Dir,
		background: req.Background,
		chatID:     chatID,
		cmd:        cmd,
		cancel:     cancel,
		buf:        buf,
		running:    true,
		startedAt:  now,
		done:       make(chan struct{}),
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		// Manager closed between our check and now. Kill the
		// process we just started.
		cancel()
		_ = cmd.Wait()
		return nil, xerrors.New("manager is closed")
	}
	m.procs[id] = proc
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		exitedAt := m.clock.Now().Unix()

		proc.mu.Lock()
		proc.running = false
		proc.exitedAt = &exitedAt
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

		// Wake any waiters blocked on new output or
		// process exit before closing the done channel.
		proc.buf.Close()
		close(proc.done)
	}()

	return proc, nil
}

// get returns a process by ID.
func (m *manager) get(id string) (*process, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.procs[id]
	return proc, ok
}

// list returns info about all tracked processes. Exited
// processes older than exitedProcessReapAge are removed.
// If chatID is non-empty, only processes belonging to that
// chat are returned.
func (m *manager) list(chatID string) []workspacesdk.ProcessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	infos := make([]workspacesdk.ProcessInfo, 0, len(m.procs))
	for id, proc := range m.procs {
		info := proc.info()
		// Reap processes that exited more than 5 minutes ago
		// to prevent unbounded map growth.
		if !info.Running && info.ExitedAt != nil {
			exitedAt := time.Unix(*info.ExitedAt, 0)
			if now.Sub(exitedAt) > exitedProcessReapAge {
				delete(m.procs, id)
				continue
			}
		}
		// Filter by chatID if provided.
		if chatID != "" && proc.chatID != chatID {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}

// signal sends a signal to a running process. It returns
// sentinel errors errProcessNotFound and errProcessNotRunning
// so callers can distinguish failure modes.
func (m *manager) signal(id string, sig string) error {
	m.mu.Lock()
	proc, ok := m.procs[id]
	m.mu.Unlock()

	if !ok {
		return errProcessNotFound
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.running {
		return errProcessNotRunning
	}

	switch sig {
	case "kill":
		// Use process group kill to ensure child processes
		// (e.g. from shell pipelines) are also killed.
		if err := signalProcess(proc.cmd.Process, syscall.SIGKILL); err != nil {
			return xerrors.Errorf("kill process: %w", err)
		}
	case "terminate":
		// Use process group signal to ensure child processes
		// are also terminated.
		if err := signalProcess(proc.cmd.Process, syscall.SIGTERM); err != nil {
			return xerrors.Errorf("terminate process: %w", err)
		}
	default:
		return xerrors.Errorf("unsupported signal %q", sig)
	}

	return nil
}

// Close kills all running processes and prevents new ones from
// starting. It cancels each process's context, which causes
// CommandContext to kill the process and its pipe goroutines to
// drain.
func (m *manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	procs := make([]*process, 0, len(m.procs))
	for _, p := range m.procs {
		procs = append(procs, p)
	}
	m.mu.Unlock()

	for _, p := range procs {
		p.cancel()
	}

	// Wait for all processes to exit.
	for _, p := range procs {
		<-p.done
	}

	return nil
}

// waitForOutput blocks until the buffer is closed (process
// exited) or the context is canceled. Returns nil when the
// buffer closed, ctx.Err() when the context expired.
func (p *process) waitForOutput(ctx context.Context) error {
	p.buf.cond.L.Lock()
	defer p.buf.cond.L.Unlock()

	nevermind := make(chan struct{})
	defer close(nevermind)
	go func() {
		select {
		case <-ctx.Done():
			// Acquire the lock before broadcasting to
			// guarantee the waiter has entered cond.Wait()
			// (which atomically releases the lock).
			// Without this, a Broadcast between the loop
			// predicate check and cond.Wait() is lost.
			p.buf.cond.L.Lock()
			defer p.buf.cond.L.Unlock()
			p.buf.cond.Broadcast()
		case <-nevermind:
		}
	}()

	for ctx.Err() == nil && !p.buf.closed {
		p.buf.cond.Wait()
	}
	return ctx.Err()
}

// resolveWorkDir returns the directory a process should start in.
// Priority: explicit request dir > agent configured dir > $HOME.
// Falls through when a candidate is empty or does not exist on
// disk, matching the behavior of SSH sessions.
func (m *manager) resolveWorkDir(requested string) string {
	if requested != "" {
		return requested
	}
	if m.workingDir != nil {
		if dir := m.workingDir(); dir != "" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}
