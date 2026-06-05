package policy

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"
)

// Transform rewrites the request body. It evaluates data.gateway.body and
// returns the whole replacement body as JSON bytes; the host re-validates by
// re-parsing it downstream. ok is false when the rule is undefined.
type Transform struct {
	prepared preparedQuery
	failMode FailMode
	name     string
}

func NewTransform(name, module string, opts ...Option) (*Transform, error) {
	o := newOptions(opts...)
	pq, err := prepare(module, ruleQuery("body"))
	if err != nil {
		return nil, err
	}
	return &Transform{prepared: pq, failMode: o.failMode, name: name}, nil
}

func (t *Transform) Evaluate(ctx context.Context, in Input) ([]byte, bool, error) {
	val, ok, err := evalSingle(ctx, t.prepared, in)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	body, err := json.Marshal(val)
	if err != nil {
		return nil, false, xerrors.Errorf("encode transformed body: %w", err)
	}
	return body, true, nil
}
