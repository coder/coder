package chatd

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
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

	vllmProviderID := uuid.New()
	togetherProviderID := uuid.New()
	vllmCfg := uuid.New()

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
	text := codersdk.ChatMessageText

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

	origin := func(providerByConfig map[uuid.UUID]string) func(uuid.NullUUID) (string, bool) {
		return func(id uuid.NullUUID) (string, bool) {
			if !id.Valid {
				return "", false
			}
			provider, ok := providerByConfig[id.UUID]
			return provider, ok
		}
	}
	resolver := origin(map[uuid.UUID]string{
		anthropicCfg: anthropic,
		openAICfg:    openai,
		vllmCfg:      vllmProviderID.String(),
	})

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
		got, stats := stripForeignProviderExecutedToolRows(rows, anthropic, resolver, nil)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("anthropic to bedrock drops provider blocks", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, anthropicCfg, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, nil)
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
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, nil)
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
		got, stats := stripForeignProviderExecutedToolRows(rows, anthropic, resolver, nil)
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
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, nil)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("empty target is a no-op", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, anthropicCfg, peCall("ws"), peResult("ws")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, "", resolver, nil)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("unknown origin fails closed", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, unknownCfg, peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, nil)
		require.Len(t, got, 1)
		require.Equal(t, []codersdk.ChatMessagePart{text("done")}, partsOf(t, got[0]))
		require.Equal(t, providerSwitchStripStats{RemovedToolResults: 1}, stats)
	})

	t.Run("unparsable foreign row kept unchanged", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{{
			Role:           database.ChatMessageRoleAssistant,
			ModelConfigID:  uuid.NullUUID{UUID: anthropicCfg, Valid: true},
			Content:        pqtype.NullRawMessage{RawMessage: []byte("{not json"), Valid: true},
			ContentVersion: chatprompt.ContentVersionV1,
		}}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, nil)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	t.Run("same type different instance drops provider blocks", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, vllmCfg, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, togetherProviderID.String(), resolver, nil)
		require.Len(t, got, 2)
		require.Equal(t, []codersdk.ChatMessagePart{text("done")}, partsOf(t, got[1]))
		require.Equal(t, providerSwitchStripStats{RemovedToolCalls: 1, RemovedToolResults: 1}, stats)
	})

	t.Run("same instance keeps provider blocks", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, vllmCfg, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, vllmProviderID.String(), resolver, nil)
		require.Equal(t, rows, got)
		require.Zero(t, stats)
	})

	signedReasoning := func(t *testing.T) codersdk.ChatMessagePart {
		t.Helper()
		metadata, err := json.Marshal(fantasy.ProviderMetadata{
			fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
				Signature: "sig-1",
			},
		})
		require.NoError(t, err)
		p := codersdk.ChatMessageReasoning("thinking")
		p.ProviderMetadata = metadata
		return p
	}
	signedRunPredicate := func(parts []codersdk.ChatMessagePart) bool {
		return chatprompt.PartsHaveAnthropicSignedReasoning(slogtest.Make(t, nil), parts)
	}

	t.Run("signed reasoning protects latest assistant run", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, unknownCfg, signedReasoning(t), peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, signedRunPredicate)
		require.Equal(t, rows, got)
		require.Equal(t, providerSwitchStripStats{ProtectedRows: 1}, stats)
	})

	t.Run("signed run protects adjacent trailing assistant rows", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, unknownCfg, peCall("a"), peResult("a"), text("step1")),
			assistantRow(t, unknownCfg, signedReasoning(t), text("step2")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, signedRunPredicate)
		require.Equal(t, rows, got)
		require.Equal(t, providerSwitchStripStats{ProtectedRows: 1}, stats)
	})

	t.Run("signed row outside latest run still stripped", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			assistantRow(t, unknownCfg, signedReasoning(t), peCall("old"), text("old")),
			userRow(t, "mid"),
			assistantRow(t, unknownCfg, text("new")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, signedRunPredicate)
		require.Len(t, got, 3)
		require.Len(t, partsOf(t, got[0]), 2)
		require.Equal(t, providerSwitchStripStats{RemovedToolCalls: 1}, stats)
	})

	t.Run("nil predicate strips signed run", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, unknownCfg, signedReasoning(t), peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, nil)
		require.Len(t, got, 2)
		require.Len(t, partsOf(t, got[1]), 2)
		require.Equal(t, providerSwitchStripStats{RemovedToolCalls: 1, RemovedToolResults: 1}, stats)
	})

	t.Run("redacted reasoning protects latest assistant run", func(t *testing.T) {
		t.Parallel()
		metadata, err := json.Marshal(fantasy.ProviderMetadata{
			fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
				RedactedData: "redacted-payload",
			},
		})
		require.NoError(t, err)
		redacted := codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeReasoning,
			ProviderMetadata: metadata,
		}
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, unknownCfg, redacted, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, signedRunPredicate)
		require.Equal(t, rows, got)
		require.Equal(t, providerSwitchStripStats{ProtectedRows: 1}, stats)
	})

	t.Run("undecodable reasoning metadata protects run", func(t *testing.T) {
		t.Parallel()
		corrupt := codersdk.ChatMessageReasoning("thinking")
		corrupt.ProviderMetadata = json.RawMessage(`"not-a-metadata-object"`)
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			assistantRow(t, unknownCfg, corrupt, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, signedRunPredicate)
		require.Equal(t, rows, got)
		require.Equal(t, providerSwitchStripStats{ProtectedRows: 1}, stats)
	})

	t.Run("unparseable row in latest run protects run", func(t *testing.T) {
		t.Parallel()
		rows := []database.ChatMessage{
			userRow(t, "hi"),
			{
				Role:           database.ChatMessageRoleAssistant,
				ModelConfigID:  uuid.NullUUID{UUID: unknownCfg, Valid: true},
				Content:        pqtype.NullRawMessage{RawMessage: []byte("{not json"), Valid: true},
				ContentVersion: chatprompt.ContentVersionV1,
			},
			assistantRow(t, unknownCfg, peCall("ws"), peResult("ws"), text("done")),
		}
		got, stats := stripForeignProviderExecutedToolRows(rows, bedrock, resolver, signedRunPredicate)
		require.Equal(t, rows, got)
		require.Equal(t, providerSwitchStripStats{ProtectedRows: 1}, stats)
	})
}

func TestSignedReasoningRunProtection(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	require.NotNil(t, signedReasoningRunProtection("anthropic", logger))
	require.Nil(t, signedReasoningRunProtection("openai", logger))
	require.Nil(t, signedReasoningRunProtection("bedrock", logger))
	require.Nil(t, signedReasoningRunProtection("", logger))
}

func TestOriginProviderIdentityToleratesUnusableConfigs(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()
	cfg := database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		Deleted:      true,
		Enabled:      false,
	}
	require.Equal(t, providerID.String(), originProviderIdentity(cfg))

	require.Empty(t, originProviderIdentity(database.ChatModelConfig{Deleted: true}))
}

func TestModelConfigProviderIdentity(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()

	t.Run("AIProviderID valid returns provider UUID", func(t *testing.T) {
		t.Parallel()
		cfg := database.ChatModelConfig{
			AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		}
		got := modelConfigProviderIdentity(cfg, "openai-compat")
		require.Equal(t, providerID.String(), got)
	})

	t.Run("AIProviderID invalid falls back to normalized type", func(t *testing.T) {
		t.Parallel()
		cfg := database.ChatModelConfig{
			AIProviderID: uuid.NullUUID{},
		}
		got := modelConfigProviderIdentity(cfg, "anthropic")
		require.Equal(t, "anthropic", got)
	})

	t.Run("same type different provider IDs are distinguished", func(t *testing.T) {
		t.Parallel()
		otherProviderID := uuid.New()
		cfgA := database.ChatModelConfig{
			AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
		}
		cfgB := database.ChatModelConfig{
			AIProviderID: uuid.NullUUID{UUID: otherProviderID, Valid: true},
		}
		require.NotEqual(t,
			modelConfigProviderIdentity(cfgA, "openai-compat"),
			modelConfigProviderIdentity(cfgB, "openai-compat"),
		)
	})
}
