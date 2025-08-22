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

func TestGenerateFallback(t *testing.T) {
	t.Parallel()

	name := taskname.GenerateFallback()
	err := codersdk.NameValid(name)
	require.NoErrorf(t, err, "expected fallback to be valid workspace name, instead found %s", name)
}

func TestGenerateTaskName(t *testing.T) {
	t.Parallel()

	t.Run("Fallback", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		name, err := taskname.Generate(ctx, "Some random prompt")
		require.ErrorIs(t, err, taskname.ErrNoAPIKey)
		require.Equal(t, "", name)
	})

	t.Run("Anthropic", func(t *testing.T) {
		t.Parallel()

		apiKey := os.Getenv(anthropicEnvVar)
		if apiKey == "" {
			t.Skipf("Skipping test as %s not set", anthropicEnvVar)
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		name, err := taskname.Generate(ctx, "Create a finance planning app", taskname.WithAPIKey(apiKey))
		require.NoError(t, err)
		require.NotEqual(t, "", name)

		err = codersdk.NameValid(name)
		require.NoError(t, err, "name should be valid")
	})
}
