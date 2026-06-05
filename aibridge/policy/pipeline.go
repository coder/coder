package policy

import (
	"context"
	"maps"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

// Pipeline runs an ordered set of policy kinds against a single Input for one
// hook. Evaluation is sequential and the Input is threaded copy-on-write so
// each stage sees prior stages' mutations: classify annotations, then a route
// model override, then decisions, then a transform.
type Pipeline struct {
	classify  []*Classify
	route     *Route
	decide    []*Decide
	transform []*Transform
}

// PipelineConfig declares the policies for each stage. classify and transform
// are capped at one policy for now (multiple mutative policies need a defined
// composition model); decide may be many and is reduced.
type PipelineConfig struct {
	Classify  []*Classify
	Route     *Route
	Decide    []*Decide
	Transform []*Transform
}

func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if len(cfg.Classify) > 1 {
		return nil, xerrors.New("at most one classify policy per stage")
	}
	if len(cfg.Transform) > 1 {
		return nil, xerrors.New("at most one transform policy per stage")
	}
	return &Pipeline{
		classify:  cfg.Classify,
		route:     cfg.Route,
		decide:    cfg.Decide,
		transform: cfg.Transform,
	}, nil
}

// Result is the combined outcome of a pipeline.
type Result struct {
	// Verdict is the reduced decision across the pipeline.
	Verdict Verdict
	// BlockedBy is the name of the policy that produced the BLOCK verdict, or
	// empty when the verdict is not BLOCK. Used to surface a useful error to
	// the client.
	BlockedBy string
	// Annotations are the classify outputs, recorded under
	// Metadata["classifications"].
	Annotations map[string]any
	// Modifications records request mutations made by policies, keyed by the
	// policy name. Recorded under Metadata["modifications"]. A route policy that
	// changes the model adds {"original_model": "<previous model>"}.
	Modifications map[string]any
	// RequestBody is the final request body after route/transform, or nil when
	// neither mutated it.
	RequestBody []byte
}

// Evaluate runs every stage against in and returns the combined Result. On a
// fail-closed stage error it returns a BLOCK verdict and short-circuits; a
// fail-open stage error skips that stage.
func (p *Pipeline) Evaluate(ctx context.Context, in Input) (Result, error) {
	cur := in
	res := Result{Verdict: VerdictAllow}
	var finalBody []byte
	mutated := false

	// classify: merge annotations into the threaded Input.
	for _, c := range p.classify {
		ann, ok, err := c.Evaluate(ctx, cur)
		if err != nil {
			if c.failMode == FailClosed {
				return blockedBy(c.name), nil
			}
			continue
		}
		if !ok {
			continue
		}
		cur, err = cur.WithAnnotations(ann)
		if err != nil {
			return Result{}, err
		}
		res.Annotations = mergeAnnotations(res.Annotations, ann)
	}

	// route: override input.request.model so later stages see the new model.
	if p.route != nil {
		model, ok, err := p.route.Evaluate(ctx, cur)
		switch {
		case err != nil:
			if p.route.failMode == FailClosed {
				return blockedBy(p.route.name), nil
			}
		case ok:
			body, err := cur.Request()
			if err != nil {
				return Result{}, err
			}
			original := gjson.GetBytes(body, "model").String()
			// Only mutate and record when the model actually changes; a route
			// that returns the current model is a no-op.
			if model != original {
				body, err = sjson.SetBytes(body, "model", model)
				if err != nil {
					return Result{}, xerrors.Errorf("override model: %w", err)
				}
				cur, err = cur.WithRequest(body)
				if err != nil {
					return Result{}, err
				}
				finalBody = body
				mutated = true
				res.Modifications = addModification(res.Modifications, p.route.name, map[string]any{
					"original_model": original,
				})
			}
		}
	}

	// decide: reduce verdicts, BLOCK short-circuits.
	var blockingDecide string
	verdicts := []Verdict{VerdictAllow}
	for _, d := range p.decide {
		v, err := d.Evaluate(ctx, cur)
		if err != nil {
			if d.failMode == FailClosed {
				return blockedBy(d.name), nil
			}
			continue
		}
		verdicts = append(verdicts, v)
		if v.Blocks() {
			blockingDecide = d.name
			break
		}
	}
	res.Verdict = ReduceVerdicts(verdicts...)
	if res.Verdict.Blocks() {
		return blockedBy(blockingDecide), nil
	}

	// transform: replace the whole request body (host re-validates downstream).
	for _, t := range p.transform {
		body, ok, err := t.Evaluate(ctx, cur)
		if err != nil {
			if t.failMode == FailClosed {
				return blockedBy(t.name), nil
			}
			continue
		}
		if !ok {
			continue
		}
		cur, err = cur.WithRequest(body)
		if err != nil {
			return Result{}, err
		}
		finalBody = body
		mutated = true
	}

	if mutated {
		res.RequestBody = finalBody
	}
	return res, nil
}

func blockedBy(name string) Result { return Result{Verdict: VerdictBlock, BlockedBy: name} }

// addModification records a single policy's modification entry keyed by policy
// name, allocating the map on first use.
func addModification(dst map[string]any, name string, mod map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any, 1)
	}
	dst[name] = mod
	return dst
}

func mergeAnnotations(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	maps.Copy(dst, src)
	return dst
}
