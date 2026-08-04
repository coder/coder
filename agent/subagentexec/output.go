package subagentexec

import (
	"bytes"
	"strings"
	"sync"
)

const (
	// maxLogLineBytes bounds one emitted record. A driver that writes a huge
	// unterminated blob is emitted in bounded chunks instead of buffered
	// without limit.
	maxLogLineBytes = 4096
	// maxPendingBytes is the hard bound on buffered output. It only matters
	// when the secret is long enough that no safe cut point exists inside a
	// full buffer, which cannot happen with an agent token.
	maxPendingBytes = 2 * maxLogLineBytes
	// redactedPlaceholder replaces any occurrence of the child's token in
	// driver output.
	redactedPlaceholder = "[redacted]"
)

// redactingWriter turns driver output into records, removes the exact child
// token from them, and bounds how much it emits.
//
// It is the launcher's only path from driver output to the parent agent's
// log for now. A driver that prints the contents of its token file, or
// echoes a command line containing it, still cannot leak the token through
// this writer, including when the token straddles a write boundary or the
// size bound at which an unterminated line is flushed. Redaction is
// line-scoped: a secret containing a newline is out of scope, because an
// agent token never does.
type redactingWriter struct {
	mu sync.Mutex
	// buf holds the output that has not been emitted yet: the current
	// partial line, minus whatever a size-bounded flush already emitted.
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

	rest := p
	for len(rest) > 0 {
		segment := rest
		newline := false
		if idx := bytes.IndexByte(rest, '\n'); idx >= 0 {
			segment, rest, newline = rest[:idx], rest[idx+1:], true
		} else {
			rest = nil
		}
		for len(segment) > 0 {
			room := maxPendingBytes - w.buf.Len()
			if w.buf.Len() < maxLogLineBytes {
				room = maxLogLineBytes - w.buf.Len()
			}
			if room <= 0 {
				// A buffer at the hard bound always makes progress.
				w.flushChunkLocked()
				continue
			}
			if room > len(segment) {
				room = len(segment)
			}
			w.buf.Write(segment[:room])
			segment = segment[room:]
			if w.buf.Len() >= maxLogLineBytes {
				w.flushChunkLocked()
			}
		}
		if newline {
			w.flushLineLocked()
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
	w.flushLineLocked()
	w.closed = true
}

// flushLineLocked emits the buffer as a complete record. The line is
// complete, so every occurrence of the secret in it is a complete match and
// nothing has to be retained.
func (w *redactingWriter) flushLineLocked() {
	if w.buf.Len() == 0 {
		return
	}
	line := strings.TrimSuffix(w.redact(w.buf.String()), "\r")
	w.buf.Reset()
	w.emitLocked(line)
}

// flushChunkLocked emits as much of the buffered partial line as it can
// without publishing anything that could still turn out to be the start of
// the secret: complete matches are redacted, and the trailing bytes that a
// later write could still complete into a match are retained for the next
// flush. Without that retention a flush at the size bound would split the
// token across two records, leaking both halves.
//
// It returns whether it emitted a record.
func (w *redactingWriter) flushChunkLocked() bool {
	if w.buf.Len() == 0 {
		return false
	}
	pending := w.redact(w.buf.String())
	cut := safeCutPoint(pending, w.secret, len(pending)-w.retainBytes())
	if cut <= 0 {
		w.buf.Reset()
		w.buf.WriteString(pending)
		if w.buf.Len() < maxPendingBytes {
			return false
		}
		// The entire buffer could still become the secret, which needs a
		// secret longer than the size bound. It is replaced rather than
		// emitted in fragments or buffered without limit.
		w.buf.Reset()
		w.emitLocked(redactedPlaceholder)
		return true
	}
	w.buf.Reset()
	w.buf.WriteString(pending[cut:])
	w.emitLocked(pending[:cut])
	return true
}

func (w *redactingWriter) emitLocked(line string) {
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

func (w *redactingWriter) redact(s string) string {
	if w.secret == "" {
		return s
	}
	return strings.ReplaceAll(s, w.secret, redactedPlaceholder)
}

// retainBytes is how many trailing bytes a size-bounded flush keeps. Any
// occurrence of the secret that spans the flush needs at least one byte
// beyond the retained tail, so retaining len(secret)-1 bytes is enough to
// match it on a later flush.
func (w *redactingWriter) retainBytes() int {
	if w.secret == "" {
		return 0
	}
	keep := len(w.secret) - 1
	if keep > maxLogLineBytes {
		keep = maxLogLineBytes
	}
	return keep
}

// safeCutPoint shortens cut until s[:cut] does not end in a possible start
// of secret, so an emitted record never carries a reconstructable prefix of
// it. A cut of zero means nothing can be emitted yet.
func safeCutPoint(s, secret string, cut int) int {
	if cut > len(s) {
		cut = len(s)
	}
	if secret == "" {
		return cut
	}
	for cut > 0 {
		overlap := prefixSuffixOverlap(s[:cut], secret)
		if overlap == 0 {
			return cut
		}
		cut -= overlap
	}
	return 0
}

// prefixSuffixOverlap returns the length of the longest nonempty proper
// prefix of secret that is a suffix of s.
func prefixSuffixOverlap(s, secret string) int {
	longest := len(secret) - 1
	if longest > len(s) {
		longest = len(s)
	}
	for k := longest; k > 0; k-- {
		if strings.HasSuffix(s, secret[:k]) {
			return k
		}
	}
	return 0
}
