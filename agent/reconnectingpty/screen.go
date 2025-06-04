package reconnectingpty

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/pty"
)

// screenReconnectingPTY provides a reconnectable PTY via `screen`.
type screenReconnectingPTY struct {
	execer  agentexec.Execer
	command *pty.Cmd

	// id holds the id of the session for both creating and attaching.  This will
	// be generated uniquely for each session because without control of the
	// screen daemon we do not have its PID and without the PID screen will do
	// partial matching.  Enforcing a unique ID should guarantee we match on the
	// right session.
	id string

	// mutex prevents concurrent attaches to the session.  Screen will happily
	// spawn two separate sessions with the same name if multiple attaches happen
	// in a close enough interval.  We are not able to control the screen daemon
	// ourselves to prevent this because the daemon will spawn with a hardcoded
	// 24x80 size which results in confusing padding above the prompt once the
	// attach comes in and resizes.
	mutex sync.Mutex

	configFile string

	metrics *prometheus.CounterVec

	state *ptyState
	// timer will close the reconnecting pty when it expires.  The timer will be
	// reset as long as there are active connections.
	timer   *time.Timer
	timeout time.Duration
}

// newScreen creates a new screen-backed reconnecting PTY.  It writes config
// settings and creates the socket directory.  If we could, we would want to
// spawn the daemon here and attach each connection to it but since doing that
// spawns the daemon with a hardcoded 24x80 size it is not a very good user
// experience.  Instead we will let the attach command spawn the daemon on its
// own which causes it to spawn with the specified size.
func newScreen(ctx context.Context, logger slog.Logger, execer agentexec.Execer, cmd *pty.Cmd, options *Options) *screenReconnectingPTY {
	rpty := &screenReconnectingPTY{
		execer:  execer,
		command: cmd,
		metrics: options.Metrics,
		state:   newState(),
		timeout: options.Timeout,
	}

	// Socket paths are limited to around 100 characters on Linux and macOS which
	// depending on the temporary directory can be a problem.  To give more leeway
	// use a short ID.
	buf := make([]byte, 4)
	_, err := rand.Read(buf)
	if err != nil {
		rpty.state.setState(StateDone, xerrors.Errorf("generate screen id: %w", err))
		return rpty
	}
	rpty.id = hex.EncodeToString(buf)

	settings := []string{
		// Disable the startup message that appears for five seconds.
		"startup_message off",
		// Some message are hard-coded, the best we can do is set msgwait to 0
		// which seems to hide them. This can happen for example if screen shows
		// the version message when starting up.
		"msgminwait 0",
		"msgwait 0",
		// Tell screen not to handle motion for xterm* terminals which allows
		// scrolling the terminal via the mouse wheel or scroll bar (by default
		// screen uses it to cycle through the command history).  There does not
		// seem to be a way to make screen itself scroll on mouse wheel.  tmux can
		// do it but then there is no scroll bar and it kicks you into copy mode
		// where keys stop working until you exit copy mode which seems like it
		// could be confusing.
		"termcapinfo xterm* ti@:te@",
		// Enable alternate screen emulation otherwise applications get rendered in
		// the current window which wipes out visible output resulting in missing
		// output when scrolling back with the mouse wheel (copy mode still works
		// since that is screen itself scrolling).
		"altscreen on",
		// Remap the control key to C-s since C-a may be used in applications.  C-s
		// is chosen because it cannot actually be used because by default it will
		// pause and C-q to resume will just kill the browser window.  We may not
		// want people using the control key anyway since it will not be obvious
		// they are in screen and doing things like switching windows makes mouse
		// wheel scroll wonky due to the terminal doing the scrolling rather than
		// screen itself (but again copy mode will work just fine).
		"escape ^Ss",
	}

	rpty.configFile = filepath.Join(os.TempDir(), "coder-screen", "config")
	err = os.MkdirAll(filepath.Dir(rpty.configFile), 0o700)
	if err != nil {
		rpty.state.setState(StateDone, xerrors.Errorf("make screen config dir: %w", err))
		return rpty
	}

	err = os.WriteFile(rpty.configFile, []byte(strings.Join(settings, "\n")), 0o600)
	if err != nil {
		rpty.state.setState(StateDone, xerrors.Errorf("create config file: %w", err))
		return rpty
	}

	go rpty.lifecycle(ctx, logger)

	return rpty
}

// lifecycle manages the lifecycle of the reconnecting pty.  If the context ends
// the reconnecting pty will be closed.
func (rpty *screenReconnectingPTY) lifecycle(ctx context.Context, logger slog.Logger) {
	rpty.timer = time.AfterFunc(attachTimeout, func() {
		rpty.Close(xerrors.New("reconnecting pty timeout"))
	})

	logger.Debug(ctx, "reconnecting pty ready")
	rpty.state.setState(StateReady, nil)

	state, reasonErr := rpty.state.waitForStateOrContext(ctx, StateClosing)
	if state < StateClosing {
		// If we have not closed yet then the context is what unblocked us (which
		// means the agent is shutting down) so move into the closing phase.
		rpty.Close(reasonErr)
	}
	rpty.timer.Stop()

	// If the command errors that the session is already gone that is fine.
	err := rpty.sendCommand(context.Background(), "quit", []string{"No screen session found"})
	if err != nil {
		logger.Error(ctx, "close screen session", slog.Error(err))
	}

	logger.Info(ctx, "closed reconnecting pty")
	rpty.state.setState(StateDone, reasonErr)
}

func (rpty *screenReconnectingPTY) Attach(ctx context.Context, _ string, conn net.Conn, height, width uint16, logger slog.Logger) error {
	logger.Info(ctx, "attach to reconnecting pty")

	// This will kill the heartbeat once we hit EOF or an error.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, err := rpty.state.waitForStateOrContext(ctx, StateReady)
	if state != StateReady {
		return err
	}

	go heartbeat(ctx, rpty.timer, rpty.timeout)

	ptty, process, err := rpty.doAttach(ctx, conn, height, width, logger)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Likely the process was too short-lived and canceled the version command.
			// TODO: Is it worth distinguishing between that and a cancel from the
			//       Attach() caller?  Additionally, since this could also happen if
			//       the command was invalid, should we check the process's exit code?
			return nil
		}
		return err
	}

	defer func() {
		// Log only for debugging since the process might have already exited on its
		// own.
		err := ptty.Close()
		if err != nil {
			logger.Debug(ctx, "closed ptty with error", slog.Error(err))
		}
		err = process.Kill()
		if err != nil {
			logger.Debug(ctx, "killed process with error", slog.Error(err))
		}
	}()

	// Pipe conn -> pty and block.
	readConnLoop(ctx, conn, ptty, rpty.metrics, logger)
	return nil
}

// doAttach spawns the screen client and starts the heartbeat.  It exists
// separately only so we can defer the mutex unlock which is not possible in
// Attach since it blocks.
func (rpty *screenReconnectingPTY) doAttach(ctx context.Context, conn net.Conn, height, width uint16, logger slog.Logger) (pty.PTYCmd, pty.Process, error) {
	// Ensure another attach does not come in and spawn a duplicate session.
	rpty.mutex.Lock()
	defer rpty.mutex.Unlock()

	logger.Debug(ctx, "spawning screen client", slog.F("screen_id", rpty.id))

	// Wrap the command with screen and tie it to the connection's context.
	cmd := rpty.execer.PTYCommandContext(ctx, "screen", append([]string{
		// -S is for setting the session's name.
		"-S", rpty.id,
		// -U tells screen to use UTF-8 encoding.
		// -x allows attaching to an already attached session.
		// -RR reattaches to the daemon or creates the session daemon if missing.
		// -q disables the "New screen..." message that appears for five seconds
		//    when creating a new session with -RR.
		// -c is the flag for the config file.
		"-UxRRqc", rpty.configFile,
		rpty.command.Path,
		// pty.Cmd duplicates Path as the first argument so remove it.
	}, rpty.command.Args[1:]...)...)
	//nolint:gocritic
	cmd.Env = append(rpty.command.Env, "TERM=xterm-256color")
	cmd.Dir = rpty.command.Dir
	ptty, process, err := pty.Start(cmd, pty.WithPTYOption(
		pty.WithSSHRequest(ssh.Pty{
			Window: ssh.Window{
				// Make sure to spawn at the right size because if we resize afterward it
				// leaves confusing padding (screen will resize such that the screen
				// contents are aligned to the bottom).
				Height: int(height),
				Width:  int(width),
			},
		}),
	))
	if err != nil {
		rpty.metrics.WithLabelValues("screen_spawn").Add(1)
		return nil, nil, err
	}

	// This context lets us abort the version command if the process dies.
	versionCtx, versionCancel := context.WithCancel(ctx)
	defer versionCancel()

	// Pipe pty -> conn and close the connection when the process exits.
	// We do not need to separately monitor for the process exiting.  When it
	// exits, our ptty.OutputReader() will return EOF after reading all process
	// output.
	go func() {
		defer versionCancel()
		defer func() {
			err := conn.Close()
			if err != nil {
				// Log only for debugging since the connection might have already closed
				// on its own.
				logger.Debug(ctx, "closed connection with error", slog.Error(err))
			}
		}()
		buffer := make([]byte, 1024)
		for {
			read, err := ptty.OutputReader().Read(buffer)
			if err != nil {
				// When the PTY is closed, this is triggered.
				// Error is typically a benign EOF, so only log for debugging.
				if errors.Is(err, io.EOF) {
					logger.Debug(ctx, "unable to read pty output; screen might have exited", slog.Error(err))
				} else {
					logger.Warn(ctx, "unable to read pty output; screen might have exited", slog.Error(err))
					rpty.metrics.WithLabelValues("screen_output_reader").Add(1)
				}
				// The process might have died because the session itself died or it
				// might have been separately killed and the session is still up (for
				// example `exit` or we killed it when the connection closed).  If the
				// session is still up we might leave the reconnecting pty in memory
				// around longer than it needs to be but it will eventually clean up
				// with the timer or context, or the next attach will respawn the screen
				// daemon which is fine too.
				break
			}
			part := buffer[:read]
			_, err = conn.Write(part)
			if err != nil {
				// Connection might have been closed.
				if errors.Unwrap(err).Error() != "endpoint is closed for send" {
					logger.Warn(ctx, "error writing to active conn", slog.Error(err))
					rpty.metrics.WithLabelValues("screen_write").Add(1)
				}
				break
			}
		}
	}()

	// Version seems to be the only command without a side effect (other than
	// making the version pop up briefly) so use it to wait for the session to
	// come up.  If we do not wait we could end up spawning multiple sessions with
	// the same name.
	err = rpty.sendCommand(versionCtx, "version", nil)
	if err != nil {
		// Log only for debugging since the process might already have closed.
		closeErr := ptty.Close()
		if closeErr != nil {
			logger.Debug(ctx, "closed ptty with error", slog.Error(closeErr))
		}
		killErr := process.Kill()
		if killErr != nil {
			logger.Debug(ctx, "killed process with error", slog.Error(killErr))
		}
		rpty.metrics.WithLabelValues("screen_wait").Add(1)
		return nil, nil, err
	}

	return ptty, process, nil
}

// sendCommand runs a screen command against a running screen session.  If the
// command fails with an error matching anything in successErrors it will be
// considered a success state (for example "no session" when quitting and the
// session is already dead).  The command will be retried until successful, the
// timeout is reached, or the context ends.  A canceled context will return the
// canceled context's error as-is while a timed-out context returns together
// with the last error from the command.
func (rpty *screenReconnectingPTY) sendCommand(ctx context.Context, command string, successErrors []string) error {
	ctx, cancel := context.WithTimeout(ctx, attachTimeout)
	defer cancel()

	var lastErr error
	run := func() (bool, error) {
		var stdout bytes.Buffer
		//nolint:gosec
		cmd := rpty.execer.CommandContext(ctx, "screen",
			// -x targets an attached session.
			"-x", rpty.id,
			// -c is the flag for the config file.
			"-c", rpty.configFile,
			// -X runs a command in the matching session.
			"-X", command,
		)
		//nolint:gocritic
		cmd.Env = append(rpty.command.Env, "TERM=xterm-256color")
		cmd.Dir = rpty.command.Dir
		cmd.Stdout = &stdout
		err := cmd.Run()
		if err == nil {
			return true, nil
		}

		stdoutStr := stdout.String()
		for _, se := range successErrors {
			if strings.Contains(stdoutStr, se) {
				return true, nil
			}
		}

		// Things like "exit status 1" are imprecise so include stdout as it may
		// contain more information ("no screen session found" for example).
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			lastErr = xerrors.Errorf("`screen -x %s -X %s`: %w: %s", rpty.id, command, err, stdoutStr)
		}

		return false, nil
	}

	// Run immediately.
	done, err := run()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	// Then run on an interval.
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return errors.Join(ctx.Err(), lastErr)
		case <-ticker.C:
			done, err := run()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

func (rpty *screenReconnectingPTY) Wait() {
	_, _ = rpty.state.waitForState(StateClosing)
}

func (rpty *screenReconnectingPTY) Close(err error) {
	// The closing state change will be handled by the lifecycle.
	rpty.state.setState(StateClosing, err)
}
