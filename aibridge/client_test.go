package aibridge_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
)

func TestGuessClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userAgent  string
		headers    map[string]string
		wantClient aibridge.Client
	}{
		{
			name:       "mux",
			userAgent:  "mux/0.19.0-next.2.gcceff159 ai-sdk/openai/3.0.36 ai-sdk/provider-utils/4.0.15 runtime/node.js/22",
			wantClient: aibridge.ClientMux,
		},
		{
			name:       "claude_code",
			userAgent:  "claude-cli/2.0.67 (external, cli)",
			wantClient: aibridge.ClientClaudeCode,
		},
		{
			name:       "codex_cli",
			userAgent:  "codex_cli_rs/0.87.0 (Mac OS 26.2.0; arm64) ghostty/1.3.0-main_250877ef",
			wantClient: aibridge.ClientCodex,
		},
		{
			name:       "zed",
			userAgent:  "Zed/0.219.4+stable.119.abc123 (macos; aarch64)",
			wantClient: aibridge.ClientZed,
		},
		{
			name:       "github_copilot_vsc",
			userAgent:  "GitHubCopilotChat/0.37.2026011603",
			wantClient: aibridge.ClientCopilotVSC,
		},
		{
			name:       "github_copilot_cli",
			userAgent:  "copilot/0.0.403 (client/cli linux v24.11.1)",
			wantClient: aibridge.ClientCopilotCLI,
		},
		{
			name:       "kilo_code_user_agent",
			userAgent:  "kilo-code/5.1.0 (darwin 25.2.0; arm64) node/22.21.1",
			wantClient: aibridge.ClientKilo,
		},
		{
			name:       "kilo_code_originator",
			headers:    map[string]string{"Originator": "kilo-code"},
			wantClient: aibridge.ClientKilo,
		},
		{
			name:       "roo_code_user_agent",
			userAgent:  "roo-code/3.45.0 (darwin 25.2.0; arm64) node/22.21.1",
			wantClient: aibridge.ClientRoo,
		},
		{
			name:       "roo_code_originator",
			headers:    map[string]string{"Originator": "roo-code"},
			wantClient: aibridge.ClientRoo,
		},
		{
			name:       "coder_agents",
			userAgent:  "coder-agents/v2.24.0 (linux/amd64)",
			wantClient: aibridge.ClientCoderAgents,
		},
		{
			name:       "coder_agents_dev",
			userAgent:  "coder-agents/v0.0.0-devel (darwin/arm64)",
			wantClient: aibridge.ClientCoderAgents,
		},
		{
			name:       "charm_crush_space",
			userAgent:  "Charm Crush/0.1.11",
			wantClient: aibridge.ClientCrush,
		},
		{
			name:       "charm_crush_hyphen",
			userAgent:  "Charm-Crush/0.2.0 (https://charm.land/crush)",
			wantClient: aibridge.ClientCrush,
		},
		{
			name:       "cursor_x_cursor_client_version",
			userAgent:  "connect-es/1.6.1",
			headers:    map[string]string{"X-Cursor-client-version": "0.50.0"},
			wantClient: aibridge.ClientCursor,
		},
		{
			name:       "cursor_x_cursor_some_other_header",
			headers:    map[string]string{"x-cursor-client-version": "abc123"},
			wantClient: aibridge.ClientCursor,
		},
		{
			name:       "unknown_client",
			userAgent:  "ccclaude-cli/calude-with-wrong-prefix",
			wantClient: aibridge.ClientUnknown,
		},
		{
			name:       "empty_user_agent",
			userAgent:  "",
			wantClient: aibridge.ClientUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "", nil)
			require.NoError(t, err)

			req.Header.Set("User-Agent", tt.userAgent)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			got := aibridge.GuessClient(req)
			require.Equal(t, tt.wantClient, got)
		})
	}
}
