package harness

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// Results is the full compiled results for a set of test runs.
type Results struct {
	TotalRuns int              `json:"total_runs"`
	TotalPass int              `json:"total_pass"`
	TotalFail int              `json:"total_fail"`
	Elapsed   httpapi.Duration `json:"elapsed"`
	ElapsedMS int64            `json:"elapsed_ms"`

	Runs map[string]RunResult `json:"runs"`
}

// RunResult is the result of a single test run.
type RunResult struct {
	FullID     string           `json:"full_id"`
	TestName   string           `json:"test_name"`
	ID         string           `json:"id"`
	Logs       string           `json:"logs"`
	Error      error            `json:"error"`
	StartedAt  time.Time        `json:"started_at"`
	Duration   httpapi.Duration `json:"duration"`
	DurationMS int64            `json:"duration_ms"`
}

// Results returns the results of the test run. Panics if the test run is not
// done yet.
func (r *TestRun) Result() RunResult {
	select {
	case <-r.done:
	default:
		panic("cannot get results of a test run that is not done yet")
	}

	return RunResult{
		FullID:     r.FullID(),
		TestName:   r.testName,
		ID:         r.id,
		Logs:       r.logs.String(),
		Error:      r.err,
		StartedAt:  r.started,
		Duration:   httpapi.Duration(r.duration),
		DurationMS: r.duration.Milliseconds(),
	}
}

// Results collates the results of all the test runs and returns them.
func (h *TestHarness) Results() Results {
	if !h.started {
		panic("harness has not started")
	}
	select {
	case <-h.done:
	default:
		panic("harness has not finished")
	}

	results := Results{
		TotalRuns: len(h.runs),
		Runs:      make(map[string]RunResult, len(h.runs)),
		Elapsed:   httpapi.Duration(h.elapsed),
		ElapsedMS: h.elapsed.Milliseconds(),
	}
	for _, run := range h.runs {
		runRes := run.Result()
		results.Runs[runRes.FullID] = runRes

		if runRes.Error == nil {
			results.TotalPass++
		} else {
			results.TotalFail++
		}
	}

	return results
}

// PrintText prints the results as human-readable text to the given writer.
func (r *Results) PrintText(w io.Writer) {
	var totalDuration time.Duration
	for _, run := range r.Runs {
		totalDuration += time.Duration(run.Duration)
		if run.Error == nil {
			continue
		}

		_, _ = fmt.Fprintf(w, "\n== FAIL: %s\n\n", run.FullID)
		_, _ = fmt.Fprintf(w, "\tError: %s\n\n", run.Error)

		// Print log lines indented.
		_, _ = fmt.Fprintf(w, "\tLog:\n")
		rd := bufio.NewReader(strings.NewReader(run.Logs))
		for {
			line, err := rd.ReadBytes('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				_, _ = fmt.Fprintf(w, "\n\tLOG PRINT ERROR: %+v\n", err)
			}

			_, _ = fmt.Fprintf(w, "\t\t%s", line)
		}
	}

	_, _ = fmt.Fprintln(w, "\n\nTest results:")
	if r.TotalRuns == 0 {
		_, _ = fmt.Fprintln(w, "\tNo tests run")
		return
	}
	_, _ = fmt.Fprintf(w, "\tPass:  %d\n", r.TotalPass)
	_, _ = fmt.Fprintf(w, "\tFail:  %d\n", r.TotalFail)
	_, _ = fmt.Fprintf(w, "\tTotal: %d\n", r.TotalRuns)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintf(w, "\tTotal duration: %s\n", time.Duration(r.Elapsed))
	_, _ = fmt.Fprintf(w, "\tAvg. duration:  %s\n", totalDuration/time.Duration(r.TotalRuns))
}
