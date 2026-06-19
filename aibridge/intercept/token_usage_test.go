package intercept_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept"
)

// captureSink records log entries for assertions.
type captureSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *captureSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*captureSink) Sync() {}

func TestNonNegativeInputTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		total       int64
		cached      int64
		expected    int64
		expectError bool
	}{
		{name: "no_cached_tokens", total: 100, cached: 0, expected: 100},
		{name: "cached_less_than_total", total: 100, cached: 25, expected: 75},
		{name: "cached_equals_total", total: 50, cached: 50, expected: 0},
		{name: "zero", total: 0, cached: 0, expected: 0},
		// AIGOV-452: cached > total must clamp to zero and log an error.
		{name: "cached_greater_than_total_clamps_and_logs", total: 805, cached: 4352, expected: 0, expectError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := &captureSink{}
			logger := slog.Make(sink)

			got := intercept.NonNegativeInputTokens(t.Context(), logger, "test-provider", "/v1/test", tc.total, tc.cached)
			require.Equal(t, tc.expected, got)
			require.GreaterOrEqual(t, got, int64(0), "result must never be negative")

			if !tc.expectError {
				require.Empty(t, sink.entries)
				return
			}

			require.Len(t, sink.entries, 1)
			e := sink.entries[0]
			require.Equal(t, slog.LevelError, e.Level)
			fields := fieldMap(e.Fields)
			require.Equal(t, "test-provider", fields["provider"])
			require.Equal(t, "/v1/test", fields["endpoint"])
			require.Equal(t, tc.total, fields["total_input_tokens"])
			require.Equal(t, tc.cached, fields["cached_input_tokens"])
		})
	}
}

func fieldMap(fields slog.Map) map[string]any {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.Name] = f.Value
	}
	return m
}
