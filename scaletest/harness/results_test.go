package harness_test
import (
	"errors"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/scaletest/harness"
)
type testError struct {
	hidden error
}
func (e testError) Error() string {
	return e.hidden.Error()
}
func Test_Results(t *testing.T) {
	t.Parallel()
	now := time.Date(2023, 10, 5, 12, 3, 56, 395813665, time.UTC)
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
				Error:      errors.New("test-0/0 error"),
				StartedAt:  now,
				Duration:   httpapi.Duration(time.Second),
				DurationMS: 1000,
			},
			"test-0/1": {
				FullID:     "test-0/1",
				TestName:   "test-0",
				ID:         "1",
				Logs:       "test-0/1 log line 1\ntest-0/1 log line 2",
				Error:      nil,
				StartedAt:  now.Add(333 * time.Millisecond),
				Duration:   httpapi.Duration(time.Second),
				DurationMS: 1000,
			},
			"test-0/2": {
				FullID:     "test-0/2",
				TestName:   "test-0",
				ID:         "2",
				Logs:       "test-0/2 log line 1\ntest-0/2 log line 2",
				Error:      testError{hidden: errors.New("test-0/2 error")},
				StartedAt:  now.Add(666 * time.Millisecond),
				Duration:   httpapi.Duration(time.Second),
				DurationMS: 1000,
			},
		},
		Elapsed:   httpapi.Duration(time.Second),
		ElapsedMS: 1000,
	}
	wantText := `
== FAIL: test-0/0
	Error: test-0/0 error
	Log:
		test-0/0 log line 1
== FAIL: test-0/2
	Error: test-0/2 error
	Log:
		test-0/2 log line 1
Test results:
	Pass:  8
	Fail:  2
	Total: 10
	Total duration: 1s
	Avg. duration:  300ms
`
	wantJSON := `{
	"total_runs": 10,
	"total_pass": 8,
	"total_fail": 2,
	"elapsed": "1s",
	"elapsed_ms": 1000,
	"runs": {
		"test-0/0": {
			"full_id": "test-0/0",
			"test_name": "test-0",
			"id": "0",
			"logs": "test-0/0 log line 1\ntest-0/0 log line 2",
			"started_at": "2023-10-05T12:03:56.395813665Z",
			"duration": "1s",
			"duration_ms": 1000,
			"error": "test-0/0 error:\n    github.com/coder/coder/v2/scaletest/harness_test.Test_Results\n        [working_directory]/results_test.go:43"
		},
		"test-0/1": {
			"full_id": "test-0/1",
			"test_name": "test-0",
			"id": "1",
			"logs": "test-0/1 log line 1\ntest-0/1 log line 2",
			"started_at": "2023-10-05T12:03:56.728813665Z",
			"duration": "1s",
			"duration_ms": 1000,
			"error": "\u003cnil\u003e"
		},
		"test-0/2": {
			"full_id": "test-0/2",
			"test_name": "test-0",
			"id": "2",
			"logs": "test-0/2 log line 1\ntest-0/2 log line 2",
			"started_at": "2023-10-05T12:03:57.061813665Z",
			"duration": "1s",
			"duration_ms": 1000,
			"error": "test-0/2 error"
		}
	}
}
`
	wd, err := os.Getwd()
	require.NoError(t, err)
	wd = filepath.ToSlash(wd) // Hello there Windows, my friend...
	wantJSON = strings.Replace(wantJSON, "[working_directory]", wd, 1)
	out := bytes.NewBuffer(nil)
	results.PrintText(out)
	assert.Empty(t, cmp.Diff(wantText, out.String()), "text result does not match (-want +got)")
	out.Reset()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "\t")
	err = enc.Encode(results)
	require.NoError(t, err)
	assert.Empty(t, cmp.Diff(wantJSON, out.String()), "JSON result does not match (-want +got)")
}
