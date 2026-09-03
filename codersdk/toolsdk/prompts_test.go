package toolsdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/toolsdk"
)

func TestChatPrompts(t *testing.T) {
	t.Parallel()

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()
		names := map[string]bool{}
		for _, prompt := range toolsdk.AllPrompts {
			require.NotEmpty(t, prompt.Name)
			require.NotEmpty(t, prompt.Description)
			require.NotNil(t, prompt.Render)
			require.NotEmpty(t, prompt.RequiredTools)
			toolNames := make(map[string]bool, len(toolsdk.All))
			for _, tool := range toolsdk.All {
				toolNames[tool.Name] = true
			}
			for _, name := range prompt.RequiredTools {
				require.True(t, toolNames[name], "prompt %q requires unknown tool %q", prompt.Name, name)
			}
			require.False(t, names[prompt.Name], "duplicate prompt name %q", prompt.Name)
			names[prompt.Name] = true
			for _, arg := range prompt.Arguments {
				require.NotEmpty(t, arg.Name)
				require.NotEmpty(t, arg.Description)
			}
		}
	})

	t.Run("DelegateRequiresTask", func(t *testing.T) {
		t.Parallel()
		_, err := toolsdk.AgentsDelegate.Render(nil)
		require.ErrorContains(t, err, "missing required prompt argument: task")
		_, err = toolsdk.AgentsDelegate.Render(map[string]string{"task": "  "})
		require.ErrorContains(t, err, "missing required prompt argument: task")
	})

	t.Run("Delegate", func(t *testing.T) {
		t.Parallel()
		text, err := toolsdk.AgentsDelegate.Render(map[string]string{"task": "Fix the flaky test."})
		require.NoError(t, err)
		require.Contains(t, text, "Fix the flaky test.")
		for _, tool := range toolsdk.AgentsDelegate.RequiredTools {
			require.Contains(t, text, tool)
		}
	})

	t.Run("DelegateWithModelConfig", func(t *testing.T) {
		t.Parallel()
		text, err := toolsdk.AgentsDelegate.Render(map[string]string{
			"task":            "Fix the flaky test.",
			"model_config_id": "a2913789-b213-45e3-9d18-561fbb1ec97c",
		})
		require.NoError(t, err)
		require.Contains(t, text, "a2913789-b213-45e3-9d18-561fbb1ec97c")
		require.NotContains(t, text, toolsdk.ToolNameListChatModelConfigs)
	})

	t.Run("CheckRequiresChatID", func(t *testing.T) {
		t.Parallel()
		_, err := toolsdk.AgentsCheck.Render(map[string]string{})
		require.ErrorContains(t, err, "missing required prompt argument: chat_id")
	})

	t.Run("Check", func(t *testing.T) {
		t.Parallel()
		text, err := toolsdk.AgentsCheck.Render(map[string]string{"chat_id": "0bb52d1a-e239-4e7a-ae2a-5abbd7fbf9b5"})
		require.NoError(t, err)
		require.Contains(t, text, "0bb52d1a-e239-4e7a-ae2a-5abbd7fbf9b5")
		for _, tool := range toolsdk.AgentsCheck.RequiredTools {
			require.Contains(t, text, tool)
		}
	})
}
