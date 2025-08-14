package backedpipe

import (
	"io"
	"sync"

	"golang.org/x/xerrors"
)

var (
	ErrWriterClosed          = xerrors.New("cannot reconnect closed writer")
	ErrNilWriter             = xerrors.New("new writer cannot be nil")
	ErrFutureSequence        = xerrors.New("cannot replay from future sequence")
	ErrReplayDataUnavailable = xerrors.New("failed to read replay data")
	ErrReplayFailed          = xerrors.New("replay failed")
	ErrPartialReplay         = xerrors.New("partial replay")
)

// BackedWriter wraps an unreliable io.Writer and makes it resilient to disconnections.
// It maintains a ring buffer of recent writes for replay during reconnection.
type BackedWriter struct {
	mu          sync.Mutex
	cond        *sync.Cond
	writer      io.Writer
	buffer      *ringBuffer
	sequenceNum uint64 // total bytes written
	closed      bool

	// Error channel to notify parent when connection fails
	errorChan chan<- error
}

// NewBackedWriter creates a new BackedWriter with the specified buffer capacity.
// The writer is initially disconnected and will block writes until connected.
// The errorChan is required and will receive connection errors.
// Capacity must be > 0.
func NewBackedWriter(capacity int, errorChan chan<- error) *BackedWriter {
	if capacity <= 0 {
		panic("backed writer capacity must be > 0")
	}
	if errorChan == nil {
		panic("error channel cannot be nil")
	}
	bw := &BackedWriter{
		buffer:    newRingBuffer(capacity),
		errorChan: errorChan,
	}
	bw.cond = sync.NewCond(&bw.mu)
	return bw
}

// blockUntilConnectedOrClosed blocks until either a writer is available or the BackedWriter is closed.
// Returns io.EOF if closed while waiting, nil if connected.
func (bw *BackedWriter) blockUntilConnectedOrClosed() error {
	for bw.writer == nil && !bw.closed {
		bw.cond.Wait()
	}
	if bw.closed {
		return io.EOF
	}
	return nil
}

// Write implements io.Writer.
// When connected, it writes to both the ring buffer (to preserve data in case we need to replay it)
// and the underlying writer.
// If the underlying write fails, the writer is marked as disconnected and the write blocks
// until reconnection occurs.
func (bw *BackedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return 0, io.EOF
	}

	// Block until connected
	if err := bw.blockUntilConnectedOrClosed(); err != nil {
		return 0, err
	}

	// Write to buffer
	bw.buffer.Write(p)
	bw.sequenceNum += uint64(len(p))

	// Try to write to underlying writer
	n, err := bw.writer.Write(p)
	if err != nil {
		// Connection failed, mark as disconnected
		bw.writer = nil

		// Notify parent of error
		select {
		case bw.errorChan <- err:
		default:
		}

		// Block until reconnected - reconnection will replay this data
		if err := bw.blockUntilConnectedOrClosed(); err != nil {
			return 0, err
		}

		// Don't retry - reconnection replay handled it
		return len(p), nil
	}

	if n != len(p) {
		// Partial write - treat as failure
		bw.writer = nil
		err = xerrors.Errorf("short write: %d bytes written, %d expected", n, len(p))
		select {
		case bw.errorChan <- err:
		default:
		}

		// Block until reconnected - reconnection will replay this data
		if err := bw.blockUntilConnectedOrClosed(); err != nil {
			return 0, err
		}

		// Don't retry - reconnection replay handled it
		return len(p), nil
	}

	// Write succeeded
	return len(p), nil
}

// Reconnect replaces the current writer with a new one and replays data from the specified
// sequence number. If the requested sequence number is no longer in the buffer,
// returns an error indicating data loss.
func (bw *BackedWriter) Reconnect(replayFromSeq uint64, newWriter io.Writer) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return ErrWriterClosed
	}

	if newWriter == nil {
		return ErrNilWriter
	}

	// Check if we can replay from the requested sequence number
	if replayFromSeq > bw.sequenceNum {
		return ErrFutureSequence
	}

	// Calculate how many bytes we need to replay
	replayBytes := bw.sequenceNum - replayFromSeq

	var replayData []byte
	if replayBytes > 0 {
		// Get the last replayBytes from buffer
		// If the buffer doesn't have enough data (some was evicted),
		// ReadLast will return an error
		var err error
		// Safe conversion: replayBytes is always non-negative due to the check above
		// No overflow possible since replayBytes is calculated as sequenceNum - replayFromSeq
		// and uint64->int conversion is safe for reasonable buffer sizes
		//nolint:gosec // Safe conversion: replayBytes is calculated from uint64 subtraction
		replayData, err = bw.buffer.ReadLast(int(replayBytes))
		if err != nil {
			return ErrReplayDataUnavailable
		}
	}

	// Clear the current writer first in case replay fails
	bw.writer = nil

	// Replay data if needed. We keep the writer as nil during replay to ensure
	// no concurrent writes can happen, then set it only after successful replay.
	if len(replayData) > 0 {
		bw.mu.Unlock()
		n, err := newWriter.Write(replayData)
		bw.mu.Lock()

		if err != nil {
			// Reconnect failed, writer remains nil
			return ErrReplayFailed
		}

		if n != len(replayData) {
			// Reconnect failed, writer remains nil
			return ErrPartialReplay
		}
	}

	// Set new writer only after successful replay. This ensures no concurrent
	// writes can interfere with the replay operation.
	bw.writer = newWriter

	// Wake up any operations waiting for connection
	bw.cond.Broadcast()

	return nil
}

// Close closes the writer and prevents further writes.
// After closing, all Write calls will return io.EOF.
// This code keeps the Close() signature consistent with io.Closer,
// but it never actually returns an error.
func (bw *BackedWriter) Close() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return nil
	}

	bw.closed = true
	bw.writer = nil

	// Wake up any blocked operations
	bw.cond.Broadcast()

	return nil
}

// SequenceNum returns the current sequence number (total bytes written).
func (bw *BackedWriter) SequenceNum() uint64 {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.sequenceNum
}

// Connected returns whether the writer is currently connected.
func (bw *BackedWriter) Connected() bool {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.writer != nil
}
