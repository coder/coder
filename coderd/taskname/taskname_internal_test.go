package taskname

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestGenerateFallback(t *testing.T) {
	t.Parallel()

	taskName := generateFallback()
	err := codersdk.NameValid(taskName.Name)
	require.NoErrorf(t, err, "expected fallback to be valid workspace name, instead found %s", taskName.Name)
	require.NotEmpty(t, taskName.DisplayName)
}

func TestGenerateFromPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		prompt              string
		expectError         bool
		expectedName        string
		expectedDisplayName string
	}{
		{
			name:        "EmptyPrompt",
			prompt:      "",
			expectError: true,
		},
		{
			name:        "OnlySpaces",
			prompt:      "     ",
			expectError: true,
		},
		{
			name:        "OnlySpecialCharacters",
			prompt:      "!@#$%^&*()",
			expectError: true,
		},
		{
			name:                "UppercasePrompt",
			prompt:              "BUILD MY APP",
			expectError:         false,
			expectedName:        "build-my-app",
			expectedDisplayName: "BUILD MY APP",
		},
		{
			name:                "PromptWithApostrophes",
			prompt:              "fix user's dashboard",
			expectError:         false,
			expectedName:        "fix-users-dashboard",
			expectedDisplayName: "Fix user's dashboard",
		},
		{
			name:                "LongPrompt",
			prompt:              strings.Repeat("a", 100),
			expectError:         false,
			expectedName:        strings.Repeat("a", 27),
			expectedDisplayName: "A" + strings.Repeat("a", 62) + "…",
		},
		{
			name:                "PromptWithMultipleSpaces",
			prompt:              "build    my    app",
			expectError:         false,
			expectedName:        "build-my-app",
			expectedDisplayName: "Build    my    app",
		},
		{
			name:                "PromptWithNewlines",
			prompt:              "build\nmy\napp",
			expectError:         false,
			expectedName:        "build-my-app",
			expectedDisplayName: "Build my app",
		},
		{
			name:                "TruncatesLongPromptAtWordBoundary",
			prompt:              "implement real-time notifications dashboard",
			expectError:         false,
			expectedName:        "implement-real-time",
			expectedDisplayName: "Implement real-time notifications dashboard",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			taskName, err := generateFromPrompt(tc.prompt)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Validate task name
			require.Contains(t, taskName.Name, fmt.Sprintf("%s-", tc.expectedName))
			require.NoError(t, codersdk.NameValid(taskName.Name))

			// Validate task display name
			require.NotEmpty(t, taskName.DisplayName)
			require.Equal(t, tc.expectedDisplayName, taskName.DisplayName)
		})
	}
}

func TestExtractJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "BareJSON",
			input:    `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
			expected: `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
		},
		{
			name:     "FencedJSON",
			input:    "```json\n{\"display_name\": \"Fix bug\", \"task_name\": \"fix-bug\"}\n```",
			expected: `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
		},
		{
			name:     "FencedNoLanguage",
			input:    "```\n{\"display_name\": \"Fix bug\", \"task_name\": \"fix-bug\"}\n```",
			expected: `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
		},
		{
			name:     "FencedWithSurroundingWhitespace",
			input:    "  \n```json\n{\"display_name\": \"Fix bug\", \"task_name\": \"fix-bug\"}\n```\n  ",
			expected: `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
		},
		{
			name:     "BareJSONWithWhitespace",
			input:    "  \n{\"display_name\": \"Fix bug\", \"task_name\": \"fix-bug\"}\n  ",
			expected: `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
		},
		{
			name:     "FencedMultilineJSON",
			input:    "```json\n{\n  \"display_name\": \"Fix bug\",\n  \"task_name\": \"fix-bug\"\n}\n```",
			expected: "{\n  \"display_name\": \"Fix bug\",\n  \"task_name\": \"fix-bug\"\n}",
		},
		{
			name:     "FencedNoNewlinePassthrough",
			input:    "```json{\"display_name\": \"Fix bug\", \"task_name\": \"fix-bug\"}```",
			expected: "```json{\"display_name\": \"Fix bug\", \"task_name\": \"fix-bug\"}```",
		},
		{
			name:     "NonJSONFencedContent",
			input:    "```foo: {}, bar: {}```",
			expected: "```foo: {}, bar: {}```",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractJSON(tc.input)
			require.Equal(t, tc.expected, got)
		})
	}
}

// fakeAnthropicSSE builds a minimal Anthropic Messages SSE stream
// whose sole text content is the provided string.
func fakeAnthropicSSE(t *testing.T, text string) string {
	t.Helper()

	// Use json.Marshal to produce a correctly escaped JSON
	// string value, then strip the surrounding quotes.
	escapedBytes, err := json.Marshal(text)
	require.NoError(t, err)
	escaped := string(escapedBytes[1 : len(escapedBytes)-1])

	return fmt.Sprintf(`event: message_start
data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-haiku-4-5-20241022","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"%s"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":20}}

event: message_stop
data: {"type":"message_stop"}
`, escaped)
}

func TestGenerateFromAnthropicMock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		responseText        string
		expectedDisplayName string
		expectedNamePrefix  string
	}{
		{
			name:                "BareJSON",
			responseText:        `{"display_name": "Fix bug", "task_name": "fix-bug"}`,
			expectedDisplayName: "Fix bug",
			expectedNamePrefix:  "fix-bug-",
		},
		{
			name:                "FencedJSON",
			responseText:        "```json\n{\"display_name\": \"Debug auth\", \"task_name\": \"debug-auth\"}\n```",
			expectedDisplayName: "Debug auth",
			expectedNamePrefix:  "debug-auth-",
		},
		{
			name:                "FencedNoLanguage",
			responseText:        "```\n{\"display_name\": \"Setup CI\", \"task_name\": \"setup-ci\"}\n```",
			expectedDisplayName: "Setup CI",
			expectedNamePrefix:  "setup-ci-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(fakeAnthropicSSE(t, tc.responseText)))
			}))
			t.Cleanup(srv.Close)

			ctx := testutil.Context(t, testutil.WaitShort)

			taskName, err := generateFromAnthropic(
				ctx, "test prompt", "fake-key",
				anthropic.ModelClaudeHaiku4_5,
				anthropicoption.WithBaseURL(srv.URL),
			)
			require.NoError(t, err)
			require.NoError(t, codersdk.NameValid(taskName.Name))
			require.True(t, strings.HasPrefix(taskName.Name, tc.expectedNamePrefix),
				"expected name %q to have prefix %q", taskName.Name, tc.expectedNamePrefix)
			require.Equal(t, tc.expectedDisplayName, taskName.DisplayName)
		})
	}
}

func TestGenerateFromAnthropic(t *testing.T) {
	t.Parallel()

	apiKey := getAnthropicAPIKeyFromEnv()
	if apiKey == "" {
		t.Skip("Skipping test as ANTHROPIC_API_KEY not set")
	}

	tests := []struct {
		name   string
		prompt string
	}{
		{
			name:   "SimplePrompt",
			prompt: "Create a finance planning app",
		},
		{
			name:   "TechnicalPrompt",
			prompt: "Debug authentication middleware for OAuth2",
		},
		{
			name:   "ShortPrompt",
			prompt: "Fix bug",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			taskName, err := generateFromAnthropic(ctx, tc.prompt, apiKey, getAnthropicModelFromEnv())
			require.NoError(t, err)

			t.Log("Task name:", taskName.Name)
			t.Log("Task display name:", taskName.DisplayName)

			// Validate task name
			require.NotEmpty(t, taskName.DisplayName)
			require.NoError(t, codersdk.NameValid(taskName.Name))

			// Validate display name
			require.NotEmpty(t, taskName.DisplayName)
			require.NotEqual(t, "task-unnamed", taskName.Name)
			require.NotEqual(t, "Task Unnamed", taskName.DisplayName)
		})
	}
}
