package dbauthz

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func toNullRawMessage(t *testing.T, v any) pqtype.NullRawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return pqtype.NullRawMessage{RawMessage: b, Valid: true}
}

func TestParseMessagePartsForRedaction(t *testing.T) {
	t.Parallel()

	t.Run("EmptyContent", func(t *testing.T) {
		t.Parallel()
		_, ok := parseMessagePartsForRedaction(database.ChatMessage{})
		require.False(t, ok, "empty content must not parse")

		_, ok = parseMessagePartsForRedaction(database.ChatMessage{
			Content: pqtype.NullRawMessage{Valid: true},
		})
		require.False(t, ok, "zero-length raw content must not parse")
	})

	t.Run("StructuredParts", func(t *testing.T) {
		t.Parallel()
		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
			codersdk.ChatMessageReasoning("thinking"),
		}
		got, ok := parseMessagePartsForRedaction(database.ChatMessage{
			Content: toNullRawMessage(t, parts),
		})
		require.True(t, ok)
		require.Len(t, got, 2)
		require.Equal(t, codersdk.ChatMessagePartTypeText, got[0].Type)
		require.Equal(t, codersdk.ChatMessagePartTypeReasoning, got[1].Type)
	})

	t.Run("BareStringFallback", func(t *testing.T) {
		t.Parallel()
		got, ok := parseMessagePartsForRedaction(database.ChatMessage{
			Content: toNullRawMessage(t, "hello world"),
		})
		require.True(t, ok)
		require.Len(t, got, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeText, got[0].Type)
	})

	t.Run("BareStringEmptyRejected", func(t *testing.T) {
		t.Parallel()
		_, ok := parseMessagePartsForRedaction(database.ChatMessage{
			Content: toNullRawMessage(t, "   "),
		})
		require.False(t, ok, "whitespace-only bare string must not parse")
	})

	t.Run("LegacyToolResultRows", func(t *testing.T) {
		t.Parallel()
		rows := []legacyToolResultRow{
			{
				ToolCallID:       "call-1",
				ToolName:         "demo_tool",
				Result:           json.RawMessage(`{"ok":true}`),
				IsError:          false,
				IsMedia:          false,
				ProviderExecuted: true,
				ProviderMetadata: json.RawMessage(`{"p":"v"}`),
			},
		}
		got, ok := parseMessagePartsForRedaction(database.ChatMessage{
			Content: toNullRawMessage(t, rows),
		})
		require.True(t, ok)
		require.Len(t, got, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeToolResult, got[0].Type)
		require.True(t, got[0].ProviderExecuted)
	})

	t.Run("UnrecognizedJSONRejected", func(t *testing.T) {
		t.Parallel()
		_, ok := parseMessagePartsForRedaction(database.ChatMessage{
			Content: toNullRawMessage(t, map[string]string{"unexpected": "shape"}),
		})
		require.False(t, ok)
	})
}

func TestRedactChatMessageParts(t *testing.T) {
	t.Parallel()

	all := []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("t"),
		codersdk.ChatMessageReasoning("r"),
		codersdk.ChatMessageToolCall("id", "name", json.RawMessage(`{}`)),
		codersdk.ChatMessageToolResult("id", "name", json.RawMessage(`{}`), false, false),
		{Type: codersdk.ChatMessagePartTypeFile},
		codersdk.ChatMessageFileReference("f", 1, 2, "c"),
		{Type: codersdk.ChatMessagePartTypeContextFile, ContextFilePath: "a"},
	}

	partTypes := func(parts []codersdk.ChatMessagePart) []codersdk.ChatMessagePartType {
		out := make([]codersdk.ChatMessagePartType, 0, len(parts))
		for _, p := range parts {
			out = append(out, p.Type)
		}
		return out
	}

	t.Run("NothingShared", func(t *testing.T) {
		t.Parallel()
		got := redactChatMessageParts(all, chatShareFlags{})
		require.Equal(t, []codersdk.ChatMessagePartType{
			codersdk.ChatMessagePartTypeText,
			codersdk.ChatMessagePartTypeReasoning,
		}, partTypes(got))
	})

	t.Run("ToolsOnly", func(t *testing.T) {
		t.Parallel()
		got := redactChatMessageParts(all, chatShareFlags{shareToolCalls: true})
		require.Equal(t, []codersdk.ChatMessagePartType{
			codersdk.ChatMessagePartTypeText,
			codersdk.ChatMessagePartTypeReasoning,
			codersdk.ChatMessagePartTypeToolCall,
			codersdk.ChatMessagePartTypeToolResult,
		}, partTypes(got))
	})

	t.Run("AttachmentsOnly", func(t *testing.T) {
		t.Parallel()
		got := redactChatMessageParts(all, chatShareFlags{shareAttachments: true})
		require.Equal(t, []codersdk.ChatMessagePartType{
			codersdk.ChatMessagePartTypeText,
			codersdk.ChatMessagePartTypeReasoning,
			codersdk.ChatMessagePartTypeFile,
			codersdk.ChatMessagePartTypeFileReference,
			codersdk.ChatMessagePartTypeContextFile,
		}, partTypes(got))
	})

	t.Run("BothShared", func(t *testing.T) {
		t.Parallel()
		got := redactChatMessageParts(all, chatShareFlags{shareToolCalls: true, shareAttachments: true})
		require.Equal(t, partTypes(all), partTypes(got))
	})

	t.Run("UnknownPartTypePassesThrough", func(t *testing.T) {
		t.Parallel()
		unknown := codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartType("future-type")}
		got := redactChatMessageParts([]codersdk.ChatMessagePart{unknown}, chatShareFlags{})
		require.Len(t, got, 1)
		require.Equal(t, unknown.Type, got[0].Type)
	})
}

func TestChatShareFlagsFromACL(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	groupA := uuid.New().String()
	groupB := uuid.New().String()

	chat := database.Chat{
		UserACL: database.ChatACL{
			userID.String(): {ShareToolCalls: true},
		},
		GroupACL: database.ChatACL{
			groupA: {ShareAttachments: true},
			groupB: {ShareToolCalls: true, ShareAttachments: true},
		},
	}

	t.Run("UserEntryOnly", func(t *testing.T) {
		t.Parallel()
		flags := chatShareFlagsFromACL(chat, userID, nil)
		require.True(t, flags.shareToolCalls)
		require.False(t, flags.shareAttachments)
	})

	t.Run("UserAndSingleGroupUnion", func(t *testing.T) {
		t.Parallel()
		flags := chatShareFlagsFromACL(chat, userID, map[string]struct{}{
			groupA: {},
		})
		require.True(t, flags.shareToolCalls, "tool flag comes from user entry")
		require.True(t, flags.shareAttachments, "attachment flag comes from group entry")
	})

	t.Run("GroupEntryAloneGrantsFlag", func(t *testing.T) {
		t.Parallel()
		// The user has no direct entry for this case, so only the group's
		// attachment flag should be honored.
		other := uuid.New()
		flags := chatShareFlagsFromACL(chat, other, map[string]struct{}{
			groupA: {},
		})
		require.False(t, flags.shareToolCalls)
		require.True(t, flags.shareAttachments)
	})

	t.Run("MissingGroupIsIgnored", func(t *testing.T) {
		t.Parallel()
		flags := chatShareFlagsFromACL(chat, uuid.New(), map[string]struct{}{
			uuid.New().String(): {},
		})
		require.False(t, flags.shareToolCalls)
		require.False(t, flags.shareAttachments)
	})

	t.Run("FullGroupShortCircuits", func(t *testing.T) {
		t.Parallel()
		// A group that already sets both flags must short-circuit the
		// group loop. This guards the break inside chatShareFlagsFromACL.
		flags := chatShareFlagsFromACL(chat, uuid.New(), map[string]struct{}{
			groupB: {},
			groupA: {},
		})
		require.True(t, flags.shareToolCalls)
		require.True(t, flags.shareAttachments)
	})
}
