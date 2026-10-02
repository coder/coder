package chatd

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/codersdk"
)

// providerSwitchStripStats counts provider-specific history removed during a
// provider switch.
type providerSwitchStripStats struct {
	RemovedToolCalls   int
	RemovedToolResults int
	RemovedReasoning   int
	DroppedMessages    int
}

// modelConfigProviderIdentity returns a stable identity for the upstream provider
// behind a model config. When the config has an AIProviderID (the modern path),
// the identity is the provider instance UUID, so two providers of the same type
// (e.g. two openai-compat providers at different base URLs) are distinguished.
// When AIProviderID is invalid (legacy configs with no provider row), the
// identity falls back to the normalized provider type name.
func modelConfigProviderIdentity(modelConfig database.ChatModelConfig, normalizedProvider string) string {
	if modelConfig.AIProviderID.Valid {
		return modelConfig.AIProviderID.UUID.String()
	}
	return normalizedProvider
}

// stripForeignProviderStateRows drops provider-executed tool blocks (calls and
// results) and OpenAI reasoning state from assistant rows whose producing
// provider differs from targetIdentity. Reasoning item IDs and encrypted
// content only resolve on the provider instance that issued them. Rows with an
// unknown origin are treated as foreign (fail closed). Rows emptied by
// stripping are dropped; rows that fail to parse or re-marshal are kept
// unchanged.
//
// See modelConfigProviderIdentity for how identity is derived.
func stripForeignProviderStateRows(
	rows []database.ChatMessage,
	targetIdentity string,
	originProvider func(uuid.NullUUID) (string, bool),
) ([]database.ChatMessage, providerSwitchStripStats) {
	var stats providerSwitchStripStats
	if targetIdentity == "" || len(rows) == 0 {
		return rows, stats
	}

	out := make([]database.ChatMessage, 0, len(rows))
	for _, row := range rows {
		if row.Role != database.ChatMessageRoleAssistant {
			out = append(out, row)
			continue
		}
		if origin, ok := originProvider(row.ModelConfigID); ok && origin == targetIdentity {
			out = append(out, row)
			continue
		}

		parts, err := chatprompt.ParseContent(row)
		if err != nil {
			out = append(out, row)
			continue
		}

		kept := make([]codersdk.ChatMessagePart, 0, len(parts))
		var removedCalls, removedResults, removedReasoning int
		for _, part := range parts {
			switch {
			case part.Type == codersdk.ChatMessagePartTypeToolCall && part.ProviderExecuted:
				removedCalls++
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ProviderExecuted:
				removedResults++
			case part.Type == codersdk.ChatMessagePartTypeReasoning && hasOpenAIReasoningState(part):
				removedReasoning++
			default:
				kept = append(kept, part)
			}
		}
		if removedCalls == 0 && removedResults == 0 && removedReasoning == 0 {
			out = append(out, row)
			continue
		}
		stats.RemovedToolCalls += removedCalls
		stats.RemovedToolResults += removedResults
		stats.RemovedReasoning += removedReasoning
		if len(kept) == 0 {
			stats.DroppedMessages++
			continue
		}

		content, err := chatprompt.MarshalParts(kept)
		if err != nil {
			out = append(out, row)
			continue
		}
		row.Content = content
		row.ContentVersion = chatprompt.CurrentContentVersion
		out = append(out, row)
	}
	return out, stats
}

func hasOpenAIReasoningState(part codersdk.ChatMessagePart) bool {
	if len(part.ProviderMetadata) == 0 {
		return false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(part.ProviderMetadata, &raw); err != nil {
		return false
	}
	options, err := fantasy.UnmarshalProviderOptions(raw)
	return err == nil && chatsanitize.HasOpenAIReasoningState(options)
}

func (server *Server) sanitizeForeignProviderStateRows(
	ctx context.Context,
	logger slog.Logger,
	rows []database.ChatMessage,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
) []database.ChatMessage {
	targetCfg, targetProvider, err := server.resolveModelConfigAndNormalizedProvider(ctx, ownerID, modelConfigID)
	if err != nil || targetProvider == "" {
		logger.Debug(ctx, "skipping provider-switch sanitization: target provider unresolved",
			slog.F("model_config_id", modelConfigID),
			slog.Error(err),
		)
		return rows
	}
	targetIdentity := modelConfigProviderIdentity(targetCfg, targetProvider)

	cache := make(map[uuid.UUID]string)
	originProvider := func(id uuid.NullUUID) (string, bool) {
		if !id.Valid {
			return "", false
		}
		if identity, seen := cache[id.UUID]; seen {
			return identity, identity != ""
		}
		originCfg, provider, rErr := server.resolveModelConfigAndNormalizedProvider(ctx, ownerID, id.UUID)
		if rErr != nil {
			logger.Debug(ctx, "provider-switch sanitization: origin provider unresolved, treating as foreign",
				slog.F("model_config_id", id.UUID),
				slog.Error(rErr),
			)
			cache[id.UUID] = ""
			return "", false
		}
		identity := modelConfigProviderIdentity(originCfg, provider)
		cache[id.UUID] = identity
		return identity, identity != ""
	}

	sanitized, stats := stripForeignProviderStateRows(rows, targetIdentity, originProvider)
	if stats != (providerSwitchStripStats{}) {
		logger.Debug(ctx, "stripped foreign provider state from history",
			slog.F("phase", "provider_switch"),
			slog.F("target_provider_identity", targetIdentity),
			slog.F("removed_tool_calls", stats.RemovedToolCalls),
			slog.F("removed_tool_results", stats.RemovedToolResults),
			slog.F("removed_reasoning", stats.RemovedReasoning),
			slog.F("dropped_messages", stats.DroppedMessages),
		)
	}
	return sanitized
}
