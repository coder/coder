package taskname_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/taskname"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	anthropicEnvVar = "ANTHROPIC_API_KEY"
)

//nolint:paralleltest // test modifies env variables
func TestGenerateTaskName(t *testing.T) {
	t.Run("Fallback", func(t *testing.T) {
		if apiKey := os.Getenv(anthropicEnvVar); apiKey != "" {
			os.Setenv(anthropicEnvVar, "")

			t.Cleanup(func() {
				os.Setenv(anthropicEnvVar, apiKey)
			})
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		name, err := taskname.Generate(ctx, "Some random prompt", "task-fallback")
		require.NoError(t, err)
		require.Equal(t, "task-fallback", name)
	})

	t.Run("Anthropic", func(t *testing.T) {
		if apiKey := os.Getenv(anthropicEnvVar); apiKey == "" {
			t.Skipf("Skipping test as %s not set", anthropicEnvVar)
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		name, err := taskname.Generate(ctx, `Create a finance planning app`, "task-fallback")
		require.NoError(t, err)
		require.NotEqual(t, "task-fallback", name)

		err = codersdk.NameValid(name)
		require.NoError(t, err, "name should be valid")
	})
}
