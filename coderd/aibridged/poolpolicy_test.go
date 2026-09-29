package aibridged_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestPoolOptionsFromConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                        string
		cfg                         codersdk.AIBridgeConfig
		wantStructuredLogging       bool
		wantDisableContentRecording bool
		// wantWarning is true when the deployment drops content records
		// without exporting them anywhere, which nothing else reports.
		wantWarning bool
	}{
		{
			name: "Defaults",
		},
		{
			name: "StructuredLoggingWithoutSource",
			cfg:  codersdk.AIBridgeConfig{StructuredLogging: serpent.Bool(true)},
		},
		{
			name: "SourceCoderd",
			cfg: codersdk.AIBridgeConfig{
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceCoderd),
			},
		},
		{
			name: "SourceGateway",
			cfg: codersdk.AIBridgeConfig{
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceGateway),
			},
			wantStructuredLogging: true,
		},
		{
			name: "SourceBoth",
			cfg: codersdk.AIBridgeConfig{
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceBoth),
			},
			wantStructuredLogging: true,
		},
		{
			name: "SourceGatewayWithoutStructuredLogging",
			cfg: codersdk.AIBridgeConfig{
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceGateway),
			},
		},
		{
			// Nothing else reports that the dropped records are exported
			// nowhere, so the derivation has to.
			name:                        "ContentRecordingDisabledWithoutGatewayLogs",
			cfg:                         codersdk.AIBridgeConfig{DisableContentRecording: serpent.Bool(true)},
			wantDisableContentRecording: true,
			wantWarning:                 true,
		},
		{
			name: "ContentRecordingDisabledWithGatewayLogs",
			cfg: codersdk.AIBridgeConfig{
				DisableContentRecording: serpent.Bool(true),
				StructuredLogging:       serpent.Bool(true),
				StructuredLoggingSource: string(codersdk.AIStructuredLoggingSourceGateway),
			},
			wantStructuredLogging:       true,
			wantDisableContentRecording: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := &warningSink{}
			logger := slog.Make(sink)
			options := aibridged.PoolOptionsFromConfig(t.Context(), logger, tc.cfg)

			require.Equal(t, tc.wantStructuredLogging, options.StructuredLogging, "StructuredLogging")
			require.Equal(t, tc.wantDisableContentRecording, options.DisableContentRecording, "DisableContentRecording")
			require.Equal(t, tc.wantWarning, sink.warned("content recording is disabled"), "startup warning")

			// The record policy must not disturb the cache sizing the
			// deployment relies on.
			require.Equal(t, aibridged.DefaultPoolOptions.MaxItems, options.MaxItems, "MaxItems")
			require.Equal(t, aibridged.DefaultPoolOptions.TTL, options.TTL, "TTL")
		})
	}
}

// warningSink collects log entries so a test can assert on what the derivation
// reported.
type warningSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *warningSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*warningSink) Sync() {}

// warned reports whether a warning containing substr was logged.
func (s *warningSink) warned(substr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range s.entries {
		if entry.Level == slog.LevelWarn && strings.Contains(entry.Message, substr) {
			return true
		}
	}
	return false
}
