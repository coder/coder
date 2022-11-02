package harness

import "time"

// Results is the full compiled results for a set of test runs.
type Results struct {
	TotalRuns int
	TotalPass int
	TotalFail int

	Runs map[string]RunResult
}

// RunResult is the result of a single test run.
type RunResult struct {
	FullID    string
	TestName  string
	ID        string
	Logs      []byte
	Error     error
	StartedAt time.Time
	Duration  time.Duration
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
		FullID:    r.FullID(),
		TestName:  r.testName,
		ID:        r.id,
		Logs:      r.logs.Bytes(),
		Error:     r.err,
		StartedAt: r.started,
		Duration:  r.duration,
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
