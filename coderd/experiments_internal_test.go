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

// count returns how many entries at level carry exactly msg.
func (s *logRecorder) count(level slog.Level, msg string) int {
	var n int
	for _, m := range s.messages(level) {
		if m == msg {
			n++
		}
	}
	return n
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
	require.Equal(t, 1, rec.count(slog.LevelWarn, oauth2ExperimentDeprecatedMessage))
	require.NotContains(t, rec.messages(slog.LevelWarn), "ignoring unknown experiment",
		"oauth2 must be matched before the unknown-experiment branch")

	// A second read in the same process returns the same values and does not
	// repeat the deprecation warning. Upper-case input is matched too.
	got = parseExperiments(log, []string{"OAuth2", string(codersdk.ExperimentMCPServerHTTP)}, &once)
	require.Equal(t, codersdk.Experiments{codersdk.ExperimentMCPServerHTTP}, got)
	require.Equal(t, 1, rec.count(slog.LevelWarn, oauth2ExperimentDeprecatedMessage),
		"deprecation warning must be logged once per process")
}
