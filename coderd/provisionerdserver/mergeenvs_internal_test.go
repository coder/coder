package provisionerdserver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestEncodeSubagentEnvsErrorStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		envs      []*sdkproto.Env
		expected  map[string]string
		expectErr string
	}{
		{
			name: "error_no_duplicate",
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "bar", MergeStrategy: "error"},
			},
			expected: map[string]string{"FOO": "bar"},
		},
		{
			name: "error_then_replace",
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "first", MergeStrategy: "error"},
				{Name: "FOO", Value: "second", MergeStrategy: "replace"},
			},
			expectErr: "duplicate env var",
		},
		{
			name: "error_then_append",
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/a", MergeStrategy: "error"},
				{Name: "PATH", Value: "/b", MergeStrategy: "append"},
			},
			expectErr: "duplicate env var",
		},
		{
			name: "replace_then_error",
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "first", MergeStrategy: "replace"},
				{Name: "FOO", Value: "second", MergeStrategy: "error"},
			},
			expectErr: "duplicate env var",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := encodeSubagentEnvs(tc.envs)
			if tc.expectErr != "" {
				require.ErrorContains(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			var got map[string]string
			require.NoError(t, json.Unmarshal(raw.RawMessage, &got))
			require.Equal(t, tc.expected, got)
		})
	}
}
