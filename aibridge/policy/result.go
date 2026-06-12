package policy

import (
	"context"
	"encoding/json"
	"maps"
	"time"

	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

// evalTimeout bounds a single stage evaluation, so a pathological policy cannot
// hang a request. A stage that exceeds it is treated as an ordinary stage error
// and normalized through the stage's fail mode by synthesize: fail-closed times
// out to BLOCK, fail-open to LOG. There is no special case for timeout; an
// attacker-induced timeout bypassing a fail-open stage is what fail-open means.
const evalTimeout = time.Second

// StageResult is the single result type every stage (Rego policy or networked
// guardrail) yields. Stages do not construct it freely: each kind decodes its
// Rego output into a typed per-kind struct (Decision, Annotations, RouteChanges,
// Transformation) and that struct projects into a StageResult, so the effect
// mask is enforced by construction (a Decision cannot carry an edit). The
// annotation namespace is host-stamped at projection from the member's
// immutable name, so a stage cannot write into or spoof another stage's
// namespace.
type StageResult struct {
	// Verdict is the stage's outcome (ALLOW, LOG, or BLOCK). The zero value is
	// treated as ALLOW.
	Verdict Verdict
	// Message is surfaced to the user on a BLOCK. It is only meaningful for a
	// deliberate (non-synthesized) block.
	Message string
	// Annotations are the stage's output, already nested under the stage's
	// namespace ({stage_name: values}). The reducer unions them across stages.
	Annotations map[string]any
	// Edits are body mutations applied as an ordered chain (whole-body rewrite
	// is the degenerate root edit, Pointer == "").
	Edits []Edit
	// Headers are outgoing request header overrides (transform only).
	Headers map[string]string
	// Route is a model override (route only; pre-req only).
	Route string
	// Err is the audit-only failure record for a synthesized result. It never
	// reaches the client and rides through the reducer for host logging.
	Err *StageErr
}

// Edit is a single body mutation: set the JSON value at Pointer to Value. A
// root edit (Pointer == "") replaces the whole body and is how a transform's
// whole-body rewrite is represented; a non-root edit is an sjson path.
type Edit struct {
	Pointer string
	Value   any
}

// StageErr is the single audit-only failure record for a stage (it collapses
// the former StageError/GuardrailError twins). The failing stage's identity is
// for audit/logs only and never reaches the client-facing message.
type StageErr struct {
	Stage string
	Err   error
}

// Stage is the uniform interface every pipeline member implements. Evaluate
// never returns an error: an evaluation/decode failure is normalized into an
// ordinary StageResult through the stage's fail mode (see synthesize).
type Stage interface {
	Name() string
	Evaluate(ctx context.Context, in Input) StageResult
}

// StageOutcome pairs a stage's name with the result it produced, for the
// reducer's block attribution.
type StageOutcome struct {
	Name   string
	Result StageResult
}

// Reduced is the combined non-body outcome of a sequence of StageResults. Body
// edits are applied separately (ApplyEdits) so a sequential caller can thread
// edits between stages.
type Reduced struct {
	Verdict Verdict
	// BlockedBy is the name of the deliberately-blocking stage, or empty when
	// the verdict is not BLOCK or the block was synthesized from a failure (a
	// failure's stage identity is audit-only, never client-facing).
	BlockedBy string
	// Message is the deliberately-blocking stage's author-supplied message.
	Message     string
	Annotations map[string]any
	Errors      []StageErr
}

// Reduce folds an ordered sequence of stage outcomes under the rule "BLOCK
// freezes effects, never erases observations": the verdict reduces by
// BLOCK > LOG > ALLOW; the first blocking outcome sets BlockedBy/Message (only
// when it is a deliberate block, not a synthesized failure); annotations and
// errors from every outcome that ran are always retained. It is the single
// reducer shared by the policy pipeline and the guardrail stage.
func Reduce(outcomes []StageOutcome) Reduced {
	out := Reduced{Verdict: VerdictAllow}
	verdicts := make([]Verdict, 0, len(outcomes))
	blocked := false
	for _, o := range outcomes {
		r := o.Result
		verdicts = append(verdicts, r.Verdict)
		// Observations are always kept, even after a BLOCK and even from the
		// blocking stage itself (audit must see every signal).
		out.Annotations = mergeAnnotations(out.Annotations, r.Annotations)
		if r.Err != nil {
			out.Errors = append(out.Errors, *r.Err)
		}
		// The first blocking outcome wins attribution; a synthesized failure
		// block (Err != nil) is intentionally anonymous to the client.
		if !blocked && r.Verdict.Blocks() {
			blocked = true
			if r.Err == nil {
				out.BlockedBy = o.Name
				out.Message = r.Message
			}
		}
	}
	out.Verdict = ReduceVerdicts(verdicts...)
	return out
}

// ApplyEdits applies an ordered chain of edits to body and reports whether
// anything changed. A root edit (Pointer == "") replaces the whole body; a
// non-root edit sets an sjson path. It is the single applier shared by the
// transform stage and the guardrail masking chain.
func ApplyEdits(body []byte, edits []Edit) ([]byte, bool, error) {
	out := body
	mutated := false
	for _, e := range edits {
		if e.Pointer == "" {
			b, err := json.Marshal(e.Value)
			if err != nil {
				return nil, false, xerrors.Errorf("encode root edit: %w", err)
			}
			out = b
			mutated = true
			continue
		}
		b, err := sjson.SetBytes(out, e.Pointer, e.Value)
		if err != nil {
			return nil, false, xerrors.Errorf("apply edit %q: %w", e.Pointer, err)
		}
		out = b
		mutated = true
	}
	return out, mutated, nil
}

// Synthesize normalizes a stage failure (eval error, network error, decode
// failure, or the evaluation timeout) into an ordinary StageResult through the
// stage's fail mode: fail-closed blocks with a generic (empty) message;
// fail-open logs (LOG, not ALLOW, so a fail-open outage is visible in the log
// stream, not silent). The error rides the audit-only Err field; the failing
// stage's identity never reaches the client-facing message.
func Synthesize(stage string, fm FailMode, err error) StageResult {
	se := &StageErr{Stage: stage, Err: err}
	if fm == FailClosed {
		return StageResult{Verdict: VerdictBlock, Err: se}
	}
	return StageResult{Verdict: VerdictLog, Err: se}
}

// runStage evaluates fn under the per-stage timeout and normalizes any error
// into a StageResult via Synthesize. It is the single stage boundary every
// kind's Evaluate funnels through.
func runStage(ctx context.Context, name string, fm FailMode, fn func(context.Context) (StageResult, error)) StageResult {
	sctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	res, err := fn(sctx)
	if err != nil {
		return Synthesize(name, fm, err)
	}
	// A fired deadline/cancellation means the result is not trustworthy even
	// when the evaluator returned no error (a trivial Rego rule can complete
	// without observing the cancelled context). Normalize it like any other
	// failure so the timeout/cancellation flows through fail_mode uniformly.
	if cerr := sctx.Err(); cerr != nil {
		return Synthesize(name, fm, cerr)
	}
	return res
}

func mergeAnnotations(dst, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	maps.Copy(dst, src)
	return dst
}
