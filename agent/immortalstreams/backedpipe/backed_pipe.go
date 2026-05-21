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

// connectionState represents the current state of the BackedPipe connection.
type connectionState int

const (
	// connected indicates the pipe is connected and operational.
	connected connectionState = iota
	// disconnected indicates the pipe is not connected but not closed.
	disconnected
	// reconnecting indicates a reconnection attempt is in progress.
	reconnecting
	// closed indicates the pipe is permanently closed.
	closed
)

// ErrorEvent represents an error from a reader or writer with connection generation info.
type ErrorEvent struct {
	Err        error
	Component  string // "reader" or "writer"
	Generation uint64 // connection generation when error occurred
}

const (
	// Default buffer capacity used by the writer - 64MB
	DefaultBufferSize = 64 * 1024 * 1024
)

// Reconnector is an interface for establishing connections when the BackedPipe needs to reconnect.
// Implementations should:
// 1. Establish a new connection to the remote side
// 2. Exchange sequence numbers with the remote side
// 3. Return the new connection and the remote's reader sequence number
//
// The readerSeqNum parameter is the local reader's current sequence number
// (total bytes successfully read from the remote). This must be sent to the
// remote so it can replay its data to us starting from this number.
//
// The returned remoteReaderSeqNum should be the remote side's reader sequence
// number (how many bytes of our outbound data it has successfully read). This
// informs our writer where to resume (i.e., which bytes to replay to the remote).
type Reconnector interface {
	Reconnect(ctx context.Context, readerSeqNum uint64) (conn io.ReadWriteCloser, remoteReaderSeqNum uint64, err error)
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

	// State machine
	state   connectionState
	connGen uint64 // Increments on each successful reconnection

	// Unified error handling with generation filtering
	errChan chan ErrorEvent

	// forceReconnectHook is a test hook invoked after ForceReconnect registers
	// with the singleflight group.
	forceReconnectHook func()

	// singleflight group to dedupe concurrent ForceReconnect calls
	sf singleflight.Group

	// Track first error per generation to avoid duplicate reconnections
	lastErrorGen uint64
}

// NewBackedPipe creates a new BackedPipe with default options and the specified reconnector.
// The pipe starts disconnected and must be connected using Connect().
func NewBackedPipe(ctx context.Context, reconnector Reconnector) *BackedPipe {
	pipeCtx, cancel := context.WithCancel(ctx)

	errChan := make(chan ErrorEvent, 1)

	bp := &BackedPipe{
		ctx:         pipeCtx,
		cancel:      cancel,
		reconnector: reconnector,
		state:       disconnected,
		connGen:     0, // Start with generation 0
		errChan:     errChan,
	}

	// Create reader and writer with typed error channel for generation-aware error reporting
	bp.reader = NewBackedReader(errChan)
	bp.writer = NewBackedWriter(DefaultBufferSize, errChan)

	// Start error handler goroutine
	go bp.handleErrors()

	return bp
}

// Connect establishes the initial connection using the reconnect function.
func (bp *BackedPipe) Connect() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.state == closed {
		return ErrPipeClosed
	}

	if bp.state == connected {
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
	state := bp.state
	bp.mu.RUnlock()

	if state == closed {
		return 0, io.EOF
	}

	return writer.Write(p)
}

// Close closes the pipe and all underlying connections.
func (bp *BackedPipe) Close() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.state == closed {
		return nil
	}

	bp.state = closed
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

	// Wait for all close operations to complete and return any error
	return g.Wait()
}

// Connected returns whether the pipe is currently connected.
func (bp *BackedPipe) Connected() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.state == connected && bp.reader.Connected() && bp.writer.Connected()
}

// reconnectLocked handles the reconnection logic. Must be called with write lock held.
func (bp *BackedPipe) reconnectLocked() error {
	if bp.state == reconnecting {
		return ErrReconnectionInProgress
	}

	bp.state = reconnecting
	defer func() {
		// Only reset to disconnected if we're still in reconnecting state
		// (successful reconnection will set state to connected)
		if bp.state == reconnecting {
			bp.state = disconnected
		}
	}()

	// Close existing connection if any
	if bp.conn != nil {
		_ = bp.conn.Close()
		bp.conn = nil
	}

	// Increment the generation and update both reader and writer.
	// We do it now to track even the connections that fail during
	// Reconnect.
	bp.connGen++
	bp.reader.SetGeneration(bp.connGen)
	bp.writer.SetGeneration(bp.connGen)

	// Reconnect reader and writer
	seqNum := make(chan uint64, 1)
	newR := make(chan io.Reader, 1)

	go bp.reader.Reconnect(seqNum, newR)

	// Get the precise reader sequence number from the reader while it holds its lock
	readerSeqNum, ok := <-seqNum
	if !ok {
		// Reader was closed during reconnection
		return ErrReconnectFailed
	}

	// Perform reconnect using the exact sequence number we just received
	conn, remoteReaderSeqNum, err := bp.reconnector.Reconnect(bp.ctx, readerSeqNum)
	if err != nil {
		// Unblock reader reconnect
		newR <- nil
		return ErrReconnectFailed
	}

	// Provide the new connection to the reader (reader still holds its lock)
	newR <- conn

	// Replay our outbound data from the remote's reader sequence number
	writerReconnectErr := bp.writer.Reconnect(remoteReaderSeqNum, conn)
	if writerReconnectErr != nil {
		return ErrReconnectWriterFailed
	}

	// Success - update state
	bp.conn = conn
	bp.state = connected

	return nil
}

// handleErrors listens for connection errors from reader/writer and triggers reconnection.
// It filters errors from old connections and ensures only the first error per generation
// triggers reconnection.
func (bp *BackedPipe) handleErrors() {
	for {
		select {
		case <-bp.ctx.Done():
			return
		case errorEvt := <-bp.errChan:
			bp.handleConnectionError(errorEvt)
		}
	}
}

// handleConnectionError handles errors from either reader or writer components.
// It filters errors from old connections and ensures only one reconnection per generation.
func (bp *BackedPipe) handleConnectionError(errorEvt ErrorEvent) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Skip if already closed
	if bp.state == closed {
		return
	}

	// Filter errors from old connections (lower generation)
	if errorEvt.Generation < bp.connGen {
		return
	}

	// Skip if not connected (already disconnected or reconnecting)
	if bp.state != connected {
		return
	}

	// Skip if we've already seen an error for this generation
	if bp.lastErrorGen >= errorEvt.Generation {
		return
	}

	// This is the first error for this generation
	bp.lastErrorGen = errorEvt.Generation

	// Mark as disconnected
	bp.state = disconnected

	// Try to reconnect using internal context
	reconnectErr := bp.reconnectLocked()

	if reconnectErr != nil {
		// Reconnection failed - log or handle as needed
		// For now, we'll just continue and wait for manual reconnection
		_ = errorEvt.Err       // Use the original error from the component
		_ = errorEvt.Component // Component info available for potential logging by higher layers
	}
}

// SetForceReconnectHookForTests sets a hook invoked after ForceReconnect
// registers with the singleflight group. It must be set before any
// concurrent ForceReconnect calls.
func (bp *BackedPipe) SetForceReconnectHookForTests(hook func()) {
	bp.forceReconnectHook = hook
}

// ForceReconnect forces a reconnection attempt immediately.
// This can be used to force a reconnection if a new connection is established.
// It prevents duplicate reconnections when called concurrently.
func (bp *BackedPipe) ForceReconnect() error {
	// Deduplicate concurrent ForceReconnect calls so only one reconnection
	// attempt runs at a time from this API. Use the pipe's internal context
	// to ensure Close() cancels any in-flight attempt.
	resultChan := bp.sf.DoChan("force-reconnect", func() (interface{}, error) {
		bp.mu.Lock()
		defer bp.mu.Unlock()

		if bp.state == closed {
			return nil, io.EOF
		}

		// Don't force reconnect if already reconnecting
		if bp.state == reconnecting {
			return nil, ErrReconnectionInProgress
		}

		return nil, bp.reconnectLocked()
	})

	if hook := bp.forceReconnectHook; hook != nil {
		hook()
	}

	result := <-resultChan
	return result.Err
}
