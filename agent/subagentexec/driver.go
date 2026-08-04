package subagentexec

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// ProtocolVersion is the driver protocol version this launcher speaks. A
// declaration that asks for another version is rejected rather than
// invoked against a contract the launcher does not implement.
const ProtocolVersion int32 = 1

// ErrUnsupportedPlatform is returned when the concrete driver cannot run
// on this platform. The execution isolation proof of concept targets
// Linux.
var ErrUnsupportedPlatform = xerrors.New("subagent execution drivers are only supported on unix platforms")

// procSignal is the platform-independent signal the launcher sends to a
// driver's process group.
type procSignal int

const (
	signalTerminate procSignal = iota
	signalKill
)

// Operation is the driver operation the launcher requests. The driver
// receives it as its first argument.
type Operation string

const (
	// OperationRun starts the execution in the foreground. The driver is
	// expected to keep running for as long as the child agent runs.
	OperationRun Operation = "run"
	// OperationCleanup releases whatever run left behind. It must be
	// idempotent: the launcher invokes it after the run ends, and again
	// after a stop it initiated itself.
	OperationCleanup Operation = "cleanup"
)

// Names inside one execution's private state directory. The directory
// itself is named after the execution's UUID, never after the declared
// name, so a declaration cannot influence the layout.
const (
	driverFileName       = "driver"
	tokenFileName        = "token"
	runInputFileName     = "run.json"
	cleanupInputFileName = "cleanup.json"
	homeDirName          = "home"
	tmpDirName           = "tmp"
	runtimeDirName       = "run"
)

const (
	// privateDirMode is the mode of the state root, the per-agent
	// directory, the per-execution directory, and the private home,
	// temporary, and runtime directories.
	privateDirMode os.FileMode = 0o700
	// executableFileMode is the mode of the driver script, which the
	// launcher executes directly.
	executableFileMode os.FileMode = 0o700
	// privateFileMode is the mode of the token file and the protocol
	// input files.
	privateFileMode os.FileMode = 0o600
)

// DefaultPath is the controlled PATH handed to a driver. The launcher
// never passes its own PATH through, so a driver cannot pick up
// interpreters or helpers the parent agent's session happened to expose.
const DefaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

const (
	// defaultStopGracePeriod is how long a driver has to exit after
	// SIGTERM before its process group is killed.
	defaultStopGracePeriod = 5 * time.Second
	// defaultCleanupTimeout bounds the cleanup operation. Cleanup must
	// never block reclaiming the token file.
	defaultCleanupTimeout = 10 * time.Second
	// defaultAgentScope names the per-agent state directory when the
	// caller does not know the launching agent's ID.
	defaultAgentScope = "agent"
	// maxDriverOutputLines bounds how many output lines one execution can
	// push into the parent agent's log.
	maxDriverOutputLines = 2048
)

// agentScopePattern constrains the per-agent path segment to something
// that cannot escape the state root.
var agentScopePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// DriverInput is the protocol v1 JSON document the launcher writes for one
// driver invocation. It deliberately carries the path of the child's token
// file rather than the token itself, so the token exists in exactly one
// place: a 0600 file the sandboxed child reads through
// CODER_AGENT_TOKEN_FILE.
type DriverInput struct {
	// Operation is "run" or "cleanup" and matches the first argument.
	Operation Operation `json:"operation"`
	// ProtocolVersion is the driver protocol this document speaks.
	ProtocolVersion int32 `json:"protocol_version"`

	ExecutionID uuid.UUID `json:"execution_id"`
	Generation  uuid.UUID `json:"generation"`

	ChildAgentID   uuid.UUID `json:"child_agent_id"`
	ChildAgentName string    `json:"child_agent_name"`

	// CoderURL is the deployment URL the child agent must reach.
	CoderURL string `json:"coder_url"`
	// CoderBinaryPath is the Coder executable the driver makes available
	// to the child.
	CoderBinaryPath string `json:"coder_binary_path"`
	// TokenFilePath is the 0600 file holding the child agent's token.
	TokenFilePath string `json:"token_file_path"`

	// SharedHostPath and SharedChildPath are the one declared shared
	// project directory, on the host and as the child sees it.
	SharedHostPath  string `json:"shared_host_path"`
	SharedChildPath string `json:"shared_child_path"`

	// StatePath is the private per-execution directory holding this
	// document, the driver, and the token.
	StatePath string `json:"state_path"`
	// HomePath, TmpPath, and RuntimePath are the child's private
	// directories. All of them live outside the shared project directory.
	HomePath    string `json:"home_path"`
	TmpPath     string `json:"tmp_path"`
	RuntimePath string `json:"runtime_path"`
}

// ScriptDriverConfig configures the default driver.
type ScriptDriverConfig struct {
	Logger slog.Logger
	// StateRoot is the absolute directory under which per-execution
	// private state is created. It must not be inside any declared shared
	// project directory. An empty StateRoot leaves the agent without a
	// driver.
	StateRoot string
	// AgentScope is one path segment naming the launching agent, normally
	// its UUID. Empty uses defaultAgentScope.
	AgentScope string
	// CoderURL is the deployment URL passed to the driver for the child
	// agent.
	CoderURL string
	// CoderBinaryPath is the Coder executable passed to the driver, so the
	// child runs the same build as its parent.
	CoderBinaryPath string
	// Path is the controlled PATH the driver runs with. Empty uses
	// DefaultPath.
	Path string
	// Execer creates the driver command. Nil uses
	// agentexec.DefaultExecer. It exists so the launcher honors the same
	// process priority management as the rest of the agent.
	Execer agentexec.Execer
	// StopGracePeriod bounds how long Stop waits after SIGTERM before
	// killing the driver's process group. Zero uses
	// defaultStopGracePeriod.
	StopGracePeriod time.Duration
	// CleanupTimeout bounds the cleanup operation. Zero uses
	// defaultCleanupTimeout.
	CleanupTimeout time.Duration
}

// ScriptDriver is the default Driver. It treats the declaration's driver
// body as an executable script, materializes it plus the child's token and
// the protocol document in a private per-execution directory, and runs the
// script in the foreground with an environment built from scratch.
//
// It is deliberately not the sandbox itself: the script it runs is the
// vetted sandbox driver. Certifying that script is the next phase.
type ScriptDriver struct {
	logger          slog.Logger
	stateRoot       string
	agentScope      string
	coderURL        string
	coderBinaryPath string
	path            string
	execer          agentexec.Execer
	stopGrace       time.Duration
	cleanupTimeout  time.Duration
}

var _ Driver = (*ScriptDriver)(nil)

// NewScriptDriver validates cfg and returns the default driver. A
// configuration error is returned rather than deferred to the first
// launch, so a misconfigured agent is visible at startup.
func NewScriptDriver(cfg ScriptDriverConfig) (*ScriptDriver, error) {
	if !platformSupported {
		return nil, ErrUnsupportedPlatform
	}
	if cfg.StateRoot == "" {
		return nil, xerrors.New("state root is required")
	}
	if !filepath.IsAbs(cfg.StateRoot) {
		return nil, xerrors.Errorf("state root %q must be absolute", cfg.StateRoot)
	}
	if cfg.CoderURL == "" {
		return nil, xerrors.New("coder url is required")
	}
	if cfg.CoderBinaryPath == "" {
		return nil, xerrors.New("coder binary path is required")
	}
	scope := cfg.AgentScope
	if scope == "" {
		scope = defaultAgentScope
	}
	if !agentScopePattern.MatchString(scope) {
		return nil, xerrors.Errorf("agent scope %q is not a safe path segment", scope)
	}
	d := &ScriptDriver{
		logger:          cfg.Logger,
		stateRoot:       filepath.Clean(cfg.StateRoot),
		agentScope:      scope,
		coderURL:        cfg.CoderURL,
		coderBinaryPath: cfg.CoderBinaryPath,
		path:            cfg.Path,
		execer:          cfg.Execer,
		stopGrace:       cfg.StopGracePeriod,
		cleanupTimeout:  cfg.CleanupTimeout,
	}
	if d.path == "" {
		d.path = DefaultPath
	}
	if d.execer == nil {
		d.execer = agentexec.DefaultExecer
	}
	if d.stopGrace <= 0 {
		d.stopGrace = defaultStopGracePeriod
	}
	if d.cleanupTimeout <= 0 {
		d.cleanupTimeout = defaultCleanupTimeout
	}
	return d, nil
}

// executionPaths is the private filesystem layout of one execution. Every
// segment is derived from UUIDs.
type executionPaths struct {
	dir          string
	driver       string
	token        string
	runInput     string
	cleanupInput string
	home         string
	tmp          string
	runtime      string
}

func (d *ScriptDriver) paths(executionID uuid.UUID) executionPaths {
	dir := filepath.Join(d.stateRoot, d.agentScope, executionID.String())
	return executionPaths{
		dir:          dir,
		driver:       filepath.Join(dir, driverFileName),
		token:        filepath.Join(dir, tokenFileName),
		runInput:     filepath.Join(dir, runInputFileName),
		cleanupInput: filepath.Join(dir, cleanupInputFileName),
		home:         filepath.Join(dir, homeDirName),
		tmp:          filepath.Join(dir, tmpDirName),
		runtime:      filepath.Join(dir, runtimeDirName),
	}
}

// Start materializes the execution's private state and runs the driver's
// run operation in the foreground. A failure anywhere removes the private
// state, including the token file, before returning.
func (d *ScriptDriver) Start(_ context.Context, launch Launch) (Process, error) {
	decl := launch.Declaration
	if err := validateDriverBody(decl); err != nil {
		return nil, err
	}
	if launch.authToken == "" {
		return nil, xerrors.New("launch carries no child auth token")
	}

	logger := d.logger.With(
		slog.F("execution_id", decl.ExecutionID),
		slog.F("generation", decl.Generation),
		slog.F("child_agent_id", launch.ChildAgentID),
	)
	paths := d.paths(decl.ExecutionID)

	if err := d.prepare(paths, launch); err != nil {
		removeState(logger, paths)
		return nil, err
	}

	proc, err := d.run(logger, paths, launch)
	if err != nil {
		removeState(logger, paths)
		return nil, err
	}
	return proc, nil
}

// prepare creates the private per-execution state: the driver script, the
// token file, both protocol documents, and the private home, temporary,
// and runtime directories.
func (d *ScriptDriver) prepare(paths executionPaths, launch Launch) error {
	if err := mkdirPrivate(d.stateRoot); err != nil {
		return err
	}
	if err := mkdirPrivate(filepath.Dir(paths.dir)); err != nil {
		return err
	}
	// The per-execution directory is keyed by execution ID, so a relaunch
	// of a new generation reuses it. The superseded generation is always
	// stopped, and therefore cleaned up, before this runs.
	if err := os.RemoveAll(paths.dir); err != nil {
		return xerrors.Errorf("remove stale execution state %s: %w", paths.dir, err)
	}
	if err := mkdirPrivate(paths.dir); err != nil {
		return err
	}
	for _, dir := range []string{paths.home, paths.tmp, paths.runtime} {
		if err := mkdirPrivate(dir); err != nil {
			return err
		}
	}

	if err := writePrivateFile(paths.driver, []byte(launch.Declaration.Driver), executableFileMode); err != nil {
		return xerrors.Errorf("write driver script: %w", err)
	}
	// The token is written to its own 0600 file and is never placed in the
	// protocol document, the argument list, or the environment.
	if err := writePrivateFile(paths.token, []byte(launch.authToken), privateFileMode); err != nil {
		return xerrors.Errorf("write child token file: %w", err)
	}
	for op, path := range map[Operation]string{
		OperationRun:     paths.runInput,
		OperationCleanup: paths.cleanupInput,
	} {
		document, err := json.Marshal(d.input(op, launch, paths))
		if err != nil {
			return xerrors.Errorf("marshal %s input: %w", op, err)
		}
		if err := writePrivateFile(path, document, privateFileMode); err != nil {
			return xerrors.Errorf("write %s input: %w", op, err)
		}
	}
	return nil
}

func (d *ScriptDriver) input(op Operation, launch Launch, paths executionPaths) DriverInput {
	decl := launch.Declaration
	return DriverInput{
		Operation:       op,
		ProtocolVersion: ProtocolVersion,
		ExecutionID:     decl.ExecutionID,
		Generation:      decl.Generation,
		ChildAgentID:    launch.ChildAgentID,
		ChildAgentName:  decl.Name,
		CoderURL:        d.coderURL,
		CoderBinaryPath: d.coderBinaryPath,
		TokenFilePath:   paths.token,
		SharedHostPath:  decl.SharedHostPath,
		SharedChildPath: decl.SharedChildPath,
		StatePath:       paths.dir,
		HomePath:        paths.home,
		TmpPath:         paths.tmp,
		RuntimePath:     paths.runtime,
	}
}

// environ builds the driver's environment from scratch. Nothing from the
// parent agent's environment is inherited, so CODER_AGENT_TOKEN,
// SSH_AUTH_SOCK, Git credential helpers, and cloud or container
// credentials cannot reach the driver or the child through it.
func (d *ScriptDriver) environ(paths executionPaths) []string {
	return []string{
		"PATH=" + d.path,
		"HOME=" + paths.home,
		"TMPDIR=" + paths.tmp,
		"XDG_RUNTIME_DIR=" + paths.runtime,
	}
}

// run starts the driver's run operation. The argv contract is exactly
// `<driver-path> <operation> <input-json-path>`: the script is executed
// directly, never through a shell, so no declared value is interpolated
// into a command line.
func (d *ScriptDriver) run(logger slog.Logger, paths executionPaths, launch Launch) (*driverProcess, error) {
	env := d.environ(paths)
	output := newRedactingWriter(launch.authToken, maxDriverOutputLines, func(line string) {
		logger.Info(context.Background(), "subagent execution driver output", slog.F("output", line))
	})

	// The run operation must outlive the reconciliation that started it, so
	// it is not bound to a caller's context: the manager ends it through
	// Stop instead. The script is vetted template content, written to a
	// private 0700 file by prepare, and executed with an explicit argument
	// list rather than through a shell.
	cmd := d.execer.CommandContext(context.Background(), paths.driver, string(OperationRun), paths.runInput)
	cmd.Env = env
	cmd.Dir = paths.dir
	cmd.Stdout = output
	cmd.Stderr = output
	configureCommand(cmd)

	if err := cmd.Start(); err != nil {
		return nil, xerrors.Errorf("start driver %s: %w", paths.driver, err)
	}

	proc := &driverProcess{
		logger:         logger,
		execer:         d.execer,
		cmd:            cmd,
		paths:          paths,
		env:            env,
		output:         output,
		stopGrace:      d.stopGrace,
		cleanupTimeout: d.cleanupTimeout,
		procExited:     make(chan struct{}),
		done:           make(chan struct{}),
	}
	go proc.supervise()
	return proc, nil
}

// driverProcess is one foreground driver run. It owns the reaping of the
// process, the cleanup operation, and the removal of the private state.
type driverProcess struct {
	logger         slog.Logger
	execer         agentexec.Execer
	cmd            *exec.Cmd
	paths          executionPaths
	env            []string
	output         *redactingWriter
	stopGrace      time.Duration
	cleanupTimeout time.Duration

	// procExited is closed as soon as the driver process is reaped, before
	// cleanup runs, so Stop can tell a graceful exit from a hang.
	procExited chan struct{}
	// done is closed once the process has exited, cleanup has run, and the
	// private state has been removed.
	done chan struct{}

	waitErr error

	stopOnce    sync.Once
	stopErr     error
	cleanupOnce sync.Once
}

var _ Process = (*driverProcess)(nil)

// supervise reaps the driver, then runs cleanup and removes the private
// state before Wait returns. Callers therefore know that once Wait has
// returned, no token material is left on disk.
func (p *driverProcess) supervise() {
	p.waitErr = p.cmd.Wait()
	close(p.procExited)
	p.finalize()
	close(p.done)
}

// Wait blocks until the driver has exited and its state has been
// reclaimed, and returns the foreground process result.
func (p *driverProcess) Wait() error {
	<-p.done
	return p.waitErr
}

// Stop signals the driver's process group with SIGTERM, escalates to
// SIGKILL after the grace period, and returns once cleanup has run.
func (p *driverProcess) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() { p.stopErr = p.terminate(ctx) })
	select {
	case <-p.done:
	case <-ctx.Done():
		if p.stopErr != nil {
			return p.stopErr
		}
		return xerrors.Errorf("wait for driver cleanup: %w", ctx.Err())
	}
	return p.stopErr
}

func (p *driverProcess) terminate(ctx context.Context) error {
	select {
	case <-p.procExited:
		return nil
	default:
	}

	if err := signalProcessGroup(p.cmd, signalTerminate); err != nil {
		// A process that exited between the check above and the signal is
		// not an error worth surfacing; the wait below settles it.
		p.logger.Debug(ctx, "terminate subagent execution driver", slog.Error(err))
	}

	timer := time.NewTimer(p.stopGrace)
	defer timer.Stop()
	select {
	case <-p.procExited:
		return nil
	case <-timer.C:
	case <-ctx.Done():
	}

	p.logger.Warn(ctx, "subagent execution driver ignored SIGTERM, killing its process group",
		slog.F("grace_period", p.stopGrace))
	if err := signalProcessGroup(p.cmd, signalKill); err != nil {
		return xerrors.Errorf("kill driver: %w", err)
	}
	return nil
}

// finalize runs the driver's cleanup operation once and then removes the
// private state. A cleanup failure is logged but never leaves the token
// file behind.
func (p *driverProcess) finalize() {
	p.cleanupOnce.Do(func() {
		p.cleanup()
		removeState(p.logger, p.paths)
		p.output.Close()
	})
}

func (p *driverProcess) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), p.cleanupTimeout)
	defer cancel()

	// Same vetted, private, shell-free invocation as run, bounded by the
	// cleanup timeout.
	cmd := p.execer.CommandContext(ctx, p.paths.driver, string(OperationCleanup), p.paths.cleanupInput)
	cmd.Env = p.env
	cmd.Dir = p.paths.dir
	cmd.Stdout = p.output
	cmd.Stderr = p.output
	configureCommand(cmd)
	if err := cmd.Run(); err != nil {
		p.logger.Warn(ctx, "subagent execution driver cleanup failed", slog.Error(err))
	}
}

// removeState removes the token file first, so a partial failure cannot
// leave credentials behind, and then the whole private directory.
func removeState(logger slog.Logger, paths executionPaths) {
	if err := os.Remove(paths.token); err != nil && !os.IsNotExist(err) {
		logger.Error(context.Background(), "remove subagent execution token file",
			slog.F("path", paths.token), slog.Error(err))
	}
	if err := os.RemoveAll(paths.dir); err != nil {
		logger.Warn(context.Background(), "remove subagent execution state",
			slog.F("path", paths.dir), slog.Error(err))
	}
}

// validateDriverBody checks the declaration against protocol v1. The body
// is executed directly, so it must name its own interpreter.
func validateDriverBody(decl agentsdk.SubagentExecution) error {
	if decl.DriverProtocol != ProtocolVersion {
		return xerrors.Errorf("unsupported driver protocol %d: this launcher speaks protocol %d",
			decl.DriverProtocol, ProtocolVersion)
	}
	body := decl.Driver
	if strings.TrimSpace(body) == "" {
		return xerrors.New("declaration has an empty driver body")
	}
	if !strings.HasPrefix(body, "#!") {
		return xerrors.New("driver body must begin with a shebang, for example #!/usr/bin/env bash")
	}
	shebang := body[len("#!"):]
	if idx := strings.IndexByte(shebang, '\n'); idx >= 0 {
		shebang = shebang[:idx]
	}
	shebang = strings.TrimSpace(shebang)
	if shebang == "" {
		return xerrors.New("driver body has an empty shebang")
	}
	if !strings.HasPrefix(shebang, "/") {
		return xerrors.Errorf("driver shebang %q must name an absolute interpreter path", shebang)
	}
	return nil
}

// mkdirPrivate creates dir with privateDirMode and tightens an existing
// directory's mode, refusing anything that is not a real directory.
func mkdirPrivate(dir string) error {
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return xerrors.Errorf("refusing to use %s: it is a symlink", dir)
		}
		if !info.IsDir() {
			return xerrors.Errorf("refusing to use %s: it is not a directory", dir)
		}
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("stat %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, privateDirMode); err != nil {
		return xerrors.Errorf("create %s: %w", dir, err)
	}
	// MkdirAll applies the process umask, and an existing directory keeps
	// whatever mode it had, so the mode is set explicitly.
	if err := os.Chmod(dir, privateDirMode); err != nil {
		return xerrors.Errorf("restrict %s: %w", dir, err)
	}
	return nil
}

// writePrivateFile creates path atomically: the content is written to a
// temporary file in the same directory, given its final restrictive mode,
// and renamed into place, so no reader ever sees a partial or
// world-readable file.
func writePrivateFile(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return xerrors.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tmpPath)
	}()

	if err := file.Chmod(mode); err != nil {
		return xerrors.Errorf("set mode on %s: %w", tmpPath, err)
	}
	if _, err := file.Write(content); err != nil {
		return xerrors.Errorf("write %s: %w", tmpPath, err)
	}
	if err := file.Sync(); err != nil {
		return xerrors.Errorf("sync %s: %w", tmpPath, err)
	}
	if err := file.Close(); err != nil {
		return xerrors.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return xerrors.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}
	return nil
}
