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

func TestGenerate(t *testing.T) {
	t.Run("FromPrompt", func(t *testing.T) {
		// Ensure no API key in env for this test
		t.Setenv("ANTHROPIC_API_KEY", "")

		ctx := testutil.Context(t, testutil.WaitShort)

		taskName := taskname.Generate(ctx, testutil.Logger(t), "Create a finance planning app")

		// Should succeed via prompt sanitization
		require.NoError(t, codersdk.NameValid(taskName.Name))
		require.Contains(t, taskName.Name, "create-a-finance-planning-")
		require.NotEmpty(t, taskName.DisplayName)
		require.Equal(t, "Create a finance planning app", taskName.DisplayName)
	})

	t.Run("FromAnthropic", func(t *testing.T) {
		apiKey := os.Getenv(anthropicEnvVar)
		if apiKey == "" {
			t.Skipf("Skipping test as %s not set", anthropicEnvVar)
		}

		// Set API key for this test
		t.Setenv("ANTHROPIC_API_KEY", apiKey)

		ctx := testutil.Context(t, testutil.WaitShort)

		taskName := taskname.Generate(ctx, testutil.Logger(t), "Create a finance planning app")

		// Should succeed with Claude-generated names
		require.NoError(t, codersdk.NameValid(taskName.Name))
		require.NotEmpty(t, taskName.DisplayName)
	})

	t.Run("Fallback", func(t *testing.T) {
		// Ensure no API key
		t.Setenv("ANTHROPIC_API_KEY", "")

		ctx := testutil.Context(t, testutil.WaitShort)

		// Use a prompt that can't be sanitized (only special chars)
		taskName := taskname.Generate(ctx, testutil.Logger(t), "!@#$%^&*()")

		// Should fall back to random name
		require.NoError(t, codersdk.NameValid(taskName.Name))
		require.NotEmpty(t, taskName.DisplayName)
	})
}
