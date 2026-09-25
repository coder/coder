package agentproc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentrunonce"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

var (
	errProcessNotFound   = xerrors.New("process not found")
	errProcessNotRunning = xerrors.New("process is not running")

	// exitedProcessReapAge is how long an exited process without
	// an idempotency key is kept before being automatically
	// removed from the map.
	exitedProcessReapAge = 5 * time.Minute

	// keyedProcessReapAge is how long a process started under an
	// idempotency key stays retrievable after it exits, so a retried
	// dispatch can still read the result.
	keyedProcessReapAge = 60 * time.Minute
)

// runOnceKey scopes an idempotency key to the chat that supplied it,
// so two chats reusing one key value get independent reservations.
type runOnceKey struct {
	chatID string
	key    string
}

// startFingerprint digests the request fields that decide what gets
// spawned. The workdir is taken as requested rather than resolved, so
// an identical retry attaches even when the default directory
// resolves differently between the two.
func startFingerprint(req workspacesdk.StartProcessRequest) string {
	h := sha256.New()
	// Length-prefix every field so no combination of values can
	// serialize to the same bytes as a different combination.
	write := func(values ...string) {
		for _, v := range values {
			_, _ = fmt.Fprintf(h, "%d:%s", len(v), v)
		}
	}
	write(req.Command, req.WorkDir, strconv.FormatBool(req.Background))
	for _, k := range slices.Sorted(maps.Keys(req.Env)) {
		write(k, req.Env[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// process represents a running or completed process.
type process struct {
	mu         sync.Mutex
	id         string
	command    string
	workDir    string
	background bool
	chatID     string
	// keyed marks a process started under an idempotency key. Reaping
	// leaves those to the registry, which retains them longer.
	keyed     bool
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	buf       *HeadTailBuffer
	logger    slog.Logger
	running   bool
	exitCode  *int
	startedAt int64
	exitedAt  *int64
	done      chan struct{} // closed when process exits
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
	mu     sync.Mutex
	logger slog.Logger
	execer agentexec.Execer
	fs     afero.Fs
	clock  quartz.Clock
	procs  map[string]*process
	// runOnce reserves idempotency keys so a retried start attaches
	// to the process the first one spawned. It holds process IDs.
	runOnce    *agentrunonce.Registry[runOnceKey, string]
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
		procs:      make(map[string]*process),
		runOnce:    agentrunonce.NewRegistry[runOnceKey, string](keyedProcessReapAge),
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
// A request whose idempotency key already started a process returns
// that process with attached true. Reusing a key within one chat with
// different parameters fails with agentrunonce.ErrInputMismatch. The
// context only bounds the wait for a concurrent start holding the
// same key; it does not cancel the spawned process.
func (m *manager) start(ctx context.Context, req workspacesdk.StartProcessRequest, chatID string) (*process, bool, error) {
	workDir := m.resolveWorkingDirectory(req.WorkDir)

	m.mu.Lock()
	m.reapExitedLocked(m.clock.Now())
	m.mu.Unlock()

	var (
		key      *runOnceKey
		reserved *agentrunonce.Ticket[runOnceKey, string]
	)
	if req.IdempotencyKey != "" {
		key = &runOnceKey{chatID: chatID, key: req.IdempotencyKey}
	}
	for key != nil {
		outcome, err := m.runOnce.Reserve(ctx, *key, startFingerprint(req))
		if err != nil {
			if errors.Is(err, agentrunonce.ErrClosed) {
				return nil, false, xerrors.New("manager is closed")
			}
			if errors.Is(err, agentrunonce.ErrPublicationPending) && m.isClosed() {
				return nil, false, xerrors.Errorf("manager is closed: %w", err)
			}
			return nil, false, err
		}
		if outcome.Ticket != nil {
			reserved = outcome.Ticket
			break
		}
		m.mu.Lock()
		existing, ok := m.procs[outcome.Value]
		m.mu.Unlock()
		if ok {
			return existing, true, nil
		}
		// Reaping removed the process after the reservation resolved.
		// Forget only the observed generation, then reserve again.
		m.runOnce.Forget(*key, outcome.Generation)
	}

	id := uuid.New().String()
	logger := m.logger
	if chatID != "" {
		logger = logger.With(slog.F("chat_id", chatID))
	}

	// Use a cancellable context so Close() can terminate
	// all processes. context.Background() is the parent so
	// the process is not tied to any HTTP request.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.execer.CommandContext(ctx, "sh", "-c", req.Command)
	cmd.Dir = workDir
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

	// Spawn, insert and publish in one critical section with the
	// closed check, so every spawned process is visible to Close's
	// kill pass and to every attaching caller.
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		releaseReservation(reserved)
		return nil, false, xerrors.New("manager is closed")
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		cancel()
		releaseReservation(reserved)
		return nil, false, xerrors.Errorf("start process: %w", err)
	}

	now := m.clock.Now().Unix()
	proc := &process{
		id:         id,
		command:    req.Command,
		workDir:    cmd.Dir,
		background: req.Background,
		chatID:     chatID,
		keyed:      key != nil,
		cmd:        cmd,
		cancel:     cancel,
		buf:        buf,
		logger:     logger,
		running:    true,
		startedAt:  now,
		done:       make(chan struct{}),
	}

	m.procs[id] = proc
	if reserved != nil {
		reserved.Publish(id)
	}
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

		// Retention runs from exit, so a retry can still read this
		// result until the reservation is reaped.
		if key != nil {
			m.runOnce.Complete(*key, m.clock.Now())
		}

		// Wake any waiters blocked on new output or
		// process exit before closing the done channel.
		proc.buf.Close()
		close(proc.done)
	}()

	return proc, false, nil
}

// releaseReservation frees a reservation that will never publish a
// process, so waiting callers reserve the key again.
func releaseReservation(reserved *agentrunonce.Ticket[runOnceKey, string]) {
	if reserved == nil {
		return
	}
	reserved.Release()
}

func (m *manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.closed
}

// get returns a process by ID.
func (m *manager) get(id string) (*process, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.procs[id]
	return proc, ok
}

// list returns info about all tracked processes, reaping exited ones
// past their retention age first. If chatID is non-empty, only
// processes belonging to that chat are returned.
func (m *manager) list(chatID string) []workspacesdk.ProcessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reapExitedLocked(m.clock.Now())

	infos := make([]workspacesdk.ProcessInfo, 0, len(m.procs))
	for _, proc := range m.procs {
		// Filter by chatID if provided.
		if chatID != "" && proc.chatID != chatID {
			continue
		}
		infos = append(infos, proc.info())
	}
	return infos
}

// reapExitedLocked removes exited processes past their retention age
// to prevent unbounded map growth. Keyed processes are retained by
// the reservation registry instead, and for longer. The manager mutex
// must be held.
func (m *manager) reapExitedLocked(now time.Time) {
	// Remove the reservation and process together so every retained
	// reservation still points to a tracked process.
	for _, id := range m.runOnce.Reap(now) {
		delete(m.procs, id)
	}
	for id, proc := range m.procs {
		if proc.keyed {
			continue
		}
		info := proc.info()
		if info.Running || info.ExitedAt == nil {
			continue
		}
		if now.Sub(time.Unix(*info.ExitedAt, 0)) <= exitedProcessReapAge {
			continue
		}
		delete(m.procs, id)
	}
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
	m.runOnce.Close()
	// The spawn path inserts under the same lock that checks closed,
	// so this snapshot is complete.
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
