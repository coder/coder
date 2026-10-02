package extract_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/extract"
)

// fieldSink captures the "note" field of each logged entry.
type fieldSink struct {
	mu    sync.Mutex
	notes []string
}

func (s *fieldSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range e.Fields {
		if f.Name == "note" {
			note, _ := f.Value.(string)
			s.notes = append(s.notes, note)
		}
	}
}

func (*fieldSink) Sync() {}

// TestParseNotesBounded checks that a note naming a traffic-controlled
// value, such as a huge SSE event name, is cut before it is logged.
func TestParseNotesBounded(t *testing.T) {
	t.Parallel()

	sink := &fieldSink{}
	notes := extract.NewParseNotes(slog.Make(sink))
	notes.Addf(t.Context(), "skipped %q event", strings.Repeat("E", 1<<20))

	require.Len(t, sink.notes, 1)
	require.Less(t, len(sink.notes[0]), 512)
	require.True(t, strings.HasPrefix(sink.notes[0], `skipped "EEE`))
}
