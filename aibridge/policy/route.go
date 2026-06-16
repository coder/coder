package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// RouteChanges is the typed decode result of a route policy: a model override.
// Provider may be added later (a widening). It projects into a StageResult's
// Route field.
type RouteChanges struct {
	Model string
}

// Project maps the route change into a StageResult's Route field. stage is
// unused: a route produces no annotations.
func (r RouteChanges) Project(string) StageResult {
	return StageResult{Route: r.Model}
}

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

// Name implements Stage.
func (r *Route) Name() string { return r.name }

// Evaluate decodes the model rule and projects it into a StageResult's Route
// field. A failure is synthesized through the stage's fail mode.
func (r *Route) Evaluate(ctx context.Context, in Input) StageResult {
	return runStage(ctx, r.name, r.failMode, func(sctx context.Context) (Projector, error) {
		rc, ok, err := r.route(sctx, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			return noop{}, nil
		}
		return rc, nil
	})
}

// route decodes data.gateway.model. ok is false when the rule is undefined.
func (r *Route) route(ctx context.Context, in Input) (RouteChanges, bool, error) {
	val, ok, err := evalSingle(ctx, r.prepared, in)
	if err != nil {
		return RouteChanges{}, false, err
	}
	if !ok {
		return RouteChanges{}, false, nil
	}
	s, ok := val.(string)
	if !ok {
		return RouteChanges{}, false, xerrors.Errorf("model is %T, want string", val)
	}
	return RouteChanges{Model: s}, true, nil
}
