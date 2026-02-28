package agentproc

import (
	"fmt"
	"strings"
	"sync"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// MaxHeadBytes is the number of bytes retained from the
	// beginning of the output for LLM consumption.
	MaxHeadBytes = 16 << 10 // 16KB

	// MaxTailBytes is the number of bytes retained from the
	// end of the output for LLM consumption.
	MaxTailBytes = 16 << 10 // 16KB

	// MaxLineLength is the maximum length of a single line
	// before it is truncated. This prevents minified files
	// or other long single-line output from consuming the
	// entire buffer.
	MaxLineLength = 2048

	// lineTruncationSuffix is appended to lines that exceed
	// MaxLineLength.
	lineTruncationSuffix = " ... [truncated]"
)

// HeadTailBuffer is a thread-safe buffer that captures process
// output and provides head+tail truncation for LLM consumption.
// It implements io.Writer so it can be used directly as
// cmd.Stdout or cmd.Stderr.
//
// The buffer stores up to MaxHeadBytes from the beginning of
// the output and up to MaxTailBytes from the end in a ring
// buffer, keeping total memory usage bounded regardless of
// how much output is written.
type HeadTailBuffer struct {
	mu         sync.Mutex
	head       []byte
	tail       []byte
	tailPos    int
	tailFull   bool
	headFull   bool
	totalBytes int
	maxHead    int
	maxTail    int
}

// NewHeadTailBuffer creates a new HeadTailBuffer with the
// default head and tail sizes.
func NewHeadTailBuffer() *HeadTailBuffer {
	return &HeadTailBuffer{
		maxHead: MaxHeadBytes,
		maxTail: MaxTailBytes,
	}
}

// NewHeadTailBufferSized creates a HeadTailBuffer with custom
// head and tail sizes. This is useful for testing truncation
// logic with smaller buffers.
func NewHeadTailBufferSized(maxHead, maxTail int) *HeadTailBuffer {
	return &HeadTailBuffer{
		maxHead: maxHead,
		maxTail: maxTail,
	}
}

// Write implements io.Writer. It is safe for concurrent use.
// All bytes are accepted; the return value always equals
// len(p) with a nil error.
func (b *HeadTailBuffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(p)
	b.totalBytes += n

	// Fill head buffer if it is not yet full.
	if !b.headFull {
		remaining := b.maxHead - len(b.head)
		if remaining > 0 {
			take := remaining
			if take > len(p) {
				take = len(p)
			}
			b.head = append(b.head, p[:take]...)
			p = p[take:]
			if len(b.head) >= b.maxHead {
				b.headFull = true
			}
		}
		if len(p) == 0 {
			return n, nil
		}
	}

	// Write remaining bytes into the tail ring buffer.
	b.writeTail(p)
	return n, nil
}

// writeTail appends data to the tail ring buffer. The caller
// must hold b.mu.
func (b *HeadTailBuffer) writeTail(p []byte) {
	if b.maxTail <= 0 {
		return
	}

	// Lazily allocate the tail buffer on first use.
	if b.tail == nil {
		b.tail = make([]byte, b.maxTail)
	}

	for len(p) > 0 {
		// Write as many bytes as fit starting at tailPos.
		space := b.maxTail - b.tailPos
		take := space
		if take > len(p) {
			take = len(p)
		}
		copy(b.tail[b.tailPos:b.tailPos+take], p[:take])
		p = p[take:]
		b.tailPos += take
		if b.tailPos >= b.maxTail {
			b.tailPos = 0
			b.tailFull = true
		}
	}
}

// tailBytes returns the current tail contents in order. The
// caller must hold b.mu.
func (b *HeadTailBuffer) tailBytes() []byte {
	if b.tail == nil {
		return nil
	}
	if !b.tailFull {
		// Haven't wrapped yet; data is [0, tailPos).
		return b.tail[:b.tailPos]
	}
	// Wrapped: data is [tailPos, maxTail) + [0, tailPos).
	out := make([]byte, b.maxTail)
	n := copy(out, b.tail[b.tailPos:])
	copy(out[n:], b.tail[:b.tailPos])
	return out
}

// Bytes returns a copy of the raw buffer contents. If no
// truncation has occurred the full output is returned;
// otherwise the head and tail portions are concatenated.
func (b *HeadTailBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	tail := b.tailBytes()
	if len(tail) == 0 {
		out := make([]byte, len(b.head))
		copy(out, b.head)
		return out
	}
	out := make([]byte, len(b.head)+len(tail))
	copy(out, b.head)
	copy(out[len(b.head):], tail)
	return out
}

// Len returns the number of bytes currently stored in the
// buffer.
func (b *HeadTailBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	tailLen := 0
	if b.tailFull {
		tailLen = b.maxTail
	} else if b.tail != nil {
		tailLen = b.tailPos
	}
	return len(b.head) + tailLen
}

// TotalWritten returns the total number of bytes written to
// the buffer, which may exceed the stored capacity.
func (b *HeadTailBuffer) TotalWritten() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.totalBytes
}

// Output returns the truncated output suitable for LLM
// consumption, along with truncation metadata. If the total
// output fits within the head buffer alone, the full output is
// returned with nil truncation info. Otherwise the head and
// tail are joined with an omission marker and long lines are
// truncated.
func (b *HeadTailBuffer) Output() (string, *workspacesdk.ProcessTruncation) {
	b.mu.Lock()
	head := make([]byte, len(b.head))
	copy(head, b.head)
	tail := b.tailBytes()
	total := b.totalBytes
	headFull := b.headFull
	b.mu.Unlock()

	storedLen := len(head) + len(tail)

	// If everything fits, no head/tail split is needed.
	if !headFull || len(tail) == 0 {
		out := truncateLines(string(head))
		if total == 0 {
			return "", nil
		}
		return out, nil
	}

	// We have both head and tail data, meaning the total
	// output exceeded the head capacity. Build the
	// combined output with an omission marker.
	omitted := total - storedLen
	headStr := truncateLines(string(head))
	tailStr := truncateLines(string(tail))

	var sb strings.Builder
	_, _ = sb.WriteString(headStr)
	if omitted > 0 {
		_, _ = sb.WriteString(fmt.Sprintf(
			"\n\n... [omitted %d bytes] ...\n\n",
			omitted,
		))
	} else {
		// Head and tail are contiguous but were stored
		// separately because the head filled up.
		_, _ = sb.WriteString("\n")
	}
	_, _ = sb.WriteString(tailStr)
	result := sb.String()

	return result, &workspacesdk.ProcessTruncation{
		OriginalBytes: total,
		RetainedBytes: len(result),
		OmittedBytes:  omitted,
		Strategy:      "head_tail",
	}
}

// truncateLines scans the input line by line and truncates
// any line longer than MaxLineLength.
func truncateLines(s string) string {
	if len(s) <= MaxLineLength {
		// Fast path: if the entire string is shorter than
		// the max line length, no line can exceed it.
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for len(s) > 0 {
		idx := strings.IndexByte(s, '\n')
		var line string
		if idx == -1 {
			line = s
			s = ""
		} else {
			line = s[:idx]
			s = s[idx+1:]
		}

		if len(line) > MaxLineLength {
			// Truncate preserving the suffix length so the
			// total does not exceed a reasonable size.
			cut := MaxLineLength - len(lineTruncationSuffix)
			if cut < 0 {
				cut = 0
			}
			_, _ = b.WriteString(line[:cut])
			_, _ = b.WriteString(lineTruncationSuffix)
		} else {
			_, _ = b.WriteString(line)
		}

		// Re-add the newline unless this was the final
		// segment without a trailing newline.
		if idx != -1 {
			_ = b.WriteByte('\n')
		}
	}

	return b.String()
}

// Reset clears the buffer, discarding all data.
func (b *HeadTailBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = nil
	b.tail = nil
	b.tailPos = 0
	b.tailFull = false
	b.headFull = false
	b.totalBytes = 0
}
