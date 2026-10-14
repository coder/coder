package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// checkSpec is one entry of the grader.args.checks DSL, mirrored by the
// Check type in shared.ts. Decoding rejects unknown fields.
type checkSpec struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Observe bool   `json:"observe,omitempty"`

	Tool           string   `json:"tool,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	Anchor         string   `json:"anchor,omitempty"`
	TolerateFailed int      `json:"tolerate_failed,omitempty"`
	Min            *int     `json:"min,omitempty"`
	Max            *int     `json:"max,omitempty"`
	Successful     bool     `json:"successful,omitempty"`
	Pattern        string   `json:"pattern,omitempty"`
	Patterns       []string `json:"patterns,omitempty"`
	After          string   `json:"after,omitempty"`
	Background     bool     `json:"background,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
	Template       string   `json:"template,omitempty"`
	Count          *int     `json:"count,omitempty"`
	Transition     string   `json:"transition,omitempty"`
	Allow          []string `json:"allow,omitempty"`
	Deny           []string `json:"deny,omitempty"`
	Require        []string `json:"require,omitempty"`
	Keep           []string `json:"keep,omitempty"`
	Branches       []string `json:"branches,omitempty"`
	MaxLimit       int      `json:"max_limit,omitempty"`
	Process        string   `json:"process,omitempty"`
	Output         string   `json:"output,omitempty"`
	ExitCode       *int     `json:"exit_code,omitempty"`
	Path           string   `json:"path,omitempty"`
}

type checkResult struct {
	ID      string
	Observe bool
	Passed  bool
	Reason  string
	Metrics map[string]any
}

var safeCheckID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// parseChecks accepts the checks argument as a JSON array or as a string
// holding one, since registry parameters substitute strings.
func parseChecks(raw json.RawMessage) ([]checkSpec, error) {
	if len(raw) == 0 {
		return nil, xerrors.New("grader args have no checks")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, xerrors.Errorf("decode checks string: %w", err)
		}
		trimmed = []byte(s)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var specs []checkSpec
	if err := dec.Decode(&specs); err != nil {
		return nil, xerrors.Errorf("decode checks: %w", err)
	}
	if len(specs) == 0 {
		return nil, xerrors.New("checks list is empty")
	}
	seen := map[string]bool{}
	for _, s := range specs {
		if !safeCheckID.MatchString(s.ID) {
			return nil, xerrors.Errorf("check id %q must match %s", s.ID, safeCheckID)
		}
		if seen[s.ID] {
			return nil, xerrors.Errorf("duplicate check id %q", s.ID)
		}
		seen[s.ID] = true
		if _, ok := checkFuncs[s.Type]; !ok {
			return nil, xerrors.Errorf("check %q has unknown type %q", s.ID, s.Type)
		}
		fields := [][2]string{{"pattern", s.Pattern}, {"after", s.After}, {"process", s.Process}, {"output", s.Output}, {"path", s.Path}}
		for i, p := range s.Patterns {
			fields = append(fields, [2]string{fmt.Sprintf("patterns[%d]", i), p})
		}
		for _, f := range fields {
			if f[1] == "" {
				continue
			}
			if _, err := regexp.Compile(f[1]); err != nil {
				return nil, xerrors.Errorf("check %q %s %q: %w", s.ID, f[0], f[1], err)
			}
		}
	}
	return specs, nil
}

type checkFunc func(in *gradeInput, s checkSpec) (bool, string, map[string]any)

var checkFuncs map[string]checkFunc

func init() {
	checkFuncs = map[string]checkFunc{
		"tool_sequence":          checkToolSequence,
		"before":                 checkBefore,
		"none_after":             checkNoneAfter,
		"tool_count":             checkToolCount,
		"tool_absent":            checkToolAbsent,
		"tool_succeeded":         checkToolSucceeded,
		"max_tool_errors":        checkMaxToolErrors,
		"no_shell_file_io":       checkNoShellFileIO,
		"no_shell_background":    checkNoShellBackground,
		"background_followed_up": checkBackgroundFollowedUp,
		"no_duplicate_run":       checkNoDuplicateRun,
		"no_destructive_git":     checkNoDestructiveGit,
		"no_text":                checkNoText,
		"final_text_matches":     checkFinalTextMatches,
		"template_chosen":        checkTemplateChosen,
		"final_workspaces":       checkFinalWorkspaces,
		"workspace_named":        checkWorkspaceNamed,
		"test_after_edit":        checkTestAfterEdit,
		"execute_matches":        checkExecuteMatches,
		"bounded_reads":          checkBoundedReads,
		"no_edits":               checkNoEdits,
		"changed_files":          checkChangedFiles,
		"commit_off_default":     checkCommitOffDefault,
	}
}

func runCheck(in *gradeInput, s checkSpec) checkResult {
	ok, reason, metrics := checkFuncs[s.Type](in, s)
	if ok && reason == "" {
		reason = "ok"
	}
	return checkResult{ID: s.ID, Observe: s.Observe, Passed: ok, Reason: reason, Metrics: metrics}
}

func toolSet(s checkSpec) map[string]bool {
	set := map[string]bool{}
	if s.Tool != "" {
		set[s.Tool] = true
	}
	for _, t := range s.Tools {
		set[t] = true
	}
	return set
}

// firstIndex returns the index of the first call to name that ok accepts,
// or -1.
func firstIndex(calls []toolCall, name string, ok func(toolCall) bool) int {
	for i, c := range calls {
		if c.Name == name && ok(c) {
			return i
		}
	}
	return -1
}

func anyCall(toolCall) bool { return true }

func successfulCall(c toolCall) bool { return c.succeeded() }

func checkToolSequence(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	match := anyCall
	if s.Successful {
		match = successfulCall
	}
	prev := -1
	for _, name := range s.Tools {
		idx := firstIndex(in.tr.Calls, name, match)
		if idx < 0 {
			return false, name + " never called", nil
		}
		if idx <= prev {
			return false, fmt.Sprintf("%s first called at #%d, before the preceding tool at #%d", name, idx, prev), nil
		}
		prev = idx
	}
	return true, "", nil
}

// checkBefore fails when a tool from Tools ran before the first
// successful Anchor call, tolerating up to TolerateFailed failed calls.
func checkBefore(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	anchor := firstIndex(in.tr.Calls, s.Anchor, successfulCall)
	end := len(in.tr.Calls)
	if anchor >= 0 {
		end = anchor
	}
	var succeeded, failed []string
	for _, c := range in.tr.Calls[:end] {
		if !tools[c.Name] {
			continue
		}
		if c.succeeded() {
			succeeded = append(succeeded, c.Name)
		} else {
			failed = append(failed, c.Name)
		}
	}
	metrics := map[string]any{"before_calls": len(succeeded) + len(failed), "before_failed": len(failed)}
	where := "before the first successful " + s.Anchor
	if anchor < 0 {
		where = "with no successful " + s.Anchor
	}
	if len(succeeded) > 0 {
		return false, fmt.Sprintf("%d successful call(s) %s (%s)", len(succeeded), where, strings.Join(succeeded, ", ")), metrics
	}
	if len(failed) > s.TolerateFailed {
		return false, fmt.Sprintf("%d failed call(s) %s, %d tolerated (%s)", len(failed), where, s.TolerateFailed, strings.Join(failed, ", ")), metrics
	}
	return true, "", metrics
}

// checkNoneAfter fails when a tool from Tools ran after the first
// successful Anchor call.
func checkNoneAfter(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	anchor := firstIndex(in.tr.Calls, s.Anchor, successfulCall)
	if anchor < 0 {
		return false, s.Anchor + " never succeeded", nil
	}
	tools := toolSet(s)
	var after []string
	for _, c := range in.tr.Calls[anchor+1:] {
		if tools[c.Name] {
			after = append(after, c.Name)
		}
	}
	if len(after) > 0 {
		return false, fmt.Sprintf("%d call(s) after %s: %s", len(after), s.Anchor, strings.Join(after, ", ")), map[string]any{"after_calls": len(after)}
	}
	return true, "", map[string]any{"after_calls": 0}
}

// pathFilter accepts the calls whose arguments name a file path matching
// Path (any call when it is empty): path, as read_file and write_file
// take it, or the path of an entry of files, as edit_files takes them.
// Paths are matched with . elements and repeated slashes removed, so
// inventory/./store.py names inventory/store.py. It also returns the
// words that describe the filter in a reason.
func pathFilter(s checkSpec) (func(toolCall) bool, string) {
	if s.Path == "" {
		return anyCall, ""
	}
	re := regexp.MustCompile(s.Path)
	return func(c toolCall) bool {
		var args struct {
			Path  string `json:"path"`
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		}
		if err := json.Unmarshal(c.Args, &args); err != nil {
			return false
		}
		paths := []string{args.Path}
		for _, f := range args.Files {
			paths = append(paths, f.Path)
		}
		return slices.ContainsFunc(paths, func(p string) bool {
			return p != "" && re.MatchString(path.Clean(p))
		})
	}, " on a path that matches " + s.Path
}

func checkToolCount(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	onPath, where := pathFilter(s)
	n := 0
	for _, c := range in.tr.Calls {
		if tools[c.Name] && (!s.Successful || c.succeeded()) && onPath(c) {
			n++
		}
	}
	metrics := map[string]any{"count": n}
	if s.Min != nil && n < *s.Min {
		return false, fmt.Sprintf("%d call(s)%s, want at least %d", n, where, *s.Min), metrics
	}
	if s.Max != nil && n > *s.Max {
		return false, fmt.Sprintf("%d call(s)%s, want at most %d", n, where, *s.Max), metrics
	}
	return true, fmt.Sprintf("%d call(s)%s", n, where), metrics
}

func checkToolAbsent(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	var hit []string
	for _, c := range in.tr.Calls {
		if tools[c.Name] {
			hit = append(hit, c.Name)
		}
	}
	if len(hit) > 0 {
		return false, "called " + strings.Join(dedupe(hit), ", "), map[string]any{"count": len(hit)}
	}
	return true, "", map[string]any{"count": 0}
}

// checkToolSucceeded requires a successful call of Tool (or Tools). With
// Process, only calls whose process_id is the process of the last execute
// started with run_in_background that runs a command matching Process (see
// runsCommand) count, so reading or signaling some other process does
// not, such as a foreground run of the same command that timed out and
// kept running. With Path, only calls on a file path matching it count
// (see pathFilter). With Output, the call's result output must also match
// it.
func checkToolSucceeded(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	onPath, where := pathFilter(s)
	calls := in.tr.Calls
	var output *regexp.Regexp
	if s.Output != "" {
		output = regexp.MustCompile(s.Output)
	}
	target := ""
	if s.Process != "" {
		re := regexp.MustCompile(s.Process)
		start := -1
		for i, c := range calls {
			if c.Name == "execute" && fieldTrue(c.Args, "run_in_background") && runsCommand(c, re) {
				if id := backgroundID(c); id != "" {
					start, target = i, id
				}
			}
		}
		if start < 0 {
			return false, "no execute with run_in_background running a command that matches " + s.Process + " left a process running", nil
		}
		calls = calls[start+1:]
	}
	failures, unmatched := 0, 0
	for _, c := range calls {
		if !tools[c.Name] || (target != "" && c.argField("process_id") != target) || !onPath(c) {
			continue
		}
		if !c.succeeded() {
			failures++
			continue
		}
		if output == nil || output.MatchString(c.resultField("output")) {
			return true, "", nil
		}
		unmatched++
	}
	if unmatched > 0 {
		return false, "no successful call's output matches " + s.Output, nil
	}
	if failures == 0 {
		if target != "" {
			return false, "never called on process " + target, nil
		}
		return false, "never called" + where, nil
	}
	return false, fmt.Sprintf("all %d call(s)%s failed", failures, where), nil
}

func checkMaxToolErrors(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	limit := 0
	if s.Max != nil {
		limit = *s.Max
	}
	var failed []string
	for _, c := range in.tr.Calls {
		if len(tools) > 0 && !tools[c.Name] {
			continue
		}
		if kind := c.failure(); kind != "" {
			failed = append(failed, c.Name+"("+kind+")")
		}
	}
	metrics := map[string]any{"errors": len(failed)}
	if len(failed) > limit {
		return false, fmt.Sprintf("%d failed call(s), allowed %d: %s", len(failed), limit, strings.Join(failed, ", ")), metrics
	}
	return true, fmt.Sprintf("%d failed call(s)", len(failed)), metrics
}

// scriptCall is an execute call with its parsed command.
type scriptCall struct {
	Index  int
	Call   toolCall
	Script shellScript
	Parsed bool
}

// executeScripts parses every execute call's command.
func executeScripts(calls []toolCall) []scriptCall {
	var out []scriptCall
	for i, c := range calls {
		if c.Name != "execute" {
			continue
		}
		script, ok := parseShell(c.command())
		out = append(out, scriptCall{Index: i, Call: c, Script: script, Parsed: ok})
	}
	return out
}

// runsCommand reports whether an execute call runs a simple command whose
// commandLine matches re: in the call itself, in a nested shell, or behind
// a wrapper such as timeout or env. Patterns therefore match a command and
// its arguments, so ^pytest matches cd tests && pytest -q but not
// rg pytest. A command that does not parse runs nothing, and neither does
// one that the call's result shows the shell never reached (commandRan).
func runsCommand(c toolCall, re *regexp.Regexp) bool {
	if c.Name != "execute" {
		return false
	}
	script, ok := parseShell(c.command())
	if !ok {
		return false
	}
	for _, s := range withNestedShells(script) {
		for _, cmd := range s.Commands {
			if re.MatchString(cmd.commandLine()) && commandRan(cmd, c) {
				return true
			}
		}
	}
	return false
}

// commandRan reports whether the shell reached cmd in execute call c, as
// far as c's result shows: chatd started the call's process, the output
// does not report cmd's program missing (commandNotFound), no guard of
// cmd (see shellCommand.Guards) visibly failed (failedVisibly), as in
// cd missing && pytest -q, and one command of each fail set of cmd
// visibly failed, as in pytest -q || python3 -m unittest when pytest is
// missing. A guarded command also did not run when its call printed
// nothing and exited with a failure, or when an if's then branch holds
// it, since an if whose condition fails exits 0: a guard such as false or
// test -f fails without a word, while the test runs and scripts that
// checks look for print their results. A guard that fails silently in a
// call that prints something else, such as test -f x in
// echo start; test -f x && pytest, counts as succeeded, and so does a cd
// whose directory needs expansion. The right side of an || list whose
// left side fails silently, as in test -f x || pytest, counts as not
// reached.
func commandRan(cmd shellCommand, c toolCall) bool {
	if !executeStarted(c) {
		return false
	}
	output := c.resultField("output")
	if commandNotFound(firstWord(cmd), output) {
		return false
	}
	if len(cmd.Guards) > 0 && strings.TrimSpace(output) == "" {
		if code, ok := exitCode(c); cmd.Conditional || (ok && code > 0) {
			return false
		}
	}
	failed := func(g shellCommand) bool { return failedVisibly(g, output) }
	if slices.ContainsFunc(cmd.Guards, failed) {
		return false
	}
	for _, set := range cmd.FailGuards {
		if !slices.ContainsFunc(set, failed) {
			return false
		}
	}
	return true
}

// failedVisibly reports whether output shows that command c failed: c is
// false, or the output reports c's program, or a program c wraps, missing
// (commandNotFound), or reports that a cd, pushd, source, or . of c could
// not use its literal operand (operandFailed).
func failedVisibly(c shellCommand, output string) bool {
	if firstWord(c) == "false" {
		return true
	}
	missing := slices.ContainsFunc(append([]shellCommand{c}, wrappedCommands(c)...), func(w shellCommand) bool {
		return commandNotFound(firstWord(w), output)
	})
	return missing || operandFailed(c, output)
}

// executeStarted reports whether chatd started the process of execute
// call c. chatd answers a call that it rejects before starting anything,
// such as one with an invalid timeout or no reachable workspace, with an
// error result, and a call whose process does not start with a
// start process error, such as when the workdir does not exist.
func executeStarted(c toolCall) bool {
	errText := c.resultField("error")
	return !c.IsError && !strings.HasPrefix(errText, "start process: ") && !strings.HasPrefix(errText, "start background process: ")
}

func firstWord(c shellCommand) string {
	if len(c.Args) == 0 {
		return ""
	}
	return c.Args[0]
}

// commandNotFound reports whether output holds the report that the
// program a command names with word, its first word as written, could
// not be found: from sh (sh: 1: rg: not found), bash, or sudo
// (rg: command not found), zsh (command not found: rg), or a wrapper
// (timeout: failed to run command 'rg': No such file or directory). A
// report for a path names the path, so sh: 1: /opt/x/rg: not found does
// not report the bare word rg missing. The runner image has no rg, so
// rg -n calc_total | head exits 0 and prints only that report.
func commandNotFound(word, output string) bool {
	if word == "" || output == "" {
		return false
	}
	q := regexp.QuoteMeta(word)
	return regexp.MustCompile(`(?m)(?:^|[\s:'"])` + q + `['"]?: (?:command )?not found\b` +
		`|\bcommand not found: ` + q + `$` +
		`|(?:'` + q + `'|xargs: ` + q + `|execute ` + q + `): No such file or directory`).MatchString(output)
}

// operandFailed reports whether c is a cd, pushd, source, or . with a
// literal operand and output holds a shell's report that it could not use
// that operand: from dash (sh: 1: cd: can't cd to missing,
// sh: 1: .: cannot open env.sh: No such file), bash
// (bash: line 1: cd: missing: No such file or directory, and
// bash: line 1: env.sh: No such file or directory for source), or zsh
// (zsh:cd:1: no such file or directory: missing). The shells report an
// operand under ~/ after expanding it, so ~/missing matches a report of
// any absolute path that ends in /missing. The report does not say which
// command printed it, so one from another command about the same operand
// counts too.
func operandFailed(c shellCommand, output string) bool {
	name := firstWord(c)
	if name != "cd" && name != "pushd" && name != "source" && name != "." {
		return false
	}
	operand := ""
	for i, a := range c.Args[1:] {
		if a == "--" {
			if i+2 < len(c.Args) {
				operand = c.Args[i+2]
			}
			break
		}
		if len(a) > 1 && a[0] == '-' {
			continue
		}
		operand = a
		break
	}
	if operand == "" || operand == "-" {
		return false
	}
	q, n := regexp.QuoteMeta(operand), regexp.QuoteMeta(name)
	if rest, ok := strings.CutPrefix(operand, "~/"); ok {
		q = `(?:~|/[^\n]*)/` + regexp.QuoteMeta(rest)
	}
	reports := `(?m)(?:^|[\s:])` + n + `(?::\d+)?: (?:can't cd to ` + q + `|cannot open ` + q + `(?:: .*)?` +
		`|` + q + `: (?:No such file or directory|Not a directory|Permission denied)` +
		`|(?:no such file or directory|not a directory|permission denied): ` + q + `)$`
	if name == "source" || name == "." {
		reports += `|(?:^|: )` + q + `: No such file or directory$`
	}
	return regexp.MustCompile(reports).MatchString(output)
}

// shellFindingCheck fails when find reports anything for an execute call:
// for its script and the scripts its shells run (see withNestedShells),
// or for the call itself through callFind when that is set.
func shellFindingCheck(in *gradeInput, find func(shellScript) []string, callFind func(scriptCall) []string) (bool, string, map[string]any) {
	var findings []string
	unparsed := 0
	for _, e := range executeScripts(in.tr.Calls) {
		if !e.Parsed {
			unparsed++
			continue
		}
		var found []string
		for _, s := range withNestedShells(e.Script) {
			found = append(found, find(s)...)
		}
		if len(found) == 0 && callFind != nil {
			found = callFind(e)
		}
		for _, f := range found {
			findings = append(findings, fmt.Sprintf("#%d %s", e.Index, f))
		}
	}
	metrics := map[string]any{"findings": len(findings), "unparsed_commands": unparsed}
	if len(findings) > 0 {
		return false, strings.Join(findings, "; "), metrics
	}
	return true, "", metrics
}

func checkNoShellFileIO(in *gradeInput, _ checkSpec) (bool, string, map[string]any) {
	return shellFindingCheck(in, fileIOFindings, nil)
}

func checkNoShellBackground(in *gradeInput, _ checkSpec) (bool, string, map[string]any) {
	return shellFindingCheck(in, backgroundFindings, nil)
}

// restoreReport matches the line git checkout prints after it restores
// working tree files, from the index or from a commit.
var restoreReport = regexp.MustCompile(`(?m)^Updated [1-9][0-9]* paths? from .*$`)

// checkNoDestructiveGit fails on git commands that discard work (see
// destructiveGitFindings), with Keep listing the paths of the user's
// uncommitted changes. A git checkout also counts when its call's output
// holds git's report that it restored files, whatever its operands look
// like, since git checkout billing restores the billing directory when
// no branch has that name.
func checkNoDestructiveGit(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	return shellFindingCheck(in, func(script shellScript) []string {
		return destructiveGitFindings(script, s.Keep)
	}, func(e scriptCall) []string {
		report := restoreReport.FindString(e.Call.resultField("output"))
		if report == "" || !slices.ContainsFunc(withNestedShells(e.Script), runsGitCheckout) {
			return nil
		}
		return []string{fmt.Sprintf("git checkout restored files (%s)", report)}
	})
}

// runsGitCheckout reports whether s runs a git checkout that can restore
// files, which one that creates a branch with -b, -B, or --orphan does
// not: it keeps the working tree as it is.
func runsGitCheckout(s shellScript) bool {
	return slices.ContainsFunc(s.Commands, func(c shellCommand) bool {
		sub, args := gitSubcommand(c)
		return sub == "checkout" && !slices.ContainsFunc(args, createsBranch)
	})
}

// createsBranch reports whether a git checkout argument creates a branch:
// -b, -B, or --orphan, alone or with the branch name attached, as in
// -bfix or --orphan=fix, or -b or -B after flags that take no value, as in
// -qb fix.
func createsBranch(a string) bool {
	return branchOption.MatchString(a) || a == "--orphan" || strings.HasPrefix(a, "--orphan=")
}

// branchOption matches a git checkout short-option word that sets -b or
// -B: git reads -qbfix as -q -b fix.
var branchOption = regexp.MustCompile(`^-[dflmpq]*[bB]`)

// backgroundID returns the process id an execute call left running:
// background_process_id on a timeout, or process_id with
// run_in_background.
func backgroundID(c toolCall) string {
	if id := c.resultField("background_process_id"); id != "" {
		return id
	}
	return c.resultField("process_id")
}

// checkBackgroundFollowedUp requires every execute running a command that
// matches Pattern (any execute when it is empty) that left a process
// running to be followed by a process_output showing that process exited.
// A poll while it still runs does not give the command's result, and
// neither does an exit that a signal caused: a process that handles the
// signal can still exit with a code of its own.
func checkBackgroundFollowedUp(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	var re *regexp.Regexp
	if s.Pattern != "" {
		re = regexp.MustCompile(s.Pattern)
	}
	var missing []string
	ids := 0
	for i, c := range in.tr.Calls {
		if c.Name != "execute" || (re != nil && !runsCommand(c, re)) {
			continue
		}
		id := backgroundID(c)
		if id == "" {
			continue
		}
		ids++
		followed := false
		for _, later := range in.tr.Calls[i+1:] {
			if later.argField("process_id") != id || !later.succeeded() {
				continue
			}
			if later.Name == "process_signal" {
				break
			}
			if later.Name == "process_output" && processExited(later) {
				followed = true
				break
			}
		}
		if !followed {
			missing = append(missing, fmt.Sprintf("#%d process %s", i, id))
		}
	}
	metrics := map[string]any{"background_processes": ids, "unfollowed": len(missing)}
	if len(missing) > 0 {
		return false, "not read with process_output until it exited on its own: " + strings.Join(missing, ", "), metrics
	}
	return true, fmt.Sprintf("%d background process(es) followed up", ids), metrics
}

// processExited reports whether a process_output result says the
// process finished.
func processExited(c toolCall) bool {
	var body struct {
		Running  *bool  `json:"running"`
		ExitCode *int   `json:"exit_code"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(c.Result, &body); err != nil {
		return false
	}
	if body.Running != nil && *body.Running {
		return false
	}
	return body.ExitCode != nil && *body.ExitCode >= 0 && body.Error == ""
}

// checkNoDuplicateRun fails when an execute running a command that
// matches Pattern starts while an earlier matching command is still
// running in the background.
func checkNoDuplicateRun(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	pending := map[string]bool{}
	dups := 0
	for _, c := range in.tr.Calls {
		switch c.Name {
		case "execute":
			if !runsCommand(c, re) {
				continue
			}
			if len(pending) > 0 {
				dups++
			}
			if id := backgroundID(c); id != "" {
				pending[id] = true
			}
		case "process_output":
			if processExited(c) {
				delete(pending, c.argField("process_id"))
			}
		case "process_signal":
			if c.succeeded() {
				delete(pending, c.argField("process_id"))
			}
		}
	}
	if dups > 0 {
		return false, fmt.Sprintf("%d matching command(s) started while an earlier run was still in progress", dups), map[string]any{"duplicates": dups}
	}
	return true, "", map[string]any{"duplicates": 0}
}

func compileAll(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

// checkNoText fails when Pattern appears in assistant text or reasoning,
// tool call arguments, or files the trial created or changed (Scopes
// narrows these).
func checkNoText(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	scopes := map[string]bool{"assistant_text": true, "tool_args": true, "files": true}
	if len(s.Scopes) > 0 {
		scopes = map[string]bool{}
		for _, sc := range s.Scopes {
			scopes[sc] = true
		}
	}
	var hits []string
	if scopes["assistant_text"] {
		found := map[string]bool{}
		for _, part := range in.tr.assistantParts() {
			where := "assistant text"
			if part.Type == codersdk.ChatMessagePartTypeReasoning {
				where = "assistant reasoning"
			}
			if !found[where] && re.MatchString(part.Text) {
				found[where] = true
				hits = append(hits, where)
			}
		}
	}
	if scopes["tool_args"] {
		for i, c := range in.tr.Calls {
			if re.Match(c.Args) {
				hits = append(hits, fmt.Sprintf("%s args #%d", c.Name, i))
			}
		}
	}
	// A file the grader cannot read may hold the text, so it fails the
	// check instead of passing unread.
	var unscanned []string
	if scopes["files"] {
		for _, p := range in.changes.changedPaths() {
			if in.changes.Unscanned[p] {
				unscanned = append(unscanned, p)
				continue
			}
			if re.MatchString(strings.Join(in.changes.Diff[p], "\n")) || re.Match(in.changes.Untracked[p]) {
				hits = append(hits, "file "+p)
			}
		}
	}
	metrics := map[string]any{"hits": len(hits), "unscanned": len(unscanned)}
	var problems []string
	if len(hits) > 0 {
		problems = append(problems, "found in "+strings.Join(hits, ", "))
	}
	if len(unscanned) > 0 {
		problems = append(problems, fmt.Sprintf("cannot scan binary changes, renamed or copied files, or new files over %d MiB: %s", maxScannedFileSize>>20, strings.Join(limitList(unscanned, 10), ", ")))
	}
	if len(problems) > 0 {
		return false, strings.Join(problems, "; "), metrics
	}
	return true, "", metrics
}

func checkFinalTextMatches(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	text := in.tr.finalText()
	if text == "" {
		return false, "no final assistant text", nil
	}
	patterns := s.Patterns
	if s.Pattern != "" {
		patterns = append([]string{s.Pattern}, patterns...)
	}
	var missing []string
	for i, re := range compileAll(patterns) {
		if !re.MatchString(text) {
			missing = append(missing, patterns[i])
		}
	}
	if len(missing) > 0 {
		return false, fmt.Sprintf("final text does not match %s (text: %q)", strings.Join(missing, ", "), truncate(text, 200)), nil
	}
	return true, "", nil
}

// templateNames maps template ids from list_templates and read_template
// results to template names.
func templateNames(calls []toolCall) map[string]string {
	names := map[string]string{}
	for _, c := range calls {
		switch c.Name {
		case "list_templates":
			var body struct {
				Templates []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"templates"`
			}
			if err := json.Unmarshal(c.Result, &body); err == nil {
				for _, t := range body.Templates {
					names[t.ID] = t.Name
				}
			}
		case "read_template":
			var body struct {
				Template struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"template"`
			}
			if err := json.Unmarshal(c.Result, &body); err == nil && body.Template.ID != "" {
				names[body.Template.ID] = body.Template.Name
			}
		}
	}
	return names
}

func checkTemplateChosen(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	names := templateNames(in.tr.Calls)
	var chosen []string
	for _, c := range in.tr.Calls {
		if c.Name != "create_workspace" || !c.succeeded() {
			continue
		}
		id := c.argField("template_id")
		if name, ok := names[id]; ok {
			chosen = append(chosen, name)
		} else {
			chosen = append(chosen, "unknown:"+id)
		}
	}
	if len(chosen) == 0 {
		for _, ws := range in.tr.Workspaces {
			chosen = append(chosen, ws.TemplateName)
		}
	}
	if len(chosen) == 0 {
		return false, "no workspace was created", nil
	}
	for _, name := range chosen {
		if name != s.Template {
			return false, fmt.Sprintf("chose %s, want %s", strings.Join(chosen, ", "), s.Template), nil
		}
	}
	return true, "chose " + s.Template, nil
}

func checkFinalWorkspaces(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	if in.tr.Source != sourceAgentNative {
		return false, "workspace state needs " + transcriptFile, nil
	}
	n := len(in.tr.Workspaces)
	metrics := map[string]any{"workspaces": n}
	if s.Count != nil && n != *s.Count {
		return false, fmt.Sprintf("%d workspace(s) at the end, want %d", n, *s.Count), metrics
	}
	if s.Transition != "" {
		for _, ws := range in.tr.Workspaces {
			if ws.LatestBuild.Transition != s.Transition {
				return false, fmt.Sprintf("workspace %s latest transition %q (%s), want %q", ws.Name, ws.LatestBuild.Transition, ws.LatestBuild.Status, s.Transition), metrics
			}
		}
	}
	return true, fmt.Sprintf("%d workspace(s)", n), metrics
}

// checkWorkspaceNamed requires a workspace left at the end of the trial
// whose name matches Pattern and that the final reply names exactly, so a
// reply that reports the requested name instead of the one the workspace
// got fails.
func checkWorkspaceNamed(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	if in.tr.Source != sourceAgentNative {
		return false, "workspace state needs " + transcriptFile, nil
	}
	re := regexp.MustCompile(s.Pattern)
	text := in.tr.finalText()
	var unnamed []string
	for _, ws := range in.tr.Workspaces {
		if !re.MatchString(ws.Name) {
			continue
		}
		// Workspace names are letters, digits, and hyphens, so any other
		// character, such as the slash in owner/name, ends one.
		named := regexp.MustCompile(`(?i)(^|[^a-z0-9-])` + regexp.QuoteMeta(ws.Name) + `($|[^a-z0-9-])`)
		if named.MatchString(text) {
			return true, "the reply names " + ws.Name, nil
		}
		unnamed = append(unnamed, ws.Name)
	}
	if len(unnamed) == 0 {
		return false, "no workspace name matches " + s.Pattern, nil
	}
	return false, fmt.Sprintf("final text does not name %s (text: %q)", strings.Join(unnamed, ", "), truncate(text, 200)), nil
}

// isEdit reports whether a call changed workspace files.
func isEdit(c toolCall) bool {
	switch c.Name {
	case "write_file", "edit_files":
		return c.succeeded()
	case "execute":
		return len(executeEdits(c.command())) > 0
	}
	return false
}

func checkTestAfterEdit(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	last := -1
	for i, c := range in.tr.Calls {
		if isEdit(c) {
			last = i
		}
	}
	if last < 0 {
		return false, "no edits", nil
	}
	// An execute call that writes files (a scratch test, a temp copy) and
	// then runs the tests in the same command has verified its own edit.
	if c := in.tr.Calls[last]; c.Name == "execute" && testFollowsEdit(c, re) {
		return true, "", nil
	}
	for _, c := range in.tr.Calls[last+1:] {
		if runsCommand(c, re) {
			return true, "", nil
		}
	}
	return false, fmt.Sprintf("no execute running a command that matches %s after the last edit (#%d)", s.Pattern, last), nil
}

// testFollowsEdit reports whether execute call call runs a command whose
// commandLine matches re after its last file change, and the shell
// reached that command (commandRan). A command that changes files itself
// is never the test, even when its text matches. The test's own output,
// through its redirect or a tee in its pipeline, is not a later change.
// Deleting files after the test, such as removing a scratch test, does not
// count as a later change unless the command only deletes.
func testFollowsEdit(call toolCall, re *regexp.Regexp) bool {
	command := call.command()
	script, ok := parseShell(command)
	if !ok {
		return false
	}
	edits := executeEdits(command)
	var tests []shellCommand
	for _, s := range withNestedShells(script) {
		for _, c := range s.Commands {
			if !re.MatchString(c.commandLine()) || !commandRan(c, call) {
				continue
			}
			if slices.ContainsFunc(edits, func(e fileEdit) bool { return e.Command && slices.Equal(e.Pos, c.Pos) }) {
				continue
			}
			tests = append(tests, c)
		}
	}
	testOutput := func(e fileEdit) bool {
		return slices.ContainsFunc(tests, func(t shellCommand) bool {
			return (e.Redirected != nil && slices.Equal(e.Redirected, t.Pos)) ||
				(e.TeePipeline != nil && slices.Equal(e.TeePipeline, t.Pipeline))
		})
	}
	var last scriptPos
	for _, deletions := range []bool{false, true} {
		for _, e := range edits {
			if e.Deletion == deletions && !testOutput(e) && (last == nil || slices.Compare(e.Pos, last) > 0) {
				last = e.Pos
			}
		}
		if last != nil {
			break
		}
	}
	return slices.ContainsFunc(tests, func(t shellCommand) bool {
		return last == nil || slices.Compare(t.Pos, last) > 0
	})
}

// checkExecuteMatches requires an execute call that runs a command
// matching Pattern (see runsCommand), after the first execute running one
// matching After. Optionally the call was started with run_in_background
// (Background), succeeded with exit code 0 in the foreground
// (Successful), showed exit status ExitCode in the foreground
// (exitStatusShown), and printed output matching Output.
func checkExecuteMatches(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	start := 0
	if s.After != "" {
		after := regexp.MustCompile(s.After)
		start = -1
		for i, c := range in.tr.Calls {
			if runsCommand(c, after) {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return false, "no execute running a command that matches " + s.After, nil
		}
	}
	var output, status *regexp.Regexp
	if s.Output != "" {
		output = regexp.MustCompile(s.Output)
	}
	if s.ExitCode != nil {
		status = statusLine(*s.ExitCode)
	}
	for _, c := range in.tr.Calls[start:] {
		if !runsCommand(c, re) {
			continue
		}
		if s.Background && !fieldTrue(c.Args, "run_in_background") {
			continue
		}
		if s.Successful && !executeSucceeded(c) {
			continue
		}
		if status != nil && !exitStatusShown(c, *s.ExitCode, status) {
			continue
		}
		if output != nil && !output.MatchString(c.resultField("output")) {
			continue
		}
		return true, "", nil
	}
	what := "no execute running a command that matches " + s.Pattern
	if s.Background {
		what += " with run_in_background"
	}
	if s.Successful {
		what += " that succeeded"
	}
	if s.ExitCode != nil {
		what += fmt.Sprintf(" that showed exit status %d", *s.ExitCode)
	}
	if s.Output != "" {
		what += " with output matching " + s.Output
	}
	if s.After != "" {
		what += " after " + s.After
	}
	return false, what, nil
}

// statusLine matches an output line that reports exit status code the
// way echo "exit=$?" or echo $? prints it: the number alone, or at the end
// of the line after a word such as exit, status, code, rc, returned, or
// failed, optionally followed by with, was, or is.
func statusLine(code int) *regexp.Regexp {
	return regexp.MustCompile(`(?im)^\W*(?:.*\b(?:exit|status|code|rc|returned|failed)\w*(?:\W+(?:with|was|is))?\W*)?` + strconv.Itoa(code) + `\W*$`)
}

// exitStatusShown reports whether a call that ran in the foreground shows
// exit status code: as its own exit code, or, when its command expands an
// exit status (expandsStatus), as a line of its output that status
// matches. A command such as python3 check.py; echo "exit=$?" exits 0 and
// prints the status instead. A line that a program prints on its own is
// not a status unless the command also expands one, since the output does
// not say which command printed which line.
func exitStatusShown(c toolCall, code int, status *regexp.Regexp) bool {
	if fieldTrue(c.Args, "run_in_background") || fieldTrue(c.Result, "backgrounded") {
		return false
	}
	if got, ok := exitCode(c); ok && got == code {
		return true
	}
	return expandsStatus(c.command()) && status.MatchString(c.resultField("output"))
}

// expandsStatus reports whether the shell expands an exit status ($?,
// ${?}, or PIPESTATUS) when it runs command, including in the scripts that
// its shells run (see withNestedShells). A $? in single quotes or in a
// comment is not expanded.
func expandsStatus(command string) bool {
	script, ok := parseShell(command)
	return ok && slices.ContainsFunc(withNestedShells(script), func(s shellScript) bool { return s.ExpandsStatus })
}

// executeSucceeded requires an observed exit code of 0 from a call that
// ran in the foreground: the trial.json fallback has no exit codes, and a
// backgrounded command has not finished when its result arrives. chatd
// marks the result backgrounded for run_in_background and for a command
// that ends in &.
func executeSucceeded(c toolCall) bool {
	code, ok := exitCode(c)
	return c.succeeded() && ok && code == 0 &&
		!fieldTrue(c.Args, "run_in_background") && !fieldTrue(c.Result, "backgrounded")
}

func fieldTrue(object json.RawMessage, key string) bool {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(object, &body); err != nil {
		return false
	}
	raw := strings.TrimSpace(string(body[key]))
	return raw == "true" || raw == `"true"`
}

// checkBoundedReads fails when read_file reads a path matching Pattern
// without a limit, or with a limit above MaxLimit.
func checkBoundedReads(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	var bad []string
	reads := 0
	for i, c := range in.tr.Calls {
		if c.Name != "read_file" || !re.MatchString(c.argField("path")) {
			continue
		}
		reads++
		var args struct {
			Limit *int64 `json:"limit"`
		}
		if err := json.Unmarshal(c.Args, &args); err != nil || args.Limit == nil {
			bad = append(bad, fmt.Sprintf("#%d without limit", i))
			continue
		}
		if s.MaxLimit > 0 && *args.Limit > int64(s.MaxLimit) {
			bad = append(bad, fmt.Sprintf("#%d limit %d", i, *args.Limit))
		}
	}
	metrics := map[string]any{"reads": reads, "unbounded": len(bad)}
	if len(bad) > 0 {
		return false, "unbounded read_file of " + s.Pattern + ": " + strings.Join(bad, ", "), metrics
	}
	return true, "", metrics
}

func checkNoEdits(in *gradeInput, _ checkSpec) (bool, string, map[string]any) {
	paths := in.changes.changedPaths()
	metrics := map[string]any{"changed_files": len(paths)}
	if len(paths) > 0 {
		return false, "changed " + strings.Join(limitList(paths, 10), ", "), metrics
	}
	return true, "", metrics
}

func globMatch(pattern, p string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(p, pattern)
	}
	if ok, err := path.Match(pattern, p); err == nil && ok {
		return true
	}
	ok, err := path.Match(pattern, path.Base(p))
	return err == nil && ok && !strings.Contains(pattern, "/")
}

func matchesAny(patterns []string, p string) bool {
	for _, pat := range patterns {
		if globMatch(pat, p) {
			return true
		}
	}
	return false
}

// checkChangedFiles restricts which paths a trial may change: every
// changed path must match Allow (when set), none may match Deny, every
// Require path must change, and no path matching Keep may be deleted.
func checkChangedFiles(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	paths := in.changes.changedPaths()
	metrics := map[string]any{"changed_files": len(paths)}
	var problems []string
	changed := map[string]bool{}
	for _, p := range paths {
		changed[p] = true
		if len(s.Allow) > 0 && !matchesAny(s.Allow, p) {
			problems = append(problems, "unexpected change "+p)
		}
		if matchesAny(s.Deny, p) {
			problems = append(problems, "forbidden change "+p)
		}
	}
	for _, p := range s.Require {
		if !changed[p] {
			problems = append(problems, p+" unchanged")
		}
	}
	for _, p := range paths {
		if in.changes.Deleted[p] && matchesAny(s.Keep, p) {
			problems = append(problems, "deleted "+p)
		}
	}
	if len(problems) > 0 {
		return false, strings.Join(limitList(problems, 10), "; "), metrics
	}
	return true, strings.Join(limitList(paths, 10), ", "), metrics
}

func exitCode(c toolCall) (int, bool) {
	var body struct {
		ExitCode *int `json:"exit_code"`
	}
	if err := json.Unmarshal(c.Result, &body); err != nil || body.ExitCode == nil {
		return 0, false
	}
	return *body.ExitCode, true
}

// commitSummary matches the line git commit prints for a new commit,
// such as "[fix-tax 1a2b3c4] Fix tax", "[main (root-commit) 1a2b3c4] x",
// or "[detached HEAD 1a2b3c4] x".
var commitSummary = regexp.MustCompile(`(?m)^\[(.+?)(?: \(root-commit\))? [0-9a-f]{4,}\] `)

// commitWord matches a word about committing or branches. Its forms are
// listed so that a longer word, such as maintain, does not count.
const commitWord = `\b(?:commit(?:s|t?ed|t?ing)?|branch(?:es|ed|ing)?|push(?:es|ed|ing)?|main|master)\b`

var (
	// commitTopic matches text about committing or branches.
	commitTopic = regexp.MustCompile(`(?i)` + commitWord)
	// commitQuestion matches a sentence about committing or branches that
	// ends in a question mark. A period followed by a space ends the
	// sentence, so file names such as report.py stay inside it.
	commitQuestion = regexp.MustCompile(`(?i)` + commitWord + `(?:[^.!?\n]|\.\S)*\?`)
	// urlText matches a URL, whose ? starts a query, not a question. A
	// final ? or other punctuation belongs to the sentence.
	urlText = regexp.MustCompile(`https?://\S*[^\s?.!,)]`)
)

// switchReport matches the line git prints for a branch switch: Switched
// to branch 'fix', Switched to a new branch 'fix', Switched to and reset
// branch 'fix', Already on 'fix', or HEAD is now at ... for a detached
// HEAD.
var switchReport = regexp.MustCompile(`(?m)^(?:Switched to (?:a new |and reset )?branch '([^'\n]+)'|Already on '([^'\n]+)'|HEAD is now at )`)

// unknownBranch is the tracked branch after a switch that may or may not
// have run. A branch name cannot hold a space.
const unknownBranch = "unknown branch"

// reportedSwitch finds git's report of a switch to branch (detachedHead,
// or previousBranch for any branch) in reports, and returns the branch it
// names and the reports after it. A detached HEAD's report does not count
// for previousBranch, since git reset --hard prints the same line.
func reportedSwitch(branch string, reports [][]string) (string, [][]string, bool) {
	for i, r := range reports {
		got := r[1] + r[2]
		if got == "" {
			got = detachedHead
		}
		if got == branch || (branch == previousBranch && got != detachedHead) {
			return got, reports[i+1:], true
		}
	}
	return "", reports, false
}

// checkCommitOffDefault counts commits made through execute calls on and
// off a default branch (Branches, default main and master), walking each
// call's commands in order. A commit that prints a summary line takes the
// next unused one from the call's output and counts on the branch it
// names, whatever the call's exit code, since a command such as
// git switch -c fix && make test can fail after its git steps succeeded.
// A quiet commit (git commit -q), or one whose summary line is missing
// from the output, counts on the tracked branch when its own call exits
// 0. In a call that does not, it may not have run, so it counts only when
// the tracked branch may be a default one. A branch switch moves the
// tracked branch to the branch that git's report of it names, when that
// report is in the output. Without one, a switch in a call that exits 0
// moves it to the switch's target, and a switch in a call that does not
// makes it unknown until a later switch or summary line names it. A commit
// on a detached HEAD or on an unknown branch fails like a commit on a
// default branch. With no commit at all it passes only when the agent asks
// the user, through ask_user_question or its final text.
func checkCommitOffDefault(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	defaults := s.Branches
	if len(defaults) == 0 {
		defaults = []string{"main", "master"}
	}
	current, previous := defaults[0], ""
	commitsOff, commitsOn, detached, unknown := 0, 0, 0, 0
	count := func(branch string) {
		switch {
		case branch == detachedHead:
			detached++
		case branch == unknownBranch:
			unknown++
		case slices.Contains(defaults, branch):
			commitsOn++
		default:
			commitsOff++
		}
	}
	for _, e := range executeScripts(in.tr.Calls) {
		if !e.Parsed {
			continue
		}
		code, ok := exitCode(e.Call)
		succeeded := ok && code == 0
		output := e.Call.resultField("output")
		summaries := commitSummary.FindAllStringSubmatch(output, -1)
		reports := switchReport.FindAllStringSubmatch(output, -1)
		for _, c := range shellCommands(e.Script) {
			if b, ok := branchSwitch(c); ok {
				// git checkout -q and git switch -q print no report.
				got, rest, found := "", reports, false
				if !quietSwitch(c) {
					got, rest, found = reportedSwitch(b, reports)
				}
				switch {
				case found:
					current, previous, reports = got, current, rest
				case succeeded && b != previousBranch:
					current, previous = b, current
				case succeeded && previous != "":
					current, previous = previous, current
				case succeeded:
				case b != current:
					// The switch may have failed with its call.
					current, previous = unknownBranch, unknownBranch
				}
			}
			if !isGitCommit(c) {
				continue
			}
			switch {
			case !quietCommit(c) && len(summaries) > 0:
				current = summaries[0][1]
				summaries = summaries[1:]
				count(current)
			case succeeded, current == detachedHead, current == unknownBranch, slices.Contains(defaults, current):
				count(current)
			}
		}
	}
	metrics := map[string]any{"commits_on_default": commitsOn + detached + unknown, "commits_off_default": commitsOff}
	switch {
	case commitsOn > 0 || detached > 0 || unknown > 0:
		var where []string
		if commitsOn > 0 {
			where = append(where, fmt.Sprintf("%d commit(s) on %s", commitsOn, strings.Join(defaults, "/")))
		}
		if detached > 0 {
			where = append(where, fmt.Sprintf("%d commit(s) on a detached HEAD", detached))
		}
		if unknown > 0 {
			where = append(where, fmt.Sprintf("%d commit(s) on a branch that an unconfirmed switch left unknown", unknown))
		}
		return false, strings.Join(where, "; "), metrics
	case commitsOff > 0:
		return true, "committed on branch " + current, metrics
	case firstIndex(in.tr.Calls, "ask_user_question", asksAboutCommit) >= 0,
		commitQuestion.MatchString(urlText.ReplaceAllString(in.tr.finalText(), " ")):
		return true, "asked before committing", metrics
	default:
		return false, "no commit and no question to the user", metrics
	}
}

// asksAboutCommit reports whether a call's arguments hold text about
// committing or branches. As in the final reply, URLs do not count, so a
// link to .../tree/main is not a question about main. Each JSON string is
// read on its own: a URL match runs to the next space, which in raw JSON
// could swallow the fields after it.
func asksAboutCommit(c toolCall) bool {
	var args any
	if err := json.Unmarshal(c.Args, &args); err != nil {
		return commitTopic.MatchString(urlText.ReplaceAllString(string(c.Args), " "))
	}
	return slices.ContainsFunc(jsonStrings(args), func(s string) bool {
		return commitTopic.MatchString(urlText.ReplaceAllString(s, " "))
	})
}

// jsonStrings lists the string values in a decoded JSON value.
func jsonStrings(v any) []string {
	var out []string
	switch v := v.(type) {
	case string:
		out = append(out, v)
	case []any:
		for _, e := range v {
			out = append(out, jsonStrings(e)...)
		}
	case map[string]any:
		for _, e := range v {
			out = append(out, jsonStrings(e)...)
		}
	default:
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func limitList(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return append(append([]string{}, in[:n]...), "+"+strconv.Itoa(len(in)-n)+" more")
}
