package backedpipe

import (
	"sync"

	"golang.org/x/xerrors"
)

// RingBuffer implements an efficient circular buffer with a fixed-size allocation.
// It supports concurrent access and handles wrap-around seamlessly.
// The buffer is designed for high-performance scenarios where avoiding
// dynamic memory allocation during operation is critical.
type RingBuffer struct {
	mu     sync.RWMutex
	buffer []byte
	start  int // index of first valid byte
	end    int // index after last valid byte
	size   int // current number of bytes in buffer
	cap    int // maximum capacity
}

// NewRingBuffer creates a new ring buffer with 64MB capacity.
func NewRingBuffer() *RingBuffer {
	const capacity = 64 * 1024 * 1024 // 64MB
	return NewRingBufferWithCapacity(capacity)
}

// NewRingBufferWithCapacity creates a new ring buffer with the specified capacity.
// If capacity is <= 0, it defaults to 64MB.
func NewRingBufferWithCapacity(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 64 * 1024 * 1024 // Default to 64MB
	}
	return &RingBuffer{
		buffer: make([]byte, capacity),
		cap:    capacity,
	}
}

// Write writes data to the ring buffer. If the buffer would overflow,
// it evicts the oldest data to make room for new data.
// Returns the number of bytes written and the number of bytes evicted.
func (rb *RingBuffer) Write(data []byte) (written int, evicted int) {
	if len(data) == 0 {
		return 0, 0
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	written = len(data)

	// If data is larger than capacity, only keep the last capacity bytes
	if len(data) > rb.cap {
		evicted = len(data) - rb.cap
		data = data[evicted:]
		written = rb.cap
		// Clear buffer and write new data
		rb.start = 0
		rb.end = 0
		rb.size = 0
	}

	// Calculate how much we need to evict to fit new data
	spaceNeeded := len(data)
	availableSpace := rb.cap - rb.size

	if spaceNeeded > availableSpace {
		bytesToEvict := spaceNeeded - availableSpace
		evicted += bytesToEvict
		rb.evict(bytesToEvict)
	}

	// Write the data
	for _, b := range data {
		rb.buffer[rb.end] = b
		rb.end = (rb.end + 1) % rb.cap
		rb.size++
	}

	return written, evicted
}

// evict removes the specified number of bytes from the beginning of the buffer.
// Must be called with lock held.
func (rb *RingBuffer) evict(count int) {
	if count >= rb.size {
		// Evict everything
		rb.start = 0
		rb.end = 0
		rb.size = 0
		return
	}

	rb.start = (rb.start + count) % rb.cap
	rb.size -= count
}

// ReadLast returns the last n bytes from the buffer.
// If n is greater than the available data, returns all available data.
// If n is 0 or negative, returns nil.
func (rb *RingBuffer) ReadLast(n int) ([]byte, error) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n <= 0 {
		return nil, nil
	}

	if rb.size == 0 {
		return nil, xerrors.New("buffer is empty")
	}

	// If requested more than available, return error
	if n > rb.size {
		return nil, xerrors.Errorf("requested %d bytes but only %d available", n, rb.size)
	}

	result := make([]byte, n)

	// Calculate where to start reading from (n bytes before the end)
	startOffset := rb.size - n
	actualStart := rb.start + startOffset
	if rb.cap > 0 {
		actualStart %= rb.cap
	}

	// Copy the last n bytes
	if actualStart+n <= rb.cap {
		// No wrap needed
		copy(result, rb.buffer[actualStart:actualStart+n])
	} else {
		// Need to wrap around
		firstChunk := rb.cap - actualStart
		copy(result[0:firstChunk], rb.buffer[actualStart:rb.cap])
		copy(result[firstChunk:], rb.buffer[0:n-firstChunk])
	}

	return result, nil
}
