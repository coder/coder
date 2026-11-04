package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendGitConfigEnv(t *testing.T) {
	t.Parallel()

	const helper = "!'/tmp/coder.abc/coder' gitcredential"

	for _, tc := range []struct {
		name   string
		envs   map[string]string
		expect map[string]string
	}{
		{
			name: "Empty",
			envs: map[string]string{},
			expect: map[string]string{
				"GIT_CONFIG_COUNT":   "1",
				"GIT_CONFIG_KEY_0":   "credential.helper",
				"GIT_CONFIG_VALUE_0": helper,
			},
		},
		{
			name: "AppendsAfterExisting",
			envs: map[string]string{
				"GIT_CONFIG_COUNT":   "1",
				"GIT_CONFIG_KEY_0":   "safe.directory",
				"GIT_CONFIG_VALUE_0": "*",
			},
			expect: map[string]string{
				"GIT_CONFIG_COUNT":   "2",
				"GIT_CONFIG_KEY_0":   "safe.directory",
				"GIT_CONFIG_VALUE_0": "*",
				"GIT_CONFIG_KEY_1":   "credential.helper",
				"GIT_CONFIG_VALUE_1": helper,
			},
		},
		{
			name: "AlreadyPresent",
			envs: map[string]string{
				"GIT_CONFIG_COUNT":   "1",
				"GIT_CONFIG_KEY_0":   "credential.helper",
				"GIT_CONFIG_VALUE_0": helper,
			},
			expect: map[string]string{
				"GIT_CONFIG_COUNT":   "1",
				"GIT_CONFIG_KEY_0":   "credential.helper",
				"GIT_CONFIG_VALUE_0": helper,
			},
		},
		{
			name: "InvalidCount",
			envs: map[string]string{
				"GIT_CONFIG_COUNT": "bogus",
			},
			expect: map[string]string{
				"GIT_CONFIG_COUNT": "bogus",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appendGitConfigEnv(tc.envs, "credential.helper", helper)
			require.Equal(t, tc.expect, tc.envs)
		})
	}
}

func TestGitCredentialHelperCommand(t *testing.T) {
	t.Parallel()

	require.Equal(t, "!'/tmp/coder.abc/coder' gitcredential", GitCredentialHelperCommand("/tmp/coder.abc/coder"))
	require.Equal(t, "!'C:/Users/Jane Doe/coder.exe' gitcredential", GitCredentialHelperCommand(`C:\Users\Jane Doe\coder.exe`))
	require.Equal(t, `!'/tmp/it'\''s/coder' gitcredential`, GitCredentialHelperCommand("/tmp/it's/coder"))
}
