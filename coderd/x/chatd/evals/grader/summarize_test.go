package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummarizeRun(t *testing.T) {
	t.Parallel()
	store, derived := t.TempDir(), t.TempDir()
	transcript, err := os.ReadFile(filepath.Join("testdata", "transcripts", "detached-bad.ndjson"))
	require.NoError(t, err)
	writeTrial := func(id, completion string, members ...tarEntry) {
		dir := filepath.Join(store, "bundles", "exp", id)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		trial := `{"trial_id":"` + id + `","task_id":"case-a","variant_id":"control",` +
			`"usage_totals":{"cost_usd":0.25},"outcome":{"completion":"` + completion + `","wall_clock_ms":1000}}`
		entries := append([]tarEntry{{hdr: tar.Header{Name: trialFile, Typeflag: tar.TypeReg, Mode: 0o644}, body: trial}}, members...)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.tar.zst"), tarZst(t, entries), 0o600))
	}
	writeTrial("case-a__control__r1", "DONE",
		tarEntry{hdr: tar.Header{Name: transcriptFile, Typeflag: tar.TypeReg, Mode: 0o644}, body: string(transcript)})
	writeTrial("case-a__control__r2", "AGENT_FAILED",
		tarEntry{hdr: tar.Header{Name: "stdout.log", Typeflag: tar.TypeReg, Mode: 0o644}, body: "starting\nerror: model returned 529\n"})
	// Guardrails never decide task success, so the failed guardrail leaves
	// the graded trial successful.
	measured := filepath.Join(derived, "case-a__control__r1")
	require.NoError(t, os.MkdirAll(measured, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(measured, "measurements.json"), []byte(
		`{"graders":{"checks":{"passed":true},"hidden_tests":{"passed":true},"guardrails":{"passed":false,"guardrail":true}}}`), 0o600))

	s, err := summarizeRun(store, derived, "candidate")
	require.NoError(t, err)
	require.Equal(t, 2, s.Trials)
	require.Equal(t, 1, s.Ungraded)
	require.Len(t, s.Failures, 1)
	require.Equal(t, "error: model returned 529", s.Failures[0].FailReason)
	require.Equal(t, rate{Passed: 1, Total: 1, Rate: 1, Low: s.Overall.Low, High: 1}, s.Overall)
	require.Equal(t, 1, s.ByCase["case-a"].Passed)
	require.Equal(t, 0, s.Graders["guardrails"].Passed)
	require.InDelta(t, 0.5, s.CostUSD, 1e-9)

	var create toolRow
	for _, row := range s.Tools {
		if row.Tool == "create_workspace" {
			create = row
		}
	}
	require.Equal(t, 2, create.Calls)
	require.Equal(t, 1, create.Failures, "the success-shaped build failure counts")
	require.Equal(t, 1, create.Kinds["error"])
}

func TestWilson(t *testing.T) {
	t.Parallel()
	low, high := wilson(5, 10)
	require.InDelta(t, 0.2366, low, 1e-4)
	require.InDelta(t, 0.7634, high, 1e-4)
	low, high = wilson(1, 1)
	require.InDelta(t, 0.2065, low, 1e-4)
	require.InDelta(t, 1.0, high, 1e-9)
}
