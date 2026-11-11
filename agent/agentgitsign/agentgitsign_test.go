package agentgitsign_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentgitsign"
)

func TestAppend(t *testing.T) {
	t.Parallel()

	envs := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "core.editor",
		"GIT_CONFIG_VALUE_0": "vim",
	}
	agentgitsign.Append(envs, "/usr/bin/coder", "ssh-ed25519 AAAA\n")
	require.Equal(t, "6", envs["GIT_CONFIG_COUNT"])
	require.Equal(t, "core.editor", envs["GIT_CONFIG_KEY_0"])
	require.Equal(t, "commit.gpgsign", envs["GIT_CONFIG_KEY_1"])
	require.Equal(t, "/usr/bin/coder", envs["GIT_CONFIG_VALUE_4"])
	require.Equal(t, "key::ssh-ed25519 AAAA", envs["GIT_CONFIG_VALUE_5"])

	// An invalid count is replaced rather than extended.
	envs = map[string]string{"GIT_CONFIG_COUNT": "-3"}
	agentgitsign.Append(envs, "coder", "ssh-ed25519 AAAA")
	require.Equal(t, "5", envs["GIT_CONFIG_COUNT"])
	require.Equal(t, "commit.gpgsign", envs["GIT_CONFIG_KEY_0"])
}
