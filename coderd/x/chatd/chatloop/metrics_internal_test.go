package chatloop

import (
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// knownStages lists every Stage value.
var knownStages = map[Stage]struct{}{
	StageChatTurn:         {},
	StageQueueWait:        {},
	StageAcquisition:      {},
	StageGenerationStep:   {},
	StagePrepare:          {},
	StageMCPConnect:       {},
	StageProviderAttempt:  {},
	StageStream:           {},
	StageTimeToFirstToken: {},
	StageThinking:         {},
	StageToolCall:         {},
	StageCommit:           {},
	StageCompaction:       {},
	StageRetryBackoff:     {},
}

// metricHelp returns the help string of the named family, which must
// have at least one series in registry.
func metricHelp(t *testing.T, registry *prometheus.Registry, name string) string {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return family.GetHelp()
		}
	}
	t.Fatalf("family %q has no series", name)
	return ""
}

// requireNamesWord fails unless help contains word as a whole word.
func requireNamesWord(t *testing.T, help, word string) {
	t.Helper()
	require.Regexp(t, regexp.MustCompile(`\b`+regexp.QuoteMeta(word)+`\b`), help,
		"help does not name %q", word)
}

// TestStageSetsConsistent checks that the stage sets agree with each
// other and with the help text that documents them.
func TestStageSetsConsistent(t *testing.T) {
	t.Parallel()

	for stage := range observedStages {
		require.Contains(t, knownStages, stage)
	}
	for stage := range modelStages {
		require.Contains(t, observedStages, stage, "model stage %q is not observed", stage)
	}

	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithOptions(registry, MetricsOptions{StageMetrics: true})
	metrics.RecordStageDuration(StageCommit, ScopeTurn, ChatKindRoot, StageModel{}, time.Second)
	help := metricHelp(t, registry, "coderd_chatd_stage_duration_seconds")
	for stage := range observedStages {
		requireNamesWord(t, help, string(stage))
	}
}
