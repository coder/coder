package policy

import (
	"context"
	"maps"

	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"
)

// Pipeline runs an ordered set of policy kinds against a single Input for one
// hook. Evaluation is sequential and the Input is threaded copy-on-write so
// each stage sees prior stages' mutations: annotate annotations, then a route
// model override, then decisions, then a transform. Every stage yields one
// StageResult; the single Reduce combines them and ApplyEdits applies body
// mutations.
type Pipeline struct {
	annotate  []*Annotate
	route     *Route
	decide    []*Decide
	transform []*Transform
}

// PipelineConfig declares the policies for each stage. annotate and transform
// are capped at one policy for now (multiple mutative policies need a defined
// composition model); decide may be many and is reduced.
type PipelineConfig struct {
	Annotate  []*Annotate
	Route     *Route
	Decide    []*Decide
	Transform []*Transform
}

func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if len(cfg.Annotate) > 1 {
		return nil, xerrors.New("at most one annotate policy per stage")
	}
	if len(cfg.Transform) > 1 {
		return nil, xerrors.New("at most one transform policy per stage")
	}
	return &Pipeline{
		annotate:  cfg.Annotate,
		route:     cfg.Route,
		decide:    cfg.Decide,
		transform: cfg.Transform,
	}, nil
}

// newDecisionOnlyPipeline builds a pipeline for a hook that permits only
// annotate and decide, rejecting the request-mutating kinds (route, transform).
// hook names the hook for error messages.
func newDecisionOnlyPipeline(hook string, cfg PipelineConfig) (*Pipeline, error) {
	if cfg.Route != nil {
		return nil, xerrors.Errorf("route policy is not valid at the %s hook", hook)
	}
	if len(cfg.Transform) > 0 {
		return nil, xerrors.Errorf("transform policy is not valid at the %s hook", hook)
	}
	return NewPipeline(cfg)
}

// NewPreAuthPipeline builds a pipeline for the pre-auth hook. Only annotate and
// decide are valid there: there is no request body to route or transform, so the
// request-mutating kinds are rejected.
func NewPreAuthPipeline(cfg PipelineConfig) (*Pipeline, error) {
	return newDecisionOnlyPipeline("pre-auth", cfg)
}

// NewToolPipeline builds a pipeline for the pre-tool hook. Only annotate and
// decide are valid there (the request is already dispatched, so route and
// transform are rejected), and annotate is capped at one.
func NewToolPipeline(cfg PipelineConfig) (*Pipeline, error) {
	return newDecisionOnlyPipeline("pre-tool", cfg)
}

// Result is the combined outcome of a pipeline (the reduced StageResults plus
// the applied body/headers/modifications).
type Result struct {
	// Verdict is the reduced decision across the pipeline.
	Verdict Verdict
	// BlockedBy is the name of the deliberately-blocking policy, or empty when
	// the verdict is not BLOCK or the block was synthesized from a failure.
	BlockedBy string
	// Message is the optional, author-supplied explanation from the blocking
	// decide policy. Empty when the verdict is not BLOCK, the blocking policy
	// supplied no message, or the block was synthesized.
	Message string
	// Annotations are the annotate/threaded outputs, namespaced per producing
	// stage, recorded under Metadata["annotations"].
	Annotations map[string]any
	// Modifications records request mutations made by policies, keyed by the
	// policy name. Recorded under Metadata["modifications"]. A route policy that
	// changes the model adds {"original_model": "<previous model>"}.
	Modifications map[string]any
	// RequestBody is the final request body after route/transform, or nil when
	// neither mutated it (or the request was blocked, which freezes effects).
	RequestBody []byte
	// Headers are request header overrides produced by a transform policy,
	// applied (set/replace) to the outgoing upstream request. Nil when no
	// transform set headers.
	Headers map[string]string
	// Errors holds every stage failure that rode through as a synthesized
	// result (fail-open errors normalized to LOG, and a fail-closed error that
	// blocked). They are surfaced for host logging.
	Errors []StageErr
}

// Evaluate runs every stage against in and returns the combined Result. Stage
// failures are normalized uniformly through each stage's fail mode by the stage
// boundary (the Failure Projector): a fail-closed error blocks (anonymous to the client)
// and short-circuits; a fail-open error rides through as LOG (never a silent
// pass-through) and evaluation continues.
func (p *Pipeline) Evaluate(ctx context.Context, in Input) (Result, error) {
	cur := in
	var (
		outcomes      []StageOutcome
		finalBody     []byte
		mutated       bool
		headers       map[string]string
		modifications map[string]any
	)

	// finalize reduces the collected outcomes and assembles the Result. On a
	// BLOCK, mutating effects (body, headers, modifications) are frozen.
	finalize := func() Result {
		r := Reduce(outcomes)
		res := Result{
			Verdict:     r.Verdict,
			BlockedBy:   r.BlockedBy,
			Message:     r.Message,
			Annotations: r.Annotations,
			Errors:      r.Errors,
		}
		if !r.Verdict.Blocks() {
			if mutated {
				res.RequestBody = finalBody
			}
			res.Headers = headers
			res.Modifications = modifications
		}
		return res
	}

	// annotate: thread each stage's namespaced annotations into the Input so
	// later stages (and later hooks) can read them.
	for _, a := range p.annotate {
		res := a.Evaluate(ctx, cur)
		outcomes = append(outcomes, StageOutcome{Name: a.name, Result: res})
		if res.Verdict.Blocks() {
			return finalize(), nil
		}
		if len(res.Annotations) > 0 {
			var err error
			cur, err = cur.WithAnnotations(res.Annotations)
			if err != nil {
				return Result{}, err
			}
		}
	}

	// route: override input.request.body.model so later stages see the new model.
	if p.route != nil {
		res := p.route.Evaluate(ctx, cur)
		outcomes = append(outcomes, StageOutcome{Name: p.route.name, Result: res})
		switch {
		case res.Verdict.Blocks():
			return finalize(), nil
		case res.Route != "":
			body, err := cur.Request()
			if err != nil {
				return Result{}, err
			}
			original := gjson.GetBytes(body, "model").String()
			// Only mutate and record when the model actually changes; a route
			// that returns the current model is a no-op.
			if res.Route != original {
				body, _, err = ApplyEdits(body, []Edit{{Pointer: "model", Value: res.Route}})
				if err != nil {
					return Result{}, xerrors.Errorf("override model: %w", err)
				}
				cur, err = cur.WithRequest(body)
				if err != nil {
					return Result{}, err
				}
				finalBody = body
				mutated = true
				modifications = addModification(modifications, p.route.name, map[string]any{
					"original_model": original,
				})
			}
		}
	}

	// decide: reduce verdicts, BLOCK short-circuits.
	for _, d := range p.decide {
		res := d.Evaluate(ctx, cur)
		outcomes = append(outcomes, StageOutcome{Name: d.name, Result: res})
		if res.Verdict.Blocks() {
			return finalize(), nil
		}
	}

	// transform: apply body edits (a whole-body rewrite is the root edit) and/or
	// override request headers. The host re-validates the mutated body downstream.
	for _, t := range p.transform {
		res := t.Evaluate(ctx, cur)
		outcomes = append(outcomes, StageOutcome{Name: t.name, Result: res})
		if res.Verdict.Blocks() {
			return finalize(), nil
		}
		if len(res.Edits) > 0 {
			body, err := cur.Request()
			if err != nil {
				return Result{}, err
			}
			body, _, err = ApplyEdits(body, res.Edits)
			if err != nil {
				return Result{}, err
			}
			cur, err = cur.WithRequest(body)
			if err != nil {
				return Result{}, err
			}
			finalBody = body
			mutated = true
		}
		if len(res.Headers) > 0 {
			headers = mergeHeaders(headers, res.Headers)
		}
	}

	return finalize(), nil
}

// addModification records a single policy's modification entry keyed by policy
// name, allocating the map on first use.
func addModification(dst map[string]any, name string, mod map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any, 1)
	}
	dst[name] = mod
	return dst
}

// mergeHeaders merges src header overrides into dst, allocating on first use.
// Later policies (there is at most one transform today) win on key conflicts.
func mergeHeaders(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	maps.Copy(dst, src)
	return dst
}
