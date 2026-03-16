package testutil

import (
	"context"
	"sync"
	"testing"

	"cdr.dev/slog/v3"
)

// FakeSink is a thread-safe slog.Sink that captures log entries so
// tests can assert on what was logged. It requires a testing.TB,
// which also prevents accidental use outside of tests.
type FakeSink struct {
	t       testing.TB
	mu      sync.Mutex
	entries []slog.SinkEntry
	notify  chan<- slog.SinkEntry
}

// NewFakeSink returns a FakeSink ready for use.
func NewFakeSink(t testing.TB) *FakeSink {
	return &FakeSink{t: t}
}

// SetNotifyChannel configures a send-only channel that receives a
// copy of every entry as it is logged. This is useful for tests
// that need to block until a specific log line arrives instead of
// polling. If the channel's buffer is full, the test is failed
// rather than silently dropping the entry.
func (s *FakeSink) SetNotifyChannel(ch chan<- slog.SinkEntry) *FakeSink {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notify = ch
	return s
}

// LogEntry implements slog.Sink. It appends the entry to the
// internal slice and, if a notify channel is set, sends a copy.
func (s *FakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	if s.notify != nil {
		select {
		case s.notify <- e:
		default:
			// Errorf is goroutine-safe unlike Fatalf.
			s.t.Errorf("FakeSink: notify channel is full, "+
				"could not deliver log entry: %s", e.Message)
		}
	}
}

// Sync implements slog.Sink.
func (*FakeSink) Sync() {}

// Entries returns a copy of the captured entries. If filters are
// provided, only entries matching ALL filters are returned. This
// lets callers compose simple predicates instead of needing
// dedicated methods for each field.
func (s *FakeSink) Entries(filters ...func(slog.SinkEntry) bool) []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []slog.SinkEntry
	for _, e := range s.entries {
		if matchAll(e, filters) {
			result = append(result, e)
		}
	}
	return result
}

// Logger returns a slog.Logger backed by this sink at the given
// level. If no level is provided it defaults to LevelDebug, which
// captures everything.
func (s *FakeSink) Logger(level ...slog.Level) slog.Logger {
	l := slog.LevelDebug
	if len(level) > 0 {
		l = level[0]
	}
	return slog.Make(s).Leveled(l)
}

func matchAll(e slog.SinkEntry, filters []func(slog.SinkEntry) bool) bool {
	for _, f := range filters {
		if f == nil {
			continue
		}
		if !f(e) {
			return false
		}
	}
	return true
}
