package taskname_test

import (
	"os"
	"testing"

	"github.com/coder/coder/v2/coderd/taskname"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/require"
)

const (
	anthropicApiKeyEnv = "ANTHROPIC_API_KEY"
)

func TestGenerateTaskName(t *testing.T) {
	t.Run("Fallback", func(t *testing.T) {
		if apiKey := os.Getenv(anthropicApiKeyEnv); apiKey != "" {
			os.Setenv(anthropicApiKeyEnv, "")

			t.Cleanup(func() {
				os.Setenv(anthropicApiKeyEnv, apiKey)
			})
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		name, err := taskname.Generate(ctx, "Some random prompt", "task-fallback")
		require.NoError(t, err)
		require.Equal(t, "task-fallback", name)
	})

	t.Run("Anthropic", func(t *testing.T) {
		if apiKey := os.Getenv(anthropicApiKeyEnv); apiKey == "" {
			t.Skipf("Skipping test as %s not set", anthropicApiKeyEnv)
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		name, err := taskname.Generate(ctx, `Create a finance planning app`, "task-fallback")
		require.NoError(t, err)
		require.NotEqual(t, "task-fallback", name)

		err = codersdk.NameValid(name)
		require.NoError(t, err, "name should be valid")
	})

}
