package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// Annotations is the typed decode result of an annotate policy: a flat,
// semantic annotation map (e.g. {"risk_score": 0.9}). It projects into a
// StageResult with Values nested under the producing stage's namespace.
type Annotations struct {
	// Values is the annotation object the policy emitted, before namespace
	// stamping.
	Values map[string]any
}

// Project maps the annotations into a StageResult with the raw emitted Values.
// The host-owned namespace ({stage_name: values}) is applied by Resolve, not
// here, so a stage cannot choose, omit, or spoof its namespace.
func (a Annotations) Project() StageResult {
	return StageResult{Annotations: a.Values}
}

// Annotate attaches metadata. It evaluates data.gateway.annotations and returns
// an annotation map to be merged into input.annotations for later stages.
//
// Renamed from the former "classify" kind: the contract is "emit annotations",
// matching its annotations entrypoint rule.
type Annotate struct {
	prepared preparedQuery
	failMode FailMode
	name     string
}

func NewAnnotate(name, module string, opts ...Option) (*Annotate, error) {
	o := newOptions(opts...)
	pq, err := prepare(module, ruleQuery("annotations"))
	if err != nil {
		return nil, err
	}
	return &Annotate{prepared: pq, failMode: o.failMode, name: name}, nil
}

// Name implements Stage.
func (a *Annotate) Name() string { return a.name }

// Evaluate decodes the annotations rule and projects it into a StageResult,
// stamping the host-owned namespace (the stage's immutable name) at projection.
// A failure is synthesized through the stage's fail mode.
func (a *Annotate) Evaluate(ctx context.Context, in Input) StageResult {
	return runStage(ctx, a.name, a.failMode, func(sctx context.Context) (Projector, error) {
		ann, ok, err := a.annotations(sctx, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			return noop{}, nil
		}
		return ann, nil
	})
}

// annotations decodes data.gateway.annotations. ok is false when the rule is
// undefined.
func (a *Annotate) annotations(ctx context.Context, in Input) (Annotations, bool, error) {
	val, ok, err := evalSingle(ctx, a.prepared, in)
	if err != nil {
		return Annotations{}, false, err
	}
	if !ok {
		return Annotations{}, false, nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return Annotations{}, false, xerrors.Errorf("annotations is %T, want object", val)
	}
	return Annotations{Values: m}, true, nil
}
