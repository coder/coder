package chatacp

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// TurnCredentials supplies a per-turn Coder token and AI Gateway URL.
// Only an empty Model keeps the adapter default.
type TurnCredentials struct {
	APIKey  string
	BaseURL string
	Model   string
}

// Harness describes one external chat runtime: the ACP adapter the
// workspace template must provide and how chatd hands it credentials.
type Harness struct {
	Runtime codersdk.ChatRuntime
	// DisplayName names the runtime in user-facing copy.
	DisplayName string
	// Command launches the adapter inside the workspace. The template
	// backing the runtime must put it on PATH.
	Command string
	// ProviderType is the AI provider whose model configs the runtime
	// accepts through the AI Gateway.
	ProviderType codersdk.AIProviderType
	// ProviderLabel names that provider in user-facing copy.
	ProviderLabel string
	// DefaultSessionMode is the ACP session mode applied when the
	// organization config leaves permission_mode empty. Every harness
	// defaults to its least restrictive mode: the workspace already
	// isolates the agent, and the modes that prompt cannot be answered
	// because chatd auto-declines permission requests.
	DefaultSessionMode string
	// Env builds the adapter process environment for one turn.
	Env func(TurnCredentials) map[string]string
}

var harnesses = []Harness{
	{
		Runtime:     codersdk.ChatRuntimeClaudeCode,
		DisplayName: "Claude Code",
		// Ships in the @agentclientprotocol/claude-agent-acp npm package
		// (the renamed successor of the deprecated
		// @zed-industries/claude-code-acp).
		Command:            "claude-agent-acp",
		ProviderType:       codersdk.AIProviderTypeAnthropic,
		ProviderLabel:      "Anthropic",
		DefaultSessionMode: "bypassPermissions",
		Env:                claudeCodeEnv,
	},
	{
		Runtime:     codersdk.ChatRuntimeCodex,
		DisplayName: "Codex",
		// Ships in the @agentclientprotocol/codex-acp npm package
		// (verified against 1.8.0), which bundles @openai/codex.
		Command:       "codex-acp",
		ProviderType:  codersdk.AIProviderTypeOpenAI,
		ProviderLabel: "OpenAI",
		// Codex's restrictive modes also enable its own sandbox.
		DefaultSessionMode: "agent-full-access",
		Env:                codexEnv,
	},
}

// Harnesses returns every external runtime chatd can drive, so tests
// and callers enumerate the supported set from one table.
func Harnesses() []Harness {
	return slices.Clone(harnesses)
}

// HarnessFor returns the harness for an external runtime. The built-in
// coder runtime has none.
func HarnessFor(runtime codersdk.ChatRuntime) (Harness, bool) {
	for _, harness := range harnesses {
		if harness.Runtime == runtime {
			return harness, true
		}
	}
	return Harness{}, false
}

func claudeCodeEnv(creds TurnCredentials) map[string]string {
	env := map[string]string{
		"ANTHROPIC_AUTH_TOKEN": creds.APIKey,
		"ANTHROPIC_API_KEY":    "",
		"ANTHROPIC_BASE_URL":   creds.BaseURL,
	}
	if creds.Model != "" {
		env["ANTHROPIC_MODEL"] = creds.Model
	}
	return env
}

// codexModelProviderID names the Codex model_providers entry that routes
// requests through Coder's AI Gateway.
const codexModelProviderID = "coder"

// codexConfig is the subset of Codex's config.toml that chatd passes
// through codex-acp's CODEX_CONFIG environment variable.
type codexConfig struct {
	Model          string                              `json:"model,omitempty"`
	ModelProvider  string                              `json:"model_provider,omitempty"`
	ModelProviders map[string]codexModelProviderConfig `json:"model_providers,omitempty"`
}

type codexModelProviderConfig struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	EnvKey  string `json:"env_key"`
	WireAPI string `json:"wire_api"`
}

// codexEnv routes through the gateway even when the adapter chooses the model.
// codex-acp may persist the Coder token during API-key login; chatd revokes
// it after the turn without disturbing the sessions needed for resume.
func codexEnv(creds TurnCredentials) map[string]string {
	config := codexConfig{
		Model:         creds.Model,
		ModelProvider: codexModelProviderID,
		ModelProviders: map[string]codexModelProviderConfig{
			codexModelProviderID: {
				Name:    "Coder",
				BaseURL: codexBaseURL(creds.BaseURL),
				EnvKey:  "OPENAI_API_KEY",
				WireAPI: "responses",
			},
		},
	}
	// A struct of plain strings cannot fail to marshal.
	encoded, _ := json.Marshal(config)
	return map[string]string{
		"OPENAI_API_KEY":       creds.APIKey,
		"NO_BROWSER":           "1",
		"DEFAULT_AUTH_REQUEST": `{"methodId":"api-key"}`,
		"MODEL_PROVIDER":       codexModelProviderID,
		"CODEX_CONFIG":         string(encoded),
	}
}

// codexBaseURL normalizes a provider base URL to the /v1 root Codex
// expects for OpenAI-compatible providers.
func codexBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed
	}
	return trimmed + "/v1"
}
