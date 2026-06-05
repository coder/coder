package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// Route surgically overrides the upstream model. It evaluates
// data.gateway.model and returns the replacement model name. ok is false when
// the rule is undefined (no override).
type Route struct {
	prepared preparedQuery
	failMode FailMode
	name     string
}

func NewRoute(name, module string, opts ...Option) (*Route, error) {
	o := newOptions(opts...)
	pq, err := prepare(module, ruleQuery("model"))
	if err != nil {
		return nil, err
	}
	return &Route{prepared: pq, failMode: o.failMode, name: name}, nil
}

func (r *Route) Evaluate(ctx context.Context, in Input) (string, bool, error) {
	val, ok, err := evalSingle(ctx, r.prepared, in)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	s, ok := val.(string)
	if !ok {
		return "", false, xerrors.Errorf("model is %T, want string", val)
	}
	return s, true, nil
}
