package autostart

import (
	"encoding/json"
	"fmt"
	"io"
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

// PrintText writes the results in a human-readable text format.
func (r RunResults) PrintText(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Autostart Scale Test Results\n")
	_, _ = fmt.Fprintf(w, "=============================\n\n")

	_, _ = fmt.Fprintf(w, "Total Runs:      %d\n", r.TotalRuns)
	_, _ = fmt.Fprintf(w, "Successful:      %d\n", r.SuccessfulRuns)
	_, _ = fmt.Fprintf(w, "Failed:          %d\n\n", r.FailedRuns)

	if r.SuccessfulRuns > 0 {
		_, _ = fmt.Fprintf(w, "End-to-End Latency (Config → Completion)\n")
		_, _ = fmt.Fprintf(w, "-----------------------------------------\n")
		_, _ = fmt.Fprintf(w, "P50: %v\n", r.EndToEndLatencyP50.Round(time.Millisecond))
		_, _ = fmt.Fprintf(w, "P95: %v\n", r.EndToEndLatencyP95.Round(time.Millisecond))
		_, _ = fmt.Fprintf(w, "P99: %v\n\n", r.EndToEndLatencyP99.Round(time.Millisecond))

		_, _ = fmt.Fprintf(w, "Trigger to Completion Latency (Scheduled Time → Completion)\n")
		_, _ = fmt.Fprintf(w, "------------------------------------------------------------\n")
		_, _ = fmt.Fprintf(w, "P50: %v\n", r.TriggerToCompletionP50.Round(time.Millisecond))
		_, _ = fmt.Fprintf(w, "P95: %v\n", r.TriggerToCompletionP95.Round(time.Millisecond))
		_, _ = fmt.Fprintf(w, "P99: %v\n\n", r.TriggerToCompletionP99.Round(time.Millisecond))
	}

	if r.FailedRuns > 0 {
		_, _ = fmt.Fprintf(w, "Failed Runs\n")
		_, _ = fmt.Fprintf(w, "-----------\n")
		for _, run := range r.Runs {
			if !run.Success {
				_, _ = fmt.Fprintf(w, "- %s (%s): %s\n", run.WorkspaceName, run.WorkspaceID, run.Error)
			}
		}
	}
}

// MarshalJSON implements json.Marshaler to provide custom JSON output.
func (r RunResults) MarshalJSON() ([]byte, error) {
	// Convert durations to milliseconds for JSON output.
	type jsonResults struct {
		TotalRuns      int `json:"total_runs"`
		SuccessfulRuns int `json:"successful_runs"`
		FailedRuns     int `json:"failed_runs"`

		EndToEndLatencyP50MS int64 `json:"end_to_end_latency_p50_ms"`
		EndToEndLatencyP95MS int64 `json:"end_to_end_latency_p95_ms"`
		EndToEndLatencyP99MS int64 `json:"end_to_end_latency_p99_ms"`

		TriggerToCompletionP50MS int64 `json:"trigger_to_completion_p50_ms"`
		TriggerToCompletionP95MS int64 `json:"trigger_to_completion_p95_ms"`
		TriggerToCompletionP99MS int64 `json:"trigger_to_completion_p99_ms"`

		Runs []struct {
			WorkspaceID   string `json:"workspace_id"`
			WorkspaceName string `json:"workspace_name"`
			Success       bool   `json:"success"`
			Error         string `json:"error,omitempty"`

			EndToEndLatencyMS     int64 `json:"end_to_end_latency_ms"`
			TriggerToCompletionMS int64 `json:"trigger_to_completion_ms"`
		} `json:"runs"`
	}

	jr := jsonResults{
		TotalRuns:      r.TotalRuns,
		SuccessfulRuns: r.SuccessfulRuns,
		FailedRuns:     r.FailedRuns,

		EndToEndLatencyP50MS: r.EndToEndLatencyP50.Milliseconds(),
		EndToEndLatencyP95MS: r.EndToEndLatencyP95.Milliseconds(),
		EndToEndLatencyP99MS: r.EndToEndLatencyP99.Milliseconds(),

		TriggerToCompletionP50MS: r.TriggerToCompletionP50.Milliseconds(),
		TriggerToCompletionP95MS: r.TriggerToCompletionP95.Milliseconds(),
		TriggerToCompletionP99MS: r.TriggerToCompletionP99.Milliseconds(),
	}

	for _, run := range r.Runs {
		jr.Runs = append(jr.Runs, struct {
			WorkspaceID   string `json:"workspace_id"`
			WorkspaceName string `json:"workspace_name"`
			Success       bool   `json:"success"`
			Error         string `json:"error,omitempty"`

			EndToEndLatencyMS     int64 `json:"end_to_end_latency_ms"`
			TriggerToCompletionMS int64 `json:"trigger_to_completion_ms"`
		}{
			WorkspaceID:   run.WorkspaceID.String(),
			WorkspaceName: run.WorkspaceName,
			Success:       run.Success,
			Error:         run.Error,

			EndToEndLatencyMS:     run.EndToEndLatency().Milliseconds(),
			TriggerToCompletionMS: run.TriggerToCompletionLatency().Milliseconds(),
		})
	}

	return json.Marshal(jr)
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
