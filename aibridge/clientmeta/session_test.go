package clientmeta_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/clientmeta"
)

func TestGuessSessionID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		client    clientmeta.Client
		body      string
		headers   map[string]string
		sessionID *string
	}{
		// Claude Code.
		{
			name:      "claude_code_header_takes_precedence",
			client:    clientmeta.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": "header-session-id"},
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_body-session-id"}}`,
			sessionID: new("header-session-id"),
		},
		{
			name:      "claude_code_header_only",
			client:    clientmeta.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": "aabb-ccdd"},
			body:      `{"model":"claude-3"}`,
			sessionID: new("aabb-ccdd"),
		},
		{
			name:      "claude_code_empty_header_falls_back_to_body",
			client:    clientmeta.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": ""},
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`,
			sessionID: new("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		},
		{
			name:      "claude_code_whitespace_header_falls_back_to_body",
			client:    clientmeta.ClientClaudeCode,
			headers:   map[string]string{"X-Claude-Code-Session-Id": "   "},
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`,
			sessionID: new("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		},
		{
			name:      "claude_code_with_valid_session",
			client:    clientmeta.ClientClaudeCode,
			body:      `{"metadata":{"user_id":"user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`,
			sessionID: new("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		},
		{
			name:      "claude_code_with_valid_session_new_format",
			client:    clientmeta.ClientClaudeCode,
			body:      `{"metadata":{"user_id":"{\"device_id\":\"45aa15c8c244ea2582f8144dde91a50ec3815851f6f648abef4ee15b173cc927\",\"account_uuid\":\"\",\"session_id\":\"54c1eb09-bc4c-4d2f-98eb-6d2ab2d5e2fe\"}"}}`,
			sessionID: new("54c1eb09-bc4c-4d2f-98eb-6d2ab2d5e2fe"),
		},
		{
			name:   "claude_code_new_format_empty_session_id",
			client: clientmeta.ClientClaudeCode,
			body:   `{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"\",\"session_id\":\"\"}"}}`,
		},
		{
			name:   "claude_code_new_format_no_session_id_field",
			client: clientmeta.ClientClaudeCode,
			body:   `{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"\"}"}}`,
		},
		{
			name:   "claude_code_missing_metadata",
			client: clientmeta.ClientClaudeCode,
			body:   `{"model":"claude-3"}`,
		},
		{
			name:   "claude_code_missing_user_id",
			client: clientmeta.ClientClaudeCode,
			body:   `{"metadata":{}}`,
		},
		{
			name:   "claude_code_user_id_without_session",
			client: clientmeta.ClientClaudeCode,
			body:   `{"metadata":{"user_id":"user_abc123_account_456"}}`,
		},
		{
			name:   "claude_code_empty_body",
			client: clientmeta.ClientClaudeCode,
			body:   ``,
		},
		{
			name:   "claude_code_invalid_json",
			client: clientmeta.ClientClaudeCode,
			body:   `not json at all`,
		},
		// Codex.
		{
			name:      "codex_with_session_header",
			client:    clientmeta.ClientCodex,
			headers:   map[string]string{"session_id": "codex-session-123"},
			sessionID: new("codex-session-123"),
		},
		{
			name:      "codex_with_hyphenated_session_header",
			client:    clientmeta.ClientCodex,
			headers:   map[string]string{"session-id": "codex-session-456"},
			sessionID: new("codex-session-456"),
		},
		{
			name:      "codex_hyphenated_header_takes_precedence",
			client:    clientmeta.ClientCodex,
			headers:   map[string]string{"session-id": "codex-session-new", "session_id": "codex-session-old"},
			sessionID: new("codex-session-new"),
		},
		{
			name:      "codex_with_whitespace_in_header",
			client:    clientmeta.ClientCodex,
			headers:   map[string]string{"session_id": "  codex-session-123  "},
			sessionID: new("codex-session-123"),
		},
		{
			name:   "codex_without_session_header",
			client: clientmeta.ClientCodex,
		},
		// Other clients shouldn't use others' logic.
		{
			name:   "unknown_client_returns_empty",
			client: clientmeta.ClientUnknown,
			body:   `{"metadata":{"user_id":"user_abc_account_456_session_some-id"}}`,
		},
		{
			name:    "zed_returns_empty",
			client:  clientmeta.ClientZed,
			headers: map[string]string{"session_id": "zed-session"},
			body:    `{"metadata":{"user_id":"user_abc_account_456_session_some-id"}}`,
		},
		// Xum.
		{
			name:      "xum_with_workspace_header",
			client:    clientmeta.ClientXum,
			headers:   map[string]string{"X-Mux-Workspace-Id": "ws-abc-123"},
			sessionID: new("ws-abc-123"),
		},
		{
			name:   "xum_without_workspace_header",
			client: clientmeta.ClientXum,
		},
		// Copilot VS Code.
		{
			name:      "copilot_vsc_with_interaction_id",
			client:    clientmeta.ClientCopilotVSC,
			headers:   map[string]string{"x-interaction-id": "interaction-xyz"},
			sessionID: new("interaction-xyz"),
		},
		{
			name:   "copilot_vsc_without_interaction_id",
			client: clientmeta.ClientCopilotVSC,
		},
		// Copilot CLI.
		{
			name:      "copilot_cli_with_session_header",
			client:    clientmeta.ClientCopilotCLI,
			headers:   map[string]string{"X-Client-Session-Id": "cli-sess-456"},
			sessionID: new("cli-sess-456"),
		},
		{
			name:   "copilot_cli_without_session_header",
			client: clientmeta.ClientCopilotCLI,
		},
		// Kilo.
		{
			name:      "kilo_with_task_id",
			client:    clientmeta.ClientKilo,
			headers:   map[string]string{"X-KILOCODE-TASKID": "task-789"},
			sessionID: new("task-789"),
		},
		{
			name:   "kilo_without_task_id",
			client: clientmeta.ClientKilo,
		},
		// Coder Agents.
		{
			name:      "coder_agents_with_chat_id",
			client:    clientmeta.ClientCoderAgents,
			headers:   map[string]string{"X-Coder-Chat-Id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
			sessionID: new("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
		},
		{
			name:   "coder_agents_without_chat_id",
			client: clientmeta.ClientCoderAgents,
		},
		// OpenCode.
		{
			name:      "opencode_with_session_header",
			client:    clientmeta.ClientOpenCode,
			headers:   map[string]string{"X-OpenCode-Session": "ses_15a48edefffe7oY0YcIHRv29dD"},
			sessionID: new("ses_15a48edefffe7oY0YcIHRv29dD"),
		},
		{
			name:      "opencode_with_whitespace_in_header",
			client:    clientmeta.ClientOpenCode,
			headers:   map[string]string{"X-OpenCode-Session": "  ses_15a48edefffe7oY0YcIHRv29dD  "},
			sessionID: new("ses_15a48edefffe7oY0YcIHRv29dD"),
		},
		{
			name:      "opencode_zen_header_takes_precedence_over_session_affinity",
			client:    clientmeta.ClientOpenCode,
			headers:   map[string]string{"X-OpenCode-Session": "zen-session", "x-session-affinity": "other-session"},
			sessionID: new("zen-session"),
		},
		{
			name:      "opencode_session_affinity_fallback",
			client:    clientmeta.ClientOpenCode,
			headers:   map[string]string{"x-session-affinity": "affinity-session-123"},
			sessionID: new("affinity-session-123"),
		},
		{
			name:   "opencode_without_session_header",
			client: clientmeta.ClientOpenCode,
		},
		// Crush.
		{
			name:   "crush_returns_empty",
			client: clientmeta.ClientCrush,
		},
		// Roo.
		{
			name:   "roo_returns_empty",
			client: clientmeta.ClientRoo,
		},
		// Cursor.
		{
			name:   "cursor_returns_empty",
			client: clientmeta.ClientCursor,
		},
		// Other cases.
		{
			name:      "empty session ID value",
			client:    clientmeta.ClientKilo,
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

			got := clientmeta.GuessSessionID(tc.client, req)
			require.Equal(t, tc.sessionID, got)
			require.Equal(t, tc.sessionID, clientmeta.GuessSessionIDFromPayload(tc.client, req, []byte(body)))

			// Verify the body was restored and can be read again.
			restored, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, string(restored))
		})
	}
}

func TestGuessSessionIDRejectsOverlongValues(t *testing.T) {
	t.Parallel()

	const maxRunes = 256
	validHeader := strings.Repeat("界", maxRunes)
	validBody := strings.Repeat("界", maxRunes)
	overlongHeader := strings.Repeat("界", maxRunes+1)
	overlongBody := strings.Repeat("界", maxRunes+1)
	for _, tc := range []struct {
		name   string
		client clientmeta.Client
		body   string
		header http.Header
		want   *string
	}{
		{name: "HeaderAtLimit", client: clientmeta.ClientCodex, header: http.Header{"Session-Id": {validHeader}}, want: new(validHeader)},
		{name: "HeaderOverLimit", client: clientmeta.ClientCodex, header: http.Header{"Session-Id": {overlongHeader}}},
		{name: "BodyAtLimit", client: clientmeta.ClientClaudeCode, body: `{"metadata":{"user_id":"user_hash_session_` + validBody + `"}}`, want: new(validBody)},
		{name: "BodyOverLimit", client: clientmeta.ClientClaudeCode, body: `{"metadata":{"user_id":"user_hash_session_` + overlongBody + `"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header = tc.header.Clone()
			require.Equal(t, tc.want, clientmeta.GuessSessionID(tc.client, req))
		})
	}
}

func TestGuessSessionIDHeaderDoesNotReadBody(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		client clientmeta.Client
		header string
	}{
		{name: "Codex", client: clientmeta.ClientCodex, header: "session-id"},
		{name: "ClaudeCode", client: clientmeta.ClientClaudeCode, header: "X-Claude-Code-Session-Id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", &errReader{})
			require.NoError(t, err)
			req.Header.Set(tc.header, "header-id")
			require.Equal(t, new("header-id"), clientmeta.GuessSessionID(tc.client, req))
		})
	}
}

func TestGuessSessionIDMissingBody(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body io.Reader
	}{
		{name: "Nil"},
		{name: "Unreadable", body: &errReader{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", tc.body)
			require.NoError(t, err)
			require.Nil(t, clientmeta.GuessSessionID(clientmeta.ClientClaudeCode, req))
		})
	}
}

// errReader is an io.Reader that always returns an error.
type errReader struct{}

func (*errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
