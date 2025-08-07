package backedpipe

import (
	"context"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

// BackedWriter wraps an unreliable io.Writer and makes it resilient to disconnections.
// It maintains a ring buffer of recent writes for replay during reconnection and
// always writes to the buffer even when disconnected.
type BackedWriter struct {
	mu          sync.Mutex
	cond        *sync.Cond
	writer      io.Writer
	buffer      *RingBuffer
	sequenceNum uint64 // total bytes written
	closed      bool

	// Error callback to notify parent when connection fails
	onError func(error)
}

// NewBackedWriter creates a new BackedWriter with a 64MB ring buffer.
// The writer is initially disconnected and will buffer writes until connected.
func NewBackedWriter() *BackedWriter {
	return NewBackedWriterWithCapacity(64 * 1024 * 1024)
}

// NewBackedWriterWithCapacity creates a new BackedWriter with the specified buffer capacity.
// The writer is initially disconnected and will buffer writes until connected.
func NewBackedWriterWithCapacity(capacity int) *BackedWriter {
	bw := &BackedWriter{
		buffer: NewRingBufferWithCapacity(capacity),
	}
	bw.cond = sync.NewCond(&bw.mu)
	return bw
}

// Write implements io.Writer. It always writes to the ring buffer, even when disconnected.
// When connected, it also writes to the underlying writer. If the underlying write fails,
// the writer is marked as disconnected but the buffer write still succeeds.
func (bw *BackedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return 0, io.ErrClosedPipe
	}

	// Always write to buffer first
	written, _ := bw.buffer.Write(p)
	//nolint:gosec // Safe conversion: written is always non-negative from buffer.Write
	bw.sequenceNum += uint64(written)

	// If connected, also write to underlying writer
	if bw.writer != nil {
		// Unlock during actual write to avoid blocking other operations
		bw.mu.Unlock()
		n, err := bw.writer.Write(p)
		bw.mu.Lock()

		if n != len(p) {
			err = xerrors.Errorf("partial write: wrote %d of %d bytes", n, len(p))
		}

		if err != nil {
			// Connection failed, mark as disconnected
			bw.writer = nil

			// Notify parent of error if callback is set
			if bw.onError != nil {
				bw.onError(err)
			}
		}
	}

	return written, nil
}

// Reconnect replaces the current writer with a new one and replays data from the specified
// sequence number. If the requested sequence number is no longer in the buffer,
// returns an error indicating data loss.
func (bw *BackedWriter) Reconnect(replayFromSeq uint64, newWriter io.Writer) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.closed {
		return xerrors.New("cannot reconnect closed writer")
	}

	if newWriter == nil {
		return xerrors.New("new writer cannot be nil")
	}

	// Check if we can replay from the requested sequence number
	if replayFromSeq > bw.sequenceNum {
		return xerrors.Errorf("cannot replay from future sequence %d: current sequence is %d", replayFromSeq, bw.sequenceNum)
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
			return xerrors.Errorf("failed to read replay data: %w", err)
		}
	}

	// Set new writer
	bw.writer = newWriter

	// Replay data if needed
	if len(replayData) > 0 {
		bw.mu.Unlock()
		n, err := newWriter.Write(replayData)
		bw.mu.Lock()

		if err != nil {
			bw.writer = nil
			return xerrors.Errorf("replay failed: %w", err)
		}

		if n != len(replayData) {
			bw.writer = nil
			return xerrors.Errorf("partial replay: wrote %d of %d bytes", n, len(replayData))
		}
	}

	// Wake up any operations waiting for connection
	bw.cond.Broadcast()

	return nil
}

// Close closes the writer and prevents further writes.
// After closing, all Write calls will return io.ErrClosedPipe.
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

// SetErrorCallback sets the callback function that will be called when
// a connection error occurs.
func (bw *BackedWriter) SetErrorCallback(fn func(error)) {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	bw.onError = fn
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

// CanReplayFrom returns true if the writer can replay data from the given sequence number.
func (bw *BackedWriter) CanReplayFrom(seqNum uint64) bool {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return seqNum <= bw.sequenceNum && bw.sequenceNum-seqNum <= DefaultBufferSize
}

// WaitForConnection blocks until the writer is connected or the context is canceled.
func (bw *BackedWriter) WaitForConnection(ctx context.Context) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	return bw.waitForConnectionLocked(ctx)
}

// waitForConnectionLocked waits for connection with lock held.
func (bw *BackedWriter) waitForConnectionLocked(ctx context.Context) error {
	for bw.writer == nil && !bw.closed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Use a timeout to avoid infinite waiting
			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					bw.cond.Broadcast()
				case <-done:
				}
			}()

			bw.cond.Wait()
			close(done)

			// Check context again after waking up
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
	}

	if bw.closed {
		return io.ErrClosedPipe
	}

	return nil
}
