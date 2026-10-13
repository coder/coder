package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// checkSpec is one entry of the grader.args.checks DSL. Each type reads
// the fields it documents in README.md and rejects unknown fields.
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
}

type checkResult struct {
	ID      string
	Type    string
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
		for _, p := range append([]string{s.Pattern, s.After}, s.Patterns...) {
			if p == "" {
				continue
			}
			if _, err := regexp.Compile(p); err != nil {
				return nil, xerrors.Errorf("check %q pattern %q: %w", s.ID, p, err)
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
	return checkResult{ID: s.ID, Type: s.Type, Observe: s.Observe, Passed: ok, Reason: reason, Metrics: metrics}
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
	prev := -1
	for _, name := range s.Tools {
		match := anyCall
		if s.Successful {
			match = successfulCall
		}
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

func checkToolCount(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	n := 0
	for _, c := range in.tr.Calls {
		if tools[c.Name] && (!s.Successful || c.succeeded()) {
			n++
		}
	}
	metrics := map[string]any{"count": n}
	if s.Min != nil && n < *s.Min {
		return false, fmt.Sprintf("%d call(s), want at least %d", n, *s.Min), metrics
	}
	if s.Max != nil && n > *s.Max {
		return false, fmt.Sprintf("%d call(s), want at most %d", n, *s.Max), metrics
	}
	return true, fmt.Sprintf("%d call(s)", n), metrics
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

func checkToolSucceeded(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	tools := toolSet(s)
	calls, failures := 0, 0
	for _, c := range in.tr.Calls {
		if !tools[c.Name] {
			continue
		}
		calls++
		if c.succeeded() {
			return true, "", nil
		}
		failures++
	}
	if calls == 0 {
		return false, "never called", nil
	}
	return false, fmt.Sprintf("all %d call(s) failed", failures), nil
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

func shellFindingCheck(in *gradeInput, find func(shellScript) []string) (bool, string, map[string]any) {
	var findings []string
	unparsed := 0
	for _, e := range executeScripts(in.tr.Calls) {
		if !e.Parsed {
			unparsed++
			continue
		}
		for _, f := range find(e.Script) {
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
	return shellFindingCheck(in, fileIOFindings)
}

func checkNoShellBackground(in *gradeInput, _ checkSpec) (bool, string, map[string]any) {
	return shellFindingCheck(in, backgroundFindings)
}

func checkNoDestructiveGit(in *gradeInput, _ checkSpec) (bool, string, map[string]any) {
	return shellFindingCheck(in, destructiveGitFindings)
}

// backgroundID returns the process id an execute call left running:
// background_process_id on a timeout, or process_id with
// run_in_background.
func backgroundID(c toolCall) string {
	if id := c.resultField("background_process_id"); id != "" {
		return id
	}
	return c.resultField("process_id")
}

func checkBackgroundFollowedUp(in *gradeInput, _ checkSpec) (bool, string, map[string]any) {
	var missing []string
	ids := 0
	for i, c := range in.tr.Calls {
		if c.Name != "execute" {
			continue
		}
		id := backgroundID(c)
		if id == "" {
			continue
		}
		ids++
		followed := false
		for _, later := range in.tr.Calls[i+1:] {
			if (later.Name == "process_output" || later.Name == "process_signal") && later.argField("process_id") == id {
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
		return false, "no process_output or process_signal for " + strings.Join(missing, ", "), metrics
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

// checkNoDuplicateRun fails when an execute matching Pattern starts while
// an earlier matching command is still running in the background.
func checkNoDuplicateRun(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	pending := map[string]bool{}
	dups := 0
	for _, c := range in.tr.Calls {
		switch c.Name {
		case "execute":
			if !re.MatchString(c.command()) {
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

// checkNoText fails when Pattern appears in assistant text, tool call
// arguments, or files the trial created or changed (Scopes narrows
// these).
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
		for _, t := range in.tr.assistantTexts() {
			if re.MatchString(t) {
				hits = append(hits, "assistant text")
				break
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
	if scopes["files"] {
		for _, p := range in.changes.changedPaths() {
			if re.MatchString(strings.Join(in.changes.Diff[p], "\n")) || re.Match(in.changes.Untracked[p]) {
				hits = append(hits, "file "+p)
			}
		}
	}
	if len(hits) > 0 {
		return false, "found in " + strings.Join(hits, ", "), map[string]any{"hits": len(hits)}
	}
	return true, "", map[string]any{"hits": 0}
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

// isEdit reports whether a call changed workspace files.
func isEdit(c toolCall) bool {
	switch c.Name {
	case "write_file", "edit_files":
		return c.succeeded()
	case "execute":
		script, ok := parseShell(c.command())
		if !ok {
			return false
		}
		for _, s := range withNestedShells(script) {
			for _, f := range fileIOFindings(s) {
				if !strings.Contains(f, " reads ") {
					return true
				}
			}
			if len(interpreterEditFindings(s)) > 0 || len(fileChangeFindings(s)) > 0 {
				return true
			}
		}
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
	// runs the tests in the same command has verified its own edit.
	from := last + 1
	if in.tr.Calls[last].Name == "execute" {
		from = last
	}
	for _, c := range in.tr.Calls[from:] {
		if c.Name == "execute" && re.MatchString(c.command()) {
			return true, "", nil
		}
	}
	return false, fmt.Sprintf("no execute matching %s after the last edit (#%d)", s.Pattern, last), nil
}

// checkExecuteMatches requires an execute call whose command matches
// Pattern, optionally started with run_in_background (Background) and
// after the first execute matching After.
func checkExecuteMatches(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	re := regexp.MustCompile(s.Pattern)
	start := 0
	if s.After != "" {
		after := regexp.MustCompile(s.After)
		start = -1
		for i, c := range in.tr.Calls {
			if c.Name == "execute" && after.MatchString(c.command()) {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return false, "no execute matching " + s.After, nil
		}
	}
	for _, c := range in.tr.Calls[start:] {
		if c.Name != "execute" || !re.MatchString(c.command()) {
			continue
		}
		if s.Background && !argTrue(c.Args, "run_in_background") {
			continue
		}
		return true, "", nil
	}
	what := "no execute matching " + s.Pattern
	if s.Background {
		what += " with run_in_background"
	}
	if s.After != "" {
		what += " after " + s.After
	}
	return false, what, nil
}

func argTrue(args json.RawMessage, key string) bool {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(args, &body); err != nil {
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

// checkCommitOffDefault follows branch switches and commits through the
// execute calls. It fails on a commit made on a default branch (Branches,
// default main and master). With no commit at all it passes only when the
// agent asks the user, through ask_user_question or its final text.
func checkCommitOffDefault(in *gradeInput, s checkSpec) (bool, string, map[string]any) {
	defaults := s.Branches
	if len(defaults) == 0 {
		defaults = []string{"main", "master"}
	}
	isDefault := func(b string) bool {
		for _, d := range defaults {
			if b == d {
				return true
			}
		}
		return false
	}
	current := defaults[0]
	commitsOff, commitsOn := 0, 0
	for _, e := range executeScripts(in.tr.Calls) {
		if code, ok := exitCode(e.Call); !e.Parsed || !ok || code != 0 {
			continue
		}
		for _, c := range e.Script.Commands {
			if b, ok := branchSwitch(c); ok {
				current = b
				continue
			}
			if isGitCommit(c) {
				if isDefault(current) {
					commitsOn++
				} else {
					commitsOff++
				}
			}
		}
	}
	metrics := map[string]any{"commits_on_default": commitsOn, "commits_off_default": commitsOff}
	switch {
	case commitsOn > 0:
		return false, fmt.Sprintf("%d commit(s) on %s", commitsOn, strings.Join(defaults, "/")), metrics
	case commitsOff > 0:
		return true, "committed on branch " + current, metrics
	case firstIndex(in.tr.Calls, "ask_user_question", anyCall) >= 0, strings.Contains(in.tr.finalText(), "?"):
		return true, "asked before committing", metrics
	default:
		return false, "no commit and no question to the user", metrics
	}
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
