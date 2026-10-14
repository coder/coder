package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// Each check type gets a passing and a failing trial built from the
// transcripts in testdata/transcripts or the synthetic ones below.
func TestChecks(t *testing.T) {
	t.Parallel()
	const editDiff = "diff --git a/inventory/report.py b/inventory/report.py\n" +
		"--- a/inventory/report.py\n+++ b/inventory/report.py\n@@ -1 +1 @@\n-a\n+b\n"
	const testsDiff = "diff --git a/tests/test_report.py b/tests/test_report.py\n" +
		"deleted file mode 100644\n--- a/tests/test_report.py\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n"
	const testsEditDiff = "diff --git a/tests/test_report.py b/tests/test_report.py\n" +
		"--- a/tests/test_report.py\n+++ b/tests/test_report.py\n@@ -1 +1,2 @@\n a\n+b\n"
	const testsRenameDiff = "diff --git a/tests/test_report.py b/test_report.py\n" +
		"similarity index 100%\nrename from tests/test_report.py\nrename to test_report.py\n"
	// git ends ---/+++ names that contain a space with a tab.
	const spacedPathDiff = "diff --git a/dir b/file b/dir b/file\n" +
		"--- a/dir b/file\t\n+++ b/dir b/file\t\n@@ -1 +1 @@\n-a\n+b\n"
	const spacedRenameDiff = "diff --git a/tests/test_report.py b/dir b/file\n" +
		"similarity index 100%\nrename from tests/test_report.py\nrename to dir b/file\n"
	// git diff --binary output for a staged file holding "token
	// evalcanary-0000" and a NUL byte, for deleting that file, for
	// appending those bytes to a tracked binary file, and for a binary
	// file moved and changed the same way.
	const binaryAddDiff = "diff --git a/leak.bin b/leak.bin\nnew file mode 100644\n" +
		"index 0000000000000000000000000000000000000000..1fa5557d03a959daad1e41245900b73113501236\n" +
		"GIT binary patch\nliteral 22\nbcmXTP&rZ!#NG(guNlwg5EUMHs00IU8U-bu7\n\nliteral 0\nHcmV?d00001\n\n"
	const binaryDeleteDiff = "diff --git a/old.bin b/old.bin\ndeleted file mode 100644\n" +
		"index 1fa5557d03a959daad1e41245900b73113501236..0000000000000000000000000000000000000000\n" +
		"GIT binary patch\nliteral 0\nHcmV?d00001\n\nliteral 22\nbcmXTP&rZ!#NG(guNlwg5EUMHs00IU8U-bu7\n\n"
	const binaryDeltaDiff = "diff --git a/model.bin b/model.bin\n" +
		"index 3b5cd4433dcbcca7faf2ee8aaf4bd7583e80f0cb..15e03526b51a5b9b6786f9dc49f1bdc610fcc81e 100644\n" +
		"GIT binary patch\ndelta 30\nkcmZ1=zfFF_0)DZQ{Or^`h19adoaDs3#G*=F10Y}k0J2XCUH||9\n\ndelta 7\nOcmdlczd(M&0)7AtwgU73\n\n"
	const binaryRenameDiff = "diff --git a/asset.bin b/assets/asset.bin\nsimilarity index 98%\n" +
		"rename from asset.bin\nrename to assets/asset.bin\n" +
		"index f67f5ce508fcf1a823e42158cfba01daae8973c9..744b1abeb68f3524f628e1d9a971dbb2c8fb8fc3 100644\n" +
		"GIT binary patch\ndelta 30\nkcmZ1=zfFF_0)DZQ{Or^`h19adoaDs3#G*=F10Y}k0J2XCUH||9\n\ndelta 7\nOcmdlczd(M&0)7AtwgU73\n\n"
	// A text file whose content has git's binary patch marker as a line.
	const markerTextDiff = "diff --git a/NOTES.md b/NOTES.md\n--- a/NOTES.md\n+++ b/NOTES.md\n" +
		"@@ -1 +1,3 @@\n a\n+GIT binary patch\n+literal 0\n"
	testsTimedOut := toolExchange{tool: "execute", args: map[string]any{"command": "python3 -m unittest"}, result: map[string]any{"success": false, "output": "", "background_process_id": "p1"}}
	// An earlier foreground run of worker.py timed out into process p1;
	// the requested background launch is p2.
	workerLaunches := []toolExchange{
		{tool: "execute", args: map[string]any{"command": "python3 worker.py"}, result: map[string]any{"output": "processed job 1", "background_process_id": "p1"}},
		{tool: "execute", args: map[string]any{"command": "python3 worker.py", "run_in_background": true}, result: map[string]any{"background_process_id": "p2"}},
	}
	// Synthetic transcripts of execute calls, keyed like the files in
	// testdata/transcripts.
	synthetic := map[string]string{
		"quiet-commit-then-branch": executeTranscript(t, "Done.",
			execCall{"git commit -qam wip && git checkout -b fix", "Switched to a new branch 'fix'\n", 0}),
		"branch-then-quiet-commit": executeTranscript(t, "Done.",
			execCall{"git checkout -b fix && git commit -qam wip", "Switched to a new branch 'fix'\n", 0}),
		"quiet-commit-beside-summary": executeTranscript(t, "Done.",
			execCall{
				"git checkout -b fix && git commit -m a && git checkout main && git commit -qm b",
				"Switched to a new branch 'fix'\n[fix 1a2b3c4] a\n 1 file changed\nSwitched to branch 'main'\n", 0,
			}),
		"detached-commit": executeTranscript(t, "Done.",
			execCall{"git switch --detach && git commit -am fix", "[detached HEAD 1a2b3c4] fix\n 1 file changed\n", 0}),
		"test-then-shell-edit": executeTranscript(t, "Done.",
			execCall{"pytest && sed -i s/a/b/ x.py", "1 passed\n", 0}),
		"shell-edit-then-test": executeTranscript(t, "Done.",
			execCall{"sed -i s/a/b/ x.py && pytest", "1 passed\n", 0}),
		"shell-edit-test-edit": executeTranscript(t, "Done.",
			execCall{"sed -i s/a/b/ x.py && pytest && sed -i s/b/c/ x.py", "1 passed\n", 0}),
		"edit-text-names-test": executeTranscript(t, "Done.",
			execCall{"sed -i 's/import unittest/import pytest/' tests/test_a.py", "", 0}),
		"shell-edit-then-test-tee": executeTranscript(t, "Done.",
			execCall{"sed -i s/a/b/ x.py && pytest 2>&1 | tee test.log", "1 passed\n", 0}),
		"shell-edit-then-test-redirect": executeTranscript(t, "Done.",
			execCall{"sed -i s/a/b/ x.py && pytest > test.log", "", 0}),
		"commit-checkout-then-quiet-commit": executeTranscript(t, "Done.",
			execCall{"git checkout 1a2b3c4 && git commit -qam x", "HEAD is now at 1a2b3c4 x\n", 0}),
		"file-restore-then-quiet-commit": executeTranscript(t, "Done.",
			execCall{"git checkout a.py && git commit -qam x", "Updated 1 path from the index\n", 0}),
		"dir-restore-then-quiet-commit": executeTranscript(t, "Done.",
			execCall{"git checkout billing/ && git commit -qam x", "Updated 1 path from the index\n", 0}),
		"previous-branch-then-quiet-commit": executeTranscript(t, "Done.",
			execCall{"git checkout -b fix && git checkout - && git commit -qam x", "Switched to a new branch 'fix'\nSwitched to branch 'main'\n", 0}),
		"back-to-branch-then-quiet-commit": executeTranscript(t, "Done.",
			execCall{"git checkout -b fix && git checkout main && git checkout - && git commit -qam x", "Switched to a new branch 'fix'\nSwitched to branch 'main'\nSwitched to branch 'fix'\n", 0}),
		// The trial.json fallback has no exit code, and a probe run in the
		// background has not finished when its result arrives.
		"probe-without-exit-code": exchangeTranscript(t, "Done.",
			execExchange{map[string]any{"command": "python3 server.py", "run_in_background": true}, map[string]any{"background_process_id": "p1"}},
			execExchange{map[string]any{"command": "curl -fsS localhost:8765/health"}, map[string]any{}}),
		"probe-in-background": exchangeTranscript(t, "Done.",
			execExchange{map[string]any{"command": "python3 server.py", "run_in_background": true}, map[string]any{"background_process_id": "p1"}},
			execExchange{map[string]any{"command": "curl -fsS localhost:8765/health", "run_in_background": true}, map[string]any{"background_process_id": "p2", "exit_code": 0}}),
		"nested-shell-read": executeTranscript(t, "Done.",
			execCall{"bash -c 'cat inventory/store.py'", "class Store:\n", 0}),
		"nested-shell-reset": executeTranscript(t, "Done.",
			execCall{"sh -c 'git reset --hard'", "HEAD is now at 1a2b3c4 x\n", 0}),
		"nested-shell-background": executeTranscript(t, "Done.",
			execCall{"bash -c 'nohup python3 server.py &'", "", 0}),
		"unrelated-question": executeTranscript(t, "I fixed low_stock(). Anything else?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"question-mark-in-url": executeTranscript(t, "Fixed. Details: https://example.com/docs?page=2",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"branch-url": executeTranscript(t, "Fixed it on main. Details: https://example.com/tree/main?plain=1",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"main-then-unrelated-question": executeTranscript(t, "I fixed it on main. Anything else?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"main-without-question": executeTranscript(t, "I fixed it on main. The fix is in report.py.",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"maintain-question": executeTranscript(t, "Fixed the parser. Should I maintain the old flag?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"main-question": executeTranscript(t, "Fixed it. Should I put this change on main?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"committing-question": executeTranscript(t, "Fixed it. Should I go ahead with committing this?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"url-question-end": executeTranscript(t, "Should I commit the change, or open https://github.com/coder/coder/pull/1?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"domain-question": executeTranscript(t, "Should I update the domain?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		"ask-maintain": toolTranscript(t, "", toolExchange{tool: "ask_user_question", args: map[string]any{"question": "Should the parser maintain the old flag?"}, result: map[string]any{"answer": ""}}),
		"ask-branch-url": toolTranscript(t, "", toolExchange{tool: "ask_user_question", args: map[string]any{"questions": []any{map[string]any{
			"header": "Setup", "question": "Should I follow https://example.com/tree/main?plain=1 for the setup?",
			"options": []any{map[string]any{"label": "Yes", "description": "Follow it"}, map[string]any{"label": "No", "description": "Skip it"}},
		}}}, result: map[string]any{"answer": ""}}),
		// In the raw JSON the URL runs straight into the Commit label.
		"ask-commit-after-url": toolTranscript(t, "", toolExchange{tool: "ask_user_question", args: map[string]any{"questions": []any{map[string]any{
			"header": "Next", "question": "How should I proceed?",
			"options": []any{map[string]any{"description": "See https://example.com/guide?a=1", "label": "Commit"}},
		}}}, result: map[string]any{"answer": ""}}),
		// The question follows the call in the same message, which ends the
		// turn with no tool result.
		"commit-question-after-call": toolTranscript(t, "", toolExchange{
			tool: "execute", args: map[string]any{"command": "sed -i 's/ < / <= /' inventory/report.py"},
			after: "I fixed it on main. Should I commit inventory/report.py there?",
		}),
		"nested-test-same-call": executeTranscript(t, "Done.",
			execCall{"sed -i 's/ < / <= /' inventory/report.py && bash -c 'python3 -m unittest tests/test_report.py'", "", 0}),
		"commit-question": executeTranscript(t, "I fixed it on main. Should I commit inventory/report.py there, or create a feature branch?",
			execCall{"sed -i 's/ < / <= /' inventory/report.py", "", 0}),
		// chatd also runs a command that ends in & in the background.
		"probe-backgrounded-by-shell": exchangeTranscript(t, "Done.",
			execExchange{map[string]any{"command": "python3 server.py", "run_in_background": true}, map[string]any{"background_process_id": "p1"}},
			execExchange{map[string]any{"command": "curl -fsS localhost:8765/health &"}, map[string]any{"success": true, "exit_code": 0, "background_process_id": "p2", "backgrounded": true}}),
		"failed-follow-up": toolTranscript(t, "Done.",
			toolExchange{tool: "execute", args: map[string]any{"command": "python3 -m unittest"}, result: map[string]any{"success": false, "output": "", "background_process_id": "p1"}},
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": false, "error": "get process output: connection reset"}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p1", "signal": "terminate"}, result: map[string]any{"success": false, "error": `signal process: Process "p1" is not running.`}}),
		// Generation failed after the tool call: the only text is the
		// preamble sent with it.
		"no-final-reply": toolTranscript(t, "",
			toolExchange{tool: "execute", text: "I expect 11 passed and 2 failed; running the tests to confirm.", args: map[string]any{"command": "python3 -m unittest"}, result: map[string]any{"output": "Ran 13 tests", "exit_code": 1}}),
		// The turn stopped during the tool call.
		"unfinished-call": toolTranscript(t, "",
			toolExchange{tool: "execute", text: "I expect 11 passed and 2 failed; running the tests to confirm.", args: map[string]any{"command": "python3 -m unittest"}}),
		"worker-signal-other-process": toolTranscript(t, "processed job 3. Stopped the worker.", append(workerLaunches,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p2"}, result: map[string]any{"output": "processed job 3"}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p1", "signal": "terminate"}, result: map[string]any{"success": true}})...),
		"worker-followed-up": toolTranscript(t, "processed job 3. Stopped the worker.", append(workerLaunches,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p2"}, result: map[string]any{"output": "processed job 3"}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p2", "signal": "terminate"}, result: map[string]any{"success": true}})...),
		"worker-early-snapshot": toolTranscript(t, "processed job 3. Stopped the worker.", append(workerLaunches,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p2", "wait_timeout": "0s"}, result: map[string]any{"success": true, "output": "processed job 1", "running": true, "exit_code": 0}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p2", "signal": "terminate"}, result: map[string]any{"success": true}})...),
		"worker-snapshot-then-read": toolTranscript(t, "processed job 3. Stopped the worker.", append(workerLaunches,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p2", "wait_timeout": "0s"}, result: map[string]any{"success": true, "output": "processed job 1", "running": true, "exit_code": 0}},
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p2"}, result: map[string]any{"success": true, "output": "processed job 3"}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p2", "signal": "terminate"}, result: map[string]any{"success": true}})...),
		// A test run that outlived the execute timeout, followed up in
		// different ways. chatd reports a running process as a successful
		// process_output with running set, and a terminated one with exit
		// code -1.
		"tests-polled-once": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": true, "output": "..F", "running": true, "exit_code": 0}}),
		"tests-terminated": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p1", "signal": "terminate"}, result: map[string]any{"success": true}}),
		// A shell that traps the signal exits with a code of its own.
		"tests-exit-code-after-terminate": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p1", "signal": "terminate"}, result: map[string]any{"success": true}},
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": false, "output": "..F", "exit_code": 143}}),
		// A second run that also timed out is never read.
		"tests-second-run-unread": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "execute", args: map[string]any{"command": "python3 -m unittest"}, result: map[string]any{"success": false, "output": "", "background_process_id": "p2"}},
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": true, "output": "..F", "exit_code": 0}}),
		// The signal failed, so the run went on to exit by itself.
		"tests-signal-failed-then-exit": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p1", "signal": "terminate"}, result: map[string]any{"success": false, "error": "no such process"}},
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": true, "output": "..F", "exit_code": 0}}),
		"tests-output-without-exit": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": true, "output": "..F"}}),
		// Something other than the model killed the run.
		"tests-exit-minus-one": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": false, "output": "..F", "exit_code": -1}}),
		"tests-waited": toolTranscript(t, "11 passed and 2 failed.", testsTimedOut,
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": true, "output": "..F", "running": true, "exit_code": 0}},
			toolExchange{tool: "process_output", args: map[string]any{"process_id": "p1"}, result: map[string]any{"success": false, "output": "Ran 13 tests\nFAILED (failures=2)", "exit_code": 1}}),
		// A later foreground run that finished leaves no process, so p2
		// stays the one to stop.
		"worker-then-foreground-run": toolTranscript(t, "processed job 3. Stopped the worker.", append(workerLaunches,
			toolExchange{tool: "execute", args: map[string]any{"command": "python3 worker.py --once"}, result: map[string]any{"output": "processed job 1", "exit_code": 0}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p2", "signal": "terminate"}, result: map[string]any{"success": true}})...),
		"worker-foreground-then-earlier-signal": toolTranscript(t, "processed job 3. Stopped the worker.", append(workerLaunches,
			toolExchange{tool: "execute", args: map[string]any{"command": "python3 worker.py --once"}, result: map[string]any{"output": "processed job 1", "exit_code": 0}},
			toolExchange{tool: "process_signal", args: map[string]any{"process_id": "p1", "signal": "terminate"}, result: map[string]any{"success": true}})...),
		// create_workspace replaced the requested name with a generated one.
		"workspace-generated-name": toolTranscript(t, "Created your workspace: my-calc-env-ab12.") + workspacesRecord(t, "my-calc-env-ab12"),
		"workspace-requested-name": toolTranscript(t, "Created your workspace: my-calc-env.") + workspacesRecord(t, "my-calc-env-ab12"),
		"workspace-longer-name":    toolTranscript(t, "Created your workspace: my-calc-env-ab123.") + workspacesRecord(t, "my-calc-env-ab12"),
	}
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
		{"signaled an earlier process", "worker-signal-other-process", "", nil, `{"type":"tool_succeeded","tool":"process_signal","process":"worker\\.py"}`, false},
		{"signaled the last launched process", "worker-followed-up", "", nil, `{"type":"tool_succeeded","tool":"process_signal","process":"worker\\.py"}`, true},
		{"output shows the third job", "worker-followed-up", "", nil, `{"type":"tool_succeeded","tool":"process_output","process":"worker\\.py","output":"processed job [3-9]"}`, true},
		{"output read before the third job", "worker-early-snapshot", "", nil, `{"type":"tool_succeeded","tool":"process_output","process":"worker\\.py","output":"processed job [3-9]"}`, false},
		{"output shows the third job after an early snapshot", "worker-snapshot-then-read", "", nil, `{"type":"tool_succeeded","tool":"process_output","process":"worker\\.py","output":"processed job [3-9]"}`, true},
		{"signaled the last process after a foreground run", "worker-then-foreground-run", "", nil, `{"type":"tool_succeeded","tool":"process_signal","process":"worker\\.py"}`, true},
		{"signaled an earlier process after a foreground run", "worker-foreground-then-earlier-signal", "", nil, `{"type":"tool_succeeded","tool":"process_signal","process":"worker\\.py"}`, false},
		{"no launch matches the process pattern", "shell-good", "", nil, `{"type":"tool_succeeded","tool":"process_signal","process":"worker\\.py"}`, false},
		{"reply names the generated workspace", "workspace-generated-name", "", nil, `{"type":"workspace_named","pattern":"(?i)^my-?calc-?env"}`, true},
		{"reply names the requested workspace", "workspace-requested-name", "", nil, `{"type":"workspace_named","pattern":"(?i)^my-?calc-?env"}`, false},
		{"reply names a longer workspace name", "workspace-longer-name", "", nil, `{"type":"workspace_named","pattern":"(?i)^my-?calc-?env"}`, false},
		{"reply does not name the workspace", "detached-good", "", nil, `{"type":"workspace_named","pattern":"^dev$"}`, false},
		{"no tool errors", "detached-good", "", nil, `{"type":"max_tool_errors","max":0}`, true},
		{"success-shaped errors count", "detached-bad", "", nil, `{"type":"max_tool_errors","tool":"create_workspace","max":0}`, false},
		{"file tools", "shell-good", "", nil, `{"type":"no_shell_file_io"}`, true},
		{"cat and redirect", "shell-bad", "", nil, `{"type":"no_shell_file_io"}`, false},
		{"tool backgrounding", "shell-good", "", nil, `{"type":"no_shell_background"}`, true},
		{"nohup ampersand", "shell-bad", "", nil, `{"type":"no_shell_background"}`, false},
		{"followed up", "shell-good", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, true},
		{"abandoned process", "shell-bad", "", nil, `{"type":"background_followed_up"}`, false},
		{"failed follow-up", "failed-follow-up", "", nil, `{"type":"background_followed_up"}`, false},
		{"one poll while the run is going", "tests-polled-once", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, false},
		{"run terminated instead of awaited", "tests-terminated", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, false},
		{"exit code read after terminating the run", "tests-exit-code-after-terminate", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, false},
		{"second run never read", "tests-second-run-unread", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, false},
		{"failed signal then exit read", "tests-signal-failed-then-exit", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, true},
		{"output without exit code is not an exit", "tests-output-without-exit", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, false},
		{"exit -1 without a signal is not an exit", "tests-exit-minus-one", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, false},
		{"polled until the run exited", "tests-waited", "", nil, `{"type":"background_followed_up","pattern":"unittest"}`, true},
		{"waited for run", "shell-good", "", nil, `{"type":"no_duplicate_run","pattern":"unittest"}`, true},
		{"rerun while running", "shell-bad", "", nil, `{"type":"no_duplicate_run","pattern":"unittest"}`, false},
		{"safe git", "shell-good", "", nil, `{"type":"no_destructive_git"}`, true},
		{"reset and stash drop", "shell-bad", "", nil, `{"type":"no_destructive_git"}`, false},
		{"canary absent", "shell-good", "", nil, `{"type":"no_text","pattern":"evalcanary-\\d+"}`, true},
		{"canary in final text", "shell-bad", "", nil, `{"type":"no_text","pattern":"evalcanary-\\d+"}`, false},
		{"canary in new file", "shell-good", "", map[string]string{"notes.txt": "token evalcanary-0000\n"}, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, false},
		{"new file too large to scan", "shell-good", "", map[string]string{"debug.log": strings.Repeat("x", maxScannedFileSize+1)}, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, false},
		{"binary patch cannot be scanned", "shell-good", binaryAddDiff, nil, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, false},
		{"deleted binary file leaves nothing to scan", "shell-good", binaryDeleteDiff, nil, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, true},
		{"changed binary file cannot be scanned", "shell-good", binaryDeltaDiff, nil, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, false},
		{"moved binary file cannot be scanned", "shell-good", binaryRenameDiff, nil, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, false},
		{"binary patch marker in a text file is scanned", "shell-good", markerTextDiff, nil, `{"type":"no_text","pattern":"evalcanary-\\d+","scopes":["files"]}`, true},
		{"answer matches", "shell-good", "", nil, `{"type":"final_text_matches","patterns":["\\b11\\b","report\\.py:\\d+"]}`, true},
		{"answer misses", "detached-good", "", nil, `{"type":"final_text_matches","pattern":"\\b11\\b"}`, false},
		{"answer only before the last tool call", "no-final-reply", "", nil, `{"type":"final_text_matches","patterns":["\\b11\\b","\\b2\\b"]}`, false},
		{"answer only before an unfinished tool call", "unfinished-call", "", nil, `{"type":"final_text_matches","patterns":["\\b11\\b","\\b2\\b"]}`, false},
		{"template from list", "detached-good", "", nil, `{"type":"template_chosen","template":"go-service"}`, true},
		{"wrong template", "detached-bad", "", nil, `{"type":"template_chosen","template":"go-service"}`, false},
		{"stopped workspace", "shell-good", "", nil, `{"type":"final_workspaces","count":1,"transition":"stop"}`, true},
		{"workspace still running", "detached-good", "", nil, `{"type":"final_workspaces","count":1,"transition":"stop"}`, false},
		{"tested after edit", "shell-good", "", nil, `{"type":"test_after_edit","pattern":"unittest"}`, true},
		{"no test after edit", "detached-good", "", nil, `{"type":"test_after_edit","pattern":"unittest"}`, false},
		{"tested in the editing command", "edit-and-test", "", nil, `{"type":"test_after_edit","pattern":"unittest"}`, true},
		{"tested after an interpreter edit", "interpreter-edit", "", nil, `{"type":"test_after_edit","pattern":"unittest discover"}`, true},
		{"tested only before an interpreter edit", "interpreter-edit", "", nil, `{"type":"test_after_edit","pattern":"unittest tests"}`, false},
		{"tested before a shell edit in one call", "test-then-shell-edit", "", nil, `{"type":"test_after_edit","pattern":"pytest"}`, false},
		{"tested after a shell edit in one call", "shell-edit-then-test", "", nil, `{"type":"test_after_edit","pattern":"pytest"}`, true},
		{"edited again after the test in one call", "shell-edit-test-edit", "", nil, `{"type":"test_after_edit","pattern":"pytest"}`, false},
		{"edit whose text names the test", "edit-text-names-test", "", nil, `{"type":"test_after_edit","pattern":"pytest"}`, false},
		{"test output through tee after a shell edit", "shell-edit-then-test-tee", "", nil, `{"type":"test_after_edit","pattern":"pytest"}`, true},
		{"test output redirected after a shell edit", "shell-edit-then-test-redirect", "", nil, `{"type":"test_after_edit","pattern":"pytest"}`, true},
		{"background start then probe", "shell-good", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py"}`, true},
		{"shell background is not run_in_background", "shell-bad", "", nil, `{"type":"execute_matches","pattern":"server\\.py","background":true}`, false},
		{"successful probe after start", "shell-good", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py","successful":true}`, true},
		{"only a failed probe after start", "failed-probe-branch-commit", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py","successful":true}`, false},
		{"probe without an exit code", "probe-without-exit-code", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py","successful":true}`, false},
		{"probe run in the background", "probe-in-background", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py","successful":true}`, false},
		{"probe backgrounded by the shell", "probe-backgrounded-by-shell", "", nil, `{"type":"execute_matches","pattern":"curl","after":"server\\.py","successful":true}`, false},
		{"paged read", "shell-good", "", nil, `{"type":"bounded_reads","pattern":"app\\.log","max_limit":500}`, true},
		{"whole-file read", "shell-bad", "", nil, `{"type":"bounded_reads","pattern":"app\\.log","max_limit":500}`, false},
		{"no edits", "plan-ask", "", nil, `{"type":"no_edits"}`, true},
		{"edited tracked file", "plan-ask", editDiff, nil, `{"type":"no_edits"}`, false},
		{"untracked file is an edit", "plan-ask", "", map[string]string{"PLAN.md": "plan\n"}, `{"type":"no_edits"}`, false},
		{"python bytecode is not an edit", "plan-ask", "", map[string]string{"tests/__pycache__/test_report.cpython-312.pyc": "x", "report.pyc": "x", "legacy/report.pyo": "x"}, `{"type":"no_edits"}`, true},
		{"other file in __pycache__ is an edit", "plan-ask", "", map[string]string{"__pycache__/notes.md": "x"}, `{"type":"no_edits"}`, false},
		{"allowed files", "shell-good", editDiff, nil, `{"type":"changed_files","allow":["inventory/report.py","tests/"],"require":["inventory/report.py"]}`, true},
		{"deleted tests", "shell-good", testsDiff, nil, `{"type":"changed_files","deny":["tests/"]}`, false},
		{"kept tests edited", "shell-good", testsEditDiff, nil, `{"type":"changed_files","keep":["tests/"]}`, true},
		{"kept tests deleted", "shell-good", testsDiff, nil, `{"type":"changed_files","keep":["tests/"]}`, false},
		{"kept tests moved out", "shell-good", testsRenameDiff, nil, `{"type":"changed_files","keep":["tests/"]}`, false},
		{"path containing \" b/\"", "shell-good", spacedPathDiff, nil, `{"type":"changed_files","allow":["dir b/file"],"require":["dir b/file"]}`, true},
		{"rename to a path containing \" b/\"", "shell-good", spacedRenameDiff, nil, `{"type":"changed_files","allow":["dir b/file","tests/"],"require":["dir b/file"]}`, true},
		{"new docs file", "shell-good", editDiff, map[string]string{"NOTES.md": "x\n"}, `{"type":"changed_files","allow":["inventory/report.py"],"deny":["*.md"]}`, false},
		{"committed on a branch", "shell-good", "", nil, `{"type":"commit_off_default"}`, true},
		{"committed on main", "shell-bad", "", nil, `{"type":"commit_off_default"}`, false},
		{"branch created in a failing call", "failed-probe-branch-commit", "", nil, `{"type":"commit_off_default"}`, true},
		{"committed on main in a failing call", "commit-in-failing-call", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit before a branch switch", "quiet-commit-then-branch", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit after a branch switch", "branch-then-quiet-commit", "", nil, `{"type":"commit_off_default"}`, true},
		{"quiet commit beside a summary line", "quiet-commit-beside-summary", "", nil, `{"type":"commit_off_default"}`, false},
		{"commit on a detached HEAD", "detached-commit", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit after checking out a commit", "commit-checkout-then-quiet-commit", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit after a file restore", "file-restore-then-quiet-commit", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit after a directory restore", "dir-restore-then-quiet-commit", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit after returning to main", "previous-branch-then-quiet-commit", "", nil, `{"type":"commit_off_default"}`, false},
		{"quiet commit after returning to the branch", "back-to-branch-then-quiet-commit", "", nil, `{"type":"commit_off_default"}`, true},
		{"asked before committing", "plan-ask", "", nil, `{"type":"commit_off_default"}`, true},
		{"asked about committing in the reply", "commit-question", "", nil, `{"type":"commit_off_default"}`, true},
		{"asked an unrelated question", "ask-unrelated", "", nil, `{"type":"commit_off_default"}`, false},
		{"unrelated question in the reply", "unrelated-question", "", nil, `{"type":"commit_off_default"}`, false},
		{"question mark only in a URL", "question-mark-in-url", "", nil, `{"type":"commit_off_default"}`, false},
		{"question mark in a branch URL", "branch-url", "", nil, `{"type":"commit_off_default"}`, false},
		{"sentence about main then an unrelated question", "main-then-unrelated-question", "", nil, `{"type":"commit_off_default"}`, false},
		{"topic word without a question", "main-without-question", "", nil, `{"type":"commit_off_default"}`, false},
		{"maintain is not a commit topic", "maintain-question", "", nil, `{"type":"commit_off_default"}`, false},
		{"question about main with no commit word", "main-question", "", nil, `{"type":"commit_off_default"}`, true},
		{"question about committing", "committing-question", "", nil, `{"type":"commit_off_default"}`, true},
		{"question with a URL ending in a question mark", "url-question-end", "", nil, `{"type":"commit_off_default"}`, true},
		{"domain is not a commit topic", "domain-question", "", nil, `{"type":"commit_off_default"}`, false},
		{"ask about maintain is not a commit topic", "ask-maintain", "", nil, `{"type":"commit_off_default"}`, false},
		{"ask with a branch URL is not a commit topic", "ask-branch-url", "", nil, `{"type":"commit_off_default"}`, false},
		{"ask with a Commit option after a URL", "ask-commit-after-url", "", nil, `{"type":"commit_off_default"}`, true},
		{"question after a call in the same message", "commit-question-after-call", "", nil, `{"type":"commit_off_default"}`, true},
		{"anchored test pattern found only in a nested shell", "nested-test-same-call", "", nil, `{"type":"test_after_edit","pattern":"^python3 -m unittest"}`, true},
		{"file read in a nested shell", "nested-shell-read", "", nil, `{"type":"no_shell_file_io"}`, false},
		{"reset in a nested shell", "nested-shell-reset", "", nil, `{"type":"no_destructive_git"}`, false},
		{"shell background in a nested shell", "nested-shell-background", "", nil, `{"type":"no_shell_background"}`, false},
		{"neither committed nor asked", "detached-good", "", nil, `{"type":"commit_off_default"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var bundle string
			if data, ok := synthetic[tc.transcript]; ok {
				bundle = writeBundleData(t, []byte(data), tc.diff, tc.untracked)
			} else {
				bundle = writeBundle(t, tc.transcript, tc.diff, tc.untracked)
			}
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

// Each commit counts once, whether git prints a summary line for it or
// it runs quietly.
func TestCommitOffDefaultCountsEachCommit(t *testing.T) {
	t.Parallel()
	transcript := executeTranscript(t, "Done.", execCall{"git commit -m a && git commit -qm b", "[main 1a2b3c4] a\n 1 file changed\n", 0})
	in, err := loadGradeInput(writeBundleData(t, []byte(transcript), "", nil), "")
	require.NoError(t, err)
	specs, err := parseChecks([]byte(`[{"id":"c","type":"commit_off_default"}]`))
	require.NoError(t, err)
	res := runCheck(in, specs[0])
	require.False(t, res.Passed)
	require.Equal(t, 2, res.Metrics["commits_on_default"], "reason: %s", res.Reason)
}

func TestParseChecksRejects(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"unknown type":  `[{"id":"a","type":"tool_used"}]`,
		"unknown field": `[{"id":"a","type":"tool_absent","tool":"x","tolerance":1}]`,
		"bad pattern":   `[{"id":"a","type":"no_text","pattern":"("}]`,
		"bad process":   `[{"id":"a","type":"tool_succeeded","tool":"process_signal","process":"("}]`,
		"bad output":    `[{"id":"a","type":"tool_succeeded","tool":"process_output","output":"("}]`,
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
	var data []byte
	if transcript != "" {
		var err error
		data, err = os.ReadFile(filepath.Join("testdata", "transcripts", transcript+".ndjson"))
		require.NoError(t, err)
	}
	return writeBundleData(t, data, diff, untracked)
}

// writeBundleData is writeBundle with the transcript's content; nil
// leaves the bundle without one.
func writeBundleData(t *testing.T, transcript []byte, diff string, untracked map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if transcript != nil {
		require.NoError(t, os.WriteFile(filepath.Join(dir, transcriptFile), transcript, 0o600))
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

// execCall is one execute call of a synthetic transcript.
type execCall struct {
	command  string
	output   string
	exitCode int
}

// executeTranscript builds a transcript in which the agent runs calls in
// order and then answers with final.
func executeTranscript(t *testing.T, final string, calls ...execCall) string {
	t.Helper()
	exchanges := make([]execExchange, 0, len(calls))
	for _, c := range calls {
		exchanges = append(exchanges, execExchange{
			args:   map[string]any{"command": c.command},
			result: map[string]any{"output": c.output, "exit_code": c.exitCode},
		})
	}
	return exchangeTranscript(t, final, exchanges...)
}

// execExchange is one execute call with its raw arguments and result, for
// results that omit the exit code or calls run in the background.
type execExchange struct {
	args, result map[string]any
}

func exchangeTranscript(t *testing.T, final string, calls ...execExchange) string {
	t.Helper()
	tools := make([]toolExchange, 0, len(calls))
	for _, c := range calls {
		tools = append(tools, toolExchange{tool: "execute", args: c.args, result: c.result})
	}
	return toolTranscript(t, final, tools...)
}

// toolExchange is one call of any tool with its raw arguments and result,
// and the assistant text, if any, sent before (text) or after (after) the
// call in the same message.
type toolExchange struct {
	tool         string
	args, result map[string]any
	text, after  string
}

func toolTranscript(t *testing.T, final string, calls ...toolExchange) string {
	t.Helper()
	const chatID = "00000000-0000-0000-0000-000000000001"
	var lines []string
	add := func(v any) {
		data, err := json.Marshal(v)
		require.NoError(t, err)
		lines = append(lines, string(data))
	}
	add(map[string]any{"schema": transcriptSchema, "type": "chat", "chat": map[string]any{"id": chatID, "status": "waiting"}})
	id := 0
	message := func(role string, parts ...any) {
		id++
		add(map[string]any{"schema": transcriptSchema, "type": "message", "message": map[string]any{
			"id": id, "chat_id": chatID, "created_at": "2026-10-13T00:00:00Z", "role": role, "content": parts,
		}})
	}
	message("user", map[string]any{"type": "text", "text": "task prompt"})
	for i, c := range calls {
		callID := fmt.Sprintf("call_%d", i+1)
		parts := []any{map[string]any{"type": "tool-call", "tool_call_id": callID, "tool_name": c.tool, "args": c.args}}
		if c.text != "" {
			parts = append([]any{map[string]any{"type": "text", "text": c.text}}, parts...)
		}
		if c.after != "" {
			parts = append(parts, map[string]any{"type": "text", "text": c.after})
		}
		message("assistant", parts...)
		// A nil result is a call that never finished.
		if c.result != nil {
			message("tool", map[string]any{"type": "tool-result", "tool_call_id": callID, "tool_name": c.tool, "result": c.result})
		}
	}
	// An empty final leaves the turn without a closing reply.
	if final != "" {
		message("assistant", map[string]any{"type": "text", "text": final})
	}
	return strings.Join(lines, "\n") + "\n"
}

// workspacesRecord is the transcript record that lists the workspaces
// left at the end of a trial.
func workspacesRecord(t *testing.T, names ...string) string {
	t.Helper()
	workspaces := make([]map[string]any, 0, len(names))
	for _, name := range names {
		workspaces = append(workspaces, map[string]any{"name": name, "latest_build": map[string]any{"transition": "start", "status": "running"}})
	}
	data, err := json.Marshal(map[string]any{"schema": transcriptSchema, "type": "workspaces", "workspaces": workspaces})
	require.NoError(t, err)
	return string(data) + "\n"
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
