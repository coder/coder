package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const completionDone = "DONE"

// trialSummary is one landed (or failed) trial joined with its grading.
type trialSummary struct {
	TrialID     string               `json:"trial_id"`
	TaskID      string               `json:"task_id"`
	VariantID   string               `json:"variant_id"`
	Completion  string               `json:"completion"`
	FailReason  string               `json:"fail_reason,omitempty"`
	CostUSD     float64              `json:"cost_usd"`
	WallMS      int64                `json:"wall_ms"`
	Graded      bool                 `json:"graded"`
	TaskSuccess bool                 `json:"task_success"`
	Graders     map[string]bool      `json:"graders,omitempty"`
	Tools       map[string]*toolStat `json:"-"`
}

// rate is a success count with its Wilson 95% interval.
type rate struct {
	Passed int     `json:"passed"`
	Total  int     `json:"total"`
	Rate   float64 `json:"rate"`
	Low    float64 `json:"ci_low"`
	High   float64 `json:"ci_high"`
}

func newRate(passed, total int) rate {
	r := rate{Passed: passed, Total: total}
	if total == 0 {
		return r
	}
	r.Rate = float64(passed) / float64(total)
	r.Low, r.High = wilson(passed, total)
	return r
}

// wilson returns the Wilson score interval at 95% confidence.
func wilson(passed, total int) (low, high float64) {
	if total == 0 {
		return 0, 0
	}
	const z = 1.959963984540054
	n := float64(total)
	p := float64(passed) / n
	denom := 1 + z*z/n
	center := (p + z*z/(2*n)) / denom
	margin := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / denom
	return math.Max(0, center-margin), math.Min(1, center+margin)
}

type toolRow struct {
	Tool     string         `json:"tool"`
	Calls    int            `json:"calls"`
	Failures int            `json:"failures"`
	Rate     float64        `json:"failure_rate"`
	Kinds    map[string]int `json:"failure_kinds,omitempty"`
}

type runSummary struct {
	Label       string            `json:"label"`
	Trials      int               `json:"trials"`
	Completions map[string]int    `json:"completions"`
	Failures    []trialSummary    `json:"failures,omitempty"`
	Ungraded    int               `json:"ungraded"`
	ByArm       map[string]rate   `json:"task_success_by_arm"`
	ByCase      map[string]rate   `json:"task_success_by_case"`
	Graders     map[string]rate   `json:"grader_pass_rates"`
	Tools       []toolRow         `json:"tools"`
	Overall     rate              `json:"task_success"`
	CostUSD     float64           `json:"cost_usd"`
	WallMS      int64             `json:"wall_ms"`
	TrialsList  []trialSummary    `json:"trials_list"`
	JournalOnly map[string]string `json:"journal_failures,omitempty"`
}

func summarizeCommand(args []string, stdout io.Writer) error {
	fsFlags := flag.NewFlagSet("summarize", flag.ContinueOnError)
	store := fsFlags.String("store", "", "aegis store directory")
	derived := fsFlags.String("derived", "", "aegis collect output directory")
	label := fsFlags.String("label", "candidate", "label for this run")
	baseStore := fsFlags.String("baseline-store", "", "baseline aegis store directory")
	baseDerived := fsFlags.String("baseline-derived", "", "baseline aegis collect output directory")
	baseLabel := fsFlags.String("baseline-label", "baseline", "label for the baseline run")
	jsonOut := fsFlags.String("json", "", "also write the summary as JSON to this file")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}
	if *store == "" {
		return xerrors.New("--store is required")
	}
	cand, err := summarizeRun(*store, *derived, *label)
	if err != nil {
		return err
	}
	out := map[string]*runSummary{"candidate": cand}
	var base *runSummary
	if *baseStore != "" {
		base, err = summarizeRun(*baseStore, *baseDerived, *baseLabel)
		if err != nil {
			return xerrors.Errorf("baseline: %w", err)
		}
		out["baseline"] = base
	}
	writeRunText(stdout, cand)
	if base != nil {
		_, _ = fmt.Fprintln(stdout)
		writeRunText(stdout, base)
		_, _ = fmt.Fprintln(stdout)
		writeComparison(stdout, cand, base)
	}
	if *jsonOut != "" {
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func summarizeRun(store, derived, label string) (*runSummary, error) {
	bundles, err := filepath.Glob(filepath.Join(store, "bundles", "*", "*", "bundle.tar.zst"))
	if err != nil {
		return nil, err
	}
	sort.Strings(bundles)
	s := &runSummary{
		Label:       label,
		Completions: map[string]int{},
		ByArm:       map[string]rate{},
		ByCase:      map[string]rate{},
		Graders:     map[string]rate{},
		JournalOnly: map[string]string{},
	}
	landed := map[string]bool{}
	tools := map[string]*toolStat{}
	for _, b := range bundles {
		t, err := summarizeTrial(b, derived)
		if err != nil {
			return nil, err
		}
		landed[t.TrialID] = true
		s.TrialsList = append(s.TrialsList, t)
		for name, st := range t.Tools {
			agg, ok := tools[name]
			if !ok {
				agg = &toolStat{Kinds: map[string]int{}}
				tools[name] = agg
			}
			agg.Calls += st.Calls
			agg.Failures += st.Failures
			for k, v := range st.Kinds {
				agg.Kinds[k] += v
			}
		}
	}
	journals, _ := filepath.Glob(filepath.Join(store, "bundles", "*", "journal.json"))
	for _, j := range journals {
		readJournalFailures(j, landed, s.JournalOnly)
	}
	armPass, armTotal := map[string]int{}, map[string]int{}
	casePass, caseTotal := map[string]int{}, map[string]int{}
	gPass, gTotal := map[string]int{}, map[string]int{}
	passed, graded := 0, 0
	for _, t := range s.TrialsList {
		s.Trials++
		s.Completions[t.Completion]++
		s.CostUSD += t.CostUSD
		s.WallMS += t.WallMS
		if t.Completion != completionDone {
			s.Failures = append(s.Failures, t)
		}
		if !t.Graded {
			s.Ungraded++
			continue
		}
		graded++
		armTotal[t.VariantID]++
		caseKey := t.TaskID
		caseTotal[caseKey]++
		if t.TaskSuccess {
			passed++
			armPass[t.VariantID]++
			casePass[caseKey]++
		}
		for g, ok := range t.Graders {
			gTotal[g]++
			if ok {
				gPass[g]++
			}
		}
	}
	s.Trials += len(s.JournalOnly)
	for arm, n := range armTotal {
		s.ByArm[arm] = newRate(armPass[arm], n)
	}
	for c, n := range caseTotal {
		s.ByCase[c] = newRate(casePass[c], n)
	}
	for g, n := range gTotal {
		s.Graders[g] = newRate(gPass[g], n)
	}
	s.Overall = newRate(passed, graded)
	for name, st := range tools {
		row := toolRow{Tool: name, Calls: st.Calls, Failures: st.Failures, Kinds: st.Kinds}
		if st.Calls > 0 {
			row.Rate = float64(st.Failures) / float64(st.Calls)
		}
		s.Tools = append(s.Tools, row)
	}
	sort.Slice(s.Tools, func(i, j int) bool { return s.Tools[i].Tool < s.Tools[j].Tool })
	return s, nil
}

func summarizeTrial(bundlePath, derived string) (trialSummary, error) {
	members, err := readTarZstMembers(bundlePath, trialFile, "run.json", transcriptFile, "stdout.log")
	if err != nil {
		return trialSummary{}, err
	}
	var trial struct {
		TrialID     string `json:"trial_id"`
		TaskID      string `json:"task_id"`
		VariantID   string `json:"variant_id"`
		UsageTotals struct {
			CostUSD float64 `json:"cost_usd"`
		} `json:"usage_totals"`
		Outcome struct {
			Completion  string `json:"completion"`
			WallClockMS int64  `json:"wall_clock_ms"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(members[trialFile], &trial); err != nil {
		return trialSummary{}, xerrors.Errorf("%s: trial.json: %w", bundlePath, err)
	}
	t := trialSummary{
		TrialID:    trial.TrialID,
		TaskID:     trial.TaskID,
		VariantID:  trial.VariantID,
		Completion: trial.Outcome.Completion,
		CostUSD:    trial.UsageTotals.CostUSD,
		WallMS:     trial.Outcome.WallClockMS,
	}
	if t.TrialID == "" {
		t.TrialID = filepath.Base(filepath.Dir(bundlePath))
	}
	if t.Completion != completionDone {
		t.FailReason = lastErrorLine(members["stdout.log"])
	}
	var tr *transcript
	if data, ok := members[transcriptFile]; ok {
		tr, err = parseTranscript(data)
	} else {
		tr, err = parseTrialToolCalls(members[trialFile])
	}
	if err != nil {
		return trialSummary{}, xerrors.Errorf("%s: %w", bundlePath, err)
	}
	t.Tools = toolStats(tr.Calls)
	if derived != "" {
		if err := applyMeasurements(&t, filepath.Join(derived, t.TrialID, "measurements.json")); err != nil {
			return trialSummary{}, err
		}
	}
	return t, nil
}

// applyMeasurements reads a collect output. Task success follows aegis:
// every grader that is neither a guardrail nor a metric must pass.
func applyMeasurements(t *trialSummary, path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var m struct {
		Graders map[string]struct {
			Passed    bool `json:"passed"`
			Guardrail bool `json:"guardrail"`
			Metric    bool `json:"metric"`
		} `json:"graders"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return xerrors.Errorf("parse %s: %w", path, err)
	}
	t.Graded = true
	t.Graders = map[string]bool{}
	success, primary := true, 0
	for id, g := range m.Graders {
		t.Graders[id] = g.Passed
		if g.Guardrail || g.Metric {
			continue
		}
		primary++
		success = success && g.Passed
	}
	t.TaskSuccess = success && primary > 0
	return nil
}

func lastErrorLine(log []byte) string {
	var last string
	scanner := bufio.NewScanner(bytes.NewReader(log))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fail") {
			last = strings.TrimSpace(line)
		}
	}
	return truncate(last, 300)
}

func readJournalFailures(path string, landed map[string]bool, out map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var j struct {
		Entries []struct {
			TrialID string `json:"trial_id"`
			Status  string `json:"status"`
			Error   string `json:"error"`
		} `json:"entries"`
	}
	if json.Unmarshal(data, &j) != nil {
		return
	}
	for _, e := range j.Entries {
		if landed[e.TrialID] || e.Status == "completed" || e.Status == "skipped" {
			continue
		}
		out[e.TrialID] = strings.TrimSpace(e.Status + ": " + truncate(e.Error, 300))
	}
}

func pct(r rate) string {
	if r.Total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d %5.1f%% [%.0f-%.0f%%]", r.Passed, r.Total, 100*r.Rate, 100*r.Low, 100*r.High)
}

func writeRunText(w io.Writer, s *runSummary) {
	_, _ = fmt.Fprintf(w, "== %s: %d trial(s)", s.Label, s.Trials)
	var comps []string
	for c, n := range s.Completions {
		comps = append(comps, fmt.Sprintf("%s %d", c, n))
	}
	sort.Strings(comps)
	if len(comps) > 0 {
		_, _ = fmt.Fprintf(w, " (%s)", strings.Join(comps, ", "))
	}
	_, _ = fmt.Fprintln(w)
	if len(s.Failures) > 0 || len(s.JournalOnly) > 0 {
		_, _ = fmt.Fprintln(w, "Failed trials (not DONE):")
		for _, f := range s.Failures {
			_, _ = fmt.Fprintf(w, "  %s: %s %s\n", f.TrialID, f.Completion, f.FailReason)
		}
		ids := sortedKeys(s.JournalOnly)
		for _, id := range ids {
			_, _ = fmt.Fprintf(w, "  %s: %s (no bundle)\n", id, s.JournalOnly[id])
		}
	}
	if s.Ungraded > 0 {
		_, _ = fmt.Fprintf(w, "Ungraded trials (no measurements.json): %d\n", s.Ungraded)
	}
	_, _ = fmt.Fprintf(w, "Task success: %s\n", pct(s.Overall))
	if len(s.ByArm) > 1 {
		for _, arm := range sortedKeys(s.ByArm) {
			_, _ = fmt.Fprintf(w, "  arm %-24s %s\n", arm, pct(s.ByArm[arm]))
		}
	}
	if len(s.ByCase) > 0 {
		_, _ = fmt.Fprintln(w, "Per case:")
		for _, c := range sortedKeys(s.ByCase) {
			_, _ = fmt.Fprintf(w, "  %-36s %s\n", c, pct(s.ByCase[c]))
		}
	}
	if len(s.Graders) > 0 {
		_, _ = fmt.Fprintln(w, "Grader pass rates:")
		for _, g := range sortedKeys(s.Graders) {
			_, _ = fmt.Fprintf(w, "  %-36s %s\n", g, pct(s.Graders[g]))
		}
	}
	if len(s.Tools) > 0 {
		_, _ = fmt.Fprintf(w, "Tools:\n  %-22s %6s %8s %7s  %s\n", "tool", "calls", "failures", "rate", "failure kinds")
		for _, r := range s.Tools {
			_, _ = fmt.Fprintf(w, "  %-22s %6d %8d %6.1f%%  %s\n", r.Tool, r.Calls, r.Failures, 100*r.Rate, formatKinds(r.Kinds))
		}
	}
	mean := time.Duration(0)
	if len(s.TrialsList) > 0 {
		mean = time.Duration(s.WallMS/int64(len(s.TrialsList))) * time.Millisecond
	}
	_, _ = fmt.Fprintf(w, "Cost: $%.4f total; wall time %s total, %s mean per trial\n",
		s.CostUSD, (time.Duration(s.WallMS) * time.Millisecond).Round(time.Second), mean.Round(time.Second))
}

func formatKinds(kinds map[string]int) string {
	var parts []string
	for _, k := range sortedKeys(kinds) {
		parts = append(parts, fmt.Sprintf("%s=%d", k, kinds[k]))
	}
	return strings.Join(parts, " ")
}

func writeComparison(w io.Writer, cand, base *runSummary) {
	_, _ = fmt.Fprintf(w, "== %s vs %s\n", cand.Label, base.Label)
	// Labels name the builds, which can be wider than the rate columns.
	width := max(28, len(cand.Label), len(base.Label))
	_, _ = fmt.Fprintf(w, "  %-36s %-*s %-*s\n", "case", width, cand.Label, width, base.Label)
	cases := map[string]bool{}
	for c := range cand.ByCase {
		cases[c] = true
	}
	for c := range base.ByCase {
		cases[c] = true
	}
	for _, c := range sortedKeys(cases) {
		_, _ = fmt.Fprintf(w, "  %-36s %-*s %-*s\n", c, width, pct(cand.ByCase[c]), width, pct(base.ByCase[c]))
	}
	_, _ = fmt.Fprintf(w, "  %-36s %-*s %-*s\n", "overall", width, pct(cand.Overall), width, pct(base.Overall))
	_, _ = fmt.Fprintf(w, "  %-36s $%-*.4f $%-*.4f\n", "cost", width-1, cand.CostUSD, width-1, base.CostUSD)
	_, _ = fmt.Fprintf(w, "Tool failure rates (%s vs %s):\n", cand.Label, base.Label)
	rows := map[string][2]toolRow{}
	for _, r := range cand.Tools {
		pair := rows[r.Tool]
		pair[0] = r
		rows[r.Tool] = pair
	}
	for _, r := range base.Tools {
		pair := rows[r.Tool]
		pair[1] = r
		rows[r.Tool] = pair
	}
	for _, name := range sortedKeys(rows) {
		pair := rows[name]
		_, _ = fmt.Fprintf(w, "  %-22s %4d/%-5d %6.1f%%   %4d/%-5d %6.1f%%\n", name,
			pair[0].Failures, pair[0].Calls, 100*pair[0].Rate, pair[1].Failures, pair[1].Calls, 100*pair[1].Rate)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
