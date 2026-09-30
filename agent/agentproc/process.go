package agentproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

var (
	errProcessNotFound   = xerrors.New("process not found")
	errProcessNotRunning = xerrors.New("process is not running")

	// exitedProcessReapAge is how long an exited process is
	// kept before being automatically removed from the map, so
	// that a chatd retry of its execute tool call can still read
	// the output.
	exitedProcessReapAge = time.Hour
)

// process represents a running or completed process.
type process struct {
	mu         sync.Mutex
	id         string
	command    string
	workDir    string
	background bool
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	buf        *HeadTailBuffer
	logger     slog.Logger
	running    bool
	canceled   atomic.Bool // set if its tool call is canceled while it runs
	waitUntil  time.Time   // when TimeoutMs passes; zero if none
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

// procKey identifies a process by chat ID and process ID. chatID is
// uuid.Nil for processes started without a chat.
type procKey struct {
	chatID uuid.UUID
	id     string
}

// manager tracks processes spawned by the agent.
type manager struct {
	mu         sync.Mutex
	logger     slog.Logger
	execer     agentexec.Execer
	fs         afero.Fs
	clock      quartz.Clock
	procs      map[procKey]*process
	closed     bool
	updateEnv  func(current []string) (updated []string, err error)
	workingDir func() string
	envInfo    usershell.EnvInfoer
}

// newManager creates a new process manager.
func newManager(logger slog.Logger, execer agentexec.Execer, fs afero.Fs, envInfo usershell.EnvInfoer, updateEnv func(current []string) (updated []string, err error), workingDir func() string) *manager {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	if envInfo == nil {
		envInfo = &usershell.SystemEnvInfo{}
	}
	return &manager{
		logger:     logger,
		execer:     execer,
		fs:         fs,
		clock:      quartz.NewReal(),
		procs:      make(map[procKey]*process),
		updateEnv:  updateEnv,
		workingDir: workingDir,
		envInfo:    envInfo,
	}
}

// start spawns a new process. Both foreground and background
// processes use a long-lived context so the process survives
// the HTTP request lifecycle. The background flag only affects
// client-side polling behavior.
//
// A repeated start with the same chatID and id returns the existing
// process. The lookup and insert are not atomic; the tool call
// middleware serializes requests per tool call, and other starts get
// random IDs.
func (m *manager) start(req workspacesdk.StartProcessRequest, chatID uuid.UUID, id string) (*process, error) {
	k := procKey{chatID: chatID, id: id}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, xerrors.New("manager is closed")
	}
	if proc, ok := m.procs[k]; ok {
		m.mu.Unlock()
		return proc, nil
	}
	m.mu.Unlock()

	logger := m.logger
	if chatID != uuid.Nil {
		logger = logger.With(slog.F("chat_id", chatID.String()))
	}

	// Use a cancellable context so Close() can terminate
	// all processes. context.Background() is the parent so
	// the process is not tied to any HTTP request.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.execer.CommandContext(ctx, "sh", "-c", req.Command)
	cmd.Dir = m.resolveWorkingDirectory(req.WorkDir)
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
			logger.Warn(
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
	// Propagate the chat ID so child processes (e.g.
	// GIT_ASKPASS) can send it back to the server.
	if chatID != uuid.Nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CODER_CHAT_ID=%s", chatID))
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, xerrors.Errorf("start process: %w", err)
	}

	now := m.clock.Now()
	proc := &process{
		id:         id,
		command:    req.Command,
		workDir:    cmd.Dir,
		background: req.Background,
		cmd:        cmd,
		cancel:     cancel,
		buf:        buf,
		logger:     logger,
		running:    true,
		startedAt:  now.Unix(),
		done:       make(chan struct{}),
	}
	if req.TimeoutMs > 0 {
		proc.waitUntil = now.Add(time.Duration(req.TimeoutMs) * time.Millisecond)
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
	m.procs[k] = proc
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
				proc.logger.Warn(
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

// get returns chat chatID's process with ID id.
func (m *manager) get(chatID uuid.UUID, id string) (*process, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.procs[procKey{chatID: chatID, id: id}]
	return proc, ok
}

// list returns info about chat chatID's processes. It also reaps
// processes of all chats that exited more than exitedProcessReapAge ago.
func (m *manager) list(chatID uuid.UUID) []workspacesdk.ProcessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	infos := make([]workspacesdk.ProcessInfo, 0, len(m.procs))
	for k, proc := range m.procs {
		info := proc.info()
		// Reap processes that exited more than exitedProcessReapAge ago
		// to prevent unbounded map growth.
		if !info.Running && info.ExitedAt != nil {
			exitedAt := time.Unix(*info.ExitedAt, 0)
			if now.Sub(exitedAt) > exitedProcessReapAge {
				delete(m.procs, k)
				continue
			}
		}
		if k.chatID != chatID {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}

// signal sends a signal to a running process. It returns
// sentinel errors errProcessNotFound and errProcessNotRunning
// so callers can distinguish failure modes.
func (m *manager) signal(chatID uuid.UUID, id string, sig string) error {
	proc, ok := m.get(chatID, id)

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

// resolveWorkingDirectory returns the directory a process should start in.
// Priority: explicit request dir > agent configured dir > user home.
// The configured dir > home tail is shared with SSH sessions via
// usershell.ResolveWorkingDirectory so the two cannot drift.
func (m *manager) resolveWorkingDirectory(requested string) string {
	if requested != "" {
		return requested
	}
	var configured string
	if m.workingDir != nil {
		configured = m.workingDir()
	}
	dir, err := usershell.ResolveWorkingDirectory(m.fs, m.envInfo, configured)
	if err != nil {
		return ""
	}
	return dir
}
