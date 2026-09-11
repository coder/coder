package coderd

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// logRecorder keeps every entry so a test can count log lines.
type logRecorder struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *logRecorder) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*logRecorder) Sync() {}

func (s *logRecorder) messages(level slog.Level) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, e := range s.entries {
		if e.Level == level {
			out = append(out, e.Message)
		}
	}
	return out
}

func TestReadExperimentsDeprecatedOAuth2(t *testing.T) {
	t.Parallel()

	rec := &logRecorder{}
	log := slog.Make(rec)
	var once sync.Once
	raw := []string{string(codersdk.ExperimentOAuth2), string(codersdk.ExperimentMCPServerHTTP)}

	got := parseExperiments(log, raw, &once)
	require.Equal(t, codersdk.Experiments{codersdk.ExperimentMCPServerHTTP}, got,
		"the oauth2 experiment must be dropped, not passed through")
	require.Equal(t, []string{oauth2ExperimentDeprecatedMessage, "🐉 HERE BE DRAGONS: opting into hidden experiment"},
		rec.messages(slog.LevelWarn))

	// A second read in the same process returns the same slice and does not
	// repeat the deprecation warning. Upper-case input is matched too.
	got = parseExperiments(log, []string{"OAuth2", string(codersdk.ExperimentMCPServerHTTP)}, &once)
	require.Equal(t, codersdk.Experiments{codersdk.ExperimentMCPServerHTTP}, got)
	var deprecations int
	for _, m := range rec.messages(slog.LevelWarn) {
		if m == oauth2ExperimentDeprecatedMessage {
			deprecations++
		}
	}
	require.Equal(t, 1, deprecations, "deprecation warning must be logged once per process")
}
