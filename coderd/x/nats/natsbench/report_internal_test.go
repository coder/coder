package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

func TestFormatInt(t *testing.T) {
	t.Parallel()

	require.Equal(t, "0", formatInt(0))
	require.Equal(t, "999", formatInt(999))
	require.Equal(t, "1,000", formatInt(1000))
	require.Equal(t, "123,456,789", formatInt(123456789))
	require.Equal(t, "-1,234", formatInt(-1234))
}

func TestFormatPayload(t *testing.T) {
	t.Parallel()

	require.Equal(t, "8 KiB", formatPayload(Payload8KB))
	require.Equal(t, "64 KiB", formatPayload(Payload64KB))
	require.Equal(t, "100 B", formatPayload(100))
}

func TestRenderMarkdown(t *testing.T) {
	t.Parallel()

	valid := ScenarioResult{
		Scenario: Scenario{
			Name:   "8KiB-1r",
			Config: Config{Messages: 100000, PayloadSize: Payload8KB, Replicas: 1},
		},
		Result: &Result{
			Config:           Config{Messages: 100000, PayloadSize: Payload8KB, Replicas: 1},
			Published:        100000,
			Delivered:        500000,
			PublishDuration:  time.Second,
			DeliverDuration:  2 * time.Second,
			PubsPerSec:       100000,
			DeliveriesPerSec: 250000,
		},
	}
	dropped := ScenarioResult{
		Scenario: Scenario{
			Name:   "8KiB-5r",
			Config: Config{Messages: 100000, PayloadSize: Payload8KB, Replicas: 5},
		},
		Result: &Result{
			Config: Config{Messages: 100000, PayloadSize: Payload8KB, Replicas: 5},
			Drops:  3,
		},
		Err: xerrors.New("invalid run: 3 dropped-message signals observed"),
	}
	failed := ScenarioResult{
		Scenario: Scenario{
			Name:   "64KiB-10r",
			Config: Config{Messages: 20000, PayloadSize: Payload64KB, Replicas: 10},
		},
		Err: xerrors.New("readiness gate: timed out\nsecond line is omitted"),
	}

	var b strings.Builder
	require.NoError(t, RenderMarkdown(&b, []ScenarioResult{valid, dropped, failed}))
	out := b.String()

	require.Contains(t, out, "### Payload 8 KiB")
	require.Contains(t, out, "### Payload 64 KiB")
	// The 8 KiB group has an invalid run, so it carries a Status column.
	require.Contains(t, out, "Status")
	require.Contains(t, out, "100,000")
	require.Contains(t, out, "250,000")
	require.Contains(t, out, "ok")
	// Invalid runs never render a throughput number.
	require.Contains(t, out, "INVALID")
	require.Contains(t, out, "invalid run: 3 dropped-message signals observed")
	require.Contains(t, out, "readiness gate: timed out")
	require.NotContains(t, out, "second line is omitted")

	// Every body row has the same width as the header, so columns line
	// up in a terminal.
	assertAlignedTable(t, out)
}

// assertAlignedTable checks that all table rows in a rendered report
// (lines starting with "|") within each contiguous block share the same
// rune width.
func assertAlignedTable(t *testing.T, out string) {
	t.Helper()
	width := -1
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "|") {
			width = -1
			continue
		}
		if width < 0 {
			width = len([]rune(line))
			continue
		}
		require.Equal(t, width, len([]rune(line)), "row width mismatch:\n%s", line)
	}
}

func TestRenderMarkdownCleanGroupOmitsStatus(t *testing.T) {
	t.Parallel()

	result := func(replicas int, pubs, dels float64) ScenarioResult {
		cfg := Config{
			Messages: 100000, PayloadSize: Payload8KB, Replicas: replicas,
			Subjects: 10, Publishers: 10, Subscribers: 50,
		}
		return ScenarioResult{
			Scenario: Scenario{Config: cfg},
			Result:   &Result{Config: cfg, PubsPerSec: pubs, DeliveriesPerSec: dels},
		}
	}

	var b strings.Builder
	require.NoError(t, RenderMarkdown(&b, []ScenarioResult{
		result(1, 100000, 250000),
		result(5, 90000, 220000),
	}))
	out := b.String()

	// The shape columns are reported alongside throughput.
	for _, header := range []string{"Replicas", "Subjects", "Publishers", "Subscribers", "Messages", "Pubs/sec", "Deliveries/sec"} {
		require.Contains(t, out, header)
	}
	require.NotContains(t, out, "Status")
	require.NotContains(t, out, "Scenario")
	require.NotContains(t, out, "Drops")
	require.Contains(t, out, "250,000")
	require.Contains(t, out, "220,000")
	assertAlignedTable(t, out)
}

func TestDefaultScenarios(t *testing.T) {
	t.Parallel()

	scenarios := DefaultScenarios()
	require.Len(t, scenarios, 6)
	seen := make(map[string]struct{})
	for _, sc := range scenarios {
		seen[sc.Name] = struct{}{}
		cfg := sc.Config
		cfg.Timeout = testutil.WaitShort
		require.NoError(t, cfg.validate())
		if sc.Config.PayloadSize == Payload64KB && sc.Config.Replicas > 1 {
			require.Less(t, sc.Config.Messages, DefaultMessages,
				"64KiB cluster runs must reduce the message count")
		} else {
			require.Equal(t, DefaultMessages, sc.Config.Messages)
		}
	}
	require.Len(t, seen, 6, "scenario names must be unique")
}
