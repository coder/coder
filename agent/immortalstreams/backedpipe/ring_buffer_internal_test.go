package backedpipe

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRingBuffer_ClearInternal(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(10)
	rb.Write([]byte("hello"))
	require.Equal(t, 5, rb.size)

	rb.Clear()
	require.Equal(t, 0, rb.size)
	require.Equal(t, "", rb.String())
}

func TestRingBuffer_Available(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(10)
	require.Equal(t, 10, rb.Available())

	rb.Write([]byte("hello"))
	require.Equal(t, 5, rb.Available())

	rb.Write([]byte("world"))
	require.Equal(t, 0, rb.Available())
}

func TestRingBuffer_StringInternal(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(10)
	require.Equal(t, "", rb.String())

	rb.Write([]byte("hello"))
	require.Equal(t, "hello", rb.String())

	rb.Write([]byte("world"))
	require.Equal(t, "helloworld", rb.String())
}

func TestRingBuffer_StringWithWrapAround(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(5)
	rb.Write([]byte("hello"))
	require.Equal(t, "hello", rb.String())

	rb.Write([]byte("world"))
	require.Equal(t, "world", rb.String())
}

func TestRingBuffer_ConcurrentAccessWithString(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(1000)
	var wg sync.WaitGroup

	// Start multiple goroutines writing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := fmt.Sprintf("data-%d", id)
			for j := 0; j < 100; j++ {
				rb.Write([]byte(data))
			}
		}(i)
	}

	wg.Wait()

	// Verify buffer is still in valid state
	require.NotEmpty(t, rb.String())
}

func TestRingBuffer_EdgeCaseEvictionWithString(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(3)
	rb.Write([]byte("hello"))
	rb.Write([]byte("world"))

	// Should evict "he" and keep "llo world"
	require.Equal(t, "rld", rb.String())

	// Write more data to cause more eviction
	rb.Write([]byte("test"))
	require.Equal(t, "est", rb.String())
}

// TestRingBuffer_ComplexWrapAroundScenarioWithString tests complex wrap-around with String
func TestRingBuffer_ComplexWrapAroundScenarioWithString(t *testing.T) {
	t.Parallel()

	rb := NewRingBufferWithCapacity(5)

	// Fill buffer
	rb.Write([]byte("abcde"))
	require.Equal(t, "abcde", rb.String())

	// Write more to cause wrap-around
	rb.Write([]byte("fgh"))
	require.Equal(t, "defgh", rb.String())

	// Write even more
	rb.Write([]byte("ijklmn"))
	require.Equal(t, "jklmn", rb.String())
}

// Helper function to get available space (for internal tests only)
func (rb *RingBuffer) Available() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.cap - rb.size
}

// Helper function to clear buffer (for internal tests only)
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.start = 0
	rb.end = 0
	rb.size = 0
}

// Helper function to get string representation (for internal tests only)
func (rb *RingBuffer) String() string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.size == 0 {
		return ""
	}

	// readAllInternal equivalent for internal tests
	if rb.size == 0 {
		return ""
	}

	result := make([]byte, rb.size)

	if rb.start+rb.size <= rb.cap {
		// No wrap needed
		copy(result, rb.buffer[rb.start:rb.start+rb.size])
	} else {
		// Need to wrap around
		firstChunk := rb.cap - rb.start
		copy(result[0:firstChunk], rb.buffer[rb.start:rb.cap])
		copy(result[firstChunk:], rb.buffer[0:rb.size-firstChunk])
	}

	return string(result)
}
