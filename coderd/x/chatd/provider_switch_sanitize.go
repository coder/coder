package chatd

import (
	"bytes"
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

// reasoningProvenance identifies the provider instance and model that
// produced OpenAI reasoning state. Reasoning item IDs only resolve on the
// issuing provider instance, and encrypted content can be rejected by a
// different model.
type reasoningProvenance struct {
	ProviderIdentity string
	Model            string
}

// stripForeignProviderStateRows drops provider-executed tool blocks (calls and
// results) from assistant rows whose producing provider differs from the
// target's, and OpenAI reasoning state whose provenance differs from target.
// A reasoning part's recorded provenance takes precedence over its row's
// origin, since the row's model config may have been changed. Rows with an
// unknown origin are treated as foreign (fail closed). Rows emptied by
// stripping are dropped; rows that fail to parse or re-marshal are kept
// unchanged.
//
// See modelConfigProviderIdentity for how identity is derived.
func stripForeignProviderStateRows(
	rows []database.ChatMessage,
	target reasoningProvenance,
	originOf func(uuid.NullUUID) reasoningProvenance,
) ([]database.ChatMessage, providerSwitchStripStats) {
	var stats providerSwitchStripStats
	if target.ProviderIdentity == "" || len(rows) == 0 {
		return rows, stats
	}

	out := make([]database.ChatMessage, 0, len(rows))
	for _, row := range rows {
		if row.Role != database.ChatMessageRoleAssistant {
			out = append(out, row)
			continue
		}
		origin := originOf(row.ModelConfigID)
		nativeRow := origin.ProviderIdentity == target.ProviderIdentity
		// Only stamped reasoning parts can be foreign when the row origin matches.
		if origin == target && !bytes.Contains(row.Content.RawMessage, []byte(`"provider_identity"`)) {
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
			case !nativeRow && part.Type == codersdk.ChatMessagePartTypeToolCall && part.ProviderExecuted:
				removedCalls++
			case !nativeRow && part.Type == codersdk.ChatMessagePartTypeToolResult && part.ProviderExecuted:
				removedResults++
			case part.Type == codersdk.ChatMessagePartTypeReasoning && isForeignReasoning(part, origin, target):
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

func isForeignReasoning(part codersdk.ChatMessagePart, rowOrigin, target reasoningProvenance) bool {
	source := rowOrigin
	if part.ProviderIdentity != "" {
		source = reasoningProvenance{ProviderIdentity: part.ProviderIdentity, Model: part.ProviderModel}
	}
	return source != target && hasOpenAIReasoningState(part)
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
	target reasoningProvenance,
) []database.ChatMessage {
	cache := make(map[uuid.UUID]reasoningProvenance)
	originOf := func(id uuid.NullUUID) reasoningProvenance {
		if !id.Valid {
			return reasoningProvenance{}
		}
		if origin, seen := cache[id.UUID]; seen {
			return origin
		}
		originCfg, provider, rErr := server.resolveModelConfigAndNormalizedProvider(ctx, ownerID, id.UUID)
		if rErr != nil {
			logger.Debug(ctx, "provider-switch sanitization: origin provider unresolved, treating as foreign",
				slog.F("model_config_id", id.UUID),
				slog.Error(rErr),
			)
			cache[id.UUID] = reasoningProvenance{}
			return reasoningProvenance{}
		}
		origin := reasoningProvenance{
			ProviderIdentity: modelConfigProviderIdentity(originCfg, provider),
			Model:            originCfg.Model,
		}
		cache[id.UUID] = origin
		return origin
	}

	sanitized, stats := stripForeignProviderStateRows(rows, target, originOf)
	if stats != (providerSwitchStripStats{}) {
		logger.Debug(ctx, "stripped foreign provider state from history",
			slog.F("phase", "provider_switch"),
			slog.F("target_provider_identity", target.ProviderIdentity),
			slog.F("target_model", target.Model),
			slog.F("removed_tool_calls", stats.RemovedToolCalls),
			slog.F("removed_tool_results", stats.RemovedToolResults),
			slog.F("removed_reasoning", stats.RemovedReasoning),
			slog.F("dropped_messages", stats.DroppedMessages),
		)
	}
	return sanitized
}
