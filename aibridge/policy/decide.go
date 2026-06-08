package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// Decide is a pure decision policy. It evaluates data.gateway.verdict and
// returns a Verdict. An undefined rule defaults to ALLOW (registration requires
// a default verdict, so this should not happen for stored policies); an
// unrecognized verdict value is an error (which fails closed in a pipeline).
type Decide struct {
	prepared preparedQuery
	failMode FailMode
	name     string
}

func NewDecide(name, module string, opts ...Option) (*Decide, error) {
	o := newOptions(opts...)
	pq, err := prepare(module, ruleQuery("verdict"))
	if err != nil {
		return nil, err
	}
	return &Decide{prepared: pq, failMode: o.failMode, name: name}, nil
}

func (d *Decide) Evaluate(ctx context.Context, in Input) (Verdict, error) {
	val, ok, err := evalSingle(ctx, d.prepared, in)
	if err != nil {
		return "", err
	}
	if !ok {
		return VerdictAllow, nil
	}
	s, ok := val.(string)
	if !ok {
		return "", xerrors.Errorf("verdict is %T, want string", val)
	}
	v := Verdict(s)
	switch v {
	case VerdictAllow, VerdictLog, VerdictBlock:
		return v, nil
	default:
		return "", xerrors.Errorf("unknown verdict %q", s)
	}
}
