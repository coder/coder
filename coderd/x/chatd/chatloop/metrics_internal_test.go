package chatloop

import (
	"regexp"
	"strings"
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

// namesWord reports whether help contains word as a whole word.
func namesWord(help, word string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`).MatchString(help)
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
	metrics.RecordStageDuration(StageStream, ScopeTurn, ChatKindRoot, StageModel{ProviderType: "p", Model: "m"}, time.Second)

	// The stage family help lists exactly the observed stages.
	help := metricHelp(t, registry, "coderd_chatd_stage_duration_seconds")
	match := regexp.MustCompile(`Observed: ([a-z_, ]+); other stages are span-only`).FindStringSubmatch(help)
	require.Len(t, match, 2, "stage help has no observed list")
	listed := map[Stage]struct{}{}
	for _, name := range strings.Split(match[1], ", ") {
		listed[Stage(name)] = struct{}{}
	}
	require.Equal(t, observedStages, listed)

	// The model family help names the model stages and no other stage.
	modelHelp := metricHelp(t, registry, "coderd_chatd_model_stage_duration_seconds")
	for stage := range knownStages {
		_, want := modelStages[stage]
		require.Equal(t, want, namesWord(modelHelp, string(stage)),
			"model help naming %q", stage)
	}
}
