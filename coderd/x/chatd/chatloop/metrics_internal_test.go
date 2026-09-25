package chatloop

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

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

// metricHelp requires the family to have at least one series.
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

func namesWord(help, word string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`).MatchString(help)
}

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
	metrics.recordStageDuration(StageStream, ScopeTurn, ChatKindRoot, StageModel{ProviderType: "p", Model: "m"}, time.Second)

	help := metricHelp(t, registry, "coderd_chatd_stage_duration_seconds")
	match := regexp.MustCompile(`Observed: ([a-z_, ]+); other stages are span-only`).FindStringSubmatch(help)
	require.Len(t, match, 2, "stage help has no observed list")
	listed := map[Stage]struct{}{}
	for name := range strings.SplitSeq(match[1], ", ") {
		listed[Stage(name)] = struct{}{}
	}
	require.Equal(t, observedStages, listed)

	modelHelp := metricHelp(t, registry, "coderd_chatd_model_stage_duration_seconds")
	for stage := range knownStages {
		_, want := modelStages[stage]
		require.Equal(t, want, namesWord(modelHelp, string(stage)),
			"model help naming %q", stage)
	}

	// A recorded stage takes its full duration and an attributing stage
	// its own time, so no stage may be both.
	for stage, category := range attributingStages {
		require.Contains(t, knownStages, stage)
		require.NotContains(t, recordedStageCategories, stage, "stage %q is both attributing and recorded", stage)
		require.Contains(t, turnTimeCategories, category)
	}
	for stage, category := range recordedStageCategories {
		require.Contains(t, knownStages, stage)
		require.Contains(t, turnTimeCategories, category)
	}

	metrics.RecordTurnCategory(TurnCategoryStreaming, ChatKindRoot, TurnOutcomeCompleted, time.Second)
	turnHelp := metricHelp(t, registry, "coderd_chatd_turn_time_seconds_total")
	for _, category := range turnTimeCategories {
		requireNamesWord(t, turnHelp, string(category))
	}
}
