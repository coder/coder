package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// Decision is the outcome of a decision policy: a Verdict plus an optional,
// author-supplied Message surfaced to the user when the verdict blocks.
type Decision struct {
	// Verdict is the policy's outcome (ALLOW, LOG, or BLOCK).
	Verdict Verdict
	// Message is an optional explanation the policy author may supply via a
	// `message` rule, surfaced to the user when the verdict blocks. It is empty
	// when the policy defines no message rule, the rule is undefined for this
	// input, or the verdict does not block.
	Message string
}

// Decide is a pure decision policy. It evaluates data.gateway.verdict and
// returns a Verdict. An undefined rule defaults to ALLOW (registration requires
// a default verdict, so this should not happen for stored policies); an
// unrecognized verdict value is an error (which fails closed in a pipeline).
//
// A policy may optionally define a data.gateway.message string rule to override
// the generic "request blocked by policy" message shown to the user on a BLOCK.
type Decide struct {
	verdict  preparedQuery
	message  preparedQuery
	failMode FailMode
	name     string
}

func NewDecide(name, module string, opts ...Option) (*Decide, error) {
	o := newOptions(opts...)
	vq, err := prepare(module, ruleQuery("verdict"))
	if err != nil {
		return nil, err
	}
	// The message rule is optional; preparing a query for a rule the module
	// never defines is valid and simply evaluates to undefined at eval time.
	mq, err := prepare(module, ruleQuery("message"))
	if err != nil {
		return nil, err
	}
	return &Decide{verdict: vq, message: mq, failMode: o.failMode, name: name}, nil
}

// Name implements Stage.
func (d *Decide) Name() string { return d.name }

// Evaluate decodes the verdict (and optional message) and projects it into a
// StageResult. A failure is synthesized through the stage's fail mode.
func (d *Decide) Evaluate(ctx context.Context, in Input) StageResult {
	return runStage(ctx, d.name, d.failMode, func(sctx context.Context) (StageResult, error) {
		dec, err := d.decision(sctx, in)
		if err != nil {
			return StageResult{}, err
		}
		return StageResult{Verdict: dec.Verdict, Message: dec.Message}, nil
	})
}

// decision decodes data.gateway.verdict (+ optional message).
func (d *Decide) decision(ctx context.Context, in Input) (Decision, error) {
	val, ok, err := evalSingle(ctx, d.verdict, in)
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return Decision{Verdict: VerdictAllow}, nil
	}
	s, ok := val.(string)
	if !ok {
		return Decision{}, xerrors.Errorf("verdict is %T, want string", val)
	}
	v := Verdict(s)
	switch v {
	case VerdictAllow, VerdictLog, VerdictBlock:
	default:
		return Decision{}, xerrors.Errorf("unknown verdict %q", s)
	}
	dec := Decision{Verdict: v}
	// The message only influences the error surfaced to the user on a block, so
	// it is evaluated solely on the blocking path. A missing, undefined, or
	// malformed message is ignored rather than erroring, so an author's message
	// bug cannot downgrade a deliberate block (e.g. fail-open skipping the
	// stage) or otherwise alter the verdict.
	if v.Blocks() {
		dec.Message = d.evalMessage(ctx, in)
	}
	return dec, nil
}

// evalMessage returns the author-supplied block message, or "" when the policy
// defines no message rule, the rule is undefined for this input, errors, or
// does not evaluate to a string.
func (d *Decide) evalMessage(ctx context.Context, in Input) string {
	val, ok, err := evalSingle(ctx, d.message, in)
	if err != nil || !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}
