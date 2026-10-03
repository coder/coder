package autostart

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scaletest/harness"
)

// RunResults contains the aggregated metrics from all autostart test runs.
type RunResults struct {
	TotalRuns      int
	SuccessfulRuns int
	FailedRuns     int

	// Individual run results.
	Runs []RunResult

	// Aggregate latency statistics (end-to-end).
	EndToEndLatencyP50 time.Duration
	EndToEndLatencyP95 time.Duration
	EndToEndLatencyP99 time.Duration

	// Aggregate latency statistics (trigger to completion).
	TriggerToCompletionP50 time.Duration
	TriggerToCompletionP95 time.Duration
	TriggerToCompletionP99 time.Duration
}

// NewRunResults creates a RunResults from a slice of RunResult.
func NewRunResults(runs []RunResult) RunResults {
	results := RunResults{
		TotalRuns: len(runs),
		Runs:      runs,
	}

	var (
		endToEndLatencies            []time.Duration
		triggerToCompletionLatencies []time.Duration
	)

	for _, run := range runs {
		if run.Success {
			results.SuccessfulRuns++
			endToEndLatencies = append(endToEndLatencies, run.EndToEndLatency())
			triggerToCompletionLatencies = append(triggerToCompletionLatencies, run.TriggerToCompletionLatency())
		} else {
			results.FailedRuns++
		}
	}

	// Calculate percentiles for end-to-end latency.
	if len(endToEndLatencies) > 0 {
		sort.Slice(endToEndLatencies, func(i, j int) bool {
			return endToEndLatencies[i] < endToEndLatencies[j]
		})
		results.EndToEndLatencyP50 = percentile(endToEndLatencies, 0.50)
		results.EndToEndLatencyP95 = percentile(endToEndLatencies, 0.95)
		results.EndToEndLatencyP99 = percentile(endToEndLatencies, 0.99)
	}

	// Calculate percentiles for trigger to completion latency.
	if len(triggerToCompletionLatencies) > 0 {
		sort.Slice(triggerToCompletionLatencies, func(i, j int) bool {
			return triggerToCompletionLatencies[i] < triggerToCompletionLatencies[j]
		})
		results.TriggerToCompletionP50 = percentile(triggerToCompletionLatencies, 0.50)
		results.TriggerToCompletionP95 = percentile(triggerToCompletionLatencies, 0.95)
		results.TriggerToCompletionP99 = percentile(triggerToCompletionLatencies, 0.99)
	}

	return results
}

// percentile calculates the percentile value from a sorted slice of durations.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * p)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// ToHarnessResults converts autostart-specific results into the standard
// harness.Results format for use with existing output functions.
func (r RunResults) ToHarnessResults() harness.Results {
	harnessRuns := make(map[string]harness.RunResult)

	for i, run := range r.Runs {
		id := fmt.Sprintf("%d", i)
		var err error
		if !run.Success {
			err = xerrors.New(run.Error)
		}

		harnessRuns[id] = harness.RunResult{
			FullID:   fmt.Sprintf("autostart/%s", run.WorkspaceName),
			TestName: "autostart",
			ID:       id,
			Error:    err,
			Metrics: map[string]any{
				"end_to_end_latency_seconds":    run.EndToEndLatency().Seconds(),
				"trigger_to_completion_seconds": run.TriggerToCompletionLatency().Seconds(),
				"workspace_id":                  run.WorkspaceID.String(),
				"workspace_name":                run.WorkspaceName,
			},
		}
	}

	return harness.Results{
		TotalRuns: r.TotalRuns,
		TotalPass: r.SuccessfulRuns,
		TotalFail: r.FailedRuns,
		Runs:      harnessRuns,
	}
}
