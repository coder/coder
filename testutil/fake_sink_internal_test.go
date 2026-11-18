package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
)

func TestFakeSinkCleanupDuringLogging(t *testing.T) {
	t.Parallel()

	tb := &fakeSinkTB{}
	sink := NewFakeSink(tb)
	tb.log = func() {
		// Cleanup uses this lock to prevent writes after the test completes.
		// It must remain held until TB.Log returns.
		if sink.mu.TryLock() {
			sink.mu.Unlock()
			t.Error("cleanup can complete while a test log is still in progress")
		}
	}
	sink.LogEntry(t.Context(), slog.SinkEntry{Message: "before cleanup"})
	tb.cleanup()
	tb.log = func() { t.Error("test log called after cleanup") }
	sink.LogEntry(t.Context(), slog.SinkEntry{Message: "after cleanup"})
	require.Len(t, sink.Entries(), 2)
}

type fakeSinkTB struct {
	testing.TB
	cleanup func()
	log     func()
}

func (t *fakeSinkTB) Cleanup(f func()) { t.cleanup = f }

func (t *fakeSinkTB) Log(...any) { t.log() }
