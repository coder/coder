package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

const graderAPIVersion = "aegis.grader.v1"

// maxReasonBytes keeps verdict reasons readable in aegis reports.
const maxReasonBytes = 2000

type graderRequest struct {
	APIVersion string `json:"api_version"`
	Task       struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags"`
	} `json:"task"`
	Trial struct {
		ID           string `json:"id"`
		ExperimentID string `json:"experiment_id"`
		VariantID    string `json:"variant_id"`
		RepeatIndex  int    `json:"repeat_index"`
	} `json:"trial"`
	Paths struct {
		BundleDir    string `json:"bundle_dir"`
		WorkspaceDir string `json:"workspace_dir"`
		TracePath    string `json:"trace_path"`
	} `json:"paths"`
	Grader struct {
		ID       string                     `json:"id"`
		Type     string                     `json:"type"`
		Args     map[string]json.RawMessage `json:"args"`
		TimeoutS int                        `json:"timeout_s"`
	} `json:"grader"`
}

type graderVerdict struct {
	Passed  bool           `json:"passed"`
	Score   float64        `json:"score"`
	Reason  string         `json:"reason"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

// grade answers one aegis.grader.v1 request. Malformed requests and
// unsupported protocol versions exit non-zero without a verdict so aegis
// records a protocol error instead of an ordinary failed assertion.
func grade(stdin io.Reader, stdout, stderr io.Writer) int {
	req, err := decodeRequest(stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chatd-eval-grader: %v\n", err)
		return 2
	}
	mode, err := stringArg(req.Grader.Args, "mode")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chatd-eval-grader: %v\n", err)
		return 2
	}
	var v graderVerdict
	switch mode {
	case "", "checks":
		v, err = gradeChecks(req)
	case "terminal_bench":
		v, err = gradeTerminalBench(req, stderr)
	default:
		err = xerrors.Errorf("unknown grader mode %q", mode)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chatd-eval-grader: %v\n", err)
		return 2
	}
	v.Reason = truncate(v.Reason, maxReasonBytes)
	if v.Passed {
		v.Score = 1
	}
	if err := json.NewEncoder(stdout).Encode(v); err != nil {
		_, _ = fmt.Fprintf(stderr, "chatd-eval-grader: write verdict: %v\n", err)
		return 1
	}
	return 0
}

func decodeRequest(r io.Reader) (graderRequest, error) {
	var req graderRequest
	dec := json.NewDecoder(io.LimitReader(r, 16<<20))
	if err := dec.Decode(&req); err != nil {
		return req, xerrors.Errorf("decode request: %w", err)
	}
	if req.APIVersion != graderAPIVersion {
		return req, xerrors.Errorf("unsupported api_version %q, want %q", req.APIVersion, graderAPIVersion)
	}
	if req.Paths.BundleDir == "" {
		return req, xerrors.New("request has no paths.bundle_dir")
	}
	return req, nil
}

// stringArg reads an optional string argument. Registry parameter
// substitution always produces strings, so other JSON types are errors.
func stringArg(args map[string]json.RawMessage, key string) (string, error) {
	raw, ok := args[key]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", xerrors.Errorf("grader arg %q must be a string: %w", key, err)
	}
	return s, nil
}

func gradeChecks(req graderRequest) (graderVerdict, error) {
	specs, err := parseChecks(req.Grader.Args["checks"])
	if err != nil {
		return graderVerdict{}, err
	}
	in, err := loadGradeInput(req.Paths.BundleDir, req.Paths.TracePath)
	if err != nil {
		return graderVerdict{
			Passed: false,
			Reason: "cannot load trial evidence: " + err.Error(),
		}, nil
	}
	results := make([]checkResult, 0, len(specs))
	for _, spec := range specs {
		results = append(results, runCheck(in, spec))
	}
	return summarizeResults(in, results), nil
}

func summarizeResults(in *gradeInput, results []checkResult) graderVerdict {
	metrics := map[string]any{}
	var failed []string
	required, passed := 0, 0
	for _, r := range results {
		metrics["check."+r.ID] = boolMetric(r.Passed)
		for k, v := range r.Metrics {
			metrics[r.ID+"."+k] = v
		}
		if r.Observe {
			continue
		}
		required++
		if r.Passed {
			passed++
			continue
		}
		failed = append(failed, r.ID+": "+r.Reason)
	}
	for name, st := range toolStats(in.tr.Calls) {
		metrics["tool."+name+".calls"] = st.Calls
		metrics["tool."+name+".failures"] = st.Failures
	}
	metrics["tool_calls"] = len(in.tr.Calls)
	metrics["transcript_agent_native"] = boolMetric(in.tr.Source == sourceAgentNative)

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%d/%d required checks passed", passed, required)
	if len(failed) > 0 {
		_, _ = b.WriteString("; failed: ")
		_, _ = b.WriteString(strings.Join(failed, "; "))
	}
	var observed []string
	for _, r := range results {
		if r.Observe && !r.Passed {
			observed = append(observed, r.ID)
		}
	}
	if len(observed) > 0 {
		sort.Strings(observed)
		_, _ = b.WriteString("; observe-only misses: ")
		_, _ = b.WriteString(strings.Join(observed, ", "))
	}
	for _, note := range in.tr.Notes {
		_, _ = b.WriteString("; note: ")
		_, _ = b.WriteString(note)
	}
	return graderVerdict{
		Passed:  len(failed) == 0,
		Reason:  b.String(),
		Metrics: metrics,
	}
}

// boolMetric encodes a check outcome as a 0 or 1 metric for aegis.
//
//nolint:revive // v is the value being converted, not a control flag.
func boolMetric(v bool) int {
	if v {
		return 1
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
