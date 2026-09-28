package agentproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

var (
	errProcessNotFound   = xerrors.New("process not found")
	errProcessNotRunning = xerrors.New("process is not running")

	// exitedProcessReapAge is how long an exited process without
	// a tool call is kept before being automatically removed from
	// the map.
	exitedProcessReapAge = 5 * time.Minute

	// sweepInterval is how often the manager evicts idle tool call
	// records and reaps exited processes.
	sweepInterval = time.Minute
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
	logger     slog.Logger
	running    bool
	exitCode   *int
	exitedAt   *int64
	done       chan struct{} // closed when process exits
	// startTime and exitTime come from the manager clock. The
	// run time reported as duration_ms is measured between them.
	startTime time.Time
	exitTime  time.Time
	// deadline is the execute deadline, zero when the start had no
	// timeout_ms.
	deadline time.Time
	// toolCall is the tool call that started the process, nil for
	// a start without tool call headers.
	toolCall *agenttoolcall.Key
	// canceled is set when a tool call cancel killed the process.
	canceled bool
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
		StartedAt:  p.startTime.Unix(),
		ExitedAt:   p.exitedAt,
	}
}

// runState reports whether p ran past its execute deadline while
// running, whether a cancel killed it, and how long it ran. info is the
// caller's snapshot of p, so the answer agrees with it.
func (p *process) runState(info workspacesdk.ProcessInfo, now time.Time) (timedOut, canceled bool, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	end := now
	if !info.Running {
		end = p.exitTime
	}
	timedOut = info.Running && !p.deadline.IsZero() && !now.Before(p.deadline)
	return timedOut, p.canceled, end.Sub(p.startTime)
}

// cancelToolCall is the cancel hook of a tool call process. It kills
// the process group on Unix and the process on Windows, and marks the
// process canceled only when the kill succeeded. A process that already
// exited is left as is.
func (p *process) cancelToolCall() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}
	err := signalProcess(p.cmd.Process, syscall.SIGKILL)
	switch {
	case err == nil:
		p.canceled = true
		return nil
	case errors.Is(err, syscall.ESRCH), errors.Is(err, os.ErrProcessDone):
		// The process exited before the exit goroutine marked it
		// as not running; done closes shortly.
		return nil
	default:
		return xerrors.Errorf("kill process: %w", err)
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
	fs         afero.Fs
	clock      quartz.Clock
	procs      map[string]*process
	closed     bool
	updateEnv  func(current []string) (updated []string, err error)
	workingDir func() string
	envInfo    usershell.EnvInfoer
	// toolCallStore decides when exited tool call processes are
	// reaped. It may be nil.
	toolCallStore *agenttoolcall.Store
	stopSweep     context.CancelFunc
	sweepDone     quartz.Waiter
}

// newManager creates a new process manager and starts its periodic
// sweep on clock.
func newManager(logger slog.Logger, execer agentexec.Execer, fs afero.Fs, envInfo usershell.EnvInfoer, updateEnv func(current []string) (updated []string, err error), workingDir func() string, clock quartz.Clock, toolCallStore *agenttoolcall.Store) *manager {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	if envInfo == nil {
		envInfo = &usershell.SystemEnvInfo{}
	}
	m := &manager{
		logger:        logger,
		execer:        execer,
		fs:            fs,
		clock:         clock,
		procs:         make(map[string]*process),
		updateEnv:     updateEnv,
		workingDir:    workingDir,
		envInfo:       envInfo,
		toolCallStore: toolCallStore,
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stopSweep = cancel
	m.sweepDone = clock.TickerFunc(ctx, sweepInterval, func() error {
		m.sweep()
		return nil
	}, "agentproc", "sweep")
	return m
}

// sweep evicts idle tool call records, then reaps the exited processes
// they no longer retain. Without the sweep, reaping would run only when
// processes are listed.
func (m *manager) sweep() {
	if m.toolCallStore != nil {
		m.toolCallStore.Sweep()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reapLocked()
}

// reapLocked removes exited processes from the map. A process started
// by a tool call is kept while the store retains the tool call's record,
// so a repeated start or an output read finds it. Other processes are
// kept for exitedProcessReapAge after they exit. m.mu must be held.
func (m *manager) reapLocked() {
	now := m.clock.Now()
	for id, proc := range m.procs {
		proc.mu.Lock()
		running, exitTime, key := proc.running, proc.exitTime, proc.toolCall
		proc.mu.Unlock()
		if running {
			continue
		}
		var reap bool
		if key != nil && m.toolCallStore != nil {
			reap = !m.toolCallStore.Retained(*key)
		} else {
			reap = now.Sub(exitTime) > exitedProcessReapAge
		}
		if reap {
			delete(m.procs, id)
		}
	}
}

// start spawns a new process. With toolCall set, the process ID is the
// tool call UUID and the tool call's cancel hook kills the process;
// otherwise the ID is random. Both foreground and background
// processes use a long-lived context so the process survives
// the HTTP request lifecycle. The background flag only affects
// client-side polling behavior.
func (m *manager) start(req workspacesdk.StartProcessRequest, chatID string, toolCall *agenttoolcall.ToolCall) (*process, error) {
	id := uuid.New().String()
	var key *agenttoolcall.Key
	if toolCall != nil {
		id = toolCall.UUID.String()
		key = &toolCall.Key
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, xerrors.New("manager is closed")
	}
	// The store runs a tool call's start once, so an existing
	// process with the same ID means the store lost its record.
	if _, ok := m.procs[id]; ok {
		m.mu.Unlock()
		return nil, xerrors.Errorf("process %q already exists", id)
	}
	m.mu.Unlock()

	logger := m.logger
	if chatID != "" {
		logger = logger.With(slog.F("chat_id", chatID))
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
	if chatID != "" {
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
		chatID:     chatID,
		cmd:        cmd,
		cancel:     cancel,
		buf:        buf,
		logger:     logger,
		running:    true,
		startTime:  now,
		toolCall:   key,
		done:       make(chan struct{}),
	}
	if req.TimeoutMs > 0 {
		proc.deadline = now.Add(time.Duration(req.TimeoutMs) * time.Millisecond)
	}

	m.mu.Lock()
	_, exists := m.procs[id]
	if m.closed || exists {
		m.mu.Unlock()
		// Manager closed or the ID was taken between our check
		// and now. Kill the process we just started.
		cancel()
		_ = cmd.Wait()
		if exists {
			return nil, xerrors.Errorf("process %q already exists", id)
		}
		return nil, xerrors.New("manager is closed")
	}
	m.procs[id] = proc
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		exitTime := m.clock.Now()
		exitedAt := exitTime.Unix()

		proc.mu.Lock()
		proc.running = false
		proc.exitTime = exitTime
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

	if toolCall != nil {
		toolCall.OnCancel(proc.cancelToolCall, proc.done)
	}

	return proc, nil
}

// get returns a process by ID.
func (m *manager) get(id string) (*process, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.procs[id]
	return proc, ok
}

// list returns info about all tracked processes after reaping
// exited ones. If chatID is non-empty, only processes belonging
// to that chat are returned.
func (m *manager) list(chatID string) []workspacesdk.ProcessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reapLocked()
	infos := make([]workspacesdk.ProcessInfo, 0, len(m.procs))
	for _, proc := range m.procs {
		info := proc.info()
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

// Close stops the periodic sweep, kills all running processes, and
// prevents new ones from starting. It cancels each process's
// context, which causes CommandContext to kill the process and its
// pipe goroutines to drain.
func (m *manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()

	// The sweep takes m.mu, so wait for it without holding the lock.
	m.stopSweep()
	_ = m.sweepDone.Wait()

	m.mu.Lock()
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

// waitForExit blocks until p exits, its execute deadline passes on the
// manager clock, maxWait passes on the manager clock, or ctx ends.
func (m *manager) waitForExit(ctx context.Context, p *process, maxWait time.Duration) {
	p.mu.Lock()
	deadline := p.deadline
	p.mu.Unlock()

	wait := maxWait
	if !deadline.IsZero() {
		wait = min(wait, deadline.Sub(m.clock.Now()))
	}
	if wait <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := m.clock.AfterFunc(wait, cancel, "agentproc", "wait")
	defer timer.Stop()
	_ = p.waitForOutput(ctx)
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
