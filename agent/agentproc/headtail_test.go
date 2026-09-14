package agentproc_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentproc"
)

func TestHeadTailBuffer_EmptyBuffer(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()
	out, info := buf.Output()
	require.Empty(t, out)
	require.Nil(t, info)
	require.Equal(t, 0, buf.Len())
	require.Equal(t, 0, buf.TotalWritten())
	require.Empty(t, buf.Bytes())
}

func TestHeadTailBuffer_SmallOutput(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()
	data := "hello world\n"
	n, err := buf.Write([]byte(data))
	require.NoError(t, err)
	require.Equal(t, len(data), n)

	out, info := buf.Output()
	require.Equal(t, data, out)
	require.Nil(t, info, "small output should not be truncated")
	require.Equal(t, len(data), buf.Len())
	require.Equal(t, len(data), buf.TotalWritten())
}

func TestHeadTailBuffer_ExactlyHeadSize(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()

	// Build data that is exactly MaxHeadBytes using short
	// lines so that line truncation does not apply.
	line := strings.Repeat("x", 79) + "\n" // 80 bytes per line
	count := agentproc.MaxHeadBytes / len(line)
	pad := agentproc.MaxHeadBytes - (count * len(line))
	data := strings.Repeat(line, count) + strings.Repeat("y", pad)
	require.Equal(t, agentproc.MaxHeadBytes, len(data),
		"test data must be exactly MaxHeadBytes")

	n, err := buf.Write([]byte(data))
	require.NoError(t, err)
	require.Equal(t, agentproc.MaxHeadBytes, n)

	out, info := buf.Output()
	require.Equal(t, data, out)
	require.Nil(t, info, "output fitting in head should not be truncated")
	require.Equal(t, agentproc.MaxHeadBytes, buf.Len())
}

func TestHeadTailBuffer_HeadPlusTailNoOmission(t *testing.T) {
	t.Parallel()

	// Use a small buffer so we can test the boundary where
	// head fills and tail starts but nothing is omitted.
	// With maxHead=10, maxTail=10, writing exactly 20 bytes
	// means head gets 10, tail gets 10, omitted = 0.
	buf := agentproc.NewHeadTailBufferSized(10, 10)

	data := "0123456789abcdefghij" // 20 bytes
	n, err := buf.Write([]byte(data))
	require.NoError(t, err)
	require.Equal(t, 20, n)

	out, info := buf.Output()
	require.NotNil(t, info)
	require.Equal(t, 0, info.OmittedBytes)
	require.Equal(t, "head_tail", info.Strategy)
	// The output should contain both head and tail.
	require.Contains(t, out, "0123456789")
	require.Contains(t, out, "abcdefghij")
}

func TestHeadTailBuffer_LargeOutputTruncation(t *testing.T) {
	t.Parallel()

	// Use small head/tail so truncation is easy to verify.
	buf := agentproc.NewHeadTailBufferSized(10, 10)

	// Write 100 bytes: head=10, tail=10, omitted=80.
	data := strings.Repeat("A", 50) + strings.Repeat("Z", 50)
	n, err := buf.Write([]byte(data))
	require.NoError(t, err)
	require.Equal(t, 100, n)

	out, info := buf.Output()
	require.NotNil(t, info)
	require.Equal(t, 100, info.OriginalBytes)
	require.Equal(t, 80, info.OmittedBytes)
	require.Equal(t, "head_tail", info.Strategy)

	// Head should be first 10 bytes (all A's).
	require.True(t, strings.HasPrefix(out, "AAAAAAAAAA"))
	// Tail should be last 10 bytes (all Z's).
	require.True(t, strings.HasSuffix(out, "ZZZZZZZZZZ"))
	// Omission marker should be present.
	require.Contains(t, out, "... [omitted 80 bytes] ...")

	require.Equal(t, 20, buf.Len())
	require.Equal(t, 100, buf.TotalWritten())
}

func TestHeadTailBuffer_MultiMBStaysBounded(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()

	// Write 5MB of data in chunks.
	chunk := []byte(strings.Repeat("x", 4096) + "\n")
	totalWritten := 0
	for totalWritten < 5*1024*1024 {
		n, err := buf.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
		totalWritten += n
	}

	// Memory should be bounded to head+tail.
	require.LessOrEqual(t, buf.Len(),
		agentproc.MaxHeadBytes+agentproc.MaxTailBytes)
	require.Equal(t, totalWritten, buf.TotalWritten())

	out, info := buf.Output()
	require.NotNil(t, info)
	require.Equal(t, totalWritten, info.OriginalBytes)
	require.Greater(t, info.OmittedBytes, 0)
	require.NotEmpty(t, out)
}

func TestHeadTailBuffer_LongLineTruncation(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()

	// Write a line longer than MaxLineLength.
	longLine := strings.Repeat("m", agentproc.MaxLineLength+500)
	_, err := buf.Write([]byte(longLine + "\n"))
	require.NoError(t, err)

	out, _ := buf.Output()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 1)
	require.LessOrEqual(t, len(lines[0]), agentproc.MaxLineLength)
	require.True(t, strings.HasSuffix(lines[0], "... [truncated]"))
}

func TestHeadTailBuffer_LongLineInTail(t *testing.T) {
	t.Parallel()

	// Use small buffers so we can force data into the tail.
	buf := agentproc.NewHeadTailBufferSized(20, 5000)

	// Fill head with short data.
	_, err := buf.Write([]byte("head data goes here\n"))
	require.NoError(t, err)

	// Now write a very long line into the tail.
	longLine := strings.Repeat("T", agentproc.MaxLineLength+100)
	_, err = buf.Write([]byte(longLine + "\n"))
	require.NoError(t, err)

	out, info := buf.Output()
	require.NotNil(t, info)
	// The long line in the tail should be truncated.
	require.Contains(t, out, "... [truncated]")
}

func TestHeadTailBuffer_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()

	const goroutines = 10
	const writes = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func() {
			defer wg.Done()
			line := fmt.Sprintf("goroutine-%d: data\n", g)
			for range writes {
				_, err := buf.Write([]byte(line))
				assert.NoError(t, err)
			}
		}()
	}

	wg.Wait()

	// Verify totals are consistent.
	require.Greater(t, buf.TotalWritten(), 0)
	require.Greater(t, buf.Len(), 0)

	out, _ := buf.Output()
	require.NotEmpty(t, out)
}

func TestHeadTailBuffer_TruncationInfoFields(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBufferSized(10, 10)

	// Write enough to cause omission.
	data := strings.Repeat("D", 50)
	_, err := buf.Write([]byte(data))
	require.NoError(t, err)

	_, info := buf.Output()
	require.NotNil(t, info)
	require.Equal(t, 50, info.OriginalBytes)
	require.Equal(t, 30, info.OmittedBytes)
	require.Equal(t, "head_tail", info.Strategy)
	// RetainedBytes is the length of the formatted output
	// string including the omission marker.
	require.Greater(t, info.RetainedBytes, 0)
}

func TestHeadTailBuffer_MultipleSmallWrites(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()

	// Write one byte at a time.
	expected := "hello world"
	for i := range len(expected) {
		n, err := buf.Write([]byte{expected[i]})
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}

	out, info := buf.Output()
	require.Equal(t, expected, out)
	require.Nil(t, info)
}

func TestHeadTailBuffer_WriteEmptySlice(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()
	n, err := buf.Write([]byte{})
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, 0, buf.TotalWritten())
}

func TestHeadTailBuffer_Reset(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()
	_, err := buf.Write([]byte("some data"))
	require.NoError(t, err)
	require.Greater(t, buf.Len(), 0)

	buf.Reset()

	require.Equal(t, 0, buf.Len())
	require.Equal(t, 0, buf.TotalWritten())
	out, info := buf.Output()
	require.Empty(t, out)
	require.Nil(t, info)
}

func TestHeadTailBuffer_BytesReturnsCopy(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()
	_, err := buf.Write([]byte("original"))
	require.NoError(t, err)

	b := buf.Bytes()
	require.Equal(t, []byte("original"), b)

	// Mutating the returned slice should not affect the
	// buffer.
	b[0] = 'X'
	require.Equal(t, []byte("original"), buf.Bytes())
}

func TestHeadTailBuffer_RingBufferWraparound(t *testing.T) {
	t.Parallel()

	// Use a tail of 10 bytes and write enough to wrap
	// around multiple times.
	buf := agentproc.NewHeadTailBufferSized(5, 10)

	// Fill head (5 bytes).
	_, err := buf.Write([]byte("HEADD"))
	require.NoError(t, err)

	// Write 25 bytes into tail, wrapping 2.5 times.
	_, err = buf.Write([]byte("0123456789"))
	require.NoError(t, err)
	_, err = buf.Write([]byte("abcdefghij"))
	require.NoError(t, err)
	_, err = buf.Write([]byte("ABCDE"))
	require.NoError(t, err)

	out, info := buf.Output()
	require.NotNil(t, info)
	// Tail should contain the last 10 bytes: "fghijABCDE".
	require.True(t, strings.HasSuffix(out, "fghijABCDE"),
		"expected tail to be last 10 bytes, got: %q", out)
}

func TestHeadTailBuffer_MultipleLinesTruncated(t *testing.T) {
	t.Parallel()

	buf := agentproc.NewHeadTailBuffer()

	short := "short line\n"
	long := strings.Repeat("L", agentproc.MaxLineLength+100) + "\n"
	_, err := buf.Write([]byte(short + long + short))
	require.NoError(t, err)

	out, _ := buf.Output()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 3)
	require.Equal(t, "short line", lines[0])
	require.True(t, strings.HasSuffix(lines[1], "... [truncated]"))
	require.Equal(t, "short line", lines[2])
}
