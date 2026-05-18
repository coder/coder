package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunEmptyInputWritesNoMarkdownAndEmptyFailures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jsonFile := filepath.Join(dir, "go-test.json")
	failuresFile := filepath.Join(dir, "failures.ndjson")
	require.NoError(t, os.WriteFile(jsonFile, nil, 0o600))

	var stdout bytes.Buffer
	err := run(context.Background(), config{
		JSONFile:       jsonFile,
		MarkdownOut:    "-",
		FailuresOut:    failuresFile,
		MaxOutputBytes: 8192,
	}, &stdout, ioDiscard{}, emptyEnv)
	require.NoError(t, err)
	require.Empty(t, stdout.String())
	assertFileContent(t, failuresFile, "")
}

func TestRunPassingInputWritesNoMarkdown(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestOK", Output: "ok\n"},
		testEvent{Action: "pass", Package: "example.com/pkg", Test: "TestOK", Elapsed: 0.01},
		testEvent{Action: "pass", Package: "example.com/pkg", Elapsed: 0.02},
	)
	failuresFile := filepath.Join(t.TempDir(), "failures.ndjson")

	var stdout bytes.Buffer
	err := run(context.Background(), config{
		JSONFile:       jsonFile,
		MarkdownOut:    "-",
		FailuresOut:    failuresFile,
		MaxOutputBytes: 8192,
	}, &stdout, ioDiscard{}, emptyEnv)
	require.NoError(t, err)
	require.Empty(t, stdout.String())
	assertFileContent(t, failuresFile, "")
}

func TestRunSingleFailureRendersBoundedOutput(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestFail", Output: "prefix-" + strings.Repeat("x", 20)},
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestFail", Elapsed: 1.25},
		testEvent{Action: "fail", Package: "example.com/pkg", Elapsed: 1.50},
	)

	markdown := runMarkdown(t, jsonFile, config{MaxOutputBytes: 10})
	require.Contains(t, markdown, "## Go test failures (2 in 1 packages)")
	require.Contains(t, markdown, "| example.com/pkg | TestFail | 1.25s |")
	require.NotContains(t, markdown, "prefix")
	require.Contains(t, markdown, strings.Repeat("x", 10))
}

func TestRunSubtestFailureCapturesSlashName(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestParent/subcase", Output: "subtest failed\n"},
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestParent/subcase", Elapsed: 0.20},
	)

	markdown := runMarkdown(t, jsonFile, config{MaxOutputBytes: 8192})
	require.Contains(t, markdown, "TestParent/subcase")
	require.Contains(t, markdown, "subtest failed")
}

func TestRunRerunPassRemovesPriorFailure(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestFlake", Output: "first run failed\n"},
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestFlake", Elapsed: 0.10},
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestFlake", Output: "retry passed\n"},
		testEvent{Action: "pass", Package: "example.com/pkg", Test: "TestFlake", Elapsed: 0.05},
	)

	markdown := runMarkdown(t, jsonFile, config{MaxOutputBytes: 8192})
	require.Empty(t, markdown)
}

func TestRunStripsANSIOutput(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestFail", Output: "\x1b[31mred\x1b[0m\n"},
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestFail", Elapsed: 0.10},
	)

	markdown := runMarkdown(t, jsonFile, config{MaxOutputBytes: 8192})
	require.Contains(t, markdown, "red")
	require.NotContains(t, markdown, "\x1b")
}

func TestRunEscapesTripleBackticksInOutput(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Test: "TestFail", Output: "before ``` after\n"},
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestFail", Elapsed: 0.10},
	)

	markdown := runMarkdown(t, jsonFile, config{MaxOutputBytes: 8192})
	require.Contains(t, markdown, "before `` after")
	require.Equal(t, 2, strings.Count(markdown, "```"))
}

func TestRunMaxFailuresAddsOmittedLine(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestA", Elapsed: 0.10},
		testEvent{Action: "fail", Package: "example.com/pkg", Test: "TestB", Elapsed: 0.20},
	)

	markdown := runMarkdown(t, jsonFile, config{
		MaxOutputBytes: 8192,
		MaxFailures:    1,
		FailuresOut:    filepath.Join(t.TempDir(), "failures.ndjson"),
	})
	require.Contains(t, markdown, "TestA")
	require.NotContains(t, markdown, "<code>TestB</code>")
	require.Contains(t, markdown, "_... and 1 more failed tests omitted. Download the failures-only artifact for the full list._")
}

func TestWriteFailuresNDJSONAppliesCap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "failures.ndjson")
	failures := []failure{
		{Package: "example.com/pkg", Test: "TestA", Elapsed: 0.10, Output: strings.Repeat("a", 1000)},
		{Package: "example.com/pkg", Test: "TestB", Elapsed: 0.20, Output: "second"},
	}
	summaryLine, err := marshalRecord(truncationRecord{Truncated: true, RemainingFailures: 1})
	require.NoError(t, err)
	minimumLine, err := marshalRecord(failureRecord{
		Package:         failures[0].Package,
		Test:            failures[0].Test,
		ElapsedS:        failures[0].Elapsed,
		Output:          "",
		OutputTruncated: true,
	})
	require.NoError(t, err)
	capBytes := len(summaryLine) + len(minimumLine) + 20

	require.NoError(t, writeFailuresNDJSON(path, failures, capBytes))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.LessOrEqual(t, len(content), capBytes)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 2)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.Equal(t, true, first["output_truncated"])
	require.Equal(t, "TestA", first["test"])
	require.Less(t, len(first["output"].(string)), 1000)
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
	require.Equal(t, true, second["truncated"])
	require.Equal(t, float64(1), second["remaining_failures"])
}

func TestRunPackageLevelFailure(t *testing.T) {
	t.Parallel()

	jsonFile := writeEvents(t,
		testEvent{Action: "output", Package: "example.com/pkg", Output: "setup failed\n"},
		testEvent{Action: "fail", Package: "example.com/pkg", Elapsed: 0.30},
	)

	markdown := runMarkdown(t, jsonFile, config{MaxOutputBytes: 8192})
	require.Contains(t, markdown, "(package)")
	require.Contains(t, markdown, "setup failed")
}

func runMarkdown(t *testing.T, jsonFile string, cfg config) string {
	t.Helper()
	cfg.JSONFile = jsonFile
	cfg.MarkdownOut = "-"
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 8192
	}
	var stdout bytes.Buffer
	err := run(context.Background(), cfg, &stdout, ioDiscard{}, emptyEnv)
	require.NoError(t, err)
	return stdout.String()
}

func writeEvents(t *testing.T, events ...testEvent) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go-test.json")
	var content strings.Builder
	for _, event := range events {
		line, err := json.Marshal(event)
		require.NoError(t, err)
		_, _ = content.Write(line)
		_ = content.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(path, []byte(content.String()), 0o600))
	return path
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, string(content))
}

func emptyEnv(string) string { return "" }

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
