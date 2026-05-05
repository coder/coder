package chatd //nolint:testpackage // Keeps internal helper tests in-package.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func Test_extractManualTitleTurns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []database.ChatMessage
		want     []manualTitleTurn
	}{
		{
			name: "filters to visible user and assistant text turns",
			messages: []database.ChatMessage{
				mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "  review quickgen helpers  "},
				),
				mustChatMessage(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "  drafted a plan  "},
				),
				mustChatMessage(t, database.ChatMessageRoleSystem, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "system prompt"},
				),
				mustChatMessage(t, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "tool output"},
				),
				mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hidden model note"},
				),
				mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "   "},
				),
				mustChatMessage(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: "reasoning only"},
				),
				mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeFile, MediaType: "text/plain"},
				),
			},
			want: []manualTitleTurn{
				{role: "user", text: "review quickgen helpers"},
				{role: "assistant", text: "drafted a plan"},
			},
		},
		{
			name: "reuses text extraction for multi-part content",
			messages: []database.ChatMessage{
				mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth,
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "first chunk"},
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: "skip me"},
					codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: " second chunk "},
				),
			},
			want: []manualTitleTurn{{role: "user", text: "first chunk second chunk"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := extractManualTitleTurns(tt.messages)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_selectManualTitleTurnIndexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		turns []manualTitleTurn
		want  []int
	}{
		{
			name: "single user turn",
			turns: []manualTitleTurn{
				{role: "user", text: "one"},
			},
			want: []int{0},
		},
		{
			name: "first user plus trailing window",
			turns: []manualTitleTurn{
				{role: "user", text: "one"},
				{role: "assistant", text: "two"},
				{role: "user", text: "three"},
				{role: "assistant", text: "four"},
				{role: "user", text: "five"},
			},
			want: []int{0, 2, 3, 4},
		},
		{
			name: "two turns returns both",
			turns: []manualTitleTurn{
				{role: "user", text: "one"},
				{role: "assistant", text: "two"},
			},
			want: []int{0, 1},
		},
		{
			name: "prepends first user when before trailing window",
			turns: []manualTitleTurn{
				{role: "assistant", text: "intro"},
				{role: "assistant", text: "setup"},
				{role: "user", text: "goal"},
				{role: "assistant", text: "a"},
				{role: "assistant", text: "b"},
				{role: "assistant", text: "c"},
			},
			want: []int{2, 3, 4, 5},
		},
		{
			name: "ten plus turns keeps first user and last three",
			turns: []manualTitleTurn{
				{role: "assistant", text: "0"},
				{role: "assistant", text: "1"},
				{role: "user", text: "2"},
				{role: "assistant", text: "3"},
				{role: "assistant", text: "4"},
				{role: "assistant", text: "5"},
				{role: "assistant", text: "6"},
				{role: "assistant", text: "7"},
				{role: "assistant", text: "8"},
				{role: "user", text: "9"},
				{role: "assistant", text: "10"},
				{role: "user", text: "11"},
			},
			want: []int{2, 9, 10, 11},
		},
		{
			name: "no user turns",
			turns: []manualTitleTurn{
				{role: "assistant", text: "one"},
				{role: "assistant", text: "two"},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := selectManualTitleTurnIndexes(tt.turns)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_buildManualTitleContext(t *testing.T) {
	t.Parallel()

	longConversationText := strings.Repeat("a", 3500)
	longLatestUserText := strings.Repeat("z", 1200)

	tests := []struct {
		name                   string
		turns                  []manualTitleTurn
		selected               []int
		wantConversation       string
		wantConversationEmpty  bool
		wantConversationHasGap bool
		wantConversationRunes  int
		wantLatestUser         string
		wantLatestUserRunes    int
		wantLatestUserContains string
		wantLatestUserNotEmpty bool
	}{
		{
			name: "adds gap marker when selected turns skip earlier context",
			turns: []manualTitleTurn{
				{role: "user", text: "open pull request"},
				{role: "assistant", text: "checked CI"},
				{role: "user", text: "review logs"},
				{role: "assistant", text: "found flaky test"},
				{role: "user", text: "update chat title"},
			},
			selected:               []int{0, 3, 4},
			wantConversationHasGap: true,
			wantLatestUser:         "update chat title",
		},
		{
			name: "omits gap marker for contiguous selection",
			turns: []manualTitleTurn{
				{role: "user", text: "open pull request"},
				{role: "assistant", text: "checked CI"},
				{role: "user", text: "update chat title"},
			},
			selected:               []int{0, 1, 2},
			wantConversation:       "[user]: open pull request\n[assistant]: checked CI\n[user]: update chat title",
			wantConversationHasGap: false,
			wantLatestUser:         "update chat title",
		},
		{
			name:                  "single useful user turn returns empty conversation block",
			turns:                 []manualTitleTurn{{role: "user", text: "rename helper"}},
			selected:              []int{0},
			wantConversationEmpty: true,
			wantLatestUser:        "rename helper",
		},
		{
			name: "truncates conversation block at six thousand runes",
			turns: []manualTitleTurn{
				{role: "user", text: longConversationText},
				{role: "assistant", text: longConversationText},
				{role: "user", text: "latest"},
			},
			selected:              []int{0, 1, 2},
			wantConversationRunes: 6000,
			wantLatestUser:        "latest",
		},
		{
			name: "truncates latest user message at one thousand runes",
			turns: []manualTitleTurn{
				{role: "user", text: "first"},
				{role: "assistant", text: "reply"},
				{role: "user", text: longLatestUserText},
			},
			selected:               []int{0, 1, 2},
			wantLatestUserRunes:    1000,
			wantLatestUserContains: strings.Repeat("z", 1000),
			wantLatestUserNotEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conversationBlock, latestUserMsg := buildManualTitleContext(tt.turns, tt.selected)

			if tt.wantConversationEmpty {
				require.Empty(t, conversationBlock)
			}
			if tt.wantConversation != "" {
				require.Equal(t, tt.wantConversation, conversationBlock)
			}
			if tt.wantConversationHasGap {
				require.Contains(t, conversationBlock, "[... 2 earlier turns omitted ...]")
			} else if !tt.wantConversationEmpty {
				require.NotContains(t, conversationBlock, "earlier turns omitted")
			}
			if tt.wantConversationRunes > 0 {
				require.Len(t, []rune(conversationBlock), tt.wantConversationRunes)
			}
			if tt.wantLatestUser != "" {
				require.Equal(t, tt.wantLatestUser, latestUserMsg)
			}
			if tt.wantLatestUserRunes > 0 {
				require.Len(t, []rune(latestUserMsg), tt.wantLatestUserRunes)
			}
			if tt.wantLatestUserContains != "" {
				require.Equal(t, tt.wantLatestUserContains, latestUserMsg)
			}
			if tt.wantLatestUserNotEmpty {
				require.NotEmpty(t, latestUserMsg)
			}
		})
	}
}

func Test_renderManualTitlePrompt(t *testing.T) {
	t.Parallel()

	longFirstUserText := strings.Repeat("b", 1501)

	tests := []struct {
		name                   string
		conversationBlock      string
		firstUserText          string
		latestUserMsg          string
		wantConversationSample bool
		wantLatestSection      bool
	}{
		{
			name:                   "includes conversation sample when provided",
			conversationBlock:      "[user]: inspect logs\n[assistant]: found flaky test",
			firstUserText:          "inspect logs",
			latestUserMsg:          "update quickgen title",
			wantConversationSample: true,
			wantLatestSection:      true,
		},
		{
			name:                   "omits optional sections when not needed",
			conversationBlock:      "",
			firstUserText:          "inspect logs",
			latestUserMsg:          "inspect logs",
			wantConversationSample: false,
			wantLatestSection:      false,
		},
		{
			name:                   "latest section compares trimmed text",
			conversationBlock:      "",
			firstUserText:          "inspect logs",
			latestUserMsg:          " inspect logs ",
			wantConversationSample: false,
			wantLatestSection:      false,
		},
		{
			name:                   "omits latest section when same message truncated",
			conversationBlock:      "",
			firstUserText:          longFirstUserText,
			latestUserMsg:          truncateRunes(longFirstUserText, 1000),
			wantConversationSample: false,
			wantLatestSection:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prompt := renderManualTitlePrompt(tt.conversationBlock, tt.firstUserText, tt.latestUserMsg)

			require.Contains(t, prompt, "Primary user objective:")
			require.Contains(t, prompt, "Requirements:")
			require.Contains(t, prompt, "- Return only the title text in 2-8 words.")
			require.Contains(t, prompt, "Do not answer the user or describe the title-writing task")
			require.Contains(t, prompt, "stay close to the user's wording")

			if tt.wantConversationSample {
				require.Contains(t, prompt, "Conversation sample:")
				require.Contains(t, prompt, tt.conversationBlock)
			} else {
				require.NotContains(t, prompt, "Conversation sample:")
			}

			if tt.wantLatestSection {
				require.Contains(t, prompt, "The user's most recent message:")
				require.Contains(t, prompt, "Note: Weight the overall conversation arc more heavily than just the latest message.")
				require.Contains(t, prompt, strings.TrimSpace(tt.latestUserMsg))
			} else {
				require.NotContains(t, prompt, "The user's most recent message:")
				require.NotContains(t, prompt, "Weight the overall conversation arc more heavily")
			}
		})
	}
}

func Test_titleGenerationPrompt_UsesSlimRules(t *testing.T) {
	t.Parallel()

	require.Contains(t, titleGenerationPrompt, "Return only the title text in 2-8 words")
	require.Contains(t, titleGenerationPrompt, "Do not answer the user or describe the title-writing task")
	require.Contains(t, titleGenerationPrompt, "stay close to the user's wording")
	require.NotContains(t, titleGenerationPrompt, "I am a title generator")
}

func Test_generateManualTitle_UsesTimeout(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("refresh chat title"),
		),
	}

	model := &chattest.FakeModel{
		GenerateObjectFn: func(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "manual title generation should set a deadline")
			require.WithinDuration(
				t,
				time.Now().Add(30*time.Second),
				deadline,
				2*time.Second,
			)
			require.Len(t, call.Prompt, 2)
			require.Equal(t, "propose_title", call.SchemaName)
			return &fantasy.ObjectResponse{Object: map[string]any{"title": "Refresh title"}}, nil
		},
	}

	title, _, err := generateManualTitle(
		context.Background(),
		messages,
		model,
	)
	require.NoError(t, err)
	require.Equal(t, "Refresh title", title)
}

func Test_generateManualTitle_TruncatesFirstUserInput(t *testing.T) {
	t.Parallel()

	longFirstUserText := strings.Repeat("a", 1500)
	messages := []database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText(longFirstUserText),
		),
	}

	model := &chattest.FakeModel{
		GenerateObjectFn: func(_ context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			require.Len(t, call.Prompt, 2)
			systemText, ok := call.Prompt[0].Content[0].(fantasy.TextPart)
			require.True(t, ok)
			require.Contains(t, systemText.Text, truncateRunes(longFirstUserText, 1000))

			userText, ok := call.Prompt[1].Content[0].(fantasy.TextPart)
			require.True(t, ok)
			require.Equal(t, truncateRunes(longFirstUserText, 1000), userText.Text)
			return &fantasy.ObjectResponse{Object: map[string]any{"title": "Refresh title"}}, nil
		},
	}

	_, _, err := generateManualTitle(
		context.Background(),
		messages,
		model,
	)
	require.NoError(t, err)
}

func Test_generateManualTitle_ReturnsUsageForEmptyNormalizedTitle(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("refresh chat title"),
		),
	}

	model := &chattest.FakeModel{
		GenerateObjectFn: func(_ context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			return &fantasy.ObjectResponse{
				Object: map[string]any{"title": "\"\""},
				Usage: fantasy.Usage{
					InputTokens:  11,
					OutputTokens: 7,
					TotalTokens:  18,
				},
			}, nil
		},
	}

	_, usage, err := generateManualTitle(
		context.Background(),
		messages,
		model,
	)
	require.ErrorContains(t, err, "generated title was empty")
	require.Equal(t, int64(11), usage.InputTokens)
	require.Equal(t, int64(7), usage.OutputTokens)
	require.Equal(t, int64(18), usage.TotalTokens)
}

func Test_selectPreferredConfiguredShortTextModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("chooses the highest-priority configured lightweight model", func(t *testing.T) {
		t.Parallel()

		configs := []database.ChatModelConfig{
			{Provider: preferredTitleModels[2].provider, Model: preferredTitleModels[2].model},
			{Provider: preferredTitleModels[1].provider, Model: preferredTitleModels[1].model},
			{Provider: "openai", Model: "gpt-4.1"},
		}

		got, ok := selectPreferredConfiguredShortTextModelConfig(configs)
		require.True(t, ok)
		require.Equal(t, preferredTitleModels[1].provider, got.Provider)
		require.Equal(t, preferredTitleModels[1].model, got.Model)
	})

	t.Run("returns false when no preferred lightweight model is configured", func(t *testing.T) {
		t.Parallel()

		got, ok := selectPreferredConfiguredShortTextModelConfig([]database.ChatModelConfig{{
			Provider: "openai",
			Model:    "gpt-4.1",
		}})
		require.False(t, ok)
		require.Equal(t, database.ChatModelConfig{}, got)
	})
}

func Test_generateShortText_NormalizesQuotedOutput(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			return &fantasy.Response{
				Content: fantasy.ResponseContent{
					fantasy.TextContent{Text: "  \"Quoted summary\"  "},
				},
				Usage: fantasy.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
			}, nil
		},
	}

	text, err := generateShortText(context.Background(), model, "system", "user")
	require.NoError(t, err)
	require.Equal(t, "Quoted summary", text)
}

func mustChatMessage(
	t *testing.T,
	role database.ChatMessageRole,
	visibility database.ChatMessageVisibility,
	parts ...codersdk.ChatMessagePart,
) database.ChatMessage {
	t.Helper()

	content, err := json.Marshal(parts)
	require.NoError(t, err)

	return database.ChatMessage{
		Role:       role,
		Visibility: visibility,
		Content: pqtype.NullRawMessage{
			RawMessage: content,
			Valid:      len(content) > 0,
		},
	}
}
