package chatd

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// providerSwitchStripStats counts provider-executed tool history removed
// during a provider switch.
type providerSwitchStripStats struct {
	RemovedToolCalls   int
	RemovedToolResults int
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

// stripForeignProviderExecutedToolRows drops provider-executed tool blocks
// (calls and results) from assistant rows whose producing provider differs
// from targetIdentity. Rows with an unknown origin are treated as foreign
// (fail closed). Rows emptied by stripping are dropped; rows that fail to parse
// or re-marshal are kept unchanged.
//
// See modelConfigProviderIdentity for how identity is derived.
func stripForeignProviderExecutedToolRows(
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
		var removedCalls, removedResults int
		for _, part := range parts {
			switch {
			case part.Type == codersdk.ChatMessagePartTypeToolCall && part.ProviderExecuted:
				removedCalls++
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ProviderExecuted:
				removedResults++
			default:
				kept = append(kept, part)
			}
		}
		if removedCalls == 0 && removedResults == 0 {
			out = append(out, row)
			continue
		}
		stats.RemovedToolCalls += removedCalls
		stats.RemovedToolResults += removedResults
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

func (server *Server) sanitizeForeignProviderExecutedToolRows(
	ctx context.Context,
	logger slog.Logger,
	rows []database.ChatMessage,
	modelConfigID uuid.UUID,
) []database.ChatMessage {
	targetCfg, targetProvider, err := server.resolveModelConfigAndNormalizedProvider(ctx, modelConfigID)
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
		originCfg, provider, rErr := server.resolveModelConfigAndNormalizedProvider(ctx, id.UUID)
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

	sanitized, stats := stripForeignProviderExecutedToolRows(rows, targetIdentity, originProvider)
	if stats != (providerSwitchStripStats{}) {
		logger.Debug(ctx, "stripped foreign provider-executed tool history",
			slog.F("phase", "provider_switch"),
			slog.F("target_provider_identity", targetIdentity),
			slog.F("removed_tool_calls", stats.RemovedToolCalls),
			slog.F("removed_tool_results", stats.RemovedToolResults),
			slog.F("dropped_messages", stats.DroppedMessages),
		)
	}
	return sanitized
}
