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
// GuardrailOutcome; each of those implements Projector, and Resolve is the sole
// way a StageResult is built (a failure is just the Failure Projector, a no-op
// the noop Projector). The effect mask is therefore enforced by construction (a
// Decision has no Edits field, so its Project cannot populate one). The
// annotation namespace is host-stamped by Resolve from the member's immutable
// name, never by the Projector itself, so a stage cannot choose, omit, or spoof
// its namespace.
type StageResult struct {
	// Verdict is the stage's outcome (ALLOW, LOG, or BLOCK). The zero value is
	// treated as ALLOW.
	Verdict Verdict
	// Message is surfaced to the user on a BLOCK. It is only meaningful for a
	// deliberate (non-synthesized) block.
	Message string
	// Annotations are the stage's output. As returned by Project they are the
	// raw values; Resolve nests them under the stage's namespace
	// ({stage_name: values}) before the reducer unions them across stages.
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
// kind results (Decision, Annotations, RouteChanges, Transformation), the
// multi-effect guardrail result (GuardrailOutcome), and the Failure/noop paths.
// Project is the single, declared mapping of a typed output into a StageResult;
// a stage's Evaluate decodes its Rego (or network) output into one of these
// values and projects it rather than building a StageResult inline. The
// implementing type's field set is the effect mask: a Decision has no Edits
// field, so its Project physically cannot populate one.
//
// Project deliberately has no access to the stage's name: it emits *raw*
// annotations (the flat values the stage authored) and never stamps a
// namespace. Host identity, the annotation namespace and the audit-only
// Err.Stage, is applied solely by Resolve, so a stage cannot choose, omit, or
// spoof its namespace. Always obtain a StageResult via Resolve, never by calling
// Project directly.
type Projector interface {
	Project() StageResult
}

// Resolve projects p and stamps the host-owned stage identity onto the result:
// it nests the stage's raw annotations under name (the annotation namespace) and
// labels any audit-only failure record with name. It is the single site that
// writes a stage's name into a StageResult, so namespacing is enforced by
// construction rather than trusted to each Projector. name is the producer's
// immutable, pipeline-unique stage name.
func Resolve(name string, p Projector) StageResult {
	res := p.Project()
	if len(res.Annotations) > 0 {
		res.Annotations = map[string]any{name: res.Annotations}
	}
	if res.Err != nil {
		res.Err.Stage = name
	}
	return res
}

// GuardrailOutcome is the multi-effect outcome of one networked guardrail,
// decoded from its adapter Result. Unlike the four hermetic kinds a guardrail
// is deliberately not single-effect: one network response may carry
// annotations, a block, and body edits at once. A guardrail's authority is
// intrinsic to what its adapter returns (there is no advisory/enforcing mode): a
// scanner that only annotates simply leaves Block false and Edits empty, and a
// downstream decide turns its annotation into a verdict. The guardrail package
// builds this value and passes it to Resolve, so it never constructs or
// namespaces a StageResult itself.
type GuardrailOutcome struct {
	// Annotations is the guardrail's classifier output (raw values; Resolve
	// stamps the guardrail's namespace).
	Annotations map[string]any
	// Block requests an HTTP 400.
	Block bool
	// Message explains a block, surfaced to the user and audit; meaningful only
	// when Block.
	Message string
	// Edits rewrite the request body (masking/redaction).
	Edits []Edit
}

// Project maps the guardrail outcome into a StageResult with raw (un-namespaced)
// annotations; Resolve stamps the guardrail's namespace. A block sets the
// verdict and message. Edits are carried even on a block (the reducer drops
// them, since a blocked request is never forwarded).
func (g GuardrailOutcome) Project() StageResult {
	res := StageResult{Annotations: g.Annotations}
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
// audit-only Err field; Resolve labels it with the stage's identity, which
// never reaches the client-facing message. Modeling a failure as a Projector
// lets it flow through the same Project path as a success, so Resolve is the
// single way any StageResult is built.
type Failure struct {
	FailMode FailMode
	Err      error
}

// Project implements Projector for the failure path. The Err record is left
// unattributed; Resolve stamps Err.Stage from the immutable stage name.
func (f Failure) Project() StageResult {
	se := &StageErr{Err: f.Err}
	if f.FailMode == FailClosed {
		return StageResult{Verdict: VerdictBlock, Err: se}
	}
	return StageResult{Verdict: VerdictLog, Err: se}
}

// noop is the Projector for a stage whose entrypoint rule was undefined: it
// produced no effect, so it projects to the zero StageResult (ALLOW, no
// annotations, edits, or route). It keeps "every StageResult comes from Resolve"
// true even for the no-op case.
type noop struct{}

// Project implements Projector for the no-op path.
func (noop) Project() StageResult { return StageResult{} }

// runStage evaluates fn under the per-stage timeout and resolves its outcome
// under the stage's immutable name. It is the single stage boundary every
// kind's Evaluate funnels through: fn decodes the Rego output into a Projector
// (a typed kind result, or noop when the entrypoint rule is undefined), and any
// error or fired timeout replaces it with a Failure Projector, so success and
// failure alike become a StageResult via the same Resolve(name, ...) call.
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
	return Resolve(name, p)
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
