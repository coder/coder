package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// This guard enforces the output-contract backward-compatibility rule (the
// mirror of the input shape guard): the host may **widen** what a kind may emit,
// but never **narrow** it. Narrowing (e.g. decide demanding an object instead of
// a bare string, renaming the entrypoint rule, or dropping the optional message)
// silently breaks already-deployed policies.
//
// A kind's output contract is the bounded set of named rules it may produce and,
// per rule, the type the host consumes, whether the host always resolves a value
// (required), the constrained value set (enum), and whether a blank/undefined
// value falls back to the host default:
//
//   - annotate  -> annotations (object). Nothing else.
//   - route     -> model (string): the request mutation it may influence.
//   - transform -> body (any JSON value): the request mutation it may influence.
//   - decide    -> verdict (required string enum {ALLOW,LOG,BLOCK}) and message
//     (optional string; undefined or blank uses the host's default message).
//
// The contract is declared below and every declared property is verified
// **behaviorally** against the real consumer in {annotate,decide,route,
// transform}.go, so the declaration cannot drift from what the host actually
// accepts. The declaration is then pinned in a golden keyed by
// CurrentOutputSchemaVersion, coupling any contract change to a version bump,
// exactly like the input guard.
//
// To widen an output contract: bump CurrentOutputSchemaVersion, update the
// declaration and its behavioral checks, then regenerate with
//
//	POLICY_SCHEMA_UPDATE=1 go test ./aibridge/policy/ -run TestOutputContractGuard

// fieldContract structurally describes one output rule a kind may emit.
type fieldContract struct {
	// Type is the value type the host consumes: "string", "object", or "json"
	// (any JSON value).
	Type string `json:"type"`
	// Required reports that the host always resolves a value for this rule (it
	// is default-backed); an undefined rule is not a skip. Only decide's verdict
	// is required (an undefined verdict defaults to ALLOW, and registration
	// enforces a `default verdict`).
	Required bool `json:"required"`
	// Enum, when set, is the constrained set of accepted string values; any
	// other value is rejected.
	Enum []string `json:"enum,omitempty"`
	// BlankUsesDefault reports that an undefined or empty value falls back to the
	// host's default rather than taking effect.
	BlankUsesDefault bool `json:"blank_uses_default,omitempty"`
}

// outputContract declares the bounded output set per kind. It is verified
// behaviorally by TestOutputContractGuard and pinned in the golden.
func outputContract() map[string]map[string]fieldContract {
	return map[string]map[string]fieldContract{
		"annotate": {
			"annotations": {Type: "object"},
		},
		"route": {
			"model": {Type: "string"},
		},
		"transform": {
			"body":    {Type: "json"},
			"headers": {Type: "object<string>"},
		},
		"decide": {
			"verdict": {Type: "string", Required: true, Enum: []string{"ALLOW", "LOG", "BLOCK"}},
			"message": {Type: "string", BlankUsesDefault: true},
		},
	}
}

func mustReqInput(t *testing.T) Input {
	t.Helper()
	in, err := PreReqEnvelope{Request: []byte(`{"model":"x"}`)}.Build()
	require.NoError(t, err)
	return in
}

func TestOutputContractGuard(t *testing.T) {
	t.Parallel()

	contract := outputContract()

	// Every declared entrypoint rule must match EntrypointRule (the source of
	// truth a renamed rule would diverge from), and decide additionally carries
	// the optional message rule.
	for _, kind := range []Kind{KindAnnotate, KindRoute, KindTransform, KindDecide} {
		rule, ok := EntrypointRule(kind)
		require.Truef(t, ok, "no entrypoint rule for kind %q", kind)
		_, has := contract[string(kind)][rule]
		require.Truef(t, has, "contract for kind %q is missing its entrypoint rule %q", kind, rule)
	}

	// Behavioral half: prove each declared property against the real consumer.
	verifyAnnotateContract(t)
	verifyRouteContract(t)
	verifyTransformContract(t)
	verifyDecideContract(t)

	// Golden half: pin the declared contract keyed by CurrentOutputSchemaVersion.
	dir := filepath.Join("testdata", "output_shape")
	cur := filepath.Join(dir, fmt.Sprintf("v%d.json", CurrentOutputSchemaVersion))

	got, err := json.MarshalIndent(contract, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	if os.Getenv("POLICY_SCHEMA_UPDATE") != "" {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(cur, got, 0o600))
	}

	want, err := os.ReadFile(cur)
	require.NoErrorf(t, err, "no golden for CurrentOutputSchemaVersion=%d; regenerate with POLICY_SCHEMA_UPDATE=1", CurrentOutputSchemaVersion)
	require.Equalf(t, string(want), string(got),
		"output contract changed for v%d. An output rule was narrowed/renamed/dropped (a BC break) "+
			"or widened without bumping CurrentOutputSchemaVersion. To widen, bump CurrentOutputSchemaVersion "+
			"and regenerate the golden with POLICY_SCHEMA_UPDATE=1.", CurrentOutputSchemaVersion)

	for v := SchemaVersion(1); v < CurrentOutputSchemaVersion; v++ {
		prior, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("v%d.json", v)))
		if err != nil {
			continue
		}
		require.NotEqualf(t, string(prior), string(got),
			"output contract for v%d is identical to frozen v%d; a version bump must reflect a real change",
			CurrentOutputSchemaVersion, v)
	}
}

// verifyAnnotateContract proves annotations is an optional object: an object is
// accepted, a non-object is rejected, and an undefined rule is a skip. It
// exercises the typed decode method (annotations) directly, the per-kind struct
// that projects into a StageResult.
func verifyAnnotateContract(t *testing.T) {
	t.Helper()
	annotate := func(module string) (bool, error) {
		a, err := NewAnnotate("guard-annotate", module)
		require.NoError(t, err)
		_, ok, err := a.annotations(t.Context(), mustReqInput(t))
		return ok, err
	}

	ok, err := annotate(`annotations := {"risk": "high"}`)
	require.NoError(t, err)
	require.True(t, ok, "object annotations must be accepted")

	_, err = annotate(`annotations := "high"`)
	require.Error(t, err, "non-object annotations must be rejected")

	ok, err = annotate(``)
	require.NoError(t, err)
	require.False(t, ok, "undefined annotations must be a skip (not required)")
}

// verifyRouteContract proves model is an optional string: a string is accepted,
// a non-string is rejected, and an undefined rule is a skip.
func verifyRouteContract(t *testing.T) {
	t.Helper()
	route := func(module string) (bool, error) {
		r, err := NewRoute("guard-route", module)
		require.NoError(t, err)
		_, ok, err := r.route(t.Context(), mustReqInput(t))
		return ok, err
	}

	ok, err := route(`model := "gpt-4o"`)
	require.NoError(t, err)
	require.True(t, ok, "string model must be accepted")

	_, err = route(`model := 42`)
	require.Error(t, err, "non-string model must be rejected")

	ok, err = route(``)
	require.NoError(t, err)
	require.False(t, ok, "undefined model must be a skip (not required)")
}

// verifyTransformContract proves body accepts any JSON value (the host
// re-validates downstream), headers accept an object of string values (any
// non-string value rejected), and that an undefined transform is a skip.
func verifyTransformContract(t *testing.T) {
	t.Helper()
	transform := func(module string) (Transformation, bool, error) {
		tr, err := NewTransform("guard-transform", module)
		require.NoError(t, err)
		return tr.transformation(t.Context(), mustReqInput(t))
	}

	// body accepts any JSON value.
	for _, expr := range []string{`{"model": "x"}`, `"x"`, `42`, `["x"]`} {
		_, ok, err := transform(`body := ` + expr)
		require.NoErrorf(t, err, "body := %s must be accepted", expr)
		require.Truef(t, ok, "body := %s must be accepted", expr)
	}

	// headers accept an object of string values.
	tf, ok, err := transform(`headers := {"x-foo": "bar"}`)
	require.NoError(t, err)
	require.True(t, ok, "object<string> headers must be accepted")
	require.Equal(t, map[string]string{"x-foo": "bar"}, tf.Headers)
	require.Nil(t, tf.Edits, "headers-only transform leaves the body unchanged")

	// a non-string header value is rejected.
	_, _, err = transform(`headers := {"x-foo": 42}`)
	require.Error(t, err, "a non-string header value must be rejected")

	// a non-object headers rule is rejected.
	_, _, err = transform(`headers := "x"`)
	require.Error(t, err, "non-object headers must be rejected")

	// an undefined transform (neither body nor headers) is a skip.
	_, ok, err = transform(``)
	require.NoError(t, err)
	require.False(t, ok, "undefined transform must be a skip (not required)")
}

// verifyDecideContract proves the verdict enum and required-ness, and the
// optional, string-only, blank-uses-default message.
func verifyDecideContract(t *testing.T) {
	t.Helper()
	decide := func(module string) (Decision, error) {
		d, err := NewDecide("guard-decide", module)
		require.NoError(t, err)
		return d.decision(t.Context(), mustReqInput(t))
	}

	// verdict: exactly {ALLOW,LOG,BLOCK} accepted; any other value rejected.
	for _, v := range []Verdict{VerdictAllow, VerdictLog, VerdictBlock} {
		dec, err := decide(fmt.Sprintf("verdict := %q", v))
		require.NoErrorf(t, err, "verdict %q must be accepted", v)
		require.Equal(t, v, dec.Verdict)
	}
	_, err := decide(`verdict := "MAYBE"`)
	require.Error(t, err, "an unrecognized verdict string must be rejected")
	_, err = decide(`verdict := 42`)
	require.Error(t, err, "a non-string verdict must be rejected")

	// verdict required: an undefined verdict resolves to the ALLOW default, the
	// host always gets a verdict (no skip).
	dec, err := decide(``)
	require.NoError(t, err)
	require.Equal(t, VerdictAllow, dec.Verdict)

	// message: a string surfaces on a block.
	dec, err = decide("verdict := \"BLOCK\"\nmessage := \"denied\"")
	require.NoError(t, err)
	require.Equal(t, VerdictBlock, dec.Verdict)
	require.Equal(t, "denied", dec.Message)

	// message blank/undefined/non-string uses the default (empty Message), and
	// never errors or alters the verdict.
	for name, module := range map[string]string{
		"blank":     "verdict := \"BLOCK\"\nmessage := \"\"",
		"undefined": `verdict := "BLOCK"`,
		"nonstring": "verdict := \"BLOCK\"\nmessage := 42",
	} {
		dec, err := decide(module)
		require.NoErrorf(t, err, "%s message must not error", name)
		require.Equalf(t, VerdictBlock, dec.Verdict, "%s message must not alter the verdict", name)
		require.Emptyf(t, dec.Message, "%s message must fall back to the default", name)
	}

	// message is only consumed on a block: a non-blocking verdict never surfaces
	// it.
	dec, err = decide("verdict := \"LOG\"\nmessage := \"unused\"")
	require.NoError(t, err)
	require.Equal(t, VerdictLog, dec.Verdict)
	require.Empty(t, dec.Message, "message must not surface on a non-blocking verdict")
}
