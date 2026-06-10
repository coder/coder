package policy

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

// evalTimeout bounds a single policy stage evaluation. A stage that exceeds it
// fails closed (BLOCK) regardless of the stage's configured fail mode, so a
// pathological policy cannot hang a request or fail open by timing out.
const evalTimeout = time.Second

// stageBlocks reports whether a stage error should block the request. A timeout
// always blocks (fail closed); other errors honor the stage's fail mode.
func stageBlocks(failMode FailMode, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return failMode == FailClosed
}

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

// NewToolPipeline builds a pipeline for the pre-tool hook. Only classify and
// decide are valid there (the request is already dispatched, so route and
// transform are rejected), and classify is capped at one.
func NewToolPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if cfg.Route != nil {
		return nil, xerrors.New("route policy is not valid at the pre-tool hook")
	}
	if len(cfg.Transform) > 0 {
		return nil, xerrors.New("transform policy is not valid at the pre-tool hook")
	}
	return NewPipeline(cfg)
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
		ann, ok, err := evalStage(ctx, func(sctx context.Context) (map[string]any, bool, error) {
			return c.Evaluate(sctx, cur)
		})
		if err != nil {
			if stageBlocks(c.failMode, err) {
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
		model, ok, err := evalStage(ctx, func(sctx context.Context) (string, bool, error) {
			return p.route.Evaluate(sctx, cur)
		})
		switch {
		case err != nil:
			if stageBlocks(p.route.failMode, err) {
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
		v, err := evalStage1(ctx, func(sctx context.Context) (Verdict, error) {
			return d.Evaluate(sctx, cur)
		})
		if err != nil {
			if stageBlocks(d.failMode, err) {
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
		body, ok, err := evalStage(ctx, func(sctx context.Context) ([]byte, bool, error) {
			return t.Evaluate(sctx, cur)
		})
		if err != nil {
			if stageBlocks(t.failMode, err) {
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

// evalStage runs a (value, ok, error) stage under a per-stage timeout.
func evalStage[T any](ctx context.Context, fn func(context.Context) (T, bool, error)) (T, bool, error) {
	sctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	return fn(sctx)
}

// evalStage1 runs a (value, error) stage under a per-stage timeout.
func evalStage1[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	sctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	return fn(sctx)
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
