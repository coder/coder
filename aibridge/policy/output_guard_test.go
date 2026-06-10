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
// mirror of the input shape guard): the host may **widen** what a kind's
// entrypoint rule is allowed to emit, but never **narrow** it. Narrowing (e.g.
// decide demanding an object instead of a bare string, or renaming the
// entrypoint rule) silently breaks already-deployed policies that emit the old
// shape.
//
// Unlike the input envelope (which the host builds, so the guard walks the built
// value), the output is produced by Rego and consumed by the host's type
// assertions in {classify,decide,route,transform}.go. So this guard is
// behavioral: it feeds each kind's representative accepted output through the
// real consumer and asserts it is still accepted, then pins the contract
// (entrypoint rule + accepted-output descriptor) per kind in a golden keyed by
// CurrentOutputSchemaVersion. A contract change forces a new golden + a version
// bump, the same coupling as the input guard.
//
// To widen an output contract: bump CurrentOutputSchemaVersion, add the new
// accepted shape to the acceptance cases below, then regenerate with
//
//	POLICY_SCHEMA_UPDATE=1 go test ./aibridge/policy/ -run TestOutputContractGuard

// outputCase is a representative, currently-accepted output for a kind. evaluate
// runs the kind's real consumer over a module that emits sample and asserts the
// consumer accepts it.
type outputCase struct {
	kind     Kind
	accepts  string // human descriptor of the accepted output, pinned in the golden
	module   string // Rego that emits a representative valid output
	evaluate func(t *testing.T, module string)
}

func outputCases() []outputCase {
	return []outputCase{
		{
			kind:    KindDecide,
			accepts: "string verdict in {ALLOW,LOG,BLOCK}",
			module:  `default verdict := "ALLOW"`,
			evaluate: func(t *testing.T, module string) {
				d, err := NewDecide("guard-decide", module)
				require.NoError(t, err)
				v, err := d.Evaluate(t.Context(), mustReqInput(t))
				require.NoError(t, err)
				require.Equal(t, VerdictAllow, v)
			},
		},
		{
			kind:    KindClassify,
			accepts: "object of arbitrary annotation keys",
			module:  `annotations := {"risk": "high"}`,
			evaluate: func(t *testing.T, module string) {
				c, err := NewClassify("guard-classify", module)
				require.NoError(t, err)
				m, ok, err := c.Evaluate(t.Context(), mustReqInput(t))
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "high", m["risk"])
			},
		},
		{
			kind:    KindRoute,
			accepts: "string model name",
			module:  `model := "gpt-4o"`,
			evaluate: func(t *testing.T, module string) {
				r, err := NewRoute("guard-route", module)
				require.NoError(t, err)
				s, ok, err := r.Evaluate(t.Context(), mustReqInput(t))
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "gpt-4o", s)
			},
		},
		{
			kind:    KindTransform,
			accepts: "JSON object body",
			module:  `body := {"model": "gpt-4o", "max_tokens": 10}`,
			evaluate: func(t *testing.T, module string) {
				tr, err := NewTransform("guard-transform", module)
				require.NoError(t, err)
				b, ok, err := tr.Evaluate(t.Context(), mustReqInput(t))
				require.NoError(t, err)
				require.True(t, ok)
				require.JSONEq(t, `{"model":"gpt-4o","max_tokens":10}`, string(b))
			},
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

	// Behavioral half: each kind's representative output is still accepted by
	// its consumer. A narrowing (tightened type assertion or renamed entrypoint
	// rule) fails here.
	contract := map[string]map[string]string{}
	for _, tc := range outputCases() {
		tc.evaluate(t, tc.module)
		rule, ok := EntrypointRule(tc.kind)
		require.Truef(t, ok, "no entrypoint rule for kind %q", tc.kind)
		contract[string(tc.kind)] = map[string]string{"rule": rule, "accepts": tc.accepts}
	}

	// Golden half: pin (kind -> rule, accepts) keyed by CurrentOutputSchemaVersion,
	// coupling any contract change to a version bump.
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
		"output contract changed for v%d. An entrypoint rule or accepted output type was narrowed/renamed "+
			"(a BC break) or widened without bumping CurrentOutputSchemaVersion. To widen, bump "+
			"CurrentOutputSchemaVersion and regenerate the golden with POLICY_SCHEMA_UPDATE=1.", CurrentOutputSchemaVersion)

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
