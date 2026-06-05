package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// Classify attaches metadata. It evaluates data.gateway.annotations and returns
// a flat, semantic annotation map (e.g. {"risk_score": 0.9}) to be merged into
// input.annotations for later stages. ok is false when the rule is undefined.
type Classify struct {
	prepared preparedQuery
	failMode FailMode
	name     string
}

func NewClassify(name, module string, opts ...Option) (*Classify, error) {
	o := newOptions(opts...)
	pq, err := prepare(module, ruleQuery("annotations"))
	if err != nil {
		return nil, err
	}
	return &Classify{prepared: pq, failMode: o.failMode, name: name}, nil
}

func (c *Classify) Evaluate(ctx context.Context, in Input) (map[string]any, bool, error) {
	val, ok, err := evalSingle(ctx, c.prepared, in)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil, false, xerrors.Errorf("annotations is %T, want object", val)
	}
	return m, true, nil
}
