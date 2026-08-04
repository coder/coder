package subagentexec

import (
	"bytes"
	"strings"
	"sync"
)

const (
	// maxLogLineBytes bounds one emitted line. A driver that writes a huge
	// unterminated blob is truncated instead of buffered without limit.
	maxLogLineBytes = 4096
	// redactedPlaceholder replaces any occurrence of the child's token in
	// driver output.
	redactedPlaceholder = "[redacted]"
)

// redactingWriter turns driver output into whole lines, removes the exact
// child token from each one, and bounds how much it emits.
//
// It is the launcher's only path from driver output to the parent agent's
// log for now. A driver that prints the contents of its token file, or
// echoes a command line containing it, still cannot leak the token through
// this writer.
type redactingWriter struct {
	mu sync.Mutex
	// buf holds the current partial line.
	buf bytes.Buffer
	// secret is the exact byte sequence to remove. Empty disables
	// redaction, which only happens when there is no token to protect.
	secret string
	emit   func(line string)

	lines      int
	maxLines   int
	suppressed bool
	closed     bool
}

func newRedactingWriter(secret string, maxLines int, emit func(line string)) *redactingWriter {
	if maxLines <= 0 {
		maxLines = maxDriverOutputLines
	}
	return &redactingWriter{secret: secret, maxLines: maxLines, emit: emit}
}

func (w *redactingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		// Output that arrives after the run was reclaimed is dropped
		// rather than logged against a finished execution.
		return len(p), nil
	}

	for _, b := range p {
		if b == '\n' {
			w.flushLocked()
			continue
		}
		w.buf.WriteByte(b)
		if w.buf.Len() >= maxLogLineBytes {
			w.flushLocked()
		}
	}
	return len(p), nil
}

// Close emits whatever partial line is buffered and stops accepting
// output. It is safe to call more than once.
func (w *redactingWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.flushLocked()
	w.closed = true
}

func (w *redactingWriter) flushLocked() {
	if w.buf.Len() == 0 {
		return
	}
	line := strings.TrimSuffix(w.buf.String(), "\r")
	w.buf.Reset()

	if w.secret != "" {
		line = strings.ReplaceAll(line, w.secret, redactedPlaceholder)
	}

	if w.lines >= w.maxLines {
		if !w.suppressed {
			w.suppressed = true
			w.emit("subagent execution driver output truncated: line limit reached")
		}
		return
	}
	w.lines++
	w.emit(line)
}
