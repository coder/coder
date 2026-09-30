package extract

import (
	"context"
	"fmt"

	"cdr.dev/slog/v3"
)

// maxParseNotes bounds the notes logged per response so a stream of
// malformed events cannot flood the log.
const maxParseNotes = 16

// ParseNotes logs parse notes: input an extractor skipped because it could
// not be parsed. Notes never indicate a traffic error and must not quote
// traffic content. After maxParseNotes notes, further notes are dropped.
type ParseNotes struct {
	logger slog.Logger
	count  int
}

// NewParseNotes returns ParseNotes that log to logger.
func NewParseNotes(logger slog.Logger) ParseNotes {
	return ParseNotes{logger: logger}
}

// Addf logs a note.
func (n *ParseNotes) Addf(ctx context.Context, format string, args ...any) {
	n.count++
	switch {
	case n.count <= maxParseNotes:
		n.logger.Warn(ctx, "extractor parse note", slog.F("note", fmt.Sprintf(format, args...)))
	case n.count == maxParseNotes+1:
		n.logger.Warn(ctx, "extractor parse notes limit reached, dropping further notes")
	}
}
