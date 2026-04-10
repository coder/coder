package taskname

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/anthropic-sdk-go"
	anthropicoption "github.com/charmbracelet/anthropic-sdk-go/option"
	"github.com/charmbracelet/anthropic-sdk-go/packages/ssestream"
	"github.com/stretchr/testify/require"

	"github.com/coder/aisdk-go"
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

func TestMessagesToAnthropic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		messages       []aisdk.Message
		expectError    string
		expectMsgCount int
		expectSysCount int
		expectRole     anthropic.MessageParamRole
	}{
		{
			name: "SystemAndUser",
			messages: []aisdk.Message{
				{Role: "system", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: "You are helpful."}}},
				{Role: "user", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: "Hello"}}},
			},
			expectMsgCount: 1,
			expectSysCount: 1,
			expectRole:     anthropic.MessageParamRoleUser,
		},
		{
			name: "AssistantRole",
			messages: []aisdk.Message{
				{Role: "assistant", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: "I can help."}}},
				{Role: "user", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: "Thanks"}}},
			},
			expectMsgCount: 2,
			expectSysCount: 0,
			expectRole:     anthropic.MessageParamRoleAssistant,
		},
		{
			name: "UnsupportedRole",
			messages: []aisdk.Message{
				{Role: "tool", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: "result"}}},
			},
			expectError: "unsupported message role: tool",
		},
		{
			name: "EmptyAfterFiltering",
			messages: []aisdk.Message{
				{Role: "user", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: ""}}},
			},
			expectError: "no non-system messages to send",
		},
		{
			name: "SystemOnly",
			messages: []aisdk.Message{
				{Role: "system", Parts: []aisdk.Part{{Type: aisdk.PartTypeText, Text: "System prompt"}}},
			},
			expectError: "no non-system messages to send",
		},
		{
			name: "FiltersEmptyText",
			messages: []aisdk.Message{
				{Role: "user", Parts: []aisdk.Part{
					{Type: aisdk.PartTypeText, Text: ""},
					{Type: aisdk.PartTypeText, Text: "actual content"},
				}},
			},
			expectMsgCount: 1,
			expectSysCount: 0,
			expectRole:     anthropic.MessageParamRoleUser,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msgs, sys, err := messagesToAnthropic(tc.messages)

			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}

			require.NoError(t, err)
			require.Len(t, msgs, tc.expectMsgCount)
			require.Len(t, sys, tc.expectSysCount)
			require.Equal(t, tc.expectRole, msgs[0].Role)
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

func buildSSE(events ...string) string {
	return strings.Join(events, "\n\n") + "\n\n"
}

const (
	sseMessageStart = `event: message_start
data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-haiku-4-5-20241022","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`

	sseContentBlockStart = `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`

	sseContentBlockStop = `event: content_block_stop
data: {"type":"content_block_stop","index":0}`

	sseMessageStop = `event: message_stop
data: {"type":"message_stop"}`
)

func sseTextDelta(text string) string {
	escaped, _ := json.Marshal(text)
	return fmt.Sprintf(`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, string(escaped))
}

func sseMessageDelta(stopReason string, outputTokens int) string {
	return fmt.Sprintf(`event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":{"output_tokens":%d}}`, stopReason, outputTokens)
}

func streamFromSSE(t *testing.T, sse string) *ssestream.Stream[anthropic.MessageStreamEventUnion] {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	t.Cleanup(srv.Close)

	client := anthropic.NewClient(
		anthropicoption.WithAPIKey("fake-key"),
		anthropicoption.WithBaseURL(srv.URL),
	)
	return client.Messages.NewStreaming(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.ContentBlockParamUnion{
				OfText: &anthropic.TextBlockParam{Text: "test"},
			}),
		},
	})
}

func collectParts(t *testing.T, ds aisdk.DataStream) []aisdk.DataStreamPart {
	t.Helper()
	var parts []aisdk.DataStreamPart
	for part, err := range ds {
		require.NoError(t, err)
		parts = append(parts, part)
	}
	return parts
}

func TestAnthropicToDataStream(t *testing.T) {
	t.Parallel()

	t.Run("TextHappyPath", func(t *testing.T) {
		t.Parallel()
		sse := buildSSE(
			sseMessageStart,
			sseContentBlockStart,
			sseTextDelta("hello world"),
			sseContentBlockStop,
			sseMessageDelta("end_turn", 20),
			sseMessageStop,
		)
		stream := streamFromSSE(t, sse)
		parts := collectParts(t, anthropicToDataStream(stream))

		// Expect: StartStep, Text, FinishStep, FinishMessage
		require.GreaterOrEqual(t, len(parts), 4)

		_, ok := parts[0].(aisdk.StartStepStreamPart)
		require.True(t, ok, "first part should be StartStepStreamPart")

		textPart, ok := parts[1].(aisdk.TextStreamPart)
		require.True(t, ok, "second part should be TextStreamPart")
		require.Equal(t, "hello world", textPart.Content)

		finishStep, ok := parts[2].(aisdk.FinishStepStreamPart)
		require.True(t, ok, "third part should be FinishStepStreamPart")
		require.Equal(t, aisdk.FinishReasonStop, finishStep.FinishReason)

		finishMsg, ok := parts[3].(aisdk.FinishMessageStreamPart)
		require.True(t, ok, "fourth part should be FinishMessageStreamPart")
		require.Equal(t, aisdk.FinishReasonStop, finishMsg.FinishReason)
	})

	t.Run("MaxTokensStopReason", func(t *testing.T) {
		t.Parallel()
		sse := buildSSE(
			sseMessageStart,
			sseContentBlockStart,
			sseTextDelta("truncated"),
			sseContentBlockStop,
			sseMessageDelta("max_tokens", 100),
			sseMessageStop,
		)
		stream := streamFromSSE(t, sse)
		parts := collectParts(t, anthropicToDataStream(stream))

		// Find the FinishStepStreamPart
		var found bool
		for _, p := range parts {
			if fs, ok := p.(aisdk.FinishStepStreamPart); ok {
				require.Equal(t, aisdk.FinishReasonLength, fs.FinishReason)
				found = true
				break
			}
		}
		require.True(t, found, "expected FinishStepStreamPart with FinishReasonLength")
	})

	t.Run("StopSequenceStopReason", func(t *testing.T) {
		t.Parallel()
		sse := buildSSE(
			sseMessageStart,
			sseContentBlockStart,
			sseTextDelta("stopped"),
			sseContentBlockStop,
			sseMessageDelta("stop_sequence", 15),
			sseMessageStop,
		)
		stream := streamFromSSE(t, sse)
		parts := collectParts(t, anthropicToDataStream(stream))

		var found bool
		for _, p := range parts {
			if fs, ok := p.(aisdk.FinishStepStreamPart); ok {
				require.Equal(t, aisdk.FinishReasonStop, fs.FinishReason)
				found = true
				break
			}
		}
		require.True(t, found, "expected FinishStepStreamPart with FinishReasonStop")
	})

	t.Run("UsageTracking", func(t *testing.T) {
		t.Parallel()
		sse := buildSSE(
			sseMessageStart,
			sseContentBlockStart,
			sseTextDelta("test"),
			sseContentBlockStop,
			sseMessageDelta("end_turn", 42),
			sseMessageStop,
		)
		stream := streamFromSSE(t, sse)
		parts := collectParts(t, anthropicToDataStream(stream))

		var found bool
		for _, p := range parts {
			if fm, ok := p.(aisdk.FinishMessageStreamPart); ok {
				require.NotNil(t, fm.Usage.CompletionTokens)
				require.Equal(t, int64(42), *fm.Usage.CompletionTokens)
				found = true
				break
			}
		}
		require.True(t, found, "expected FinishMessageStreamPart with usage")
	})

	t.Run("AbruptTermination", func(t *testing.T) {
		t.Parallel()
		// Stream ends after content_block_delta — no message_stop.
		sse := buildSSE(
			sseMessageStart,
			sseContentBlockStart,
			sseTextDelta("partial"),
		)
		stream := streamFromSSE(t, sse)
		parts := collectParts(t, anthropicToDataStream(stream))

		// Last part should be FinishMessageStreamPart with FinishReasonError.
		require.NotEmpty(t, parts)
		lastPart := parts[len(parts)-1]
		fm, ok := lastPart.(aisdk.FinishMessageStreamPart)
		require.True(t, ok, "last part should be FinishMessageStreamPart")
		require.Equal(t, aisdk.FinishReasonError, fm.FinishReason)
	})
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
