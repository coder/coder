package natsbench

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestBenchMatrix runs the full benchmark matrix and emits the grouped
// markdown report. It is heavyweight by design and never runs in normal
// CI: set CODER_TEST_NATS_BENCH=1 to opt in.
//
// Optional overrides:
//   - CODER_TEST_NATS_BENCH_MESSAGES: total messages for every scenario
//     (for quick validation runs).
//   - CODER_TEST_NATS_BENCH_TIMEOUT: per-phase timeout as a Go duration.
//   - CODER_TEST_NATS_BENCH_OUT: also write the markdown report to this
//     file path.
func TestBenchMatrix(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	if os.Getenv("CODER_TEST_NATS_BENCH") != "1" {
		t.Skip("Set CODER_TEST_NATS_BENCH=1 to run the NATS benchmark matrix")
	}

	messagesOverride := 0
	if v := os.Getenv("CODER_TEST_NATS_BENCH_MESSAGES"); v != "" {
		parsed, err := strconv.Atoi(v)
		require.NoError(t, err, "parse CODER_TEST_NATS_BENCH_MESSAGES")
		messagesOverride = parsed
	}
	var timeoutOverride time.Duration
	if v := os.Getenv("CODER_TEST_NATS_BENCH_TIMEOUT"); v != "" {
		parsed, err := time.ParseDuration(v)
		require.NoError(t, err, "parse CODER_TEST_NATS_BENCH_TIMEOUT")
		timeoutOverride = parsed
	}

	logger := testutil.Logger(t)
	scenarios := DefaultScenarios()
	results := make([]ScenarioResult, 0, len(scenarios))

	// Scenarios run sequentially so they never compete for CPU, memory,
	// or the network stack and skew each other's numbers.
	for _, sc := range scenarios {
		cfg := sc.Config
		if messagesOverride > 0 {
			cfg.Messages = messagesOverride
		}
		if timeoutOverride > 0 {
			cfg.Timeout = timeoutOverride
		}
		cfg = cfg.withDefaults()

		// No outer context timeout: every phase inside Run is already
		// bounded by cfg.Timeout, and the go test -timeout flag bounds
		// the whole matrix.
		res, err := Run(context.Background(), logger, cfg)

		// Record the effective config so the report reflects overrides
		// even when a failed run returns no Result.
		results = append(results, ScenarioResult{
			Scenario: Scenario{Name: sc.Name, Config: cfg},
			Result:   res,
			Err:      err,
		})
		if err != nil {
			t.Errorf("scenario %s: %v", sc.Name, err)
			continue
		}
		t.Logf("scenario %s: published=%d delivered=%d pubs/sec=%.0f deliveries/sec=%.0f",
			sc.Name, res.Published, res.Delivered, res.PubsPerSec, res.DeliveriesPerSec)
	}

	var report strings.Builder
	require.NoError(t, RenderMarkdown(&report, results))
	t.Logf("benchmark report:\n\n%s", report.String())

	if out := os.Getenv("CODER_TEST_NATS_BENCH_OUT"); out != "" {
		require.NoError(t, os.WriteFile(out, []byte(report.String()), 0o600))
		t.Logf("report written to %s", out)
	}
}
