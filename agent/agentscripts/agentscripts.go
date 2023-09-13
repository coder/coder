package agentscripts

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

var (
	// ErrTimeout is returned when a script times out.
	ErrTimeout = xerrors.New("script timed out")

	parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional)
)

// Options are a set of options for the runner.
type Options struct {
	LogDir     string
	Logger     slog.Logger
	SSHServer  *agentssh.Server
	Filesystem afero.Fs
	PatchLogs  func(ctx context.Context, req agentsdk.PatchLogs) error
}

// New creates a runner for the provided scripts.
func New(ctx context.Context, opts Options) *Runner {
	return &Runner{
		Options: opts,
		cron:    cron.New(cron.WithParser(parser)),
		ctx:     ctx,
		closed:  make(chan struct{}),
	}
}

type Runner struct {
	Options

	cmdCloseWait sync.WaitGroup
	closed       chan struct{}
	closeMutex   sync.Mutex
	ctx          context.Context
	cron         *cron.Cron
	scripts      []codersdk.WorkspaceAgentScript
}

// Init initializes the runner with the provided scripts.
// It also schedules any scripts that have a schedule.
// This function must be called before Execute.
func (r *Runner) Init(scripts []codersdk.WorkspaceAgentScript) error {
	r.scripts = scripts

	for _, script := range scripts {
		if script.CRON == "" {
			continue
		}
		script := script
		_, err := r.cron.AddFunc(script.CRON, func() {
			err := r.run(script)
			if err != nil {
				r.Logger.Warn(r.ctx, "run agent script on schedule", slog.Error(err))
			}
		})
		if err != nil {
			return xerrors.Errorf("add schedule: %w", err)
		}
	}
	return nil
}

// StartCRON starts the cron scheduler.
// This is done async to allow for the caller to execute scripts prior.
func (r *Runner) StartCRON() {
	r.cron.Start()
}

// Execute runs a set of scripts according to a filter.
func (r *Runner) Execute(filter func(script codersdk.WorkspaceAgentScript) bool) error {
	if filter == nil {
		// Execute em' all!
		filter = func(script codersdk.WorkspaceAgentScript) bool {
			return true
		}
	}
	var eg errgroup.Group
	for _, script := range r.scripts {
		if !filter(script) {
			continue
		}
		script := script
		eg.Go(func() error {
			err := r.run(script)
			if err != nil {
				return xerrors.Errorf("run agent script %q: %w", script.LogPath, err)
			}
			return nil
		})
	}
	return eg.Wait()
}

// run executes the provided script with the timeout.
// If the timeout is exceeded, the process is sent an interrupt signal.
// If the process does not exit after a few seconds, it is forcefully killed.
// This function immediately returns after a timeout, and does not wait for the process to exit.
func (r *Runner) run(script codersdk.WorkspaceAgentScript) error {
	logger := r.Logger.With(slog.F("log_source", script.LogPath))
	ctx := r.ctx
	logger.Info(ctx, "running agent script", slog.F("script", script.Script))

	logPath := script.LogPath
	if logPath == "" {
		logPath = fmt.Sprintf("coder-%s-script.log", script.LogSourceID)
	}
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(r.LogDir, logPath)
	}
	fileWriter, err := r.Filesystem.OpenFile(logPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return xerrors.Errorf("open %s script log file: %w", logPath, err)
	}
	defer func() {
		err := fileWriter.Close()
		if err != nil {
			logger.Warn(ctx, fmt.Sprintf("close %s script log file", logPath), slog.Error(err))
		}
	}()

	var cmd *exec.Cmd
	if script.Timeout > 0 {
		var cancel context.CancelFunc
		// Add a buffer to forcefully kill with the context.
		ctx, cancel = context.WithTimeout(ctx, script.Timeout+(3*time.Second))
		defer cancel()
	}

	cmdPty, err := r.SSHServer.CreateCommand(ctx, script.Script, nil)
	if err != nil {
		return xerrors.Errorf("%s script: create command: %w", logPath, err)
	}
	cmd = cmdPty.AsExec()

	send, flushAndClose := agentsdk.LogsSender(script.LogSourceID, r.PatchLogs, logger)
	// If ctx is canceled here (or in a writer below), we may be
	// discarding logs, but that's okay because we're shutting down
	// anyway. We could consider creating a new context here if we
	// want better control over flush during shutdown.
	defer func() {
		if err := flushAndClose(ctx); err != nil {
			logger.Warn(ctx, "flush startup logs failed", slog.Error(err))
		}
	}()

	infoW := agentsdk.StartupLogsWriter(ctx, send, script.LogSourceID, codersdk.LogLevelInfo)
	defer infoW.Close()
	errW := agentsdk.StartupLogsWriter(ctx, send, script.LogSourceID, codersdk.LogLevelError)
	defer errW.Close()
	cmd.Stdout = io.MultiWriter(fileWriter, infoW)
	cmd.Stderr = io.MultiWriter(fileWriter, errW)

	start := time.Now()
	defer func() {
		end := time.Now()
		execTime := end.Sub(start)
		exitCode := 0
		if err != nil {
			exitCode = 255 // Unknown status.
			var exitError *exec.ExitError
			if xerrors.As(err, &exitError) {
				exitCode = exitError.ExitCode()
			}
			logger.Warn(ctx, fmt.Sprintf("%s script failed", logPath), slog.F("execution_time", execTime), slog.F("exit_code", exitCode), slog.Error(err))
		} else {
			logger.Info(ctx, fmt.Sprintf("%s script completed", logPath), slog.F("execution_time", execTime), slog.F("exit_code", exitCode))
		}
	}()

	err = cmd.Start()
	if err != nil {
		return xerrors.Errorf("%s script: start command: %w", logPath, err)
	}

	// timeout stores whether the process timed out then was gracefully killed.
	var timeout chan struct{}
	if script.Timeout > 0 {
		timeout = make(chan struct{})
		timer := time.AfterFunc(script.Timeout, func() {
			close(timeout)
			err := cmd.Process.Signal(os.Interrupt)
			if err != nil {
				logger.Warn(ctx, "send interrupt signal to script", slog.Error(err))
			}
		})
		defer timer.Stop()
	}

	cmdDone := make(chan error, 1)
	err = r.trackCommandGoroutine(func() {
		cmdDone <- cmd.Wait()
	})
	if err != nil {
		return xerrors.Errorf("%s script: track command goroutine: %w", logPath, err)
	}
	select {
	case <-timeout:
		err = ErrTimeout
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-cmdDone:
	}
	return err
}

func (r *Runner) Close() error {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()
	if r.isClosed() {
		return nil
	}
	close(r.closed)
	r.cmdCloseWait.Wait()
	return nil
}

func (r *Runner) trackCommandGoroutine(fn func()) error {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()
	if r.isClosed() {
		return xerrors.New("track command goroutine: closed")
	}
	r.cmdCloseWait.Add(1)
	go func() {
		defer r.cmdCloseWait.Done()
		fn()
	}()
	return nil
}

func (r *Runner) isClosed() bool {
	select {
	case <-r.closed:
		return true
	default:
		return false
	}
}
