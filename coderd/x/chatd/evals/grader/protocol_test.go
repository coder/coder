package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func gradeRequest(t *testing.T, apiVersion, bundle, checks string) string {
	t.Helper()
	req := map[string]any{
		"api_version": apiVersion,
		"task":        map[string]any{"id": "case"},
		"trial":       map[string]any{"id": "case__control__r1", "experiment_id": "exp", "variant_id": "control"},
		"paths":       map[string]any{"bundle_dir": bundle},
		"grader":      map[string]any{"id": "checks", "type": "subprocess", "args": map[string]any{"mode": "checks", "checks": checks}},
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	return string(data)
}

func TestGradeProtocol(t *testing.T) {
	t.Parallel()
	bundle := writeBundle(t, "detached-bad", "", nil)
	checks := `[{"id":"created","type":"tool_succeeded","tool":"create_workspace"},` +
		`{"id":"clean","type":"max_tool_errors","max":0,"observe":true},` +
		`{"id":"listed","type":"tool_count","tool":"list_templates","min":1}]`

	t.Run("verdict", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		code := run(nil, strings.NewReader(gradeRequest(t, graderAPIVersion, bundle, checks)), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		var v graderVerdict
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &v))
		require.False(t, v.Passed)
		require.Contains(t, v.Reason, "1/2 required checks passed")
		require.Contains(t, v.Reason, "listed:")
		require.Contains(t, v.Reason, "observe-only misses: clean")
		require.EqualValues(t, 0, v.Metrics["check.clean"])
		require.EqualValues(t, 2, v.Metrics["tool.create_workspace.calls"])
		require.EqualValues(t, 1, v.Metrics["tool.create_workspace.failures"])
	})
	t.Run("observe-only checks do not fail the verdict", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		only := `[{"id":"created","type":"tool_succeeded","tool":"create_workspace"},{"id":"clean","type":"max_tool_errors","max":0,"observe":true}]`
		require.Equal(t, 0, run(nil, strings.NewReader(gradeRequest(t, graderAPIVersion, bundle, only)), &stdout, &stderr), stderr.String())
		var v graderVerdict
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &v))
		require.True(t, v.Passed)
		require.EqualValues(t, 1, v.Score)
	})
	t.Run("other api versions are rejected", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 2, run(nil, strings.NewReader(gradeRequest(t, "aegis.grader.v2", bundle, checks)), &stdout, &stderr))
		require.Empty(t, stdout.String())
	})
	t.Run("invalid checks are a protocol error", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 2, run(nil, strings.NewReader(gradeRequest(t, graderAPIVersion, bundle, `[{"id":"x","type":"nope"}]`)), &stdout, &stderr))
		require.Empty(t, stdout.String())
	})
	t.Run("missing evidence fails the trial", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(nil, strings.NewReader(gradeRequest(t, graderAPIVersion, t.TempDir(), checks)), &stdout, &stderr), stderr.String())
		var v graderVerdict
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &v))
		require.False(t, v.Passed)
		require.Contains(t, v.Reason, "cannot load trial evidence")
	})
}

func TestTrialJSONFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	trial := `{"tool_calls":[{"id":"1","name":"list_templates","input":{}},{"id":"2","name":"create_workspace","input":{"template_id":"x"},"result":{"is_error":true}}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, trialFile), []byte(trial), 0o600))
	tr, err := loadTranscript(dir, "")
	require.NoError(t, err)
	require.Equal(t, sourceTrialJSON, tr.Source)
	require.Len(t, tr.Calls, 2)
	require.False(t, tr.Calls[1].succeeded())
	require.NotEmpty(t, tr.Notes)
}

func TestToolCallFailure(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		result  string
		isError bool
		want    string
	}{
		"is_error":                {`"boom"`, true, "is_error"},
		"error key":               {`{"error":"workspace build failed"}`, false, "error"},
		"empty error key":         {`{"error":"","output":"ok"}`, false, ""},
		"agent not ready":         {`{"created":true,"agent_status":"not_ready"}`, false, "agent_status:not_ready"},
		"agent selection error":   {`{"agent_status":"selection_error"}`, false, "agent_status:selection_error"},
		"startup scripts failed":  {`{"startup_scripts":"startup_scripts_failed"}`, false, "startup_scripts:startup_scripts_failed"},
		"startup scripts timeout": {`{"startup_scripts":"startup_scripts_timeout"}`, false, "startup_scripts:startup_scripts_timeout"},
		"plain success":           {`{"created":true,"agent_status":"ready"}`, false, ""},
		"non-json result":         {`"done"`, false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := toolCall{Name: "create_workspace", Result: json.RawMessage(tc.result), HasResult: true, IsError: tc.isError}
			require.Equal(t, tc.want, c.failure())
		})
	}
	require.Equal(t, "no_result", toolCall{Name: "execute"}.failure())
}

func TestExtractRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		entry tarEntry
		want  string
	}{
		"parent traversal":   {tarEntry{hdr: tar.Header{Name: "../escape.txt", Typeflag: tar.TypeReg}, body: "x"}, "unsafe archive entry"},
		"nested traversal":   {tarEntry{hdr: tar.Header{Name: "a/../../escape.txt", Typeflag: tar.TypeReg}, body: "x"}, "unsafe archive entry"},
		"absolute path":      {tarEntry{hdr: tar.Header{Name: "/etc/passwd", Typeflag: tar.TypeReg}, body: "x"}, "unsafe archive entry"},
		"workspace git":      {tarEntry{hdr: tar.Header{Name: ".git/config", Typeflag: tar.TypeReg}, body: "x"}, "writes the workspace .git"},
		"escaping symlink":   {tarEntry{hdr: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}}, "escapes the workspace"},
		"absolute symlink":   {tarEntry{hdr: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc"}}, "escapes the workspace"},
		"hardlink":           {tarEntry{hdr: tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: "README.md"}}, "unsupported type"},
		"device":             {tarEntry{hdr: tar.Header{Name: "dev", Typeflag: tar.TypeChar}}, "unsupported type"},
		"oversized declared": {tarEntry{hdr: tar.Header{Name: "big", Typeflag: tar.TypeReg, Size: maxArchiveEntry + 1}}, "cap"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			archive := filepath.Join(t.TempDir(), untrackedFile)
			var data []byte
			if tc.entry.hdr.Size > 0 {
				data = rawTarZst(t, tc.entry.hdr)
			} else {
				data = tarZst(t, []tarEntry{tc.entry})
			}
			require.NoError(t, os.WriteFile(archive, data, 0o600))
			require.ErrorContains(t, extractTarZst(archive, root), tc.want)
			_, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
	t.Run("write through an extracted symlink", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		archive := filepath.Join(t.TempDir(), untrackedFile)
		require.NoError(t, os.WriteFile(archive, tarZst(t, []tarEntry{
			{hdr: tar.Header{Name: "dir", Typeflag: tar.TypeSymlink, Linkname: "."}},
			{hdr: tar.Header{Name: "dir/file.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "x"},
		}), 0o600))
		require.ErrorContains(t, extractTarZst(archive, root), "non-directory")
	})
}

// rawTarZst writes only a header that declares more bytes than the cap, so
// the test does not have to allocate them.
func rawTarZst(t *testing.T, hdr tar.Header) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	require.NoError(t, tw.WriteHeader(&hdr))
	return compressZst(t, tarBuf.Bytes())
}

func TestRehydrate(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	seed := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(seed, "calc.py"), []byte("def add(a, b):\n    return a - b\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(seed, "data.bin"), []byte{0, 1, 2, 3}, 0o600))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "seed"}} {
		out, err := runGit(ctx, seed, args...)
		require.NoError(t, err, string(out))
	}
	work := t.TempDir()
	out, err := runGit(ctx, work, "clone", "-q", seed, "tree")
	require.NoError(t, err, string(out))
	tree := filepath.Join(work, "tree")

	// Produce the bundle the way aegis does: git diff --binary against the
	// baseline plus the untracked files.
	edit := t.TempDir()
	out, err = runGit(ctx, edit, "clone", "-q", seed, "e")
	require.NoError(t, err, string(out))
	e := filepath.Join(edit, "e")
	require.NoError(t, os.WriteFile(filepath.Join(e, "calc.py"), []byte("def add(a, b):\n    return a + b\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(e, "data.bin"), []byte{9, 8, 7}, 0o600))
	diff, err := runGit(ctx, e, "diff", "--binary", "HEAD")
	require.NoError(t, err, string(diff))
	bundle := writeBundle(t, "", string(diff), map[string]string{"pkg/new.py": "VALUE = 1\n"})

	require.NoError(t, rehydrate(ctx, tree, bundle))
	got, err := os.ReadFile(filepath.Join(tree, "calc.py"))
	require.NoError(t, err)
	require.Contains(t, string(got), "a + b")
	got, err = os.ReadFile(filepath.Join(tree, "data.bin"))
	require.NoError(t, err)
	require.Equal(t, []byte{9, 8, 7}, got)
	got, err = os.ReadFile(filepath.Join(tree, "pkg", "new.py"))
	require.NoError(t, err)
	require.Equal(t, "VALUE = 1\n", string(got))
}
