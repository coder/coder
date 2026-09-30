package extract

import (
	"bytes"
	"context"

	"cdr.dev/slog/v3"
)

// SSEStream feeds a Server-Sent Events byte stream to a ResponseExtraction,
// one OnEvent call per dispatched event. Bytes may arrive in chunks of any
// size, split anywhere. Field parsing follows the SSE specification: lines
// end in CR, LF, or CRLF, lines starting with ':' are comments, one leading
// space is stripped from values, and multiple data lines join with LF.
//
// It is an io.WriteCloser so it can sit behind an io.TeeReader; call Close
// when the stream ends. Write never fails and never blocks beyond the
// synchronous OnEvent calls. Events larger than MaxEventBytes are skipped
// with a parse note. An SSEStream is not safe for concurrent use: write the
// stream bytes sequentially, in order.
//
// It does not reuse aibridge.SSEParser: that parser lives in the root
// aibridge package (importing it here would create a cycle for any
// interceptor that later adopts extractors), reads the whole stream before
// returning, buffers every event in memory, and fails on lines longer than
// bufio.Scanner's 64 KiB default. A pass-through proxy needs incremental
// parsing with a per-event bound instead.
type SSEStream struct {
	// ctx scopes the notes logged from Write, which has no context.
	ctx context.Context
	ext ResponseExtraction

	// line holds a partial line carried across writes.
	line []byte
	// afterCR is set when the last line ended in CR, so an LF that starts
	// the next write completes that CRLF instead of ending a blank line.
	afterCR   bool
	eventType string
	data      []byte
	hasData   bool
	// skipping is set when the current event exceeded MaxEventBytes; lines
	// are discarded until the event ends.
	skipping bool
	notes    ParseNotes
}

// NewSSEStream returns an SSEStream that feeds ext and logs parse notes to
// logger. ctx scopes the logging and is usually the request context.
func NewSSEStream(ctx context.Context, logger slog.Logger, ext ResponseExtraction) *SSEStream {
	if ext == nil {
		panic("extract: NewSSEStream requires a ResponseExtraction")
	}
	return &SSEStream{ctx: ctx, ext: ext, notes: NewParseNotes(logger)}
}

// Write consumes stream bytes. It always returns len(p), nil.
func (s *SSEStream) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if s.afterCR {
			s.afterCR = false
			if p[0] == '\n' {
				p = p[1:]
				continue
			}
		}
		i := bytes.IndexAny(p, "\r\n")
		if i < 0 {
			s.appendPartial(p)
			break
		}
		s.appendPartial(p[:i])
		s.afterCR = p[i] == '\r'
		p = p[i+1:]
		s.processLine(s.line)
		s.line = s.line[:0]
	}
	return n, nil
}

// Close ends the stream. A trailing event without its terminating blank
// line is not dispatched, as the SSE specification requires, and is logged
// as a parse note. It always returns nil.
func (s *SSEStream) Close() error {
	if s.skipping || s.hasData || len(s.line) > 0 {
		s.notes.Addf(s.ctx, "stream ended mid-event: partial %q event discarded", s.eventType)
	}
	s.line, s.data = nil, nil
	s.eventType, s.hasData, s.skipping, s.afterCR = "", false, false, false
	return nil
}

func (s *SSEStream) appendPartial(p []byte) {
	// The event name counts toward the bound: it is dispatched with the
	// event.
	if !s.skipping && len(s.eventType)+len(s.line)+len(s.data)+len(p) > MaxEventBytes {
		s.startSkipping()
	}
	if s.skipping {
		// Keep at most one byte of a discarded line: enough to tell the
		// blank line that ends the event from any other line.
		if keep := 1 - len(s.line); keep > 0 {
			s.line = append(s.line, p[:min(keep, len(p))]...)
		}
		return
	}
	s.line = append(s.line, p...)
}

func (s *SSEStream) startSkipping() {
	s.notes.Addf(s.ctx, "skipped %q event: exceeds %d bytes", s.eventType, MaxEventBytes)
	nonBlank := len(s.line) > 0
	s.skipping = true
	s.line = s.line[:0]
	if nonBlank {
		s.line = append(s.line, '.')
	}
	s.data = s.data[:0]
	s.hasData = false
}

func (s *SSEStream) processLine(line []byte) {
	if len(line) == 0 {
		if s.hasData && !s.skipping {
			s.ext.OnEvent(s.eventType, s.data)
		}
		s.eventType = ""
		s.data = s.data[:0]
		s.hasData = false
		s.skipping = false
		return
	}
	if s.skipping || line[0] == ':' {
		return
	}

	field, value, _ := bytes.Cut(line, []byte{':'})
	value = bytes.TrimPrefix(value, []byte{' '})
	switch string(field) {
	case "event":
		s.eventType = string(value)
	case "data":
		if s.hasData {
			s.data = append(s.data, '\n')
		}
		s.data = append(s.data, value...)
		s.hasData = true
	}
}
