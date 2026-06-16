package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/stretchr/testify/require"
)

// This guard enforces the input-envelope backward-compatibility contract
// (design doc §10.4):
//
//   - The Rego-visible shape of each hook's envelope is frozen per generation.
//     A field may be ADDED (which bumps CurrentInputSchemaVersion), but never
//     removed, renamed, or retyped.
//   - It inspects the BUILT ast.Value (the actual contract operators write Rego
//     against), not the Go structs, so inline-built keys (tool_call.*,
//     annotations) are covered.
//   - It excludes the variable-content subtrees (request.*, tool_call.arguments.*)
//     since those mirror the provider request body, not the envelope contract.
//   - The golden for each generation lives at testdata/envelope_shape/vN.json and
//     is frozen once committed. CurrentInputSchemaVersion selects which golden the
//     current build must match, so a shape change forces a new file + a version
//     bump (Q6b): the build can no longer match the frozen prior file.
//
// To add an envelope field: bump CurrentInputSchemaVersion, then regenerate with
//
//	POLICY_SCHEMA_UPDATE=1 go test ./aibridge/policy/ -run TestInputSchemaShapeGuard
//
// and commit the new testdata/envelope_shape/vN.json. Never edit a prior file.

func TestInputSchemaShapeGuard(t *testing.T) {
	t.Parallel()

	shape := builtEnvelopeShape(t)

	dir := filepath.Join("testdata", "envelope_shape")
	cur := filepath.Join(dir, fmt.Sprintf("v%d.json", CurrentInputSchemaVersion))

	got, err := json.MarshalIndent(shape, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	if os.Getenv("POLICY_SCHEMA_UPDATE") != "" {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(cur, got, 0o600))
	}

	want, err := os.ReadFile(cur)
	require.NoErrorf(t, err, "no golden for CurrentInputSchemaVersion=%d; regenerate with POLICY_SCHEMA_UPDATE=1", CurrentInputSchemaVersion)
	require.Equalf(t, string(want), string(got),
		"input envelope shape changed for v%d. A field was removed/renamed/retyped (a BC break) "+
			"or added without bumping CurrentInputSchemaVersion. If adding a field, bump CurrentInputSchemaVersion "+
			"and regenerate the golden with POLICY_SCHEMA_UPDATE=1.", CurrentInputSchemaVersion)

	// Shape-change => version-bump coupling: the current shape must differ from
	// every frozen prior generation, so a version bump always corresponds to a
	// real change (and a real change cannot reuse an old version's frozen file).
	for v := SchemaVersion(1); v < CurrentInputSchemaVersion; v++ {
		prior, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("v%d.json", v)))
		if err != nil {
			continue
		}
		require.NotEqualf(t, string(prior), string(got),
			"input envelope shape for v%d is identical to frozen v%d; a version bump must reflect a real shape change",
			CurrentInputSchemaVersion, v)
	}
}

// builtEnvelopeShape builds each hook's envelope with representative empty
// inputs and returns a sorted path:type map of the fixed envelope skeleton,
// excluding the variable-content subtrees.
func builtEnvelopeShape(t *testing.T) map[string]string {
	t.Helper()

	preAuth, err := PreAuthEnvelope{Headers: map[string]any{}, Credential: map[string]any{}}.Build()
	require.NoError(t, err)
	preReq, err := PreReqEnvelope{Method: "POST", Path: "/v1/messages", Request: []byte(`{}`), Identity: Identity{}}.Build()
	require.NoError(t, err)
	preTool, err := PreToolEnvelope{Identity: Identity{}, ToolCall: ToolCall{}}.Build()
	require.NoError(t, err)

	// Subtrees whose contents mirror the provider request body, not the
	// envelope contract: pin the key+type but do not descend.
	opaque := map[string]bool{
		"request/body":        true,
		"tool_call/arguments": true,
	}

	shape := map[string]string{}
	for hook, in := range map[string]Input{
		"pre_auth": preAuth,
		"pre_req":  preReq,
		"pre_tool": preTool,
	} {
		walkShape(hook, "", in.val, opaque, shape)
	}
	return shape
}

// walkShape records "<hook>/<path>": <type> for every key in val, descending
// into objects except those whose path is marked opaque.
func walkShape(hook, path string, val ast.Value, opaque map[string]bool, out map[string]string) {
	obj, ok := val.(ast.Object)
	if !ok {
		return
	}
	obj.Foreach(func(k, v *ast.Term) {
		key, ok := k.Value.(ast.String)
		if !ok {
			return
		}
		child := string(key)
		if path != "" {
			child = path + "/" + string(key)
		}
		out[hook+"/"+child] = astType(v.Value)
		if opaque[child] {
			return
		}
		if _, isObj := v.Value.(ast.Object); isObj {
			walkShape(hook, child, v.Value, opaque, out)
		}
	})
}

func astType(v ast.Value) string {
	switch v.(type) {
	case ast.Object:
		return "object"
	case *ast.Array:
		return "array"
	case ast.String:
		return "string"
	case ast.Number:
		return "number"
	case ast.Boolean:
		return "boolean"
	case ast.Null:
		return "null"
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", v), "ast.")
	}
}

// sortedKeys is unused at runtime but documents that the JSON map marshals with
// sorted keys (encoding/json sorts map keys), giving a stable golden.
var _ = sort.Strings
