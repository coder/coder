package chatd

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestBuildAssistantPartsForPersist_PromotesToolAttachments(t *testing.T) {
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

	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		[]fantasy.Content{fantasy.TextContent{Text: "Here is the screenshot."}},
		[]fantasy.ToolResultContent{{
			ToolCallID:       "call-1",
			ToolName:         "computer",
			ClientMetadata:   response.Metadata,
			ProviderExecuted: false,
		}},
		chatloop.PersistedStep{
			ToolCallCreatedAt: map[string]time.Time{
				"call-1": toolCallAt,
			},
		},
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

func TestBuildAssistantPartsForPersist_PromotesProposePlanAttachment(t *testing.T) {
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

	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		[]fantasy.Content{fantasy.TextContent{Text: "Here is the proposed plan."}},
		[]fantasy.ToolResultContent{{
			ToolCallID:     "call-plan",
			ToolName:       "propose_plan",
			ClientMetadata: response.Metadata,
		}},
		chatloop.PersistedStep{},
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

func TestBuildAssistantPartsForPersist_InvalidAttachmentMetadataSkipsOnlyBrokenResult(t *testing.T) {
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

	parts := buildAssistantPartsForPersist(
		context.Background(),
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
		chatloop.PersistedStep{},
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

func TestBuildAssistantPartsForPersist_AppliesReasoningTimestamps(t *testing.T) {
	t.Parallel()

	startedAt1 := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	completedAt1 := startedAt1.Add(500 * time.Millisecond)
	startedAt2 := completedAt1.Add(time.Second)
	completedAt2 := startedAt2.Add(750 * time.Millisecond)

	// Interleave reasoning blocks with a text block to confirm the
	// index walks reasoning content in occurrence order without
	// being thrown off by non-reasoning entries.
	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		[]fantasy.Content{
			fantasy.ReasoningContent{Text: "first thought"},
			fantasy.TextContent{Text: "intermission"},
			fantasy.ReasoningContent{Text: "second thought"},
		},
		nil,
		chatloop.PersistedStep{
			ReasoningStartedAt:   []time.Time{startedAt1, startedAt2},
			ReasoningCompletedAt: []time.Time{completedAt1, completedAt2},
		},
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

func TestBuildAssistantPartsForPersist_PartialReasoningTimestamps(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)

	// Tests the persistence helper when the parallel CompletedAt
	// slot is zero-valued, ensuring it leaves CompletedAt nil rather
	// than setting it to the Go zero time. No production code path
	// currently emits a zero CompletedAt alongside a non-zero
	// StartedAt (flushActiveState always stamps both with
	// dbtime.Now()), so this is a defensive boundary test for the
	// `variants:"reasoning?"` contract.
	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		[]fantasy.Content{
			fantasy.ReasoningContent{Text: "incomplete thought"},
		},
		nil,
		chatloop.PersistedStep{
			ReasoningStartedAt:   []time.Time{startedAt},
			ReasoningCompletedAt: []time.Time{{}},
		},
		nil,
	)

	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
	require.NotNil(t, parts[0].CreatedAt)
	require.True(t, parts[0].CreatedAt.Equal(startedAt))
	require.Nil(t, parts[0].CompletedAt,
		"zero-valued ReasoningCompletedAt must not produce a stamp")
}

func TestBuildAssistantPartsForPersist_MissingReasoningTimestamps(t *testing.T) {
	t.Parallel()

	// Legacy persisted steps and steps that never observed a
	// reasoning block carry empty timestamp slices. The helper must
	// leave CreatedAt and CompletedAt nil instead of panicking on
	// the out-of-range index.
	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		[]fantasy.Content{
			fantasy.ReasoningContent{Text: "no timestamps recorded"},
		},
		nil,
		chatloop.PersistedStep{},
		nil,
	)

	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
	require.Nil(t, parts[0].CreatedAt)
	require.Nil(t, parts[0].CompletedAt)
}
