package agentacp

import (
	stdslog "log/slog" //nolint:depguard // Exercise the ACP SDK's logger interface.
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

func TestSlogHandler(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		level stdslog.Level
		want  slog.Level
	}{
		{name: "Debug", level: stdslog.LevelDebug, want: slog.LevelDebug},
		{name: "Info", level: stdslog.LevelInfo, want: slog.LevelInfo},
		{name: "Warn", level: stdslog.LevelWarn, want: slog.LevelWarn},
		{name: "Error", level: stdslog.LevelError, want: slog.LevelError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := testutil.NewFakeSink(t)
			base := stdslog.New(&slogHandler{logger: sink.Logger().Named("acp")})
			logger := base.With("harness", "fake", "raw", "private bound frame").WithGroup("connection").With("id", 42)
			logger.LogAttrs(t.Context(), tc.level, "diagnostic",
				stdslog.Group("queue", "length", 3),
				stdslog.String("raw", "private frame"),
				stdslog.Attr{},
			)
			entries := sink.Entries()
			require.Len(t, entries, 1)
			require.Equal(t, tc.want, entries[0].Level)
			require.Equal(t, "diagnostic", entries[0].Message)
			require.Equal(t, []string{"acp"}, entries[0].LoggerNames)
			require.ElementsMatch(t, slog.M(
				slog.F("harness", "fake"),
				slog.F("connection_id", int64(42)),
				slog.F("connection_queue_length", int64(3)),
			), entries[0].Fields)

			base.Info("base")
			entries = sink.Entries()
			require.Len(t, entries, 2)
			require.Empty(t, entries[1].Fields, "derived loggers must not modify the base logger")
		})
	}

	t.Run("ConfiguredLevel", func(t *testing.T) {
		t.Parallel()
		sink := testutil.NewFakeSink(t)
		logger := stdslog.New(&slogHandler{logger: sink.Logger(slog.LevelWarn)})
		logger.Debug("debug")
		logger.Info("info")
		logger.Warn("warn")
		entries := sink.Entries()
		require.Len(t, entries, 1)
		require.Equal(t, "warn", entries[0].Message)
	})
}
