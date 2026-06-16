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
// guardrail) yields. Stages never construct it freely: each kind decodes its
// Rego output into a typed per-kind struct (Decision, Annotations, RouteChanges,
// Transformation) and the guardrail decodes its network response into a
// GuardrailOutcome; each of those implements Projector, and Project is the sole
// way a StageResult is built (a failure is just the Failure Projector, a no-op
// the noop Projector). The effect mask is therefore enforced by construction (a
// Decision has no Edits field, so its Project cannot populate one). The
// annotation namespace is host-stamped at
// projection from the member's immutable name, so a stage cannot write into or
// spoof another stage's namespace.
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

// Projector is implemented by every typed stage output: the four single-effect
// kind results (Decision, Annotations, RouteChanges, Transformation) and the
// multi-effect guardrail result (GuardrailOutcome). Project is the single,
// declared mapping into a StageResult; a stage's Evaluate decodes its Rego (or
// network) output into one of these values and calls Project rather than
// building a StageResult inline. The implementing type's field set is the
// effect mask: a Decision has no Edits field, so its Project physically cannot
// populate one. stage is the producer's immutable name; it is stamped onto the
// annotation namespace (and the audit-only failure record) here and only here,
// so a stage cannot spoof another stage's namespace. Failures and no-ops flow
// through Project too, via the Failure and noop Projectors, so projection is the
// single way any StageResult is built.
type Projector interface {
	Project(stage string) StageResult
}

// GuardrailOutcome is the multi-effect outcome of one networked guardrail,
// decoded from its adapter Result. Unlike the four hermetic kinds a guardrail
// is deliberately not single-effect: one network response may carry
// annotations, a block, and body edits at once. Enforcing is the per-membership
// effect mask, applied at projection: an advisory guardrail contributes
// annotations only, its block and edits discarded. The guardrail package builds
// this value and calls Project, so it never constructs a StageResult itself.
type GuardrailOutcome struct {
	// Annotations is the guardrail's classifier output, stamped under the
	// guardrail's namespace at projection (advisory and enforcing alike).
	Annotations map[string]any
	// Enforcing reports whether the guardrail may block and rewrite the body on
	// its own authority. When false, Block and Edits are dropped.
	Enforcing bool
	// Block requests an HTTP 400; honored only when Enforcing.
	Block bool
	// Message explains a block, surfaced to the user and audit; meaningful only
	// when Block and Enforcing.
	Message string
	// Edits rewrite the request body (masking/redaction); honored only when
	// Enforcing.
	Edits []Edit
}

// Project maps the guardrail outcome into a StageResult under its effect mask.
// Annotations are always stamped under stage's namespace; the block verdict and
// edits are kept only for an enforcing guardrail. Edits are carried even on a
// block (the reducer drops them, since a blocked request is never forwarded).
func (g GuardrailOutcome) Project(stage string) StageResult {
	res := StageResult{}
	if len(g.Annotations) > 0 {
		res.Annotations = map[string]any{stage: g.Annotations}
	}
	if !g.Enforcing {
		return res
	}
	if g.Block {
		res.Verdict = VerdictBlock
		res.Message = g.Message
	}
	res.Edits = append(res.Edits, g.Edits...)
	return res
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
// ordinary StageResult through the stage's fail mode (see Failure).
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

// Failure is the Projector for a stage that could not produce a result: an eval
// error, network error, decode failure, or the per-stage timeout. It normalizes
// the failure through the stage's fail mode at projection: fail-closed blocks
// with a generic (empty) message; fail-open logs (LOG, not ALLOW, so a fail-open
// outage is visible in the log stream, not silent). The error rides the
// audit-only Err field under stage's identity; that identity never reaches the
// client-facing message. Modeling a failure as a Projector lets it flow through
// the same Project(stage) path as a success, so projection is the single way
// any StageResult is built.
type Failure struct {
	FailMode FailMode
	Err      error
}

// Project implements Projector for the failure path.
func (f Failure) Project(stage string) StageResult {
	se := &StageErr{Stage: stage, Err: f.Err}
	if f.FailMode == FailClosed {
		return StageResult{Verdict: VerdictBlock, Err: se}
	}
	return StageResult{Verdict: VerdictLog, Err: se}
}

// noop is the Projector for a stage whose entrypoint rule was undefined: it
// produced no effect, so it projects to the zero StageResult (ALLOW, no
// annotations, edits, or route). It keeps "every StageResult comes from Project"
// true even for the no-op case.
type noop struct{}

// Project implements Projector for the no-op path.
func (noop) Project(string) StageResult { return StageResult{} }

// runStage evaluates fn under the per-stage timeout and projects its outcome
// through the stage's immutable name. It is the single stage boundary every
// kind's Evaluate funnels through: fn decodes the Rego output into a Projector
// (a typed kind result, or noop when the entrypoint rule is undefined), and any
// error or fired timeout replaces it with a Failure Projector, so success and
// failure alike become a StageResult via the same Project(name) call.
func runStage(ctx context.Context, name string, fm FailMode, fn func(context.Context) (Projector, error)) StageResult {
	sctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	p, err := fn(sctx)
	switch {
	case err != nil:
		p = Failure{FailMode: fm, Err: err}
	// A fired deadline/cancellation means the result is not trustworthy even
	// when the evaluator returned no error (a trivial Rego rule can complete
	// without observing the cancelled context). Normalize it like any other
	// failure so the timeout/cancellation flows through fail_mode uniformly.
	case sctx.Err() != nil:
		p = Failure{FailMode: fm, Err: sctx.Err()}
	}
	return p.Project(name)
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
