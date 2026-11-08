package agentacp

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestProcessLogger(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}

	sink := testutil.NewFakeSink(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	p, err := launch(ctx, harnessConfig{
		slug: "fake",
		// Emit malformed output immediately, then keep stdout open.
		command: `printf '%s\n' '{"private":"workspace-secret-marker"'; cat`,
	}, t.TempDir(), &discoveryClient{}, launchOptions{
		logger:  sink.Logger(),
		clock:   quartz.NewMock(t),
		execer:  agentexec.DefaultExecer,
		envInfo: testEnv{home: t.TempDir()},
		env:     os.Environ(),
	})
	require.NoError(t, err)
	t.Cleanup(p.close)

	var entries []slog.SinkEntry
	require.Eventually(t, func() bool {
		entries = sink.Entries(func(entry slog.SinkEntry) bool {
			return entry.Message == "failed to parse incoming message"
		})
		return len(entries) > 0
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Len(t, entries, 1)
	require.Equal(t, slog.LevelError, entries[0].Level)
	require.Equal(t, []string{"transport"}, entries[0].LoggerNames)
	require.Contains(t, entries[0].Fields, slog.F("harness", "fake"))
	require.NotContains(t, fmt.Sprint(entries), "workspace-secret-marker")
}
