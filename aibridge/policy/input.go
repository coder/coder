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

// BuildInput constructs the pre-req envelope: input.request (the parsed request
// body), input.identity, and an empty input.annotations.
func BuildInput(request []byte, identity map[string]any) (Input, error) {
	reqVal, err := ast.ValueFromReader(bytes.NewReader(request))
	if err != nil {
		return Input{}, xerrors.Errorf("parse request body: %w", err)
	}
	idVal, err := ast.InterfaceToValue(identity)
	if err != nil {
		return Input{}, xerrors.Errorf("convert identity: %w", err)
	}
	return envelope(map[string]ast.Value{
		"request":     reqVal,
		"identity":    idVal,
		"annotations": ast.NewObject(),
	}), nil
}

// BuildAuthInput constructs the pre-auth envelope: input.headers and
// input.credential. It carries no request body or identity.
func BuildAuthInput(headers, credential map[string]any) (Input, error) {
	hVal, err := ast.InterfaceToValue(headers)
	if err != nil {
		return Input{}, xerrors.Errorf("convert headers: %w", err)
	}
	cVal, err := ast.InterfaceToValue(credential)
	if err != nil {
		return Input{}, xerrors.Errorf("convert credential: %w", err)
	}
	return envelope(map[string]ast.Value{
		"headers":    hVal,
		"credential": cVal,
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

// WithRequest replaces input.request with a newly parsed body and returns a new
// Input.
func (in Input) WithRequest(body []byte) (Input, error) {
	reqVal, err := ast.ValueFromReader(bytes.NewReader(body))
	if err != nil {
		return Input{}, xerrors.Errorf("parse request body: %w", err)
	}
	return in.replaceKey("request", ast.NewTerm(reqVal))
}

// Request serializes input.request back to JSON bytes.
func (in Input) Request() ([]byte, error) {
	obj, ok := in.val.(ast.Object)
	if !ok {
		return nil, xerrors.New("input is not an object")
	}
	t := obj.Get(ast.StringTerm("request"))
	if t == nil {
		return nil, xerrors.New("input has no request")
	}
	j, err := ast.JSON(t.Value)
	if err != nil {
		return nil, xerrors.Errorf("encode request: %w", err)
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
