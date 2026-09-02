package chatacp_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/codersdk"
)

// TestHarnessFor pins which runtimes exist and how each pairs with its
// adapter and provider; the behavior tests derive fixtures from these
// rows, so nothing else would notice a swapped pairing.
func TestHarnessFor(t *testing.T) {
	t.Parallel()

	_, ok := chatacp.HarnessFor(codersdk.ChatRuntimeCoder)
	require.False(t, ok, "the built-in runtime has no harness")

	want := []chatacp.Harness{
		{
			Runtime:            codersdk.ChatRuntimeClaudeCode,
			DisplayName:        "Claude Code",
			Command:            "claude-agent-acp",
			ProviderType:       codersdk.AIProviderTypeAnthropic,
			ProviderLabel:      "Anthropic",
			DefaultSessionMode: "bypassPermissions",
		},
		{
			Runtime:            codersdk.ChatRuntimeCodex,
			DisplayName:        "Codex",
			Command:            "codex-acp",
			ProviderType:       codersdk.AIProviderTypeOpenAI,
			ProviderLabel:      "OpenAI",
			DefaultSessionMode: "agent-full-access",
		},
	}
	require.Len(t, chatacp.Harnesses(), len(want))
	for _, tc := range want {
		harness, ok := chatacp.HarnessFor(tc.Runtime)
		require.True(t, ok, tc.Runtime)
		require.NotNil(t, harness.Env, tc.Runtime)
		// An empty default would leave the adapter in a mode that prompts,
		// and every prompt is auto-declined.
		require.NotEmpty(t, harness.DefaultSessionMode, tc.Runtime)
		harness.Env = nil
		require.Equal(t, tc, harness)
	}
}

// TestHarnessEnv is the executable contract with claude-agent-acp and
// codex-acp: exact equality fails on any stray or missing variable.
func TestHarnessEnv(t *testing.T) {
	t.Parallel()

	codexAuth := func(extra map[string]string) map[string]string {
		env := map[string]string{
			"OPENAI_API_KEY":       "key",
			"NO_BROWSER":           "1",
			"DEFAULT_AUTH_REQUEST": `{"methodId":"api-key"}`,
		}
		for k, v := range extra {
			env[k] = v
		}
		return env
	}
	codexGateway := func(baseURL string) map[string]string {
		return codexAuth(map[string]string{
			"MODEL_PROVIDER": "coder",
			"CODEX_CONFIG":   `{"model":"gpt-test","model_provider":"coder","model_providers":{"coder":{"name":"Coder","base_url":"` + baseURL + `","env_key":"OPENAI_API_KEY","wire_api":"responses"}}}`,
		})
	}

	tests := []struct {
		name    string
		runtime codersdk.ChatRuntime
		creds   chatacp.TurnCredentials
		wantEnv map[string]string
	}{
		{
			name:    "ClaudeCodeFull",
			runtime: codersdk.ChatRuntimeClaudeCode,
			creds:   chatacp.TurnCredentials{APIKey: "key", BaseURL: "https://gateway.example.com", Model: "claude-test"},
			wantEnv: map[string]string{
				"ANTHROPIC_API_KEY":  "key",
				"ANTHROPIC_MODEL":    "claude-test",
				"ANTHROPIC_BASE_URL": "https://gateway.example.com",
			},
		},
		{
			name:    "ClaudeCodeKeyOnlyKeepsAdapterDefaults",
			runtime: codersdk.ChatRuntimeClaudeCode,
			creds:   chatacp.TurnCredentials{APIKey: "key"},
			wantEnv: map[string]string{"ANTHROPIC_API_KEY": "key"},
		},
		{
			name:    "CodexGatewayBare",
			runtime: codersdk.ChatRuntimeCodex,
			creds:   chatacp.TurnCredentials{APIKey: "key", BaseURL: "https://gateway.example.com", Model: "gpt-test"},
			wantEnv: codexGateway("https://gateway.example.com/v1"),
		},
		{
			name:    "CodexGatewayTrailingSlash",
			runtime: codersdk.ChatRuntimeCodex,
			creds:   chatacp.TurnCredentials{APIKey: "key", BaseURL: "https://gateway.example.com/openai/v1/", Model: "gpt-test"},
			wantEnv: codexGateway("https://gateway.example.com/openai/v1"),
		},
		{
			name:    "CodexGatewayAlreadyV1",
			runtime: codersdk.ChatRuntimeCodex,
			creds:   chatacp.TurnCredentials{APIKey: "key", BaseURL: "https://gateway.example.com/openai/v1", Model: "gpt-test"},
			wantEnv: codexGateway("https://gateway.example.com/openai/v1"),
		},
		{
			name:    "CodexModelOnly",
			runtime: codersdk.ChatRuntimeCodex,
			creds:   chatacp.TurnCredentials{APIKey: "key", Model: "gpt-test"},
			wantEnv: codexAuth(map[string]string{"CODEX_CONFIG": `{"model":"gpt-test"}`}),
		},
		{
			name:    "CodexKeyOnlyKeepsAdapterDefaults",
			runtime: codersdk.ChatRuntimeCodex,
			creds:   chatacp.TurnCredentials{APIKey: "key"},
			wantEnv: codexAuth(nil),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			harness, ok := chatacp.HarnessFor(tc.runtime)
			require.True(t, ok)
			require.Equal(t, tc.wantEnv, harness.Env(tc.creds))
		})
	}
}
