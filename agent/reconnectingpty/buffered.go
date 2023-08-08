package reconnectingpty

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/armon/circbuf"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/pty"
)

// bufferedBackend provides a reconnectable PTY by using a ring buffer to store
// scrollback.
type bufferedBackend struct {
	command *pty.Cmd

	// mutex protects writing to the circular buffer and connections.
	mutex sync.RWMutex

	activeConns    map[string]net.Conn
	circularBuffer *circbuf.Buffer

	ptty    pty.PTYCmd
	process pty.Process

	metrics *prometheus.CounterVec

	// closeSession is used to close the session when the process dies.
	closeSession func(reason string)
}

// start initializes the backend and starts the pty.  It must be called only
// once.  If the context ends the process will be killed.
func (b *bufferedBackend) start(ctx context.Context, logger slog.Logger) error {
	b.activeConns = map[string]net.Conn{}

	// Default to buffer 64KiB.
	circularBuffer, err := circbuf.NewBuffer(64 << 10)
	if err != nil {
		return xerrors.Errorf("create circular buffer: %w", err)
	}
	b.circularBuffer = circularBuffer

	// pty.Cmd duplicates Path as the first argument so remove it.
	cmd := pty.CommandContext(ctx, b.command.Path, b.command.Args[1:]...)
	cmd.Env = append(b.command.Env, "TERM=xterm-256color")
	cmd.Dir = b.command.Dir
	ptty, process, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	b.ptty = ptty
	b.process = process

	// Multiplex the output onto the circular buffer and each active connection.
	//
	// We do not need to separately monitor for the process exiting.  When it
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
					b.metrics.WithLabelValues("output_reader").Add(1)
				}
				// Could have been killed externally or failed to start at all (command
				// not found for example).
				b.closeSession("unable to read pty output, command might have exited")
				break
			}
			part := buffer[:read]
			b.mutex.Lock()
			_, err = b.circularBuffer.Write(part)
			if err != nil {
				logger.Error(ctx, "write to circular buffer", slog.Error(err))
				b.metrics.WithLabelValues("write_buffer").Add(1)
			}
			for cid, conn := range b.activeConns {
				_, err = conn.Write(part)
				if err != nil {
					logger.Warn(ctx,
						"error writing to active connection",
						slog.F("connection_id", cid),
						slog.Error(err),
					)
					b.metrics.WithLabelValues("write").Add(1)
				}
			}
			b.mutex.Unlock()
		}
	}()

	return nil
}

// attach attaches to the pty and replays the buffer.  If the context closes it
// will detach the connection but leave the process up.  A connection ID is
// required so that logs in the pty goroutine can reference the same ID
// reference in logs output by each individual connection when acting on those
// connections.
func (b *bufferedBackend) attach(ctx context.Context, connID string, conn net.Conn, height, width uint16, logger slog.Logger) (pty.PTYCmd, error) {
	// Resize the PTY to initial height + width.
	err := b.ptty.Resize(height, width)
	if err != nil {
		// We can continue after this, it's not fatal!
		logger.Warn(ctx, "reconnecting PTY initial resize failed, but will continue", slog.Error(err))
		b.metrics.WithLabelValues("resize").Add(1)
	}

	// Write any previously stored data for the TTY and store the connection for
	// future writes.
	b.mutex.Lock()
	defer b.mutex.Unlock()
	prevBuf := slices.Clone(b.circularBuffer.Bytes())
	_, err = conn.Write(prevBuf)
	if err != nil {
		b.metrics.WithLabelValues("write").Add(1)
		return nil, xerrors.Errorf("write buffer to conn: %w", err)
	}
	b.activeConns[connID] = conn

	// Detach the connection when it or the reconnecting pty closes.
	go func() {
		<-ctx.Done()
		b.mutex.Lock()
		defer b.mutex.Unlock()
		delete(b.activeConns, connID)
	}()

	return b.ptty, nil
}

// close closes all connections to the reconnecting PTY, clears the circular
// buffer, and kills the process.
func (b *bufferedBackend) close(ctx context.Context, logger slog.Logger) error {
	var err error
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.circularBuffer.Reset()
	for _, conn := range b.activeConns {
		err = errors.Join(err, conn.Close())
	}
	pttyErr := b.ptty.Close()
	if pttyErr != nil {
		logger.Debug(ctx, "closed ptty with error", slog.Error(pttyErr))
	}
	procErr := b.process.Kill()
	if procErr != nil {
		logger.Debug(ctx, "killed process with error", slog.Error(procErr))
	}
	return err
}
