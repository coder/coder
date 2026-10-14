package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummarizeRun(t *testing.T) {
	t.Parallel()
	store, derived := t.TempDir(), t.TempDir()
	transcript, err := os.ReadFile(filepath.Join("testdata", "transcripts", "detached-bad.ndjson"))
	require.NoError(t, err)
	writeTrial := func(id, completion string, members ...tarEntry) {
		writeSummaryTrial(t, store, id, "case-a", "control", completion, members...)
	}
	writeTrial("case-a__control__r1", "DONE",
		tarEntry{hdr: tar.Header{Name: transcriptFile, Typeflag: tar.TypeReg, Mode: 0o644}, body: string(transcript)})
	writeTrial("case-a__control__r2", "AGENT_FAILED",
		tarEntry{hdr: tar.Header{Name: "stdout.log", Typeflag: tar.TypeReg, Mode: 0o644}, body: "starting\nerror: model returned 529\n"})
	// A failure line without the words error or fail still gives a reason.
	writeTrial("case-a__control__r4", "AGENT_FAILED",
		tarEntry{hdr: tar.Header{Name: "stdout.log", Typeflag: tar.TypeReg, Mode: 0o644}, body: "starting\nsidecar coderagents: get workspace: context deadline exceeded\n\n"})
	// The server log aegis appends to a failure is not the failure.
	writeTrial("case-a__control__r5", "AGENT_FAILED",
		tarEntry{hdr: tar.Header{Name: "stdout.log", Typeflag: tar.TypeReg, Mode: 0o644}, body: "starting\n" +
			"coderagents: coder server exited during startup (exit status 1)\nserver log tail:\n" +
			"Using built-in PostgreSQL (/tmp/coder/postgres)\nfailed to open telemetry\nStarted HTTP listener\n"})
	// Guardrails never decide task success, so the failed guardrail leaves
	// the graded trial successful.
	measured := filepath.Join(derived, "case-a__control__r1")
	require.NoError(t, os.MkdirAll(measured, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(measured, "measurements.json"), []byte(
		`{"graders":{"checks":{"passed":true},"hidden_tests":{"passed":true},"guardrails":{"passed":false,"guardrail":true}}}`), 0o600))
	// A trial that never landed a bundle is only in the journal; it must
	// still count as a failed trial with its reason. So must a trial the
	// journal lists as completed (or skipped on resume) whose bundle is
	// gone, here r6 with no directory and r7 with a regular file in its
	// slot.
	require.NoError(t, os.WriteFile(filepath.Join(store, "bundles", "exp", "case-a__control__r7"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(store, "bundles", "exp", "journal.json"), []byte(`{"entries":[`+
		`{"trial_id":"case-a__control__r1","status":"completed","attempts":1},`+
		`{"trial_id":"case-a__control__r2","status":"completed","attempts":1},`+
		`{"trial_id":"case-a__control__r3","status":"infra_failed","attempts":3,"error":"sandbox start timed out"},`+
		`{"trial_id":"case-a__control__r6","status":"completed","attempts":1},`+
		`{"trial_id":"case-a__control__r7","status":"skipped"}]}`), 0o600))

	s, err := summarizeRun(store, derived, "candidate")
	require.NoError(t, err)
	require.Equal(t, 7, s.Trials)
	require.Equal(t, map[string]string{
		"case-a__control__r3": "infra_failed: sandbox start timed out",
		"case-a__control__r6": "completed, but its bundle is missing",
		"case-a__control__r7": "skipped, but its bundle is missing",
	}, s.JournalOnly)
	require.Equal(t, 3, s.Ungraded)
	require.Len(t, s.Failures, 3)
	require.Equal(t, "error: model returned 529", s.Failures[0].FailReason)
	require.Equal(t, "sidecar coderagents: get workspace: context deadline exceeded", s.Failures[1].FailReason)
	require.Equal(t, "coderagents: coder server exited during startup (exit status 1)", s.Failures[2].FailReason)
	require.Equal(t, rate{Passed: 1, Total: 1, Rate: 1, Low: s.Overall.Low, High: 1}, s.Overall)
	require.Equal(t, 1, s.ByCase["case-a"].Passed)
	require.Equal(t, 0, s.Graders["guardrails"].Passed)
	require.InDelta(t, 1.0, s.CostUSD, 1e-9)

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

// A run with a candidate-prompt arm reports each case per arm, so a build
// comparison of two such runs never pools the arms.
func TestSummarizeRunKeepsArmsApart(t *testing.T) {
	t.Parallel()
	store, derived := t.TempDir(), t.TempDir()
	for _, arm := range []string{"control", "candidate-prompt"} {
		id := "case-a__" + arm + "__r1"
		writeSummaryTrial(t, store, id, "case-a", arm, "DONE")
		dir := filepath.Join(derived, id)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		passed := arm == "control"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "measurements.json"), []byte(
			`{"graders":{"checks":{"passed":`+strconv.FormatBool(passed)+`}}}`), 0o600))
	}
	s, err := summarizeRun(store, derived, "candidate")
	require.NoError(t, err)
	require.Equal(t, map[string]rate{
		"case-a [candidate-prompt]": newRate(0, 1),
		"case-a [control]":          newRate(1, 1),
	}, s.ByCase)

	// The bundle declares the arms: a candidate arm whose every trial
	// failed before landing still keys the control rows by arm.
	store, derived = t.TempDir(), t.TempDir()
	writeSummaryTrial(t, store, "case-a__control__r1", "case-a", "control", "DONE")
	dir := filepath.Join(derived, "case-a__control__r1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "measurements.json"), []byte(`{"graders":{"checks":{"passed":true}}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(store, "bundles", "exp", "journal.json"), []byte(`{"entries":[`+
		`{"trial_id":"case-a__control__r1","status":"completed"},`+
		`{"trial_id":"case-a__candidate-prompt__r1","status":"infra_failed","error":"sandbox start timed out"}]}`), 0o600))
	assets := filepath.Join(store, "bundle-assets", "0123abcd")
	require.NoError(t, os.MkdirAll(assets, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(assets, "eval_bundle.json"), []byte(
		`{"experiment":{"variants":[{"id":"control"},{"id":"candidate-prompt"}]}}`), 0o600))
	s, err = summarizeRun(store, derived, "candidate")
	require.NoError(t, err)
	require.Equal(t, map[string]rate{"case-a [control]": newRate(1, 1)}, s.ByCase)
}

// A journal that exists but cannot be parsed hides infrastructure
// failures, so the summary must fail instead of omitting them.
func TestSummarizeRunRejectsCorruptJournal(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeSummaryTrial(t, store, "case-a__control__r1", "case-a", "control", "DONE")
	require.NoError(t, os.WriteFile(filepath.Join(store, "bundles", "exp", "journal.json"), []byte(`{"entries":[`), 0o600))
	_, err := summarizeRun(store, "", "candidate")
	require.ErrorContains(t, err, "journal.json")
}

// A store directory that cannot be read hides trials, so the summary
// must fail, naming it, instead of reporting fewer trials.
func TestSummarizeRunRejectsUnreadableStore(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeSummaryTrial(t, store, "case-a__control__r1", "case-a", "control", "DONE")
	// A regular file where an experiment directory belongs fails to read
	// like a directory without permission, on every platform and as root.
	require.NoError(t, os.WriteFile(filepath.Join(store, "bundles", "exp2"), nil, 0o600))
	_, err := summarizeRun(store, "", "candidate")
	require.ErrorContains(t, err, "exp2")
}

func writeSummaryTrial(t *testing.T, store, id, task, variant, completion string, members ...tarEntry) {
	t.Helper()
	dir := filepath.Join(store, "bundles", "exp", id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	trial := `{"trial_id":"` + id + `","task_id":"` + task + `","variant_id":"` + variant + `",` +
		`"usage_totals":{"cost_usd":0.25},"outcome":{"completion":"` + completion + `","wall_clock_ms":1000}}`
	entries := append([]tarEntry{{hdr: tar.Header{Name: trialFile, Typeflag: tar.TypeReg, Mode: 0o644}, body: trial}}, members...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.tar.zst"), tarZst(t, entries), 0o600))
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
