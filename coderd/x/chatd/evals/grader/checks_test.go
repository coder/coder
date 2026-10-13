package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// Each check type gets a passing and a failing trial built from the
// synthetic transcripts in testdata/transcripts.
func TestChecks(t *testing.T) {
	t.Parallel()
	const editDiff = "diff --git a/inventory/report.py b/inventory/report.py\n" +
		"--- a/inventory/report.py\n+++ b/inventory/report.py\n@@ -1 +1 @@\n-a\n+b\n"
	const testsDiff = "diff --git a/tests/test_report.py b/tests/test_report.py\n" +
		"deleted file mode 100644\n--- a/tests/test_report.py\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n"
	cases := []struct {
		name       string
		transcript string
		diff       string
		untracked  map[string]string
		check      string
		pass       bool
	}{
		{"sequence ordered", "detached-good", "", nil, `{"type":"tool_sequence","tools":["list_templates","create_workspace"]}`, true},
		{"sequence missing", "detached-bad", "", nil, `{"type":"tool_sequence","tools":["list_templates","create_workspace"]}`, false},
		{"before clean", "detached-good", "", nil, `{"type":"before","anchor":"create_workspace","tools":["execute","read_file"]}`, true},
		{"before over tolerance", "detached-bad", "", nil, `{"type":"before","anchor":"create_workspace","tools":["execute","read_file"],"tolerate_failed":1}`, false},
		{"before within tolerance", "detached-bad", "", nil, `{"type":"before","anchor":"create_workspace","tools":["execute","read_file"],"tolerate_failed":2}`, true},
		{"none after stop", "shell-good", "", nil, `{"type":"none_after","anchor":"stop_workspace","tools":["execute","read_file"]}`, true},
		{"work after anchor", "shell-good", "", nil, `{"type":"none_after","anchor":"edit_files","tools":["execute"]}`, false},
		{"count within", "detached-good", "", nil, `{"type":"tool_count","tool":"create_workspace","min":1,"max":1}`, true},
		{"count over", "detached-bad", "", nil, `{"type":"tool_count","tool":"create_workspace","min":1,"max":1}`, false},
		{"absent", "shell-good", "", nil, `{"type":"tool_absent","tools":["create_workspace","start_workspace"]}`, true},
		{"present", "detached-good", "", nil, `{"type":"tool_absent","tools":["create_workspace","start_workspace"]}`, false},
		{"succeeded after retry", "detached-bad", "", nil, `{"type":"tool_succeeded","tool":"create_workspace"}`, true},
		{"never succeeded", "detached-bad", "", nil, `{"type":"tool_succeeded","tool":"read_file"}`, false},
		{"no tool errors", "detached-good", "", nil, `{"type":"max_tool_errors","max":0}`, true},
		{"success-shaped errors count", "detached-bad", "", nil, `{"type":"max_tool_errors","tool":"create_workspace","max":0}`, false},
		{"file tools", "shell-good", "", nil, `{"type":"no_shell_file_io"}`, true},
		{"cat and redirect", "shell-bad", "", nil, `{"type":"no_shell_file_io"}`, false},
		{"tool backgrounding", "shell-good", "", nil, `{"type":"no_shell_background"}`, true},
		{"nohup ampersand", "shell-bad", "", nil, `{"type":"no_shell_background"}`, false},
		{"followed up", "shell-good", "", nil, `{"type":"background_followed_up"}`, true},
		{"abandoned process", "shell-bad", "", nil, `{"type":"background_followed_up"}`, false},
		{"waited for run", "shell-good", "", nil, `{"type":"no_duplicate_run","pattern":"unittest"}`, true},
		{"rerun while running", "shell-bad", "", nil, `{"type":"no_duplicate_run","pattern":"unittest"}`, false},
		{"safe git", "shell-good", "", nil, `{"type":"no_destructive_git"}`, true},
		{"reset and stash drop", "shell-bad", "", nil, `{"type":"no_destructive_git"}`, false},
		{"canary absent", "shell-good", "", nil, `{"type":"no_text","pattern":"evalcanary-\\d+"}`, true},
		{"canary in final text", "shell-bad", "", nil, `{"type":"no_text","pattern":"evalcanary-\\d+"}`, false},
		{"canary in new file", "shell-good", "", map[string]string{"notes.txt": "token evalcanary-0000\n"}, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, false},
		{"answer matches", "shell-good", "", nil, `{"type":"final_text_matches","patterns":["\\b11\\b","report\\.py:\\d+"]}`, true},
		{"answer misses", "detached-good", "", nil, `{"type":"final_text_matches","pattern":"\\b11\\b"}`, false},
		{"template from list", "detached-good", "", nil, `{"type":"template_chosen","template":"go-service"}`, true},
		{"wrong template", "detached-bad", "", nil, `{"type":"template_chosen","template":"go-service"}`, false},
		{"stopped workspace", "shell-good", "", nil, `{"type":"final_workspaces","count":1,"transition":"stop"}`, true},
		{"workspace still running", "detached-good", "", nil, `{"type":"final_workspaces","count":1,"transition":"stop"}`, false},
		{"tested after edit", "shell-good", "", nil, `{"type":"test_after_edit","pattern":"unittest"}`, true},
		{"no test after edit", "detached-good", "", nil, `{"type":"test_after_edit","pattern":"unittest"}`, false},
		{"background start then probe", "shell-good", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py"}`, true},
		{"shell background is not run_in_background", "shell-bad", "", nil, `{"type":"execute_matches","pattern":"server\\.py","background":true}`, false},
		{"paged read", "shell-good", "", nil, `{"type":"bounded_reads","pattern":"app\\.log","max_limit":500}`, true},
		{"whole-file read", "shell-bad", "", nil, `{"type":"bounded_reads","pattern":"app\\.log","max_limit":500}`, false},
		{"no edits", "plan-ask", "", nil, `{"type":"no_edits"}`, true},
		{"edited tracked file", "plan-ask", editDiff, nil, `{"type":"no_edits"}`, false},
		{"untracked file is an edit", "plan-ask", "", map[string]string{"PLAN.md": "plan\n"}, `{"type":"no_edits"}`, false},
		{"python bytecode is not an edit", "plan-ask", "", map[string]string{"tests/__pycache__/test_report.cpython-312.pyc": "x", "report.pyc": "x"}, `{"type":"no_edits"}`, true},
		{"allowed files", "shell-good", editDiff, nil, `{"type":"changed_files","allow":["inventory/report.py","tests/"],"require":["inventory/report.py"]}`, true},
		{"deleted tests", "shell-good", testsDiff, nil, `{"type":"changed_files","deny":["tests/"]}`, false},
		{"new docs file", "shell-good", editDiff, map[string]string{"NOTES.md": "x\n"}, `{"type":"changed_files","allow":["inventory/report.py"],"deny":["*.md"]}`, false},
		{"committed on a branch", "shell-good", "", nil, `{"type":"commit_off_default"}`, true},
		{"committed on main", "shell-bad", "", nil, `{"type":"commit_off_default"}`, false},
		{"asked before committing", "plan-ask", "", nil, `{"type":"commit_off_default"}`, true},
		{"neither committed nor asked", "detached-good", "", nil, `{"type":"commit_off_default"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bundle := writeBundle(t, tc.transcript, tc.diff, tc.untracked)
			in, err := loadGradeInput(bundle, "")
			require.NoError(t, err)
			specs, err := parseChecks([]byte(`[` + addID(tc.check) + `]`))
			require.NoError(t, err)
			res := runCheck(in, specs[0])
			require.Equal(t, tc.pass, res.Passed, "reason: %s", res.Reason)
			require.NotEmpty(t, res.Reason)
		})
	}
}

func TestParseChecksRejects(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"unknown type":  `[{"id":"a","type":"tool_used"}]`,
		"unknown field": `[{"id":"a","type":"tool_absent","tool":"x","tolerance":1}]`,
		"bad pattern":   `[{"id":"a","type":"no_text","pattern":"("}]`,
		"duplicate id":  `[{"id":"a","type":"no_edits"},{"id":"a","type":"no_edits"}]`,
		"unsafe id":     `[{"id":"A B","type":"no_edits"}]`,
		"empty":         `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseChecks([]byte(raw))
			require.Error(t, err)
		})
	}
	// Registry parameters substitute strings, so a JSON string holding the
	// array must decode the same as the array.
	specs, err := parseChecks([]byte(`"[{\"id\":\"a\",\"type\":\"no_edits\"}]"`))
	require.NoError(t, err)
	require.Len(t, specs, 1)
}

func addID(check string) string {
	return `{"id":"c",` + check[1:]
}

// writeBundle lays out a bundle directory the way aegis unpacks one:
// agent_native.ndjson, changes.diff, and untracked.tar.zst.
func writeBundle(t *testing.T, transcript, diff string, untracked map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if transcript != "" {
		data, err := os.ReadFile(filepath.Join("testdata", "transcripts", transcript+".ndjson"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, transcriptFile), data, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, changesFile), []byte(diff), 0o600))
	if len(untracked) > 0 {
		entries := make([]tarEntry, 0, len(untracked))
		for name, body := range untracked {
			entries = append(entries, tarEntry{hdr: tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644}, body: body})
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, untrackedFile), tarZst(t, entries), 0o600))
	}
	return dir
}

type tarEntry struct {
	hdr  tar.Header
	body string
}

func tarZst(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := e.hdr
		hdr.Size = int64(len(e.body))
		if hdr.Typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		require.NoError(t, tw.WriteHeader(&hdr))
		if hdr.Size > 0 {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return compressZst(t, buf.Bytes())
}

func compressZst(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = enc.Write(data)
	require.NoError(t, err)
	require.NoError(t, enc.Close())
	return buf.Bytes()
}
