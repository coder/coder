package backedpipe

import (
	"context"
	"io"
	"sync"

	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"
)

const (
	// Default buffer capacity used by the writer - 64MB
	DefaultBufferSize = 64 * 1024 * 1024
)

// ReconnectFunc is called when the BackedPipe needs to establish a new connection.
// It should:
// 1. Establish a new connection to the remote side
// 2. Exchange sequence numbers with the remote side
// 3. Return the new connection and the remote's current sequence number
//
// The writerSeqNum parameter is the local writer's current sequence number,
// which should be sent to the remote side so it knows where to resume reading from.
//
// The returned readerSeqNum should be the remote side's current sequence number,
// which indicates where the local reader should resume from.
type ReconnectFunc func(ctx context.Context, writerSeqNum uint64) (conn io.ReadWriteCloser, readerSeqNum uint64, err error)

// BackedPipe provides a reliable bidirectional byte stream over unreliable network connections.
// It orchestrates a BackedReader and BackedWriter to provide transparent reconnection
// and data replay capabilities.
type BackedPipe struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	reader      *BackedReader
	writer      *BackedWriter
	reconnectFn ReconnectFunc
	conn        io.ReadWriteCloser
	connected   bool
	closed      bool

	// Reconnection state
	reconnecting bool

	// Error channel for receiving connection errors from reader/writer
	errorChan chan error

	// Connection state notification
	connectionChanged chan struct{}

	// singleflight group to dedupe concurrent ForceReconnect calls
	sf singleflight.Group
}

// NewBackedPipe creates a new BackedPipe with default options and the specified reconnect function.
// The pipe starts disconnected and must be connected using Connect().
func NewBackedPipe(ctx context.Context, reconnectFn ReconnectFunc) *BackedPipe {
	pipeCtx, cancel := context.WithCancel(ctx)

	errorChan := make(chan error, 2) // Buffer for reader and writer errors
	bp := &BackedPipe{
		ctx:               pipeCtx,
		cancel:            cancel,
		reader:            NewBackedReader(),
		writer:            NewBackedWriter(DefaultBufferSize, errorChan),
		reconnectFn:       reconnectFn,
		errorChan:         errorChan,
		connectionChanged: make(chan struct{}, 1),
	}

	// Set up error callback for reader only (writer uses error channel directly)
	bp.reader.SetErrorCallback(func(err error) {
		select {
		case bp.errorChan <- err:
		case <-bp.ctx.Done():
		}
	})

	// Start error handler goroutine
	go bp.handleErrors()

	return bp
}

// Connect establishes the initial connection using the reconnect function.
func (bp *BackedPipe) Connect(_ context.Context) error { // external ctx ignored; internal ctx used
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.closed {
		return xerrors.New("pipe is closed")
	}

	if bp.connected {
		return xerrors.New("pipe is already connected")
	}

	// Use internal context for the actual reconnect operation to ensure
	// Close() reliably cancels any in-flight attempt.
	return bp.reconnectLocked()
}

// Read implements io.Reader by delegating to the BackedReader.
func (bp *BackedPipe) Read(p []byte) (int, error) {
	bp.mu.RLock()
	reader := bp.reader
	closed := bp.closed
	bp.mu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	return reader.Read(p)
}

// Write implements io.Writer by delegating to the BackedWriter.
func (bp *BackedPipe) Write(p []byte) (int, error) {
	bp.mu.RLock()
	writer := bp.writer
	closed := bp.closed
	bp.mu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	return writer.Write(p)
}

// Close closes the pipe and all underlying connections.
func (bp *BackedPipe) Close() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.closed {
		return nil
	}

	bp.closed = true
	bp.cancel() // Cancel main context

	// Close underlying components
	var readerErr, writerErr, connErr error

	if bp.reader != nil {
		readerErr = bp.reader.Close()
	}

	if bp.writer != nil {
		writerErr = bp.writer.Close()
	}

	if bp.conn != nil {
		connErr = bp.conn.Close()
		bp.conn = nil
	}

	bp.connected = false
	bp.signalConnectionChange()

	// Return first error encountered
	if readerErr != nil {
		return readerErr
	}
	if writerErr != nil {
		return writerErr
	}
	return connErr
}

// Connected returns whether the pipe is currently connected.
func (bp *BackedPipe) Connected() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.connected
}

// signalConnectionChange signals that the connection state has changed.
func (bp *BackedPipe) signalConnectionChange() {
	select {
	case bp.connectionChanged <- struct{}{}:
	default:
		// Channel is full, which is fine - we just want to signal that something changed
	}
}

// reconnectLocked handles the reconnection logic. Must be called with write lock held.
func (bp *BackedPipe) reconnectLocked() error {
	if bp.reconnecting {
		return xerrors.New("reconnection already in progress")
	}

	bp.reconnecting = true
	defer func() {
		bp.reconnecting = false
	}()

	// Close existing connection if any
	if bp.conn != nil {
		_ = bp.conn.Close()
		bp.conn = nil
	}

	bp.connected = false
	bp.signalConnectionChange()

	// Get current writer sequence number to send to remote
	writerSeqNum := bp.writer.SequenceNum()

	// Unlock during reconnect attempt to avoid blocking reads/writes
	bp.mu.Unlock()
	conn, readerSeqNum, err := bp.reconnectFn(bp.ctx, writerSeqNum)
	bp.mu.Lock()

	if err != nil {
		return xerrors.Errorf("reconnect failed: %w", err)
	}

	// Validate sequence numbers
	if readerSeqNum > writerSeqNum {
		_ = conn.Close()
		return xerrors.Errorf("remote sequence number %d exceeds local sequence %d, cannot replay",
			readerSeqNum, writerSeqNum)
	}

	// Reconnect reader and writer
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go bp.reader.Reconnect(seqNum, newR)

	// Get sequence number and send new reader
	<-seqNum
	newR <- conn

	err = bp.writer.Reconnect(readerSeqNum, conn)
	if err != nil {
		_ = conn.Close()
		return xerrors.Errorf("reconnect writer: %w", err)
	}

	// Success - update state
	bp.conn = conn
	bp.connected = true
	bp.signalConnectionChange()

	return nil
}

// handleErrors listens for connection errors from reader/writer and triggers reconnection.
func (bp *BackedPipe) handleErrors() {
	for {
		select {
		case <-bp.ctx.Done():
			return
		case err := <-bp.errorChan:
			// Connection error occurred
			bp.mu.Lock()

			// Skip if already closed or not connected
			if bp.closed || !bp.connected {
				bp.mu.Unlock()
				continue
			}

			// Mark as disconnected
			bp.connected = false
			bp.signalConnectionChange()

			// Try to reconnect using internal context
			reconnectErr := bp.reconnectLocked()
			bp.mu.Unlock()

			if reconnectErr != nil {
				// Reconnection failed - log or handle as needed
				// For now, we'll just continue and wait for manual reconnection
				_ = err // Use the original error
			}
		}
	}
}

// ForceReconnect forces a reconnection attempt immediately.
// This can be used to force a reconnection if a new connection is established.
func (bp *BackedPipe) ForceReconnect() error {
	// Deduplicate concurrent ForceReconnect calls so only one reconnection
	// attempt runs at a time from this API. Use the pipe's internal context
	// to ensure Close() cancels any in-flight attempt.
	_, err, _ := bp.sf.Do("backedpipe-reconnect", func() (interface{}, error) {
		bp.mu.Lock()
		defer bp.mu.Unlock()

		if bp.closed {
			return nil, io.ErrClosedPipe
		}

		return nil, bp.reconnectLocked()
	})
	return err
}
