package nats

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

func TestServerLogger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		log   func(serverLogger, string, ...any)
		level slog.Level
	}{
		{name: "Notice", log: serverLogger.Noticef, level: slog.LevelInfo},
		{name: "Warn", log: serverLogger.Warnf, level: slog.LevelWarn},
		{name: "Fatal", log: serverLogger.Fatalf, level: slog.LevelCritical},
		{name: "Error", log: serverLogger.Errorf, level: slog.LevelError},
		{name: "Debug", log: serverLogger.Debugf, level: slog.LevelDebug},
		{name: "Trace", log: serverLogger.Tracef, level: slog.LevelDebug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := testutil.NewFakeSink(t)
			logger := serverLogger{logger: sink.Logger().Named("nats").With(slog.F("replica", "test"))}
			tc.log(logger, "connection %s: %d pending", "client", 42)

			entries := sink.Entries()
			require.Len(t, entries, 1)
			require.Equal(t, tc.level, entries[0].Level)
			require.Equal(t, "connection client: 42 pending", entries[0].Message)
			require.Equal(t, []string{"nats"}, entries[0].LoggerNames)
			require.Equal(t, slog.Map{slog.F("replica", "test")}, entries[0].Fields)
		})
	}
}

func TestEmbeddedServerLogging(t *testing.T) {
	t.Parallel()

	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()

			sink := testutil.NewFakeSink(t)
			ps, err := New(t.Context(), sink.Logger(level).Named("nats"), defaultTestOptions())
			require.NoError(t, err)
			t.Cleanup(func() { _ = ps.Close() })

			startup := sink.Entries(func(e slog.SinkEntry) bool {
				return e.Message == "Starting nats-server"
			})
			require.Len(t, startup, 1, "logger must be installed before the server starts")
			require.Equal(t, slog.LevelInfo, startup[0].Level)
			require.Equal(t, []string{"nats", "server"}, startup[0].LoggerNames)

			ps.Server.Debugf("debug %s", "message")
			debug := sink.Entries(func(e slog.SinkEntry) bool {
				return e.Message == "debug message"
			})
			if level == slog.LevelDebug {
				require.Len(t, debug, 1)
			} else {
				require.Empty(t, debug)
			}

			ps.Server.Tracef("trace %s", "payload")
			require.Empty(t, sink.Entries(func(e slog.SinkEntry) bool {
				return e.Message == "trace payload"
			}), "protocol tracing must remain disabled")
		})
	}
}
