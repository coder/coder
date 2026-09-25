package client_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/client"
)

func TestGuessSessionID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		client    client.Client
		body      string
		headers   map[string]string
		sessionID *string
	}{
		// Claude Code.
		{
			name:      "claude_code_header_takes_precedence",
			client:    client.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": "header-session-id"},
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_body-session-id"}}`,
			sessionID: new("header-session-id"),
		},
		{
			name:      "claude_code_header_only",
			client:    client.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": "aabb-ccdd"},
			body:      `{"model":"claude-3"}`,
			sessionID: new("aabb-ccdd"),
		},
		{
			name:      "claude_code_empty_header_falls_back_to_body",
			client:    client.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": ""},
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`,
			sessionID: new("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		},
		{
			name:      "claude_code_whitespace_header_falls_back_to_body",
			client:    client.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": "   "},
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`,
			sessionID: new("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		},
		{
			name:      "claude_code_with_valid_session",
			client:    client.ClientClaudeCode,
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`,
			sessionID: new("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		},
		{
			name:      "claude_code_with_valid_session_new_format",
			client:    client.ClientClaudeCode,
			body:      `{"metadata":{"user_id":"{\"device_id\":\"45aa15c8c244ea2582f8144dde91a50ec3815851f6f648abef4ee15b173cc927\",\"account_uuid\":\"\",\"session_id\":\"54c1eb09-bc4c-4d2f-98eb-6d2ab2d5e2fe\"}"}}`,
			sessionID: new("54c1eb09-bc4c-4d2f-98eb-6d2ab2d5e2fe"),
		},
		{
			name:   "claude_code_new_format_empty_session_id",
			client: client.ClientClaudeCode,
			body:   `{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"\",\"session_id\":\"\"}"}}`,
		},
		{
			name:   "claude_code_new_format_no_session_id_field",
			client: client.ClientClaudeCode,
			body:   `{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"\"}"}}`,
		},
		{
			name:   "claude_code_missing_metadata",
			client: client.ClientClaudeCode,
			body:   `{"model":"claude-3"}`,
		},
		{
			name:   "claude_code_missing_user_id",
			client: client.ClientClaudeCode,
			body:   `{"metadata":{}}`,
		},
		{
			name:   "claude_code_user_id_without_session",
			client: client.ClientClaudeCode,
			body:   `{"metadata":{"user_id":"user_abc123_account_456"}}`,
		},
		{
			name:   "claude_code_empty_body",
			client: client.ClientClaudeCode,
			body:   ``,
		},
		{
			name:   "claude_code_invalid_json",
			client: client.ClientClaudeCode,
			body:   `not json at all`,
		},
		// Codex.
		{
			name:      "codex_with_session_header",
			client:    client.ClientCodex,
			headers:   map[string]string{"session_id": "codex-session-123"},
			sessionID: new("codex-session-123"),
		},
		{
			name:      "codex_with_hyphenated_session_header",
			client:    client.ClientCodex,
			headers:   map[string]string{"session-id": "codex-session-456"},
			sessionID: new("codex-session-456"),
		},
		{
			name:      "codex_hyphenated_header_takes_precedence",
			client:    client.ClientCodex,
			headers:   map[string]string{"session-id": "codex-session-new", "session_id": "codex-session-old"},
			sessionID: new("codex-session-new"),
		},
		{
			name:      "codex_with_whitespace_in_header",
			client:    client.ClientCodex,
			headers:   map[string]string{"session_id": "  codex-session-123  "},
			sessionID: new("codex-session-123"),
		},
		{
			name:   "codex_without_session_header",
			client: client.ClientCodex,
		},
		// Other clients shouldn't use others' logic.
		{
			name:   "unknown_client_returns_empty",
			client: client.ClientUnknown,
			body:   `{"metadata":{"user_id":"user_abc_account_456_session_some-id"}}`,
		},
		{
			name:    "zed_returns_empty",
			client:  client.ClientZed,
			headers: map[string]string{"session_id": "zed-session"},
			body:    `{"metadata":{"user_id":"user_abc_account_456_session_some-id"}}`,
		},
		// Xum.
		{
			name:      "xum_with_workspace_header",
			client:    client.ClientXum,
			headers:   map[string]string{"X-Mux-Workspace-Id": "ws-abc-123"},
			sessionID: new("ws-abc-123"),
		},
		{
			name:   "xum_without_workspace_header",
			client: client.ClientXum,
		},
		// Copilot VS Code.
		{
			name:      "copilot_vsc_with_interaction_id",
			client:    client.ClientCopilotVSC,
			headers:   map[string]string{"x-interaction-id": "interaction-xyz"},
			sessionID: new("interaction-xyz"),
		},
		{
			name:   "copilot_vsc_without_interaction_id",
			client: client.ClientCopilotVSC,
		},
		// Copilot CLI.
		{
			name:      "copilot_cli_with_session_header",
			client:    client.ClientCopilotCLI,
			headers:   map[string]string{"X-Client-Session-Id": "cli-sess-456"},
			sessionID: new("cli-sess-456"),
		},
		{
			name:   "copilot_cli_without_session_header",
			client: client.ClientCopilotCLI,
		},
		// Kilo.
		{
			name:      "kilo_with_task_id",
			client:    client.ClientKilo,
			headers:   map[string]string{"X-KILOCODE-TASKID": "task-789"},
			sessionID: new("task-789"),
		},
		{
			name:   "kilo_without_task_id",
			client: client.ClientKilo,
		},
		// Coder Agents.
		{
			name:      "coder_agents_with_chat_id",
			client:    client.ClientCoderAgents,
			headers:   map[string]string{"X-Coder-Chat-Id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
			sessionID: new("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
		},
		{
			name:   "coder_agents_without_chat_id",
			client: client.ClientCoderAgents,
		},
		// OpenCode.
		{
			name:      "opencode_with_session_header",
			client:    client.ClientOpenCode,
			headers:   map[string]string{"X-OpenCode-Session": "ses_15a48edefffe7oY0YcIHRv29dD"},
			sessionID: new("ses_15a48edefffe7oY0YcIHRv29dD"),
		},
		{
			name:      "opencode_with_whitespace_in_header",
			client:    client.ClientOpenCode,
			headers:   map[string]string{"X-OpenCode-Session": "  ses_15a48edefffe7oY0YcIHRv29dD  "},
			sessionID: new("ses_15a48edefffe7oY0YcIHRv29dD"),
		},
		{
			name:      "opencode_zen_header_takes_precedence_over_session_affinity",
			client:    client.ClientOpenCode,
			headers:   map[string]string{"X-OpenCode-Session": "zen-session", "x-session-affinity": "other-session"},
			sessionID: new("zen-session"),
		},
		{
			name:      "opencode_session_affinity_fallback",
			client:    client.ClientOpenCode,
			headers:   map[string]string{"x-session-affinity": "affinity-session-123"},
			sessionID: new("affinity-session-123"),
		},
		{
			name:   "opencode_without_session_header",
			client: client.ClientOpenCode,
		},
		// Crush.
		{
			name:   "crush_returns_empty",
			client: client.ClientCrush,
		},
		// Roo.
		{
			name:   "roo_returns_empty",
			client: client.ClientRoo,
		},
		// Cursor.
		{
			name:   "cursor_returns_empty",
			client: client.ClientCursor,
		},
		// Other cases.
		{
			name:      "empty session ID value",
			client:    client.ClientKilo,
			headers:   map[string]string{"X-KILOCODE-TASKID": " "},
			sessionID: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := tc.body
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", strings.NewReader(body))
			require.NoError(t, err)

			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			got := client.GuessSessionID(tc.client, req)
			require.Equal(t, tc.sessionID, got)

			// Verify the body was restored and can be read again.
			restored, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, string(restored))
		})
	}
}

func TestUnreadableBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", &errReader{})
	require.NoError(t, err)

	got := client.GuessSessionID(client.ClientClaudeCode, req)
	require.Nil(t, got)
}

// errReader is an io.Reader that always returns an error.
type errReader struct{}

func (*errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
