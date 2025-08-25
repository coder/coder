package backedpipe

import "golang.org/x/xerrors"

// ringBuffer implements an efficient circular buffer with a fixed-size allocation.
// This implementation is not thread-safe and relies on external synchronization.
type ringBuffer struct {
	buffer []byte
	start  int // index of first valid byte
	end    int // index of last valid byte (-1 when empty)
}

// newRingBuffer creates a new ring buffer with the specified capacity.
// Capacity must be > 0.
func newRingBuffer(capacity int) *ringBuffer {
	if capacity <= 0 {
		panic("ring buffer capacity must be > 0")
	}
	return &ringBuffer{
		buffer: make([]byte, capacity),
		end:    -1, // -1 indicates empty buffer
	}
}

// Size returns the current number of bytes in the buffer.
func (rb *ringBuffer) Size() int {
	if rb.end == -1 {
		return 0 // Buffer is empty
	}
	if rb.start <= rb.end {
		return rb.end - rb.start + 1
	}
	// Buffer wraps around
	return len(rb.buffer) - rb.start + rb.end + 1
}

// Write writes data to the ring buffer. If the buffer would overflow,
// it evicts the oldest data to make room for new data.
func (rb *ringBuffer) Write(data []byte) {
	if len(data) == 0 {
		return
	}

	capacity := len(rb.buffer)

	// If data is larger than capacity, only keep the last capacity bytes
	if len(data) > capacity {
		data = data[len(data)-capacity:]
		// Clear buffer and write new data
		rb.start = 0
		rb.end = -1 // Will be set properly below
	}

	// Calculate how much we need to evict to fit new data
	spaceNeeded := len(data)
	availableSpace := capacity - rb.Size()

	if spaceNeeded > availableSpace {
		bytesToEvict := spaceNeeded - availableSpace
		rb.evict(bytesToEvict)
	}

	// Buffer has data, write after current end
	writePos := (rb.end + 1) % capacity
	if writePos+len(data) <= capacity {
		// No wrap needed - single copy
		copy(rb.buffer[writePos:], data)
		rb.end = (rb.end + len(data)) % capacity
	} else {
		// Need to wrap around - two copies
		firstChunk := capacity - writePos
		copy(rb.buffer[writePos:], data[:firstChunk])
		copy(rb.buffer[0:], data[firstChunk:])
		rb.end = len(data) - firstChunk - 1
	}
}

// evict removes the specified number of bytes from the beginning of the buffer.
func (rb *ringBuffer) evict(count int) {
	if count >= rb.Size() {
		// Evict everything
		rb.start = 0
		rb.end = -1
		return
	}

	rb.start = (rb.start + count) % len(rb.buffer)
	// Buffer remains non-empty after partial eviction
}

// ReadLast returns the last n bytes from the buffer.
// If n is greater than the available data, returns an error.
// If n is negative, returns an error.
func (rb *ringBuffer) ReadLast(n int) ([]byte, error) {
	if n < 0 {
		return nil, xerrors.New("cannot read negative number of bytes")
	}

	if n == 0 {
		return nil, nil
	}

	size := rb.Size()

	// If requested more than available, return error
	if n > size {
		return nil, xerrors.Errorf("requested %d bytes but only %d available", n, size)
	}

	result := make([]byte, n)
	capacity := len(rb.buffer)

	// Calculate where to start reading from (n bytes before the end)
	startOffset := size - n
	actualStart := (rb.start + startOffset) % capacity

	// Copy the last n bytes
	if actualStart+n <= capacity {
		// No wrap needed
		copy(result, rb.buffer[actualStart:actualStart+n])
	} else {
		// Need to wrap around
		firstChunk := capacity - actualStart
		copy(result[0:firstChunk], rb.buffer[actualStart:capacity])
		copy(result[firstChunk:], rb.buffer[0:n-firstChunk])
	}

	return result, nil
}
