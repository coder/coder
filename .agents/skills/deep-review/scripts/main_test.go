package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- add-finding tests ---

func TestAddFinding_BasicP2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "findings.json")

	err := cmdAddFinding([]string{
		"--output", output,
		"--severity", "P2",
		"--file", "handler.go",
		"--line", "42",
		"--summary", "Missing nil check",
		"--evidence", "The pointer is dereferenced without checking",
		"--reviewer", "test-auditor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var findings []Finding
	data, _ := os.ReadFile(output)
	if err := json.Unmarshal(data, &findings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Severity != P2 {
		t.Errorf("severity = %q, want P2", f.Severity)
	}
	if f.Summary != "Missing nil check" {
		t.Errorf("summary = %q", f.Summary)
	}
	if f.Reviewer != "test-auditor" {
		t.Errorf("reviewer = %q", f.Reviewer)
	}
	if f.File == nil || *f.File != "handler.go" {
		t.Errorf("file = %v", f.File)
	}
	if f.Line == nil || *f.Line != 42 {
		t.Errorf("line = %v", f.Line)
	}
}

func TestAddFinding_AppendsToExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "findings.json")

	for i := 0; i < 3; i++ {
		err := cmdAddFinding([]string{
			"--output", output,
			"--severity", "P3",
			"--file", "handler.go",
			"--line", "10",
			"--summary", "Finding",
			"--evidence", "Evidence",
			"--reviewer", "reviewer",
		})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}

	var findings []Finding
	data, _ := os.ReadFile(output)
	if err := json.Unmarshal(data, &findings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(findings))
	}
}

func TestAddFinding_InvalidSeverity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "findings.json")

	err := cmdAddFinding([]string{
		"--output", output,
		"--severity", "CRITICAL",
		"--summary", "Bad",
		"--reviewer", "test",
	})
	if err == nil {
		t.Fatal("expected error for invalid severity")
	}
	if !strings.Contains(err.Error(), "invalid severity") {
		t.Errorf("error = %q, want 'invalid severity'", err)
	}
}

func TestAddFinding_PLevelRequiresFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "findings.json")

	// Missing --file.
	err := cmdAddFinding([]string{
		"--output", output,
		"--severity", "P1",
		"--line", "10",
		"--summary", "Finding",
		"--evidence", "Evidence",
		"--reviewer", "test",
	})
	if err == nil || !strings.Contains(err.Error(), "--file is required") {
		t.Errorf("expected --file required error, got: %v", err)
	}

	// Missing --evidence.
	err = cmdAddFinding([]string{
		"--output", output,
		"--severity", "P1",
		"--file", "f.go",
		"--line", "10",
		"--summary", "Finding",
		"--reviewer", "test",
	})
	if err == nil || !strings.Contains(err.Error(), "--evidence is required") {
		t.Errorf("expected --evidence required error, got: %v", err)
	}
}

func TestAddFinding_ObsNoLineRequired(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "findings.json")

	err := cmdAddFinding([]string{
		"--output", output,
		"--severity", "Obs",
		"--summary", "Good pattern here",
		"--reviewer", "test-auditor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var findings []Finding
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &findings)
	if findings[0].File != nil {
		t.Error("expected nil file for Obs")
	}
	if findings[0].Line != nil {
		t.Error("expected nil line for Obs")
	}
}

func TestAddFinding_InvalidLineNumber(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "findings.json")

	err := cmdAddFinding([]string{
		"--output", output,
		"--severity", "P2",
		"--file", "f.go",
		"--line", "notanumber",
		"--summary", "Finding",
		"--evidence", "Evidence",
		"--reviewer", "test",
	})
	if err == nil || !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("expected positive integer error, got: %v", err)
	}
}

func TestAddFinding_LineExceedsFileLength(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a 5-line file.
	testFile := filepath.Join(dir, "short.go")
	os.WriteFile(testFile, []byte("1\n2\n3\n4\n5\n"), 0o644)

	output := filepath.Join(dir, "findings.json")

	// Capture stderr for the warning.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := cmdAddFinding([]string{
		"--output", output,
		"--severity", "P2",
		"--file", testFile,
		"--line", "20",
		"--summary", "Finding",
		"--evidence", "Evidence",
		"--reviewer", "test",
	})

	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read stderr output.
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	stderr := string(buf[:n])

	if !strings.Contains(stderr, "Warning") || !strings.Contains(stderr, "exceeds") {
		t.Errorf("expected warning about line exceeding file length, got: %q", stderr)
	}

	// Finding should still be written.
	var findings []Finding
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &findings)
	if len(findings) != 1 {
		t.Fatal("finding should still be written despite warning")
	}
}

// --- compile-findings tests ---

func TestCompileFindings_GroupsConvergent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Two reviewers find issues at the same location.
	writeJSON(t, filepath.Join(dir, "auditor.json"), []Finding{
		{Severity: P2, File: ptr("handler.go"), Line: ptr(42),
			Summary: "Missing check", Evidence: ptr("Evidence A"), Reviewer: "test-auditor"},
	})
	writeJSON(t, filepath.Join(dir, "security.json"), []Finding{
		{Severity: P1, File: ptr("handler.go"), Line: ptr(42),
			Summary: "Auth bypass", Evidence: ptr("Evidence B"), Reviewer: "security-reviewer"},
	})

	output := filepath.Join(dir, "output.json")
	err := cmdCompileFindings([]string{"--dir", dir, "--output", output})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result CompiledOutput
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &result)

	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 grouped finding, got %d", len(result.Findings))
	}

	cf := result.Findings[0]
	if cf.MaxSeverity != P1 {
		t.Errorf("max_severity = %q, want P1", cf.MaxSeverity)
	}
	if !cf.Convergent {
		t.Error("expected convergent = true")
	}
	if len(cf.Reviewers) != 2 {
		t.Errorf("expected 2 reviewers, got %d", len(cf.Reviewers))
	}
	// Summary should come from the highest-severity reviewer.
	if cf.Summary != "Auth bypass" {
		t.Errorf("summary = %q, want 'Auth bypass'", cf.Summary)
	}
}

func TestCompileFindings_SeverityStats(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeJSON(t, filepath.Join(dir, "r1.json"), []Finding{
		{Severity: P2, File: ptr("a.go"), Line: ptr(1),
			Summary: "A", Evidence: ptr("E"), Reviewer: "r1"},
		{Severity: Obs, Summary: "B", Reviewer: "r1"},
	})
	writeJSON(t, filepath.Join(dir, "r2.json"), []Finding{
		{Severity: Nit, File: ptr("b.go"), Line: ptr(5),
			Summary: "C", Reviewer: "r2"},
	})

	output := filepath.Join(dir, "output.json")
	err := cmdCompileFindings([]string{"--dir", dir, "--output", output})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result CompiledOutput
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &result)

	if result.Stats.TotalFindings != 3 {
		t.Errorf("total = %d, want 3", result.Stats.TotalFindings)
	}
	if result.Stats.BySeverity["P2"] != 1 {
		t.Errorf("P2 count = %d, want 1", result.Stats.BySeverity["P2"])
	}
	if result.Stats.BySeverity["Obs"] != 1 {
		t.Errorf("Obs count = %d, want 1", result.Stats.BySeverity["Obs"])
	}
	if result.Stats.BySeverity["Nit"] != 1 {
		t.Errorf("Nit count = %d, want 1", result.Stats.BySeverity["Nit"])
	}
}

func TestCompileFindings_SkipsNonArrayJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// An object, not an array — should be skipped.
	os.WriteFile(filepath.Join(dir, "context.json"), []byte(`{"pr": {}}`), 0o644)
	writeJSON(t, filepath.Join(dir, "r1.json"), []Finding{
		{Severity: P3, File: ptr("a.go"), Line: ptr(1),
			Summary: "A", Evidence: ptr("E"), Reviewer: "r1"},
	})

	output := filepath.Join(dir, "output.json")
	err := cmdCompileFindings([]string{"--dir", dir, "--output", output})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result CompiledOutput
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &result)

	if result.Stats.TotalFindings != 1 {
		t.Errorf("total = %d, want 1 (should skip non-array)", result.Stats.TotalFindings)
	}
}

// --- build-review tests ---

func TestBuildReview_InitCreatesValidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "review.json")

	err := cmdBuildReview([]string{"init", "--output", output, "--body", "Good PR. 1 P2 across 1 comment."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var review Review
	data, _ := os.ReadFile(output)
	if err := json.Unmarshal(data, &review); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if review.Event != "COMMENT" {
		t.Errorf("event = %q, want COMMENT", review.Event)
	}
	if review.Body != "Good PR. 1 P2 across 1 comment." {
		t.Errorf("body = %q", review.Body)
	}
	if len(review.Comments) != 0 {
		t.Error("expected empty comments")
	}
}

func TestBuildReview_InitErrorsOnExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "review.json")

	// Create the file first.
	os.WriteFile(output, []byte("{}"), 0o644)

	err := cmdBuildReview([]string{"init", "--output", output, "--body", "text"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestBuildReview_CommentAppendsCorrectly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "review.json")

	cmdBuildReview([]string{"init", "--output", output, "--body", "Summary"})
	err := cmdBuildReview([]string{"comment", "--output", output,
		"--path", "handler.go", "--line", "42",
		"--body", "**P2** Missing nil check *(Test Auditor)*"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var review Review
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &review)

	if len(review.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(review.Comments))
	}
	c := review.Comments[0]
	if c.Path != "handler.go" || c.Line != 42 {
		t.Errorf("comment = %+v", c)
	}
}

func TestBuildReview_MultipleCommentsAccumulate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "review.json")

	cmdBuildReview([]string{"init", "--output", output, "--body", "Summary"})
	for i := 1; i <= 3; i++ {
		cmdBuildReview([]string{"comment", "--output", output,
			"--path", "f.go", "--line", "10",
			"--body", "Comment"})
	}

	var review Review
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &review)
	if len(review.Comments) != 3 {
		t.Errorf("expected 3 comments, got %d", len(review.Comments))
	}
}

func TestBuildReview_ReplyAndResolve(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "review.json")

	cmdBuildReview([]string{"init", "--output", output, "--body", "Summary"})

	err := cmdBuildReview([]string{"reply", "--output", output,
		"--in-reply-to", "456", "--body", "Acknowledged."})
	if err != nil {
		t.Fatalf("reply error: %v", err)
	}

	err = cmdBuildReview([]string{"resolve", "--output", output,
		"--thread-id", "PRT_abc123"})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	var review Review
	data, _ := os.ReadFile(output)
	json.Unmarshal(data, &review)

	if len(review.Replies) != 1 || review.Replies[0].InReplyToID != 456 {
		t.Errorf("replies = %+v", review.Replies)
	}
	if len(review.ResolveThreadIDs) != 1 || review.ResolveThreadIDs[0] != "PRT_abc123" {
		t.Errorf("resolve_thread_ids = %+v", review.ResolveThreadIDs)
	}
}

func TestBuildReview_CommentOnUninitializedFileErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "review.json")

	err := cmdBuildReview([]string{"comment", "--output", output,
		"--path", "f.go", "--line", "10", "--body", "text"})
	if err == nil || !strings.Contains(err.Error(), "build-review init") {
		t.Errorf("expected init hint, got: %v", err)
	}
}

// --- post-review tests ---

func TestPostReview_DryRunPayload(t *testing.T) {
	dir := t.TempDir()

	// Build a review.
	reviewFile := filepath.Join(dir, "review.json")
	review := Review{
		Event: "COMMENT",
		Body:  "LGTM with one nit.",
		Comments: []ReviewComment{
			{Path: "handler.go", Line: 42, Body: "**Nit** Rename this"},
		},
	}
	writeJSON(t, reviewFile, review)

	output := captureStdout(t, func() {
		err := cmdPostReview([]string{
			"--input", reviewFile,
			"--pr", "100",
			"--repo", "test/repo",
			"--dry-run",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// Should contain the endpoint.
	if !strings.Contains(output, "repos/test/repo/pulls/100/reviews") {
		t.Errorf("output missing endpoint, got:\n%s", output)
	}
	// Should contain line + side in payload.
	if !strings.Contains(output, `"line": 42`) {
		t.Errorf("output missing line, got:\n%s", output)
	}
	if !strings.Contains(output, `"side": "RIGHT"`) {
		t.Errorf("output missing side, got:\n%s", output)
	}
}

func TestPostReview_ValidationErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Missing body.
	reviewFile := filepath.Join(dir, "review.json")
	writeJSON(t, reviewFile, Review{Event: "COMMENT", Body: ""})

	err := cmdPostReview([]string{
		"--input", reviewFile, "--pr", "1", "--repo", "o/r", "--dry-run",
	})
	if err == nil || !strings.Contains(err.Error(), "body is required") {
		t.Errorf("expected body required error, got: %v", err)
	}

	// Wrong event.
	writeJSON(t, reviewFile, Review{Event: "APPROVE", Body: "ok"})
	err = cmdPostReview([]string{
		"--input", reviewFile, "--pr", "1", "--repo", "o/r", "--dry-run",
	})
	if err == nil || !strings.Contains(err.Error(), "COMMENT") {
		t.Errorf("expected COMMENT error, got: %v", err)
	}
}

func TestPostReview_FileLevelComment(t *testing.T) {
	dir := t.TempDir()

	// Comment with line=0 should become file-level.
	reviewFile := filepath.Join(dir, "review.json")
	review := Review{
		Event: "COMMENT",
		Body:  "Summary",
		Comments: []ReviewComment{
			{Path: "handler.go", Line: 0, Body: "File-level comment"},
		},
	}
	writeJSON(t, reviewFile, review)

	output := captureStdout(t, func() {
		err := cmdPostReview([]string{
			"--input", reviewFile, "--pr", "1", "--repo", "o/r", "--dry-run",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"subject_type": "file"`) {
		t.Errorf("expected subject_type file, got:\n%s", output)
	}
}

// --- fetch-context tests (assembly/filtering only) ---

func TestFilterResolvedThreads(t *testing.T) {
	t.Parallel()

	comments := json.RawMessage(`[
		{"id": 300, "in_reply_to_id": null, "body": "Unresolved finding"},
		{"id": 301, "in_reply_to_id": 300, "body": "Reply to unresolved"},
		{"id": 400, "in_reply_to_id": null, "body": "Resolved finding"},
		{"id": 401, "in_reply_to_id": 400, "body": "Reply to resolved"}
	]`)

	threads := json.RawMessage(`[
		{"id": "PRT_300", "isResolved": false, "comments": {"nodes": [{"databaseId": 300}]}},
		{"id": "PRT_400", "isResolved": true, "comments": {"nodes": [{"databaseId": 400}]}}
	]`)

	result, err := filterResolvedThreads(comments, threads)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var filtered []map[string]interface{}
	json.Unmarshal(result, &filtered)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 comments (unresolved), got %d", len(filtered))
	}

	// Check IDs — should be 300 and 301.
	ids := []int{}
	for _, c := range filtered {
		ids = append(ids, jsonInt(c, "id"))
	}
	if ids[0] != 300 || ids[1] != 301 {
		t.Errorf("expected [300, 301], got %v", ids)
	}

	// Check thread_id was added.
	tid, ok := filtered[0]["thread_id"]
	if !ok || tid != "PRT_300" {
		t.Errorf("thread_id = %v, want PRT_300", tid)
	}

	// Reply should also get the thread_id.
	tid2, ok := filtered[1]["thread_id"]
	if !ok || tid2 != "PRT_300" {
		t.Errorf("reply thread_id = %v, want PRT_300", tid2)
	}
}

func TestFilterResolvedThreads_AllResolved(t *testing.T) {
	t.Parallel()

	comments := json.RawMessage(`[
		{"id": 100, "in_reply_to_id": null, "body": "Finding"}
	]`)
	threads := json.RawMessage(`[
		{"id": "PRT_100", "isResolved": true, "comments": {"nodes": [{"databaseId": 100}]}}
	]`)

	result, err := filterResolvedThreads(comments, threads)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var filtered []map[string]interface{}
	json.Unmarshal(result, &filtered)

	if len(filtered) != 0 {
		t.Errorf("expected 0 comments, got %d", len(filtered))
	}
}

// --- helpers ---

// captureStdout captures stdout during fn execution and returns
// the captured output as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = oldStdout

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return string(data)
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshaling JSON: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
