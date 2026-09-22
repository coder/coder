package client_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/client"
)

func TestGuessClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userAgent  string
		headers    map[string]string
		wantClient client.Client
	}{
		{
			name:       "xum",
			userAgent:  "xum/0.20.0 ai-sdk/openai/3.0.36 ai-sdk/provider-utils/4.0.15 runtime/node.js/22",
			wantClient: client.ClientXum,
		},
		{
			name:       "xum_legacy_mux_user_agent",
			userAgent:  "mux/0.19.0-next.2.gcceff159 ai-sdk/openai/3.0.36 ai-sdk/provider-utils/4.0.15 runtime/node.js/22",
			wantClient: client.ClientXum,
		},
		{
			name:       "claude_code",
			userAgent:  "claude-cli/2.0.67 (external, cli)",
			wantClient: client.ClientClaudeCode,
		},
		{
			name:       "codex_cli",
			userAgent:  "codex_cli_rs/0.87.0 (Mac OS 26.2.0; arm64) ghostty/1.3.0-main_250877ef",
			wantClient: client.ClientCodex,
		},
		{
			name:       "zed",
			userAgent:  "Zed/0.219.4+stable.119.abc123 (macos; aarch64)",
			wantClient: client.ClientZed,
		},
		{
			name:       "github_copilot_vsc",
			userAgent:  "GitHubCopilotChat/0.37.2026011603",
			wantClient: client.ClientCopilotVSC,
		},
		{
			name:       "github_copilot_cli",
			userAgent:  "copilot/0.0.403 (client/cli linux v24.11.1)",
			wantClient: client.ClientCopilotCLI,
		},
		{
			name:       "kilo_code_user_agent",
			userAgent:  "kilo-code/5.1.0 (darwin 25.2.0; arm64) node/22.21.1",
			wantClient: client.ClientKilo,
		},
		{
			name:       "kilo_code_originator",
			headers:    map[string]string{"Originator": "kilo-code"},
			wantClient: client.ClientKilo,
		},
		{
			name:       "roo_code_user_agent",
			userAgent:  "roo-code/3.45.0 (darwin 25.2.0; arm64) node/22.21.1",
			wantClient: client.ClientRoo,
		},
		{
			name:       "roo_code_originator",
			headers:    map[string]string{"Originator": "roo-code"},
			wantClient: client.ClientRoo,
		},
		{
			name:       "coder_agents",
			userAgent:  "coder-agents/v2.24.0 (linux/amd64)",
			wantClient: client.ClientCoderAgents,
		},
		{
			name:       "coder_agents_dev",
			userAgent:  "coder-agents/v0.0.0-devel (darwin/arm64)",
			wantClient: client.ClientCoderAgents,
		},
		{
			name:       "charm_crush_space",
			userAgent:  "Charm Crush/0.1.11",
			wantClient: client.ClientCrush,
		},
		{
			name:       "charm_crush_hyphen",
			userAgent:  "Charm-Crush/0.2.0 (https://charm.land/crush)",
			wantClient: client.ClientCrush,
		},
		{
			name:       "cursor_x_cursor_client_version",
			userAgent:  "connect-es/1.6.1",
			headers:    map[string]string{"X-Cursor-client-version": "0.50.0"},
			wantClient: client.ClientCursor,
		},
		{
			name:       "cursor_x_cursor_some_other_header",
			headers:    map[string]string{"x-cursor-client-version": "abc123"},
			wantClient: client.ClientCursor,
		},
		{
			name:       "opencode",
			userAgent:  "opencode/1.16.0 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14",
			wantClient: client.ClientOpenCode,
		},
		{
			name:       "junie_cli",
			userAgent:  "Junie:SNAPSHOT",
			wantClient: client.ClientJunie,
		},
		{
			name:       "unknown_client",
			userAgent:  "ccclaude-cli/calude-with-wrong-prefix",
			wantClient: client.ClientUnknown,
		},
		{
			name:       "empty_user_agent",
			userAgent:  "",
			wantClient: client.ClientUnknown,
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

			got := client.GuessClient(req)
			require.Equal(t, tt.wantClient, got)
		})
	}
}
