package backedpipe

import (
	"io"
	"sync"
)

// BackedReader wraps an unreliable io.Reader and makes it resilient to disconnections.
// It tracks sequence numbers for all bytes read and can handle reconnection,
// blocking reads when disconnected instead of erroring.
type BackedReader struct {
	mu          sync.Mutex
	cond        *sync.Cond
	reader      io.Reader
	sequenceNum uint64
	closed      bool

	// Error channel for generation-aware error reporting
	errorEventChan chan<- ErrorEvent

	// Current connection generation for error reporting
	currentGen uint64
}

// NewBackedReader creates a new BackedReader with generation-aware error reporting.
// The reader is initially disconnected and must be connected using Reconnect before
// reads will succeed. The errorEventChan will receive ErrorEvent structs containing
// error details, component info, and connection generation.
func NewBackedReader(errorEventChan chan<- ErrorEvent) *BackedReader {
	if errorEventChan == nil {
		panic("error event channel cannot be nil")
	}
	br := &BackedReader{
		errorEventChan: errorEventChan,
	}
	br.cond = sync.NewCond(&br.mu)
	return br
}

// Read implements io.Reader. It blocks when disconnected until either:
// 1. A reconnection is established
// 2. The reader is closed
//
// When connected, it reads from the underlying reader and updates sequence numbers.
// Connection failures are automatically detected and reported to the higher layer via callback.
func (br *BackedReader) Read(p []byte) (int, error) {
	br.mu.Lock()
	for {
		if err := br.blockUntilConnectedOrClosed(); err != nil {
			br.mu.Unlock()
			return 0, err
		}

		r := br.reader
		gen := br.currentGen

		br.mu.Unlock()
		n, err := r.Read(p)
		br.mu.Lock()

		if br.closed {
			br.reader = nil
			br.mu.Unlock()
			return 0, io.EOF
		}

		br.sequenceNum += uint64(n) // #nosec G115 -- n is always >= 0 per io.Reader contract
		if err == nil {
			br.mu.Unlock()
			return n, nil
		}

		// Mark reader as disconnected so future reads will wait for reconnection
		br.reader = nil

		// Notify parent of error with generation information (non-blocking)
		select {
		case br.errorEventChan <- ErrorEvent{
			Err:        err,
			Component:  "reader",
			Generation: gen,
		}:
		default:
			// Channel is full, drop the error.
		}

		if n > 0 {
			br.mu.Unlock()
			return n, nil
		}
	}
}

func (br *BackedReader) blockUntilConnectedOrClosed() error {
	for br.reader == nil && !br.closed {
		br.cond.Wait()
	}
	if br.closed {
		return io.EOF
	}
	return nil
}

// Reconnect coordinates reconnection using channels for better synchronization.
// The seqNum channel is used to send the current sequence number to the caller.
// The newR channel is used to receive the new reader from the caller.
// This allows for better coordination during the reconnection process.
func (br *BackedReader) Reconnect(seqNum chan<- uint64, newR <-chan io.Reader) {
	// Grab the lock
	br.mu.Lock()
	defer br.mu.Unlock()

	if br.closed {
		// Close the channel to indicate closed state
		close(seqNum)
		return
	}

	// Get the sequence number to send to the other side via seqNum channel
	seqNum <- br.sequenceNum
	close(seqNum)

	// Wait for the reconnect to complete, via newR channel, and give us a new io.Reader
	newReader := <-newR

	// If reconnection fails while we are starting it, the caller sends nil on newR
	if newReader == nil {
		// Reconnection failed, keep current state
		return
	}

	// Reconnection successful
	br.reader = newReader

	// Notify any waiting reads via the cond
	br.cond.Broadcast()
}

// Close the reader and wake up any blocked reads.
// After closing, all Read calls will return io.EOF.
func (br *BackedReader) Close() error {
	br.mu.Lock()
	if br.closed {
		br.mu.Unlock()
		return nil
	}

	br.closed = true
	currentReader := br.reader
	br.reader = nil

	// Wake up any blocked reads
	br.cond.Broadcast()

	br.mu.Unlock()

	if closer, ok := currentReader.(io.Closer); ok {
		_ = closer.Close()
	}

	return nil
}

// SequenceNum returns the current sequence number (total bytes read).
func (br *BackedReader) SequenceNum() uint64 {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.sequenceNum
}

// Connected returns whether the reader is currently connected.
func (br *BackedReader) Connected() bool {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.reader != nil
}

// SetGeneration sets the current connection generation for error reporting.
func (br *BackedReader) SetGeneration(generation uint64) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.currentGen = generation
}
