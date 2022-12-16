package harness_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/scaletest/harness"
)

func Test_Results(t *testing.T) {
	t.Parallel()

	results := harness.Results{
		TotalRuns: 10,
		TotalPass: 8,
		TotalFail: 2,
		Runs: map[string]harness.RunResult{
			"test-0/0": {
				FullID:     "test-0/0",
				TestName:   "test-0",
				ID:         "0",
				Logs:       "test-0/0 log line 1\ntest-0/0 log line 2",
				Error:      xerrors.New("test-0/0 error"),
				StartedAt:  time.Now(),
				Duration:   httpapi.Duration(time.Second),
				DurationMS: 1000,
			},
			"test-0/1": {
				FullID:     "test-0/1",
				TestName:   "test-0",
				ID:         "1",
				Logs:       "test-0/1 log line 1\ntest-0/1 log line 2",
				Error:      nil,
				StartedAt:  time.Now(),
				Duration:   httpapi.Duration(time.Second),
				DurationMS: 1000,
			},
		},
		Elapsed:   httpapi.Duration(time.Second),
		ElapsedMS: 1000,
	}

	expected := `
== FAIL: test-0/0

	Error: test-0/0 error

	Log:
		test-0/0 log line 1


Test results:
	Pass:  8
	Fail:  2
	Total: 10

	Total duration: 1s
	Avg. duration:  200ms
`

	out := bytes.NewBuffer(nil)
	results.PrintText(out)

	require.Equal(t, expected, out.String())
}
