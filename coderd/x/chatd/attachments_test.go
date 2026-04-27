package chatd //nolint:testpackage

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
