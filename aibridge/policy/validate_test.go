package policy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/policy"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		kind    policy.Kind
		module  string
		wantErr bool
	}{
		{
			name:   "decide with default verdict",
			kind:   policy.KindDecide,
			module: "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if input.request.body.model == \"x\"",
		},
		{
			name:    "decide missing default fails open",
			kind:    policy.KindDecide,
			module:  "verdict := \"BLOCK\" if input.request.body.model == \"x\"",
			wantErr: true,
		},
		{
			name:    "decide with wrong entrypoint rule",
			kind:    policy.KindDecide,
			module:  "default model := \"gpt-4o\"",
			wantErr: true,
		},
		{
			name:   "classify defines annotations",
			kind:   policy.KindAnnotate,
			module: "annotations := {\"risk\": \"high\"}",
		},
		{
			name:    "classify missing annotations",
			kind:    policy.KindAnnotate,
			module:  "verdict := \"ALLOW\"",
			wantErr: true,
		},
		{
			name:   "route defines model",
			kind:   policy.KindRoute,
			module: "model := \"gpt-4o-mini\" if input.request.body.model == \"gpt-4o\"",
		},
		{
			name:   "transform defines body",
			kind:   policy.KindTransform,
			module: "body := object.union(input.request.body, {\"max_tokens\": 100})",
		},
		{
			name:    "invalid rego",
			kind:    policy.KindDecide,
			module:  "default verdict :=",
			wantErr: true,
		},
		{
			name:    "unknown kind",
			kind:    policy.Kind("bogus"),
			module:  "verdict := \"ALLOW\"",
			wantErr: true,
		},
		{
			name:    "non-deterministic http.send is rejected",
			kind:    policy.KindDecide,
			module:  "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if http.send({\"method\": \"get\", \"url\": \"http://example.com\"}).status_code == 200",
			wantErr: true,
		},
		{
			name:    "non-deterministic time.now_ns is rejected",
			kind:    policy.KindDecide,
			module:  "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if time.now_ns() > 0",
			wantErr: true,
		},
		{
			name:    "non-deterministic rand.intn is rejected",
			kind:    policy.KindDecide,
			module:  "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if rand.intn(\"x\", 10) == 0",
			wantErr: true,
		},
		{
			name:   "deterministic builtins are allowed",
			kind:   policy.KindDecide,
			module: "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if contains(input.request.body.model, \"gpt\")",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := policy.Validate(tc.kind, tc.module)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestHermeticCapabilities_PrepareRejectsNonDeterministic asserts the prepare
// (load) path enforces hermeticity too, not just the validation gate, so a
// policy that somehow bypassed Validate still cannot evaluate against a
// non-deterministic builtin.
func TestHermeticCapabilities_PrepareRejectsNonDeterministic(t *testing.T) {
	t.Parallel()

	_, err := policy.NewDecide("net", "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if http.send({\"method\": \"get\", \"url\": \"http://example.com\"}).status_code == 200")
	require.Error(t, err)

	_, err = policy.NewDecide("ok", "default verdict := \"ALLOW\"\nverdict := \"BLOCK\" if contains(input.request.body.model, \"gpt\")")
	require.NoError(t, err)
}
