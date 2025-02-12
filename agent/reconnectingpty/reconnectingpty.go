package reconnectingpty

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/pty"
)

// attachTimeout is the initial timeout for attaching and will probably be far
// shorter than the reconnect timeout in most cases; in tests it might be
// longer.  It should be at least long enough for the first screen attach to be
// able to start up the daemon and for the buffered pty to start.
const attachTimeout = 30 * time.Second

// Options allows configuring the reconnecting pty.
type Options struct {
	// Timeout describes how long to keep the pty alive without any connections.
	// Once elapsed the pty will be killed.
	Timeout time.Duration
	// Metrics tracks various error counters.
	Metrics *prometheus.CounterVec
}

// ReconnectingPTY is a pty that can be reconnected within a timeout and to
// simultaneous connections.  The reconnecting pty can be backed by screen if
// installed or a (buggy) buffer replay fallback.
type ReconnectingPTY interface {
	// Attach pipes the connection and pty, spawning it if necessary, replays
	// history, then blocks until EOF, an error, or the context's end.  The
	// connection is expected to send JSON-encoded messages and accept raw output
	// from the ptty.  If the context ends or the process dies the connection will
	// be detached.
	Attach(ctx context.Context, connID string, conn net.Conn, height, width uint16, logger slog.Logger) error
	// Wait waits for the reconnecting pty to close.  The underlying process might
	// still be exiting.
	Wait()
	// Close kills the reconnecting pty process.
	Close(err error)
}

// New sets up a new reconnecting pty that wraps the provided command.  Any
// errors with starting are returned on Attach().  The reconnecting pty will
// close itself (and all connections to it) if nothing is attached for the
// duration of the timeout, if the context ends, or the process exits (buffered
// backend only).
func New(ctx context.Context, logger slog.Logger, execer agentexec.Execer, cmd *pty.Cmd, options *Options) ReconnectingPTY {
	if options.Timeout == 0 {
		options.Timeout = 5 * time.Minute
	}
	// Screen seems flaky on Darwin.  Locally the tests pass 100% of the time (100
	// runs) but in CI screen often incorrectly claims the session name does not
	// exist even though screen -list shows it.  For now, restrict screen to
	// Linux.
	backendType := "buffered"
	if runtime.GOOS == "linux" {
		_, err := exec.LookPath("screen")
		if err == nil {
			backendType = "screen"
		}
	}

	logger.Info(ctx, "start reconnecting pty", slog.F("backend_type", backendType))

	switch backendType {
	case "screen":
		return newScreen(ctx, logger, execer, cmd, options)
	default:
		return newBuffered(ctx, logger, execer, cmd, options)
	}
}

// heartbeat resets timer before timeout elapses and blocks until ctx ends.
func heartbeat(ctx context.Context, timer *time.Timer, timeout time.Duration) {
	// Reset now in case it is near the end.
	timer.Reset(timeout)

	// Reset when the context ends to ensure the pty stays up for the full
	// timeout.
	defer timer.Reset(timeout)

	heartbeat := time.NewTicker(timeout / 2)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			timer.Reset(timeout)
		}
	}
}

// State represents the current state of the reconnecting pty.  States are
// sequential and will only move forward.
type State int

const (
	// StateStarting is the default/start state.  Attaching will block until the
	// reconnecting pty becomes ready.
	StateStarting = iota
	// StateReady means the reconnecting pty is ready to be attached.
	StateReady
	// StateClosing means the reconnecting pty has begun closing.  The underlying
	// process may still be exiting.  Attaching will result in an error.
	StateClosing
	// StateDone means the reconnecting pty has completely shut down and the
	// process has exited.  Attaching will result in an error.
	StateDone
)

// ptyState is a helper for tracking the reconnecting PTY's state.
type ptyState struct {
	// cond broadcasts state changes and any accompanying errors.
	cond *sync.Cond
	// error describes the error that caused the state change, if there was one.
	// It is not safe to access outside of cond.L.
	error error
	// state holds the current reconnecting pty state.  It is not safe to access
	// this outside of cond.L.
	state State
}

func newState() *ptyState {
	return &ptyState{
		cond:  sync.NewCond(&sync.Mutex{}),
		state: StateStarting,
	}
}

// setState sets and broadcasts the provided state if it is greater than the
// current state and the error if one has not already been set.
func (s *ptyState) setState(state State, err error) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	// Cannot regress states.  For example, trying to close after the process is
	// done should leave us in the done state and not the closing state.
	if state <= s.state {
		return
	}
	s.error = err
	s.state = state
	s.cond.Broadcast()
}

// waitForState blocks until the state or a greater one is reached.
func (s *ptyState) waitForState(state State) (State, error) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for state > s.state {
		s.cond.Wait()
	}
	return s.state, s.error
}

// waitForStateOrContext blocks until the state or a greater one is reached or
// the provided context ends.
func (s *ptyState) waitForStateOrContext(ctx context.Context, state State) (State, error) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()

	nevermind := make(chan struct{})
	defer close(nevermind)
	go func() {
		select {
		case <-ctx.Done():
			// Wake up when the context ends.
			s.cond.Broadcast()
		case <-nevermind:
		}
	}()

	for ctx.Err() == nil && state > s.state {
		s.cond.Wait()
	}
	if ctx.Err() != nil {
		return s.state, ctx.Err()
	}
	return s.state, s.error
}

// readConnLoop reads messages from conn and writes to ptty as needed.  Blocks
// until EOF or an error writing to ptty or reading from conn.
func readConnLoop(ctx context.Context, conn net.Conn, ptty pty.PTYCmd, metrics *prometheus.CounterVec, logger slog.Logger) {
	decoder := json.NewDecoder(conn)
	for {
		var req workspacesdk.ReconnectingPTYRequest
		err := decoder.Decode(&req)
		if xerrors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			logger.Warn(ctx, "reconnecting pty failed with read error", slog.Error(err))
			return
		}
		_, err = ptty.InputWriter().Write([]byte(req.Data))
		if err != nil {
			logger.Warn(ctx, "reconnecting pty failed with write error", slog.Error(err))
			metrics.WithLabelValues("input_writer").Add(1)
			return
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			logger.Warn(ctx, "reconnecting pty resize failed, but will continue", slog.Error(err))
			metrics.WithLabelValues("resize").Add(1)
		}
	}
}
