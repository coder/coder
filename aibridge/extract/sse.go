package extract

import (
	"bytes"
)

// SSEStream feeds a Server-Sent Events byte stream to a ResponseExtraction,
// one OnEvent call per dispatched event. Bytes may arrive in chunks of any
// size, split anywhere. Field parsing follows the aibridge SSEParser: lines
// end in LF or CRLF, lines starting with ':' are comments, one leading space
// is stripped from values, and multiple data lines join with LF.
//
// It is an io.Writer so it can sit behind an io.TeeReader. Write never fails
// and never blocks beyond the synchronous OnEvent calls. Events larger than
// MaxEventBytes are skipped with a note. An SSEStream is not safe for
// concurrent use.
type SSEStream struct {
	ext ResponseExtraction

	// line holds a partial line carried across writes.
	line      []byte
	eventType string
	data      []byte
	hasData   bool
	// skipping is set when the current event exceeded MaxEventBytes; lines
	// are discarded until the event ends.
	skipping bool
	notes    ParseNotes
}

// NewSSEStream returns an SSEStream that feeds ext.
func NewSSEStream(ext ResponseExtraction) *SSEStream {
	if ext == nil {
		panic("extract: NewSSEStream requires a ResponseExtraction")
	}
	return &SSEStream{ext: ext}
}

// Write consumes stream bytes. It always returns len(p), nil.
func (s *SSEStream) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			s.appendPartial(p)
			break
		}
		s.appendPartial(p[:i])
		p = p[i+1:]
		line := bytes.TrimSuffix(s.line, []byte{'\r'})
		s.processLine(line)
		s.line = s.line[:0]
	}
	return n, nil
}

// Result returns the extraction's facts plus the stream's own parse notes.
// A trailing event without its terminating blank line is not dispatched,
// as the SSE specification requires, and is reported as a note.
func (s *SSEStream) Result() ResponseFacts {
	facts := s.ext.Result()
	notes := s.notes
	if s.skipping || s.hasData || len(s.line) > 0 {
		notes.Addf("stream ended mid-event: partial %q event discarded", s.eventType)
	}
	facts.ParseNotes = append(facts.ParseNotes, notes.List()...)
	return facts
}

func (s *SSEStream) appendPartial(p []byte) {
	if !s.skipping && len(s.line)+len(s.data)+len(p) > MaxEventBytes {
		s.startSkipping()
	}
	if s.skipping {
		// Keep at most two bytes of a discarded line: enough to tell a
		// blank line ("" or "\r") that ends the event from any other line.
		if keep := 2 - len(s.line); keep > 0 {
			s.line = append(s.line, p[:min(keep, len(p))]...)
		}
		return
	}
	s.line = append(s.line, p...)
}

func (s *SSEStream) startSkipping() {
	s.notes.Addf("skipped %q event: exceeds %d bytes", s.eventType, MaxEventBytes)
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
