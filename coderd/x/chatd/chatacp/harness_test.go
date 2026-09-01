package chatacp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/codersdk"
)

func TestHarnessFor(t *testing.T) {
	t.Parallel()

	_, ok := chatacp.HarnessFor(codersdk.ChatRuntimeCoder)
	require.False(t, ok, "the built-in runtime has no harness")

	for _, tc := range []struct {
		runtime  codersdk.ChatRuntime
		command  string
		provider codersdk.AIProviderType
		mode     string
	}{
		{codersdk.ChatRuntimeClaudeCode, "claude-agent-acp", codersdk.AIProviderTypeAnthropic, ""},
		{codersdk.ChatRuntimeCodex, "codex-acp", codersdk.AIProviderTypeOpenAI, "agent-full-access"},
	} {
		harness, ok := chatacp.HarnessFor(tc.runtime)
		require.True(t, ok, tc.runtime)
		require.Equal(t, tc.runtime, harness.Runtime)
		require.Equal(t, tc.command, harness.Command)
		require.Equal(t, tc.provider, harness.ProviderType)
		require.Equal(t, tc.mode, harness.DefaultSessionMode)
		require.NotEmpty(t, harness.DisplayName)
		require.NotEmpty(t, harness.ProviderLabel)
		require.NotNil(t, harness.Env)
	}
}

func TestClaudeCodeEnv(t *testing.T) {
	t.Parallel()

	harness, ok := chatacp.HarnessFor(codersdk.ChatRuntimeClaudeCode)
	require.True(t, ok)

	require.Equal(t, map[string]string{
		"ANTHROPIC_API_KEY":  "key",
		"ANTHROPIC_MODEL":    "claude-test",
		"ANTHROPIC_BASE_URL": "https://gateway.example.com",
	}, harness.Env(chatacp.TurnCredentials{APIKey: "key", BaseURL: "https://gateway.example.com", Model: "claude-test"}))

	require.Equal(t, map[string]string{
		"ANTHROPIC_API_KEY": "key",
	}, harness.Env(chatacp.TurnCredentials{APIKey: "key"}), "unset model and base URL keep the adapter defaults")
}

func TestCodexEnv(t *testing.T) {
	t.Parallel()

	harness, ok := chatacp.HarnessFor(codersdk.ChatRuntimeCodex)
	require.True(t, ok)

	type providerConfig struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		EnvKey  string `json:"env_key"`
		WireAPI string `json:"wire_api"`
	}
	type codexConfig struct {
		Model          string                    `json:"model"`
		ModelProvider  string                    `json:"model_provider"`
		ModelProviders map[string]providerConfig `json:"model_providers"`
	}
	decodeConfig := func(t *testing.T, env map[string]string) codexConfig {
		t.Helper()
		var config codexConfig
		require.NoError(t, json.Unmarshal([]byte(env["CODEX_CONFIG"]), &config))
		return config
	}
	requireAPIKeyAuth := func(t *testing.T, env map[string]string) {
		t.Helper()
		require.Equal(t, "key", env["OPENAI_API_KEY"])
		require.Equal(t, "1", env["NO_BROWSER"])
		var authRequest map[string]string
		require.NoError(t, json.Unmarshal([]byte(env["DEFAULT_AUTH_REQUEST"]), &authRequest))
		require.Equal(t, map[string]string{"methodId": "api-key"}, authRequest)
	}

	t.Run("GatewayAndModel", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name        string
			baseURL     string
			wantBaseURL string
		}{
			{name: "Bare", baseURL: "https://gateway.example.com", wantBaseURL: "https://gateway.example.com/v1"},
			{name: "TrailingSlash", baseURL: "https://gateway.example.com/openai/v1/", wantBaseURL: "https://gateway.example.com/openai/v1"},
			{name: "AlreadyV1", baseURL: "https://gateway.example.com/openai/v1", wantBaseURL: "https://gateway.example.com/openai/v1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				env := harness.Env(chatacp.TurnCredentials{APIKey: "key", BaseURL: tc.baseURL, Model: "gpt-test"})
				requireAPIKeyAuth(t, env)
				require.Equal(t, "coder", env["MODEL_PROVIDER"])
				config := decodeConfig(t, env)
				require.Equal(t, "gpt-test", config.Model)
				require.Equal(t, "coder", config.ModelProvider)
				require.Len(t, config.ModelProviders, 1)
				provider := config.ModelProviders["coder"]
				require.Equal(t, "Coder", provider.Name)
				require.Equal(t, "OPENAI_API_KEY", provider.EnvKey)
				require.Equal(t, "responses", provider.WireAPI)
				require.Equal(t, tc.wantBaseURL, provider.BaseURL)
			})
		}
	})

	t.Run("ModelOnly", func(t *testing.T) {
		t.Parallel()
		env := harness.Env(chatacp.TurnCredentials{APIKey: "key", Model: "gpt-test"})
		requireAPIKeyAuth(t, env)
		require.NotContains(t, env, "MODEL_PROVIDER")
		config := decodeConfig(t, env)
		require.Equal(t, "gpt-test", config.Model)
		require.Empty(t, config.ModelProvider)
		require.Empty(t, config.ModelProviders)
	})

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		env := harness.Env(chatacp.TurnCredentials{APIKey: "key"})
		requireAPIKeyAuth(t, env)
		require.NotContains(t, env, "MODEL_PROVIDER")
		require.NotContains(t, env, "CODEX_CONFIG")
	})
}
