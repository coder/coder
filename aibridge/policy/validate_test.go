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
			kind:   policy.KindClassify,
			module: "annotations := {\"risk\": \"high\"}",
		},
		{
			name:    "classify missing annotations",
			kind:    policy.KindClassify,
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
