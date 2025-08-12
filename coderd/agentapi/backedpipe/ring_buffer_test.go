package backedpipe_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/agentapi/backedpipe"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		// Don't run goleak on windows tests, they're super flaky right now.
		// See: https://github.com/coder/coder/issues/8954
		os.Exit(m.Run())
	}
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestRingBuffer_NewRingBuffer(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(100)
	// Test that we can write and read from the buffer
	written, evicted := rb.Write([]byte("test"))
	require.Equal(t, 4, written)
	require.Equal(t, 0, evicted)

	data, err := rb.ReadLast(4)
	require.NoError(t, err)
	require.Equal(t, []byte("test"), data)
}

func TestRingBuffer_WriteAndRead(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(10)

	// Write some data
	rb.Write([]byte("hello"))

	// Read last 4 bytes
	data, err := rb.ReadLast(4)
	require.NoError(t, err)
	require.Equal(t, "ello", string(data))

	// Write more data
	rb.Write([]byte("world"))

	// Read last 5 bytes
	data, err = rb.ReadLast(5)
	require.NoError(t, err)
	require.Equal(t, "world", string(data))

	// Read last 3 bytes
	data, err = rb.ReadLast(3)
	require.NoError(t, err)
	require.Equal(t, "rld", string(data))

	// Read more than available (should be 10 bytes total)
	_, err = rb.ReadLast(15)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requested 15 bytes but only")
}

func TestRingBuffer_OverflowEviction(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(5)

	// Fill buffer
	written, evicted := rb.Write([]byte("abcde"))
	require.Equal(t, 5, written)
	require.Equal(t, 0, evicted)

	// Overflow should evict oldest data
	written, evicted = rb.Write([]byte("fg"))
	require.Equal(t, 2, written)
	require.Equal(t, 2, evicted)

	// Should now contain "cdefg"
	data, err := rb.ReadLast(5)
	require.NoError(t, err)
	require.Equal(t, []byte("cdefg"), data)
}

func TestRingBuffer_LargeWrite(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(5)

	// Write data larger than capacity
	written, evicted := rb.Write([]byte("abcdefghij"))
	require.Equal(t, 5, written)
	require.Equal(t, 5, evicted)

	// Should contain last 5 bytes
	data, err := rb.ReadLast(5)
	require.NoError(t, err)
	require.Equal(t, []byte("fghij"), data)
}

func TestRingBuffer_WrapAround(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(5)

	// Fill buffer
	rb.Write([]byte("abcde"))

	// Write more to cause wrap-around
	rb.Write([]byte("fgh"))

	// Should contain "defgh"
	data, err := rb.ReadLast(5)
	require.NoError(t, err)
	require.Equal(t, []byte("defgh"), data)

	// Test reading last 3 bytes after wrap
	data, err = rb.ReadLast(3)
	require.NoError(t, err)
	require.Equal(t, []byte("fgh"), data)
}

func TestRingBuffer_ReadLastEdgeCases(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(3)

	// Write some data (5 bytes to a 3-byte buffer, so only last 3 bytes remain)
	rb.Write([]byte("hello"))

	// Test reading negative count
	data, err := rb.ReadLast(-1)
	require.NoError(t, err)
	require.Nil(t, data)

	// Test reading zero bytes
	data, err = rb.ReadLast(0)
	require.NoError(t, err)
	require.Nil(t, data)

	// Test reading more than available (buffer has 3 bytes, try to read 10)
	_, err = rb.ReadLast(10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requested 10 bytes but only 3 available")

	// Test reading exact amount available
	data, err = rb.ReadLast(3)
	require.NoError(t, err)
	require.Equal(t, []byte("llo"), data)
}

func TestRingBuffer_EmptyWrite(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(10)

	// Write empty data
	written, evicted := rb.Write([]byte{})
	require.Equal(t, 0, written)
	require.Equal(t, 0, evicted)

	// Buffer should still be empty
	_, err := rb.ReadLast(5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "buffer is empty")
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(1000)
	var wg sync.WaitGroup

	// Start multiple goroutines writing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("data-%d", id))
			for j := 0; j < 100; j++ {
				rb.Write(data)
			}
		}(i)
	}

	// Start multiple goroutines reading
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, err := rb.ReadLast(100)
				if err != nil {
					// Error is expected if buffer doesn't have enough data
					continue
				}
			}
		}()
	}

	wg.Wait()

	// Verify buffer is still in valid state
	data, err := rb.ReadLast(1000)
	require.NoError(t, err)
	require.NotNil(t, data)
}

func TestRingBuffer_MultipleWrites(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(10)

	// Write data in chunks
	rb.Write([]byte("ab"))
	rb.Write([]byte("cd"))
	rb.Write([]byte("ef"))

	data, err := rb.ReadLast(6)
	require.NoError(t, err)
	require.Equal(t, []byte("abcdef"), data)

	// Test partial reads
	data, err = rb.ReadLast(4)
	require.NoError(t, err)
	require.Equal(t, []byte("cdef"), data)

	data, err = rb.ReadLast(2)
	require.NoError(t, err)
	require.Equal(t, []byte("ef"), data)
}

func TestRingBuffer_EdgeCaseEviction(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(3)

	// Write data that will cause eviction
	written, evicted := rb.Write([]byte("abc"))
	require.Equal(t, 3, written)
	require.Equal(t, 0, evicted)

	// Write more to cause eviction
	written, evicted = rb.Write([]byte("d"))
	require.Equal(t, 1, written)
	require.Equal(t, 1, evicted)

	// Should now contain "bcd"
	data, err := rb.ReadLast(3)
	require.NoError(t, err)
	require.Equal(t, []byte("bcd"), data)
}

func TestRingBuffer_ComplexWrapAroundScenario(t *testing.T) {
	t.Parallel()

	rb := backedpipe.NewRingBufferWithCapacity(8)

	// Fill buffer
	rb.Write([]byte("12345678"))

	// Evict some and add more to create complex wrap scenario
	rb.Write([]byte("abcd"))
	data, err := rb.ReadLast(8)
	require.NoError(t, err)
	require.Equal(t, []byte("5678abcd"), data)

	// Add more
	rb.Write([]byte("xyz"))
	data, err = rb.ReadLast(8)
	require.NoError(t, err)
	require.Equal(t, []byte("8abcdxyz"), data)

	// Test reading various amounts from the end
	data, err = rb.ReadLast(7)
	require.NoError(t, err)
	require.Equal(t, []byte("abcdxyz"), data)

	data, err = rb.ReadLast(4)
	require.NoError(t, err)
	require.Equal(t, []byte("dxyz"), data)
}

// Benchmark tests for performance validation
func BenchmarkRingBuffer_Write(b *testing.B) {
	rb := backedpipe.NewRingBuffer()        // Use full 64MB for benchmarks
	data := bytes.Repeat([]byte("x"), 1024) // 1KB writes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(data)
	}
}

func BenchmarkRingBuffer_ReadLast(b *testing.B) {
	rb := backedpipe.NewRingBuffer() // Use full 64MB for benchmarks
	// Fill buffer with test data
	for i := 0; i < 64; i++ {
		rb.Write(bytes.Repeat([]byte("x"), 1024))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rb.ReadLast((i % 100) + 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRingBuffer_ConcurrentAccess(b *testing.B) {
	rb := backedpipe.NewRingBuffer() // Use full 64MB for benchmarks
	data := bytes.Repeat([]byte("x"), 100)

	// Pre-fill buffer with enough data
	for i := 0; i < 100; i++ {
		rb.Write(data)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rb.Write(data)
			_, err := rb.ReadLast(100) // Read only what we know is available
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
