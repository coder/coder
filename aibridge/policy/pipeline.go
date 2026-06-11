package policy

import (
	"context"
	"maps"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

// evalTimeout bounds a single policy stage evaluation, so a pathological policy
// cannot hang a request. A stage that exceeds it is treated as an ordinary
// stage error and normalized through the stage's fail mode (see stageBlocks):
// fail-closed times out to BLOCK, fail-open to LOG. There is no special case
// for timeout; an attacker-induced timeout bypassing a fail-open stage is what
// fail-open means.
const evalTimeout = time.Second

// stageBlocks reports whether a stage error should block the request. Every
// error class (eval error, timeout, conflict) is normalized the same way:
// fail-closed blocks, fail-open does not (the caller raises the verdict to LOG
// and records the error instead).
func stageBlocks(failMode FailMode, _ error) bool {
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

// newDecisionOnlyPipeline builds a pipeline for a hook that permits only
// classify and decide, rejecting the request-mutating kinds (route, transform).
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

// NewPreAuthPipeline builds a pipeline for the pre-auth hook. Only classify and
// decide are valid there: there is no request body to route or transform, so the
// request-mutating kinds are rejected.
func NewPreAuthPipeline(cfg PipelineConfig) (*Pipeline, error) {
	return newDecisionOnlyPipeline("pre-auth", cfg)
}

// NewToolPipeline builds a pipeline for the pre-tool hook. Only classify and
// decide are valid there (the request is already dispatched, so route and
// transform are rejected), and classify is capped at one.
func NewToolPipeline(cfg PipelineConfig) (*Pipeline, error) {
	return newDecisionOnlyPipeline("pre-tool", cfg)
}

// Result is the combined outcome of a pipeline.
type Result struct {
	// Verdict is the reduced decision across the pipeline.
	Verdict Verdict
	// BlockedBy is the name of the policy that produced the BLOCK verdict, or
	// empty when the verdict is not BLOCK. Used to surface a useful error to
	// the client.
	BlockedBy string
	// Message is the optional, author-supplied explanation from the blocking
	// decide policy's `message` rule. Empty when the verdict is not BLOCK or
	// the blocking policy supplied no message. When set it overrides the
	// generic block message shown to the user.
	Message string
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
	// Headers are request header overrides produced by a transform policy,
	// applied (set/replace) to the outgoing upstream request. Nil when no
	// transform set headers.
	Headers map[string]string
	// Errors holds every stage failure (eval error, timeout, conflict) that did
	// not block the request, i.e. fail-open errors that were normalized to LOG.
	// They are surfaced for host logging so a fail-open failure is visible in
	// the log stream rather than a silent pass-through. A fail-closed error
	// blocks and is reported via Verdict/BlockedBy instead.
	Errors []StageError
}

// StageError pairs a stage's policy name with the error it returned. It mirrors
// guardrail.GuardrailError so the host logs both substrates uniformly.
type StageError struct {
	Stage string
	Err   error
}

// Evaluate runs every stage against in and returns the combined Result. Stage
// failures are normalized uniformly through each stage's fail mode (the unified
// stage-result algebra): a fail-closed error blocks and short-circuits; a
// fail-open error is recorded in Result.Errors and raises the reduced verdict to
// LOG (never a silent pass-through), then evaluation continues.
func (p *Pipeline) Evaluate(ctx context.Context, in Input) (Result, error) {
	cur := in
	res := Result{Verdict: VerdictAllow}
	var finalBody []byte
	mutated := false

	// verdicts accumulates across every stage, not just decide: a fail-open
	// error in any stage appends LOG so the request passes through visibly.
	verdicts := []Verdict{VerdictAllow}
	var stageErrs []StageError
	// failOpen records a non-blocking stage error: it surfaces the error for
	// host logging and raises the floor verdict to LOG.
	failOpen := func(name string, err error) {
		stageErrs = append(stageErrs, StageError{Stage: name, Err: err})
		verdicts = append(verdicts, VerdictLog)
	}
	// blocked builds the terminal BLOCK result for a fail-closed stage error,
	// carrying the recorded fail-open errors so far for logging.
	blocked := func(name string, err error) Result {
		r := blockedBy(name)
		if err != nil {
			r.Errors = append(stageErrs, StageError{Stage: name, Err: err})
		} else {
			r.Errors = stageErrs
		}
		return r
	}

	// classify: merge annotations into the threaded Input.
	for _, c := range p.classify {
		ann, ok, err := evalStage(ctx, func(sctx context.Context) (map[string]any, bool, error) {
			return c.Evaluate(sctx, cur)
		})
		if err != nil {
			if stageBlocks(c.failMode, err) {
				return blocked(c.name, err), nil
			}
			failOpen(c.name, err)
			continue
		}
		if !ok {
			continue
		}
		// Namespace each classifier's output under its own stage name so the
		// host owns the first level of input.annotations: producers cannot
		// collide on a key, and a downstream decide reads
		// input.annotations.<classify-name>.<key>. The same classifier at a
		// later hook replaces its whole namespace (last-write-wins per
		// namespace), since WithAnnotations merges at the top level.
		ns := map[string]any{c.name: ann}
		cur, err = cur.WithAnnotations(ns)
		if err != nil {
			return Result{}, err
		}
		res.Annotations = mergeAnnotations(res.Annotations, ns)
	}

	// route: override input.request.model so later stages see the new model.
	if p.route != nil {
		model, ok, err := evalStage(ctx, func(sctx context.Context) (string, bool, error) {
			return p.route.Evaluate(sctx, cur)
		})
		switch {
		case err != nil:
			if stageBlocks(p.route.failMode, err) {
				return blocked(p.route.name, err), nil
			}
			failOpen(p.route.name, err)
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
	var blockingDecide, blockMessage string
	for _, d := range p.decide {
		dec, err := evalStage1(ctx, func(sctx context.Context) (Decision, error) {
			return d.Evaluate(sctx, cur)
		})
		if err != nil {
			if stageBlocks(d.failMode, err) {
				return blocked(d.name, err), nil
			}
			failOpen(d.name, err)
			continue
		}
		verdicts = append(verdicts, dec.Verdict)
		if dec.Verdict.Blocks() {
			blockingDecide = d.name
			blockMessage = dec.Message
			break
		}
	}
	if ReduceVerdicts(verdicts...).Blocks() {
		r := blocked(blockingDecide, nil)
		r.Message = blockMessage
		return r, nil
	}

	// transform: replace the whole request body (host re-validates downstream)
	// and/or override request headers.
	for _, t := range p.transform {
		tf, ok, err := evalStage(ctx, func(sctx context.Context) (Transformation, bool, error) {
			return t.Evaluate(sctx, cur)
		})
		if err != nil {
			if stageBlocks(t.failMode, err) {
				return blocked(t.name, err), nil
			}
			failOpen(t.name, err)
			continue
		}
		if !ok {
			continue
		}
		if tf.Body != nil {
			cur, err = cur.WithRequest(tf.Body)
			if err != nil {
				return Result{}, err
			}
			finalBody = tf.Body
			mutated = true
		}
		if len(tf.Headers) > 0 {
			res.Headers = mergeHeaders(res.Headers, tf.Headers)
		}
	}

	res.Verdict = ReduceVerdicts(verdicts...)
	res.Errors = stageErrs
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

// mergeHeaders merges src header overrides into dst, allocating on first use.
// Later policies (there is at most one transform today) win on key conflicts.
func mergeHeaders(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	maps.Copy(dst, src)
	return dst
}

func mergeAnnotations(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	maps.Copy(dst, src)
	return dst
}
