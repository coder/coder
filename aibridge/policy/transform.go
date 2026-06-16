package policy

import (
	"context"

	"golang.org/x/xerrors"
)

// Transformation is the typed decode result of a transform policy: a body
// rewrite expressed as edits plus optional request header overrides. It
// projects into a StageResult's Edits and Headers. A whole-body rewrite is the
// degenerate root edit (Pointer "").
type Transformation struct {
	// Edits are the body mutations. A transform that rewrites the whole body
	// produces a single root edit; the host re-validates the mutated body
	// downstream. Empty when the body rule is undefined.
	Edits []Edit
	// Headers are request header overrides (set/replace) applied to the outgoing
	// upstream request, or nil when the headers rule is undefined. Transport,
	// auth, and hop-by-hop headers are sanitized by the host before forwarding,
	// so a policy cannot use them to inject credentials or corrupt framing.
	Headers map[string]string
}

// Project maps the transformation into a StageResult's Edits and Headers. stage
// is unused: a transform produces no annotations.
func (t Transformation) Project(string) StageResult {
	return StageResult{Edits: t.Edits, Headers: t.Headers}
}

// Transform rewrites the outgoing request. It evaluates data.gateway.body (the
// whole replacement body, decoded into a root edit) and the optional
// data.gateway.headers (an object of string header overrides). ok is false when
// neither rule is defined.
type Transform struct {
	body     preparedQuery
	headers  preparedQuery
	failMode FailMode
	name     string
}

func NewTransform(name, module string, opts ...Option) (*Transform, error) {
	o := newOptions(opts...)
	bq, err := prepare(module, ruleQuery("body"))
	if err != nil {
		return nil, err
	}
	// The headers rule is optional; preparing a query for a rule the module
	// never defines is valid and simply evaluates to undefined at eval time.
	hq, err := prepare(module, ruleQuery("headers"))
	if err != nil {
		return nil, err
	}
	return &Transform{body: bq, headers: hq, failMode: o.failMode, name: name}, nil
}

// Name implements Stage.
func (t *Transform) Name() string { return t.name }

// Evaluate decodes the body/headers rules and projects them into a StageResult.
// A failure is synthesized through the stage's fail mode.
func (t *Transform) Evaluate(ctx context.Context, in Input) StageResult {
	return runStage(ctx, t.name, t.failMode, func(sctx context.Context) (Projector, error) {
		tf, ok, err := t.transformation(sctx, in)
		if err != nil {
			return nil, err
		}
		if !ok {
			return noop{}, nil
		}
		return tf, nil
	})
}

// transformation decodes data.gateway.body (as a root edit) and the optional
// data.gateway.headers. ok is false when neither rule is defined.
func (t *Transform) transformation(ctx context.Context, in Input) (Transformation, bool, error) {
	var out Transformation

	val, ok, err := evalSingle(ctx, t.body, in)
	if err != nil {
		return Transformation{}, false, err
	}
	if ok {
		// A whole-body rewrite is the degenerate root edit: pointer "" replaces
		// the entire body with the decoded value (re-validated downstream).
		out.Edits = []Edit{{Pointer: "", Value: val}}
	}

	hval, hok, err := evalSingle(ctx, t.headers, in)
	if err != nil {
		return Transformation{}, false, err
	}
	if hok {
		headers, err := toHeaderMap(hval)
		if err != nil {
			return Transformation{}, false, err
		}
		out.Headers = headers
	}

	return out, len(out.Edits) > 0 || out.Headers != nil, nil
}

// toHeaderMap converts a Rego object of string values into a header map. A
// non-object, or any non-string value, is an error so a malformed headers rule
// is surfaced rather than silently dropped.
func toHeaderMap(val any) (map[string]string, error) {
	m, ok := val.(map[string]any)
	if !ok {
		return nil, xerrors.Errorf("headers is %T, want object", val)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, xerrors.Errorf("header %q is %T, want string", k, v)
		}
		out[k] = s
	}
	return out, nil
}
