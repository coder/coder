package backedpipe

import (
	"context"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"
)

var (
	ErrPipeClosed             = xerrors.New("pipe is closed")
	ErrPipeAlreadyConnected   = xerrors.New("pipe is already connected")
	ErrReconnectionInProgress = xerrors.New("reconnection already in progress")
	ErrReconnectFailed        = xerrors.New("reconnect failed")
	ErrInvalidSequenceNumber  = xerrors.New("remote sequence number exceeds local sequence")
	ErrReconnectWriterFailed  = xerrors.New("reconnect writer failed")
)

const (
	// Default buffer capacity used by the writer - 64MB
	DefaultBufferSize = 64 * 1024 * 1024
)

// Reconnector is an interface for establishing connections when the BackedPipe needs to reconnect.
// Implementations should:
// 1. Establish a new connection to the remote side
// 2. Exchange sequence numbers with the remote side
// 3. Return the new connection and the remote's current sequence number
//
// The writerSeqNum parameter is the local writer's current sequence number,
// which should be sent to the remote side so it knows where to resume reading from.
//
// The returned readerSeqNum should be the remote side's current sequence number,
// which indicates where the local reader should resume from.
type Reconnector interface {
	Reconnect(ctx context.Context, writerSeqNum uint64) (conn io.ReadWriteCloser, readerSeqNum uint64, err error)
}

// BackedPipe provides a reliable bidirectional byte stream over unreliable network connections.
// It orchestrates a BackedReader and BackedWriter to provide transparent reconnection
// and data replay capabilities.
type BackedPipe struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	reader      *BackedReader
	writer      *BackedWriter
	reconnector Reconnector
	conn        io.ReadWriteCloser
	connected   bool
	closed      bool

	// Reconnection state
	reconnecting bool

	// Error channels for receiving connection errors from reader/writer separately
	readerErrorChan chan error
	writerErrorChan chan error

	// singleflight group to dedupe concurrent ForceReconnect calls
	sf singleflight.Group
}

// NewBackedPipe creates a new BackedPipe with default options and the specified reconnector.
// The pipe starts disconnected and must be connected using Connect().
func NewBackedPipe(ctx context.Context, reconnector Reconnector) *BackedPipe {
	pipeCtx, cancel := context.WithCancel(ctx)

	readerErrorChan := make(chan error, 1) // Buffer for reader errors
	writerErrorChan := make(chan error, 1) // Buffer for writer errors
	bp := &BackedPipe{
		ctx:             pipeCtx,
		cancel:          cancel,
		reader:          NewBackedReader(readerErrorChan),
		writer:          NewBackedWriter(DefaultBufferSize, writerErrorChan),
		reconnector:     reconnector,
		readerErrorChan: readerErrorChan,
		writerErrorChan: writerErrorChan,
	}

	// Start error handler goroutine
	go bp.handleErrors()

	return bp
}

// Connect establishes the initial connection using the reconnect function.
func (bp *BackedPipe) Connect() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.closed {
		return ErrPipeClosed
	}

	if bp.connected {
		return ErrPipeAlreadyConnected
	}

	// Use internal context for the actual reconnect operation to ensure
	// Close() reliably cancels any in-flight attempt.
	return bp.reconnectLocked()
}

// Read implements io.Reader by delegating to the BackedReader.
func (bp *BackedPipe) Read(p []byte) (int, error) {
	return bp.reader.Read(p)
}

// Write implements io.Writer by delegating to the BackedWriter.
func (bp *BackedPipe) Write(p []byte) (int, error) {
	bp.mu.RLock()
	writer := bp.writer
	closed := bp.closed
	bp.mu.RUnlock()

	if closed {
		return 0, io.EOF
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

	// Close all components in parallel to avoid deadlocks
	//
	// IMPORTANT: The connection must be closed first to unblock any
	// readers or writers that might be holding the mutex on Read/Write
	var g errgroup.Group

	if bp.conn != nil {
		conn := bp.conn
		g.Go(func() error {
			return conn.Close()
		})
		bp.conn = nil
	}

	if bp.reader != nil {
		reader := bp.reader
		g.Go(func() error {
			return reader.Close()
		})
	}

	if bp.writer != nil {
		writer := bp.writer
		g.Go(func() error {
			return writer.Close()
		})
	}

	bp.connected = false

	// Wait for all close operations to complete and return any error
	return g.Wait()
}

// Connected returns whether the pipe is currently connected.
func (bp *BackedPipe) Connected() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.connected
}

// reconnectLocked handles the reconnection logic. Must be called with write lock held.
func (bp *BackedPipe) reconnectLocked() error {
	if bp.reconnecting {
		return ErrReconnectionInProgress
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

	// Get current writer sequence number to send to remote
	writerSeqNum := bp.writer.SequenceNum()

	conn, readerSeqNum, err := bp.reconnector.Reconnect(bp.ctx, writerSeqNum)
	if err != nil {
		return ErrReconnectFailed
	}

	// Validate sequence numbers
	if readerSeqNum > writerSeqNum {
		_ = conn.Close()
		return ErrInvalidSequenceNumber
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
		return ErrReconnectWriterFailed
	}

	// Success - update state
	bp.conn = conn
	bp.connected = true

	return nil
}

// handleErrors listens for connection errors from reader/writer and triggers reconnection.
func (bp *BackedPipe) handleErrors() {
	for {
		select {
		case <-bp.ctx.Done():
			return
		case err := <-bp.readerErrorChan:
			// Reader connection error occurred
			bp.handleConnectionError(err, "reader")
		case err := <-bp.writerErrorChan:
			// Writer connection error occurred
			bp.handleConnectionError(err, "writer")
		}
	}
}

// handleConnectionError handles errors from either reader or writer components.
func (bp *BackedPipe) handleConnectionError(err error, component string) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Skip if already closed or not connected
	if bp.closed || !bp.connected {
		return
	}

	// Mark as disconnected
	bp.connected = false

	// Try to reconnect using internal context
	reconnectErr := bp.reconnectLocked()

	if reconnectErr != nil {
		// Reconnection failed - log or handle as needed
		// For now, we'll just continue and wait for manual reconnection
		_ = err       // Use the original error from the component
		_ = component // Component info available for potential logging by higher layers
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
			return nil, io.EOF
		}

		return nil, bp.reconnectLocked()
	})
	return err
}
