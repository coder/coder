package chatacp

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// TurnCredentials is what one turn resolved for the adapter: the
// provider key and base URL it should call and the model to run. Empty
// BaseURL and Model keep the adapter defaults.
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
	// accepts and whose credentials it forwards to the adapter.
	ProviderType codersdk.AIProviderType
	// ProviderLabel names that provider in user-facing copy.
	ProviderLabel string
	// DefaultSessionMode is the ACP session mode applied when the
	// organization config leaves permission_mode empty. Empty keeps the
	// adapter default.
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
		DefaultSessionMode: "",
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
		// Codex's restrictive modes enable its own sandbox, which has no
		// place inside a workspace that already isolates the agent.
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
		"ANTHROPIC_API_KEY": creds.APIKey,
	}
	if creds.Model != "" {
		env["ANTHROPIC_MODEL"] = creds.Model
	}
	if creds.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = creds.BaseURL
	}
	return env
}

// codexModelProviderID names the Codex model_providers entry that routes
// requests to a configured OpenAI base URL.
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

// codexEnv configures codex-acp for non-interactive API-key auth:
// NO_BROWSER hides the ChatGPT login method and DEFAULT_AUTH_REQUEST
// makes the adapter log in with OPENAI_API_KEY itself when Codex asks
// for authentication. That login writes the key to Codex's auth store
// under CODEX_HOME on the workspace's persistent home, next to the
// session storage resume depends on, so the key outlives the turn and
// chatd cannot prevent that without giving up session resume. Model and
// gateway routing travel in CODEX_CONFIG, which the adapter merges into
// the Codex session config.
func codexEnv(creds TurnCredentials) map[string]string {
	env := map[string]string{
		"OPENAI_API_KEY":       creds.APIKey,
		"NO_BROWSER":           "1",
		"DEFAULT_AUTH_REQUEST": `{"methodId":"api-key"}`,
	}
	config := codexConfig{Model: creds.Model}
	if creds.BaseURL != "" {
		env["MODEL_PROVIDER"] = codexModelProviderID
		config.ModelProvider = codexModelProviderID
		config.ModelProviders = map[string]codexModelProviderConfig{
			codexModelProviderID: {
				Name:    "Coder",
				BaseURL: codexBaseURL(creds.BaseURL),
				EnvKey:  "OPENAI_API_KEY",
				WireAPI: "responses",
			},
		}
	}
	if creds.Model != "" || creds.BaseURL != "" {
		// A struct of plain strings cannot fail to marshal.
		encoded, _ := json.Marshal(config)
		env["CODEX_CONFIG"] = string(encoded)
	}
	return env
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
