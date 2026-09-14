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

func TestDelegateOrganizationAndModel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, organization, model string }{
		{name: "Default"},
		{name: "Organization", organization: "6f225c30-08f9-475c-a64c-611da5db5a40"},
		{name: "Model", model: "a2913789-b213-45e3-9d18-561fbb1ec97c"},
		{name: "Both", organization: "6f225c30-08f9-475c-a64c-611da5db5a40", model: "a2913789-b213-45e3-9d18-561fbb1ec97c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := toolsdk.AgentsDelegate.Render(map[string]string{"task": "Fix tests", "organization_id": tc.organization, "model_config_id": tc.model})
			require.NoError(t, err)
			if tc.organization != "" {
				require.Contains(t, text, `organization_id "`+tc.organization+`"`)
			} else {
				require.Contains(t, text, "coder_list_organizations")
				require.Contains(t, text, "exactly one organization")
			}
			if tc.model != "" {
				require.Contains(t, text, `model_config_id "`+tc.model+`"`)
			} else {
				require.Contains(t, text, "Omit model_config_id")
				require.Contains(t, text, "personal override")
			}
		})
	}
	for _, name := range []string{"organization_id", "model_config_id"} {
		found := false
		for _, arg := range toolsdk.AgentsDelegate.Arguments {
			if arg.Name == name {
				found = true
				require.False(t, arg.Required)
			}
		}
		require.True(t, found, name)
	}
}
