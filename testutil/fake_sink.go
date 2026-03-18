package testutil

import (
	"context"
	"sync"
	"testing"

	"cdr.dev/slog/v3"
)

// FakeSink is a thread-safe slog.Sink that captures log entries so
// tests can assert on what was logged. It requires a testing.TB
// as it is only meant for use in tests.
type FakeSink struct {
	mu      sync.RWMutex
	entries []slog.SinkEntry
}

// NewFakeSink returns a FakeSink ready for use.
func NewFakeSink(_ testing.TB) *FakeSink {
	return &FakeSink{}
}

// LogEntry implements slog.Sink. It appends the entry to the
// internal slice.
func (s *FakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

// Sync implements slog.Sink.
func (*FakeSink) Sync() {}

// Entries returns a copy of the captured entries. If filters are
// provided, only entries matching ALL filters are returned. This
// lets callers compose simple predicates instead of needing
// dedicated methods for each field.
func (s *FakeSink) Entries(filters ...func(slog.SinkEntry) bool) []slog.SinkEntry {
	s.mu.RLock()
	cpy := make([]slog.SinkEntry, len(s.entries))
	copy(cpy, s.entries)
	s.mu.RUnlock()
	filtered := make([]slog.SinkEntry, 0)
	for _, e := range cpy {
		if !matchAll(e, filters) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// Logger returns a slog.Logger backed by this sink at the given
// level. If no level is provided it defaults to LevelDebug, which
// captures everything. If more than one level is provided, the
// first one wins.
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
