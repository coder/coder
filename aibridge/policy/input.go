package policy

import (
	"bytes"
	"encoding/json"

	"github.com/open-policy-agent/opa/v1/ast"
	"golang.org/x/xerrors"
)

// Input is a parsed policy input envelope. It is built once per hook and may be
// shared across every policy evaluated for that hook. Augmenting methods
// (WithAnnotations, WithRequest) return a *new* Input that shares the unchanged
// subtrees, so prior Inputs remain valid for concurrent readers and the
// parse-once property holds within a stage.
type Input struct {
	val ast.Value
}

// SchemaVersion identifies a generation of the input-envelope family. It is a
// **forensic stamp**, not a runtime selector: the host always builds the
// current envelope shape, and a policy version records the generation it was
// authored against (stored in ai_gateway_policy_versions.input_schema_version).
//
// The envelope contract is kept backward compatible **structurally**: a field
// is never removed, renamed, or retyped (enforced by the shape guard in
// schema_guard_test.go). Adding a field is allowed and bumps CurrentInputSchemaVersion;
// because nothing is ever removed, an old policy always still finds the paths it
// read. The one residual risk an addition carries (a policy that probed a
// previously-undefined path changes behavior when that path becomes defined) is
// accepted: additions are deliberate, reviewed, and rare, and the stamp lets
// forensics correlate a regression to the generation that introduced the field.
// This is deliberately simpler than building per-pinned-version envelopes.
type SchemaVersion int32

const (
	// SchemaV1 is the initial envelope generation.
	SchemaV1 SchemaVersion = 1

	// CurrentInputSchemaVersion is the generation new policy versions are stamped
	// with at create/edit. Bump it (additively) whenever an envelope gains a
	// field, and update the shape guard.
	CurrentInputSchemaVersion = SchemaV1
)

// Identity is the resolved end-user identity exposed to policies from pre-req
// onward (RBAC/ABAC inputs, FR7). It is a typed contract owned by this package
// so its shape is guarded (schema_guard_test.go) and, deliberately, decoupled
// from the upstream-forwarded actor metadata: fields here are for policy
// evaluation only and are NOT sent to the provider.
//
// Groups and Roles are always materialized as (possibly empty) arrays, never
// nil, so a policy reading input.identity.groups sees [] rather than undefined.
type Identity struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
	Roles    []string `json:"roles"`
}

func (id Identity) value() (ast.Value, error) {
	groups := id.Groups
	if groups == nil {
		groups = []string{}
	}
	roles := id.Roles
	if roles == nil {
		roles = []string{}
	}
	return ast.InterfaceToValue(map[string]any{
		"id":       id.ID,
		"username": id.Username,
		"groups":   groups,
		"roles":    roles,
	})
}

// ToolCall describes a single model-requested tool call to be gated at the
// pre-tool hook. Arguments is the raw JSON arguments object as assembled from
// the upstream stream; Index is the zero-based position of this call within its
// turn (so a policy can express "at most N tool calls per turn" without state).
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
	Index     int
}

// PreAuthEnvelope holds the raw inputs for the pre-auth hook: headers and
// credentials, with no resolved identity or request body.
type PreAuthEnvelope struct {
	Headers    map[string]any
	Credential map[string]any
}

// Build materializes the pre-auth Input.
func (e PreAuthEnvelope) Build() (Input, error) {
	hVal, err := ast.InterfaceToValue(e.Headers)
	if err != nil {
		return Input{}, xerrors.Errorf("convert headers: %w", err)
	}
	cVal, err := ast.InterfaceToValue(e.Credential)
	if err != nil {
		return Input{}, xerrors.Errorf("convert credential: %w", err)
	}
	return envelope(map[string]ast.Value{
		"headers":    hVal,
		"credential": cVal,
		// Seed annotations as {} like the other hooks: a pre-auth annotate can
		// feed a pre-auth decide, and a policy reading input.annotations.* should
		// see a defined-but-empty object rather than undefined.
		"annotations": ast.NewObject(),
	}), nil
}

// requestValue builds the input.request container: gateway-owned method and
// path, plus the provider-native body (parsed but otherwise opaque, since its
// shape is the upstream provider's contract, not ours).
func requestValue(method, path string, body []byte) (ast.Value, error) {
	bodyVal, err := ast.ValueFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, xerrors.Errorf("parse request body: %w", err)
	}
	return ast.NewObject(
		[2]*ast.Term{ast.StringTerm("method"), ast.StringTerm(method)},
		[2]*ast.Term{ast.StringTerm("path"), ast.StringTerm(path)},
		[2]*ast.Term{ast.StringTerm("body"), ast.NewTerm(bodyVal)},
	), nil
}

// PreReqEnvelope holds the raw inputs for the pre-req hook. It is a superset of
// the pre-auth envelope (headers + annotations) plus the resolved identity and
// the request (method, path, and provider-native body). It deliberately omits
// credential: by pre-req the credential has been resolved into identity, so
// re-exposing the raw secret is needless attack surface.
type PreReqEnvelope struct {
	Headers  map[string]any
	Method   string
	Path     string
	Request  []byte // provider-native request body
	Identity Identity
}

// Build materializes the pre-req Input.
func (e PreReqEnvelope) Build() (Input, error) {
	hVal, err := ast.InterfaceToValue(e.Headers)
	if err != nil {
		return Input{}, xerrors.Errorf("convert headers: %w", err)
	}
	reqVal, err := requestValue(e.Method, e.Path, e.Request)
	if err != nil {
		return Input{}, err
	}
	idVal, err := e.Identity.value()
	if err != nil {
		return Input{}, xerrors.Errorf("convert identity: %w", err)
	}
	return envelope(map[string]ast.Value{
		"headers":     hVal,
		"request":     reqVal,
		"identity":    idVal,
		"annotations": ast.NewObject(),
	}), nil
}

// PreToolEnvelope holds the raw inputs for the pre-tool hook: a superset of the
// pre-req envelope (headers, request, identity, annotations) plus the assembled
// tool call. Like pre-req it omits credential. Build it once per tool call.
type PreToolEnvelope struct {
	PreReqEnvelope
	ToolCall ToolCall
}

// Build materializes the pre-tool Input. Invalid tool arguments JSON is an
// error; the caller fails the call per its fail mode.
func (e PreToolEnvelope) Build() (Input, error) {
	hVal, err := ast.InterfaceToValue(e.Headers)
	if err != nil {
		return Input{}, xerrors.Errorf("convert headers: %w", err)
	}
	reqVal, err := requestValue(e.Method, e.Path, e.Request)
	if err != nil {
		return Input{}, err
	}
	idVal, err := e.Identity.value()
	if err != nil {
		return Input{}, xerrors.Errorf("convert identity: %w", err)
	}
	var args ast.Value = ast.NewObject()
	if len(bytes.TrimSpace(e.ToolCall.Arguments)) > 0 {
		av, err := ast.ValueFromReader(bytes.NewReader(e.ToolCall.Arguments))
		if err != nil {
			return Input{}, xerrors.Errorf("parse tool arguments: %w", err)
		}
		args = av
	}
	tc := ast.NewObject(
		[2]*ast.Term{ast.StringTerm("id"), ast.StringTerm(e.ToolCall.ID)},
		[2]*ast.Term{ast.StringTerm("name"), ast.StringTerm(e.ToolCall.Name)},
		[2]*ast.Term{ast.StringTerm("arguments"), ast.NewTerm(args)},
		[2]*ast.Term{ast.StringTerm("index"), ast.NewTerm(ast.IntNumberTerm(e.ToolCall.Index).Value)},
	)
	return envelope(map[string]ast.Value{
		"tool_call":   tc,
		"headers":     hVal,
		"request":     reqVal,
		"identity":    idVal,
		"annotations": ast.NewObject(),
	}), nil
}

func envelope(parts map[string]ast.Value) Input {
	pairs := make([][2]*ast.Term, 0, len(parts))
	for k, v := range parts {
		pairs = append(pairs, [2]*ast.Term{ast.StringTerm(k), ast.NewTerm(v)})
	}
	return Input{val: ast.NewObject(pairs...)}
}

// WithAnnotations shallow-merges m into input.annotations and returns a new
// Input. Later writers win on key collision.
func (in Input) WithAnnotations(m map[string]any) (Input, error) {
	obj, ok := in.val.(ast.Object)
	if !ok {
		return Input{}, xerrors.New("input is not an object")
	}
	merged := ast.NewObject()
	if t := obj.Get(ast.StringTerm("annotations")); t != nil {
		if existing, ok := t.Value.(ast.Object); ok {
			existing.Foreach(func(k, v *ast.Term) { merged.Insert(k, v) })
		}
	}
	for k, v := range m {
		vv, err := ast.InterfaceToValue(v)
		if err != nil {
			return Input{}, xerrors.Errorf("convert annotation %q: %w", k, err)
		}
		merged.Insert(ast.StringTerm(k), ast.NewTerm(vv))
	}
	return in.replaceKey("annotations", ast.NewTerm(merged))
}

// WithRequest replaces input.request.body with a newly parsed body, preserving
// the request's method and path, and returns a new Input.
func (in Input) WithRequest(body []byte) (Input, error) {
	bodyVal, err := ast.ValueFromReader(bytes.NewReader(body))
	if err != nil {
		return Input{}, xerrors.Errorf("parse request body: %w", err)
	}
	obj, ok := in.val.(ast.Object)
	if !ok {
		return Input{}, xerrors.New("input is not an object")
	}
	next := ast.NewObject()
	if t := obj.Get(ast.StringTerm("request")); t != nil {
		if reqObj, ok := t.Value.(ast.Object); ok {
			reqObj.Foreach(func(k, v *ast.Term) {
				if s, ok := k.Value.(ast.String); ok && string(s) == "body" {
					return
				}
				next.Insert(k, v)
			})
		}
	}
	next.Insert(ast.StringTerm("body"), ast.NewTerm(bodyVal))
	return in.replaceKey("request", ast.NewTerm(next))
}

// Request serializes input.request.body (the provider-native body) back to JSON
// bytes for forwarding upstream.
func (in Input) Request() ([]byte, error) {
	obj, ok := in.val.(ast.Object)
	if !ok {
		return nil, xerrors.New("input is not an object")
	}
	t := obj.Get(ast.StringTerm("request"))
	if t == nil {
		return nil, xerrors.New("input has no request")
	}
	reqObj, ok := t.Value.(ast.Object)
	if !ok {
		return nil, xerrors.New("input.request is not an object")
	}
	bt := reqObj.Get(ast.StringTerm("body"))
	if bt == nil {
		return nil, xerrors.New("input.request has no body")
	}
	j, err := ast.JSON(bt.Value)
	if err != nil {
		return nil, xerrors.Errorf("encode request body: %w", err)
	}
	return json.Marshal(j)
}

// replaceKey rebuilds the top-level object with key set to val, sharing all
// other terms unchanged (copy-on-write).
func (in Input) replaceKey(key string, val *ast.Term) (Input, error) {
	obj, ok := in.val.(ast.Object)
	if !ok {
		return Input{}, xerrors.New("input is not an object")
	}
	next := ast.NewObject()
	obj.Foreach(func(k, v *ast.Term) {
		if s, ok := k.Value.(ast.String); ok && string(s) == key {
			return
		}
		next.Insert(k, v)
	})
	next.Insert(ast.StringTerm(key), val)
	return Input{val: next}, nil
}
