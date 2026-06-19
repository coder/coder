package chatd

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func TestStripForeignProviderExecutedToolRows(t *testing.T) {
	t.Parallel()

	const (
		anthropic = "anthropic"
		bedrock   = "bedrock"
		openai    = "openai"
	)

	anthropicCfg := uuid.New()
	openAICfg := uuid.New()
	unknownCfg := uuid.New()

	peCall := func(id string) codersdk.ChatMessagePart {
		p := codersdk.ChatMessageToolCall(id, "web_search", json.RawMessage(`{"query":"x"}`))
		p.ProviderExecuted = true
		return p
	}
	peResult := func(id string) codersdk.ChatMessagePart {
		p := codersdk.ChatMessageToolResult(id, "web_search", json.RawMessage(`{"ok":true}`), false, false)
		p.ProviderExecuted = true
		return p
	}
	localCall := func(id string) codersdk.ChatMessagePart {
		return codersdk.ChatMessageToolCall(id, "read_file", json.RawMessage(`{}`))
	}
	text := func(s string) codersdk.ChatMessagePart { return codersdk.ChatMessageText(s) }

	assistantRow := func(t *testing.T, cfg uuid.UUID, parts ...codersdk.ChatMessagePart) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		return database.ChatMessage{
			Role:           database.ChatMessageRoleAssistant,
			ModelConfigID:  uuid.NullUUID{UUID: cfg, Valid: cfg != uuid.Nil},
			Content:        content,
			ContentVersion: chatprompt.ContentVersionV1,
		}
	}
	userRow := func(t *testing.T, s string) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{text(s)})
		require.NoError(t, err)
		return database.ChatMessage{
			Role:           database.ChatMessageRoleUser,
			Content:        content,
			ContentVersion: chatprompt.ContentVersionV1,
		}
	}

	// origin maps a model config ID to its normalized provider. unknownCfg is
	// intentionally absent so the resolver reports an unknown origin.
	origin := func(cfgByProvider map[uuid.UUID]string) func(uuid.NullUUID) (string, bool) {
		return func(id uuid.NullUUID) (string, bool) {
			if !id.Valid {
				return "", false
			}
			provider, ok := cfgByProvider[id.UUID]
			return provider, ok
		}
	}
	resolver := origin(map[uuid.UUID]string{
		anthropicCfg: anthropic,
		openAICfg:    openai,
	})

	// partsOf parses a row's content back into SDK parts for comparison.
	partsOf := func(t *testing.T, row database.ChatMessage) []codersdk.ChatMessagePart {
		t.Helper()
		parts, err := chatprompt.ParseContent(row)
		require.NoError(t, err)
		return parts
	}

	t.Run("same provider kept", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, anthropicCfg, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, anthropic, resolver)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("anthropic to bedrock drops provider blocks", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, anthropicCfg, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver)
		require.Len(t, got, 2)
		require.Equal(t, []codersdk.ChatMessagePart{text("done")}, partsOf(t, got[1]))
		require.Equal(t, providerSwitchStripStats{RemovedToolCalls: 1, RemovedToolResults: 1}, stats)
	})

	t.Run("foreign-only row dropped", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, anthropicCfg, peCall("ws")),
			userRow(t, "again"),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver)
		require.Len(t, got, 2)
		require.Equal(t, database.ChatMessageRoleUser, got[0].Role)
		require.Equal(t, database.ChatMessageRoleUser, got[1].Role)
		require.Equal(t, providerSwitchStripStats{RemovedToolCalls: 1, DroppedMessages: 1}, stats)
	})

	t.Run("multi-provider keeps native strips foreign", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, openAICfg, peCall("os"), peResult("os"), text("openai")),
			assistantRow(t, anthropicCfg, peCall("as"), peResult("as"), text("anthropic")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, anthropic, resolver)
		require.Len(t, got, 2)
		require.Equal(t, []codersdk.ChatMessagePart{text("openai")}, partsOf(t, got[0]))
		require.Equal(t, rows[1], got[1])
		require.Equal(t, providerSwitchStripStats{RemovedToolCalls: 1, RemovedToolResults: 1}, stats)
	})

	t.Run("non-provider-executed parts untouched", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, anthropicCfg, text("hello"), localCall("local")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("empty target is a no-op", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, anthropicCfg, peCall("ws"), peResult("ws")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, "", resolver)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("unknown origin fails closed", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, unknownCfg, peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver)
		require.Len(t, got, 1)
		require.Equal(t, []codersdk.ChatMessagePart{text("done")}, partsOf(t, got[0]))
		require.Equal(t, providerSwitchStripStats{RemovedToolResults: 1}, stats)
	})
}
