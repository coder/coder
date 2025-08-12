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

	// Error callback to notify parent when connection fails
	onError func(error)
}

// NewBackedReader creates a new BackedReader. The reader is initially disconnected
// and must be connected using Reconnect before reads will succeed.
func NewBackedReader() *BackedReader {
	br := &BackedReader{}
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
	for {
		// Step 1: Wait until we have a reader or are closed
		br.mu.Lock()
		for br.reader == nil && !br.closed {
			br.cond.Wait()
		}

		if br.closed {
			br.mu.Unlock()
			return 0, io.ErrClosedPipe
		}

		// Capture the current reader and release the lock while performing
		// the potentially blocking I/O operation to avoid deadlocks with Close().
		r := br.reader
		br.mu.Unlock()

		// Step 2: Perform the read without holding the mutex
		n, err := r.Read(p)

		// Step 3: Reacquire the lock to update state based on the result
		br.mu.Lock()
		if err == nil {
			br.sequenceNum += uint64(n) // #nosec G115 -- n is always >= 0 per io.Reader contract
			br.mu.Unlock()
			return n, nil
		}

		// Mark disconnected so future reads will wait for reconnection
		br.reader = nil

		if br.onError != nil {
			br.onError(err)
		}

		// If we got some data before the error, return it now
		if n > 0 {
			br.sequenceNum += uint64(n)
			br.mu.Unlock()
			return n, nil
		}

		// Otherwise loop and wait for reconnection or close
		br.mu.Unlock()
	}
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
		// Send 0 sequence number and close the channel to indicate closed state
		seqNum <- 0
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

// Closes the reader and wakes up any blocked reads.
// After closing, all Read calls will return io.ErrClosedPipe.
func (br *BackedReader) Close() error {
	br.mu.Lock()
	defer br.mu.Unlock()

	if br.closed {
		return nil
	}

	br.closed = true
	br.reader = nil

	// Wake up any blocked reads
	br.cond.Broadcast()

	return nil
}

// SetErrorCallback sets the callback function that will be called when
// a connection error occurs (excluding EOF).
func (br *BackedReader) SetErrorCallback(fn func(error)) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.onError = fn
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
