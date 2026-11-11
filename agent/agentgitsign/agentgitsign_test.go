package agentgitsign_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentgitsign"
)

func toEnv(envs map[string]string) []string {
	env := make([]string, 0, len(envs))
	for k, v := range envs {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(env)
	return env
}

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

func TestStrip(t *testing.T) {
	t.Parallel()

	t.Run("KeepsExistingEntries", func(t *testing.T) {
		t.Parallel()
		base := map[string]string{
			"PATH":               "/bin",
			"GIT_CONFIG_COUNT":   "1",
			"GIT_CONFIG_KEY_0":   "core.editor",
			"GIT_CONFIG_VALUE_0": "vim",
		}
		envs := map[string]string{}
		for k, v := range base {
			envs[k] = v
		}
		agentgitsign.Append(envs, "coder", "ssh-ed25519 AAAA")

		got := agentgitsign.Strip(toEnv(envs))
		sort.Strings(got)
		require.Equal(t, toEnv(base), got)
	})

	t.Run("RemovesCountWhenOnlySigning", func(t *testing.T) {
		t.Parallel()
		envs := map[string]string{"PATH": "/bin"}
		agentgitsign.Append(envs, "coder", "ssh-ed25519 AAAA")
		require.Equal(t, []string{"PATH=/bin"}, agentgitsign.Strip(toEnv(envs)))
	})

	t.Run("NoSigningConfig", func(t *testing.T) {
		t.Parallel()
		env := []string{"PATH=/bin", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.editor", "GIT_CONFIG_VALUE_0=vim"}
		require.Equal(t, env, agentgitsign.Strip(env))
		require.Equal(t, []string{"PATH=/bin"}, agentgitsign.Strip([]string{"PATH=/bin"}))
	})
}
