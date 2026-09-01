package chatacp

import (
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
