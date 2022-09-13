package backends_test

import (
	"context"
	"testing"

	"github.com/fatih/structs"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/enterprise/audit/audittest"
	"github.com/coder/coder/enterprise/audit/backends"
)

func TestSlogBackend(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())

			sink    = &fakeSink{}
			logger  = slog.Make(sink)
			backend = backends.NewSlog(logger)

			alog = audittest.RandomLog()
		)
		defer cancel()

		err := backend.Export(ctx, alog)
		require.NoError(t, err)
		require.Len(t, sink.entries, 1)
		require.Equal(t, sink.entries[0].Message, "audit_log")
		require.Len(t, sink.entries[0].Fields, len(structs.Fields(alog)))
	})
}

type fakeSink struct {
	entries []slog.SinkEntry
}

func (s *fakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.entries = append(s.entries, e)
}

func (*fakeSink) Sync() {}
