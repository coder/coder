package policy

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"
)

// Transformation is the output of a transform policy: an optional replacement
// request body and optional request header overrides. A nil field means the
// corresponding rule was undefined, so that part of the request is unchanged.
type Transformation struct {
	// Body is the whole replacement request body as JSON bytes, or nil when the
	// body rule is undefined. The host re-validates it by re-parsing downstream.
	Body []byte
	// Headers are request header overrides (set/replace) applied to the outgoing
	// upstream request, or nil when the headers rule is undefined. Transport,
	// auth, and hop-by-hop headers are sanitized by the host before forwarding,
	// so a policy cannot use them to inject credentials or corrupt framing.
	Headers map[string]string
}

// Transform rewrites the outgoing request. It evaluates data.gateway.body (the
// whole replacement body) and the optional data.gateway.headers (an object of
// string header overrides). ok is false when neither rule is defined.
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

func (t *Transform) Evaluate(ctx context.Context, in Input) (Transformation, bool, error) {
	var out Transformation

	val, ok, err := evalSingle(ctx, t.body, in)
	if err != nil {
		return Transformation{}, false, err
	}
	if ok {
		body, err := json.Marshal(val)
		if err != nil {
			return Transformation{}, false, xerrors.Errorf("encode transformed body: %w", err)
		}
		out.Body = body
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

	return out, out.Body != nil || out.Headers != nil, nil
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
