package provisionerdserver_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/provisionerdserver"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestMergeExtraEnvs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		initial   map[string]string
		envs      []*sdkproto.Env
		expected  map[string]string
		expectErr string
	}{
		{
			name:     "empty",
			initial:  map[string]string{},
			envs:     nil,
			expected: map[string]string{},
		},
		{
			name:    "default_replace",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "bar"},
			},
			expected: map[string]string{"FOO": "bar"},
		},
		{
			name:    "explicit_replace",
			initial: map[string]string{"FOO": "old"},
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "new", MergeStrategy: "replace"},
			},
			expected: map[string]string{"FOO": "new"},
		},
		{
			name:    "empty_strategy_defaults_to_replace",
			initial: map[string]string{"FOO": "old"},
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "new", MergeStrategy: ""},
			},
			expected: map[string]string{"FOO": "new"},
		},
		{
			name:    "append_to_existing",
			initial: map[string]string{"PATH": "/usr/bin"},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/custom/bin", MergeStrategy: "append"},
			},
			expected: map[string]string{"PATH": "/usr/bin:/custom/bin"},
		},
		{
			name:    "append_no_existing",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/custom/bin", MergeStrategy: "append"},
			},
			expected: map[string]string{"PATH": "/custom/bin"},
		},
		{
			name:    "append_to_empty_value",
			initial: map[string]string{"PATH": ""},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/custom/bin", MergeStrategy: "append"},
			},
			expected: map[string]string{"PATH": "/custom/bin"},
		},
		{
			name:    "prepend_to_existing",
			initial: map[string]string{"PATH": "/usr/bin"},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/custom/bin", MergeStrategy: "prepend"},
			},
			expected: map[string]string{"PATH": "/custom/bin:/usr/bin"},
		},
		{
			name:    "prepend_no_existing",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/custom/bin", MergeStrategy: "prepend"},
			},
			expected: map[string]string{"PATH": "/custom/bin"},
		},
		{
			name:    "error_no_duplicate",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "bar", MergeStrategy: "error"},
			},
			expected: map[string]string{"FOO": "bar"},
		},
		{
			name:    "error_with_duplicate",
			initial: map[string]string{"FOO": "existing"},
			envs: []*sdkproto.Env{
				{Name: "FOO", Value: "new", MergeStrategy: "error"},
			},
			expectErr: "duplicate env var",
		},
		{
			name:    "multiple_appends_same_key",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/a/bin", MergeStrategy: "append"},
				{Name: "PATH", Value: "/b/bin", MergeStrategy: "append"},
			},
			expected: map[string]string{"PATH": "/a/bin:/b/bin"},
		},
		{
			name:    "multiple_prepends_same_key",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/a/bin", MergeStrategy: "prepend"},
				{Name: "PATH", Value: "/b/bin", MergeStrategy: "prepend"},
			},
			expected: map[string]string{"PATH": "/b/bin:/a/bin"},
		},
		{
			name:    "mixed_strategies",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/first", MergeStrategy: "append"},
				{Name: "PATH", Value: "/override", MergeStrategy: "replace"},
			},
			expected: map[string]string{"PATH": "/override"},
		},
		{
			name:    "mixed_keys",
			initial: map[string]string{},
			envs: []*sdkproto.Env{
				{Name: "PATH", Value: "/a", MergeStrategy: "append"},
				{Name: "HOME", Value: "/home/user"},
				{Name: "PATH", Value: "/b", MergeStrategy: "append"},
			},
			expected: map[string]string{
				"PATH": "/a:/b",
				"HOME": "/home/user",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := make(map[string]string)
			for k, v := range tc.initial {
				env[k] = v
			}
			err := provisionerdserver.MergeExtraEnvs(env, tc.envs)
			if tc.expectErr != "" {
				require.ErrorContains(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, env)
		})
	}
}
