package reconnectingpty

import (
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/armon/circbuf"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/pty"
)

// bufferedReconnectingPTY provides a reconnectable PTY by using a ring buffer to store
// scrollback.
type bufferedReconnectingPTY struct {
	command *pty.Cmd

	activeConns    map[string]net.Conn
	circularBuffer *circbuf.Buffer

	// For channel-based output handling
	outputChan   chan []byte
	outputClosed bool
	outputMu     sync.Mutex

	ptty    pty.PTYCmd
	process pty.Process

	metrics *prometheus.CounterVec

	state *ptyState
	// timer will close the reconnecting pty when it expires.  The timer will be
	// reset as long as there are active connections.
	timer   *time.Timer
	timeout time.Duration
}

// newBuffered starts the buffered pty.  If the context ends the process will be
// killed.
func newBuffered(ctx context.Context, logger slog.Logger, execer agentexec.Execer, cmd *pty.Cmd, options *Options) *bufferedReconnectingPTY {
	rpty := &bufferedReconnectingPTY{
		activeConns: map[string]net.Conn{},
		command:     cmd,
		metrics:     options.Metrics,
		state:       newState(),
		timeout:     options.Timeout,
		outputChan:  make(chan []byte, 32), // Buffered channel to avoid blocking during output
	}

	// Default to buffer 64KiB.
	circularBuffer, err := circbuf.NewBuffer(64 << 10)
	if err != nil {
		rpty.state.setState(StateDone, xerrors.Errorf("create circular buffer: %w", err))
		return rpty
	}
	rpty.circularBuffer = circularBuffer

	// Add TERM then start the command with a pty.  pty.Cmd duplicates Path as the
	// first argument so remove it.
	cmdWithEnv := execer.PTYCommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	//nolint:gocritic
	cmdWithEnv.Env = append(rpty.command.Env, "TERM=xterm-256color")
	cmdWithEnv.Dir = rpty.command.Dir
	ptty, process, err := pty.Start(cmdWithEnv)
	if err != nil {
		rpty.state.setState(StateDone, xerrors.Errorf("start pty: %w", err))
		return rpty
	}
	rpty.ptty = ptty
	rpty.process = process

	go rpty.lifecycle(ctx, logger)

	// Multiplex the output onto the circular buffer and broadcast to the output channel.
	// We do not need to separately monitor for the process exiting. When it
	// exits, our ptty.OutputReader() will return EOF after reading all process
	// output.
	go func() {
		buffer := make([]byte, 1024)
		for {
			read, err := ptty.OutputReader().Read(buffer)
			if err != nil {
				// When the PTY is closed, this is triggered.
				// Error is typically a benign EOF, so only log for debugging.
				if errors.Is(err, io.EOF) {
					logger.Debug(ctx, "unable to read pty output, command might have exited", slog.Error(err))
				} else {
					logger.Warn(ctx, "unable to read pty output, command might have exited", slog.Error(err))
					rpty.metrics.WithLabelValues("output_reader").Add(1)
				}
				// Could have been killed externally or failed to start at all (command
				// not found for example).
				// Close the outputChan to signal all connection handlers that no more output is coming
				rpty.closeOutputChannel()
				rpty.Close(nil)
				break
			}
			
			part := buffer[:read]
			
			// Write to the circular buffer for history/scrollback
			rpty.state.cond.L.Lock()
			_, err = rpty.circularBuffer.Write(part)
			if err != nil {
				logger.Error(ctx, "write to circular buffer", slog.Error(err))
				rpty.metrics.WithLabelValues("write_buffer").Add(1)
			}
			rpty.state.cond.L.Unlock()
			
			// Send output to channel for all connected clients to consume
			// This replaces the need to iterate through all connections on each output
			select {
			case rpty.outputChan <- slices.Clone(part):
				// Successfully sent output
			default:
				// Channel buffer is full, this is unusual but we'll log it
				logger.Warn(ctx, "output channel buffer full, some output may be lost")
				rpty.metrics.WithLabelValues("output_channel_full").Add(1)
			}
		}
	}()

	return rpty
}

// lifecycle manages the lifecycle of the reconnecting pty.  If the context ends
// or the reconnecting pty closes the pty will be shut down.
func (rpty *bufferedReconnectingPTY) lifecycle(ctx context.Context, logger slog.Logger) {
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

	// Close the output channel to signal all readers that no more output is coming
	rpty.closeOutputChannel()

	rpty.state.cond.L.Lock()
	// Log these closes only for debugging since the connections or processes
	// might have already closed on their own.
	for _, conn := range rpty.activeConns {
		err := conn.Close()
		if err != nil {
			logger.Debug(ctx, "closed conn with error", slog.Error(err))
		}
	}
	// Connections get removed once they close but it is possible there is still
	// some data that will be written before that happens so clear the map now to
	// avoid writing to closed connections.
	rpty.activeConns = map[string]net.Conn{}
	rpty.state.cond.L.Unlock()

	// Log close/kill only for debugging since the process might have already
	// closed on its own.
	err := rpty.ptty.Close()
	if err != nil {
		logger.Debug(ctx, "closed ptty with error", slog.Error(err))
	}

	err = rpty.process.Kill()
	if err != nil {
		logger.Debug(ctx, "killed process with error", slog.Error(err))
	}

	logger.Info(ctx, "closed reconnecting pty")
	rpty.state.setState(StateDone, reasonErr)
}

// closeOutputChannel closes the output channel safely
func (rpty *bufferedReconnectingPTY) closeOutputChannel() {
	rpty.outputMu.Lock()
	defer rpty.outputMu.Unlock()
	
	if !rpty.outputClosed {
		close(rpty.outputChan)
		rpty.outputClosed = true
	}
}

func (rpty *bufferedReconnectingPTY) Attach(ctx context.Context, connID string, conn net.Conn, height, width uint16, logger slog.Logger) error {
	logger.Info(ctx, "attach to reconnecting pty")

	// This will kill the heartbeat once we hit EOF or an error.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := rpty.doAttach(connID, conn)
	if err != nil {
		return err
	}

	defer func() {
		rpty.state.cond.L.Lock()
		defer rpty.state.cond.L.Unlock()
		delete(rpty.activeConns, connID)
	}()

	state, err := rpty.state.waitForStateOrContext(ctx, StateReady)
	if state != StateReady {
		return err
	}

	go heartbeat(ctx, rpty.timer, rpty.timeout)

	// Resize the PTY to initial height + width.
	err = rpty.ptty.Resize(height, width)
	if err != nil {
		// We can continue after this, it's not fatal!
		logger.Warn(ctx, "reconnecting PTY initial resize failed, but will continue", slog.Error(err))
		rpty.metrics.WithLabelValues("resize").Add(1)
	}

	// Start a goroutine to read from the output channel and write to the connection
	go func() {
		for output := range rpty.outputChan {
			_, err := conn.Write(output)
			if err != nil {
				logger.Warn(ctx, "error writing to connection", 
					slog.F("connection_id", connID), 
					slog.Error(err))
				rpty.metrics.WithLabelValues("write").Add(1)
				return
			}
		}
	}()

	// Pipe conn -> pty and block.
	readConnLoop(ctx, conn, rpty.ptty, rpty.metrics, logger)
	return nil
}

// doAttach adds the connection to the map and replays the buffer.  It exists
// separately only for convenience to defer the mutex unlock which is not
// possible in Attach since it blocks.
func (rpty *bufferedReconnectingPTY) doAttach(connID string, conn net.Conn) error {
	rpty.state.cond.L.Lock()
	defer rpty.state.cond.L.Unlock()

	// Write any previously stored data for the TTY.  Since the command might be
	// short-lived and have already exited, make sure we always at least output
	// the buffer before returning, mostly just so tests pass.
	prevBuf := slices.Clone(rpty.circularBuffer.Bytes())
	_, err := conn.Write(prevBuf)
	if err != nil {
		rpty.metrics.WithLabelValues("write").Add(1)
		return xerrors.Errorf("write buffer to conn: %w", err)
	}

	rpty.activeConns[connID] = conn

	return nil
}

func (rpty *bufferedReconnectingPTY) Wait() {
	_, _ = rpty.state.waitForState(StateClosing)
}

func (rpty *bufferedReconnectingPTY) Close(err error) {
	// The closing state change will be handled by the lifecycle.
	rpty.state.setState(StateClosing, err)
}
