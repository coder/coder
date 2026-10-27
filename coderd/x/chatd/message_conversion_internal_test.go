package chatd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestBuildAssistantParts_PromotesToolAttachments(t *testing.T) {
	t.Parallel()

	fileID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	response := chattool.WithAttachments(
		fantasy.NewTextResponse(`{"ok":true}`),
		chattool.AttachmentMetadata{
			FileID:    fileID,
			MediaType: "image/png",
			Name:      "screenshot.png",
		},
	)
	toolCallAt := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	parts := buildAssistantParts(
		testutil.Logger(t),
		[]fantasy.Content{fantasy.TextContent{Text: "Here is the screenshot."}},
		[]fantasy.ToolResultContent{{
			ToolCallID:       "call-1",
			ToolName:         "computer",
			ClientMetadata:   response.Metadata,
			ProviderExecuted: false,
		}},
		stepData{
			ToolCallCreatedAt: map[string]time.Time{
				"call-1": toolCallAt,
			},
		},
		nil,
		nil,
	)

	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
	require.Equal(t, "Here is the screenshot.", parts[0].Text)
	require.Equal(t, codersdk.ChatMessagePartTypeFile, parts[1].Type)
	require.True(t, parts[1].FileID.Valid)
	require.Equal(t, fileID, parts[1].FileID.UUID)
	require.Equal(t, "image/png", parts[1].MediaType)
	require.Equal(t, "screenshot.png", parts[1].Name)
}

func TestBuildAssistantParts_PromotesProposePlanAttachment(t *testing.T) {
	t.Parallel()

	fileID := uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")
	response := chattool.WithAttachments(
		fantasy.NewTextResponse(`{"ok":true,"kind":"plan"}`),
		chattool.AttachmentMetadata{
			FileID:    fileID,
			MediaType: "text/markdown",
			Name:      "PLAN.md",
		},
	)

	parts := buildAssistantParts(
		testutil.Logger(t),
		[]fantasy.Content{fantasy.TextContent{Text: "Here is the proposed plan."}},
		[]fantasy.ToolResultContent{{
			ToolCallID:     "call-plan",
			ToolName:       "propose_plan",
			ClientMetadata: response.Metadata,
		}},
		stepData{},
		nil,
		nil,
	)

	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
	require.Equal(t, "Here is the proposed plan.", parts[0].Text)
	require.Equal(t, codersdk.ChatMessagePartTypeFile, parts[1].Type)
	require.True(t, parts[1].FileID.Valid)
	require.Equal(t, fileID, parts[1].FileID.UUID)
	require.Equal(t, "text/markdown", parts[1].MediaType)
	require.Equal(t, "PLAN.md", parts[1].Name)
}

func TestBuildAssistantParts_InvalidAttachmentMetadataSkipsOnlyBrokenResult(t *testing.T) {
	t.Parallel()

	goodFileID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	goodResponse := chattool.WithAttachments(
		fantasy.NewTextResponse(`{"ok":true}`),
		chattool.AttachmentMetadata{
			FileID:    goodFileID,
			MediaType: "image/png",
			Name:      "good.png",
		},
	)

	parts := buildAssistantParts(
		testutil.Logger(t),
		[]fantasy.Content{fantasy.TextContent{Text: "Here are the results."}},
		[]fantasy.ToolResultContent{
			{
				ToolCallID:     "call-good",
				ToolName:       "computer",
				ClientMetadata: goodResponse.Metadata,
			},
			{
				ToolCallID:     "call-bad",
				ToolName:       "attach_file",
				ClientMetadata: `{"attachments":[{"file_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}]}`,
			},
		},
		stepData{},
		nil,
		nil,
	)

	require.Len(t, parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
	require.Equal(t, codersdk.ChatMessagePartTypeFile, parts[1].Type)
	require.True(t, parts[1].FileID.Valid)
	require.Equal(t, goodFileID, parts[1].FileID.UUID)
	require.Equal(t, "image/png", parts[1].MediaType)
	require.Equal(t, "good.png", parts[1].Name)
}

func TestBuildAssistantParts_AppliesReasoningTimestamps(t *testing.T) {
	t.Parallel()

	startedAt1 := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	completedAt1 := startedAt1.Add(500 * time.Millisecond)
	startedAt2 := completedAt1.Add(time.Second)
	completedAt2 := startedAt2.Add(750 * time.Millisecond)

	// Interleave reasoning blocks with a text block to confirm the
	// index walks reasoning content in occurrence order without
	// being thrown off by non-reasoning entries.
	parts := buildAssistantParts(
		testutil.Logger(t),
		[]fantasy.Content{
			fantasy.ReasoningContent{Text: "first thought"},
			fantasy.TextContent{Text: "intermission"},
			fantasy.ReasoningContent{Text: "second thought"},
		},
		nil,
		stepData{
			ReasoningStartedAt:   []time.Time{startedAt1, startedAt2},
			ReasoningCompletedAt: []time.Time{completedAt1, completedAt2},
		},
		nil,
		nil,
	)

	require.Len(t, parts, 3)

	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
	require.Equal(t, "first thought", parts[0].Text)
	require.NotNil(t, parts[0].CreatedAt)
	require.True(t, parts[0].CreatedAt.Equal(startedAt1),
		"first reasoning part must use ReasoningStartedAt[0]")
	require.NotNil(t, parts[0].CompletedAt)
	require.True(t, parts[0].CompletedAt.Equal(completedAt1),
		"first reasoning part must use ReasoningCompletedAt[0]")

	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[1].Type)
	require.Nil(t, parts[1].CreatedAt,
		"text part must not inherit reasoning timestamps")
	require.Nil(t, parts[1].CompletedAt)

	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[2].Type)
	require.Equal(t, "second thought", parts[2].Text)
	require.NotNil(t, parts[2].CreatedAt)
	require.True(t, parts[2].CreatedAt.Equal(startedAt2),
		"second reasoning part must use ReasoningStartedAt[1]")
	require.NotNil(t, parts[2].CompletedAt)
	require.True(t, parts[2].CompletedAt.Equal(completedAt2),
		"second reasoning part must use ReasoningCompletedAt[1]")
}

func TestBuildAssistantParts_PartialReasoningTimestamps(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)

	// Missing completion timestamps must remain nil rather than being
	// serialized as the Go zero time.
	parts := buildAssistantParts(
		testutil.Logger(t),
		[]fantasy.Content{
			fantasy.ReasoningContent{Text: "incomplete thought"},
		},
		nil,
		stepData{
			ReasoningStartedAt:   []time.Time{startedAt},
			ReasoningCompletedAt: []time.Time{{}},
		},
		nil,
		nil,
	)

	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
	require.NotNil(t, parts[0].CreatedAt)
	require.True(t, parts[0].CreatedAt.Equal(startedAt))
	require.Nil(t, parts[0].CompletedAt,
		"zero-valued ReasoningCompletedAt must not produce a stamp")
}

func TestBuildAssistantParts_MissingReasoningTimestamps(t *testing.T) {
	t.Parallel()

	// Legacy persisted steps and steps that never observed a
	// reasoning block carry empty timestamp slices. The helper must
	// leave CreatedAt and CompletedAt nil instead of panicking on
	// the out-of-range index.
	parts := buildAssistantParts(
		testutil.Logger(t),
		[]fantasy.Content{
			fantasy.ReasoningContent{Text: "no timestamps recorded"},
		},
		nil,
		stepData{},
		nil,
		nil,
	)

	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
	require.Nil(t, parts[0].CreatedAt)
	require.Nil(t, parts[0].CompletedAt)
}

func TestACPResultProjectionsCommit(t *testing.T) {
	t.Parallel()
	model := json.RawMessage(`{"response":"answer"}`)
	user := json.RawMessage(`{"messages":[{"role":"assistant","content":[{"type":"reasoning","text":"private reasoning"}]}]}`)
	response := chattool.WithUserResult(fantasy.NewTextResponse(string(model)), user)
	result := fantasy.ToolResultContent{ToolCallID: "wait", ToolName: "acp_wait_agent", Result: fantasy.ToolResultOutputContentText{Text: string(model)}, ClientMetadata: response.Metadata}
	call := fantasy.ToolCallContent{ToolCallID: "wait", ToolName: "acp_wait_agent", Input: `{"harness_id":"test","session_id":"x","working_directory":"/work"}`}
	commit, err := buildCommitStepMessages(buildCommitStepMessagesInput{logger: slog.Make(), contentVersion: chatprompt.CurrentContentVersion, step: stepData{Content: []fantasy.Content{call, result}}})
	require.NoError(t, err)
	require.Len(t, commit.Messages, 3)
	require.Equal(t, database.ChatMessageVisibilityUser, commit.Messages[1].Visibility)
	require.Equal(t, database.ChatMessageVisibilityModel, commit.Messages[2].Visibility)
	require.Contains(t, string(commit.Messages[1].Content.RawMessage), "private reasoning")
	require.NotContains(t, string(commit.Messages[2].Content.RawMessage), "private reasoning")
	require.JSONEq(t, string(model), string(parseMessageParts(t, commit.Messages[2].Role, commit.Messages[2].Content)[0].Result))
	// Generation and compaction both consume this conversion of prompt rows.
	rows := make([]database.ChatMessage, 0, len(commit.Messages))
	for _, message := range commit.Messages {
		rows = append(rows, database.ChatMessage{Role: message.Role, Content: message.Content, Visibility: message.Visibility, ContentVersion: message.ContentVersion})
	}
	prompt, err := chatprompt.ConvertMessagesWithFiles(context.Background(), rows, nil, slog.Make(), nil, uuid.NullUUID{})
	require.NoError(t, err)
	encoded, err := json.Marshal(prompt)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private reasoning")
	require.Contains(t, string(encoded), "answer")
}

// TestACPResultProjectionsInterrupt covers an interrupt that wins the race
// with the step commit after a projected tool result was published. The
// persisted rows must keep the same model and user split as the commit.
func TestACPResultProjectionsInterrupt(t *testing.T) {
	t.Parallel()
	model := `{"response":"answer"}`
	user := json.RawMessage(`{"messages":[{"role":"assistant","content":[{"type":"reasoning","text":"private reasoning"}]}]}`)
	tool := fantasy.NewAgentTool("acp_wait_agent", "test tool", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return chattool.WithUserResult(fantasy.NewTextResponse(model), user), nil
	})
	clock := quartz.NewMock(t)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()
	key := messagepartbuffer.Key{ChatID: uuid.New(), HistoryVersion: 1, GenerationAttempt: 1}
	require.NoError(t, buffer.CreateEpisode(key))
	args := json.RawMessage(`{"harness_id":"test","session_id":"x","working_directory":"/work"}`)
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageToolCall("wait", "acp_wait_agent", args)))
	// Wire the buffer the way beginGenerationAttempt does.
	_, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
		Clock:       clock,
		Tools:       []fantasy.AgentTool{tool},
		ActiveTools: []string{"acp_wait_agent"},
		ToolCalls:   []fantasy.ToolCallContent{{ToolCallID: "wait", ToolName: "acp_wait_agent", Input: string(args)}},
		PublishMessagePart: func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			require.NoError(t, buffer.AddPart(key, role, part))
		},
		PublishProjectedToolResult: func(part codersdk.ChatMessagePart, modelResult json.RawMessage) {
			require.NoError(t, buffer.AddProjectedToolResult(key, part, modelResult))
		},
		Logger: testutil.Logger(t),
	})
	require.NoError(t, err)

	// The interrupt closes the episode before the step commits.
	require.NoError(t, buffer.CloseEpisode(key))
	parts, err := buffer.GetParts(key)
	require.NoError(t, err)
	partial, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         testutil.Logger(t),
		interruptedAt:  clock.Now(),
	})
	require.NoError(t, err)
	require.Len(t, partial, 3)
	require.Equal(t, database.ChatMessageVisibilityUser, partial[1].Visibility)
	require.JSONEq(t, string(user), string(parseMessageParts(t, partial[1].Role, partial[1].Content)[0].Result))
	require.Equal(t, database.ChatMessageVisibilityModel, partial[2].Visibility)
	require.JSONEq(t, model, string(parseMessageParts(t, partial[2].Role, partial[2].Content)[0].Result))
	rows := make([]database.ChatMessage, 0, len(partial))
	for _, message := range partial {
		rows = append(rows, database.ChatMessage{Role: message.Role, Content: message.Content, Visibility: message.Visibility, ContentVersion: message.ContentVersion})
	}
	prompt, err := chatprompt.ConvertMessagesWithFiles(context.Background(), rows, nil, testutil.Logger(t), nil, uuid.NullUUID{})
	require.NoError(t, err)
	encoded, err := json.Marshal(prompt)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private reasoning")
	require.Contains(t, string(encoded), "answer")
}
