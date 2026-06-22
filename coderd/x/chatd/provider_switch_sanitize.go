package chatd

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// providerSwitchStripStats counts the provider-executed tool history removed
// when sanitizing a prompt for a model-provider switch.
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

// stripForeignProviderExecutedToolRows removes provider-executed tool blocks
// (both calls and results) from assistant history rows whose producing provider
// differs from targetIdentity. Provider-executed tool blocks are only valid for
// the provider that produced them: a provider sharing another's wire format can
// still reject them (e.g. Bedrock rejects Anthropic web_search_tool_result with
// HTTP 400), so switching providers mid-chat must drop the foreign blocks.
//
// targetIdentity is a provider identity string (see modelConfigProviderIdentity):
// the AIProvider UUID when available, or the normalized provider type for legacy
// configs. originProvider resolves a row's ModelConfigID to the same identity
// shape; ok is false when the origin cannot be determined, in which case the row
// is treated as foreign (fail closed). Rows from the target provider,
// non-assistant rows, rows with no provider-executed parts, and rows that fail
// to parse or re-marshal are returned unchanged. Rows emptied by stripping are
// dropped.
//
// Provenance is the provider instance (AIProviderID), not the normalized type,
// so two providers of the same type but different instances are correctly
// distinguished. This also stays correct when requests route through aibridged,
// which serializes both Anthropic and Bedrock as the Anthropic wire format.
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
		// Provider-executed blocks that must be replayed live on assistant rows.
		// Provider-executed results orphaned onto tool rows are dropped during
		// prompt conversion, so they never reach the provider.
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
			// Leave unparsable rows untouched; the converter handles them.
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
		if len(kept) == 0 {
			stats.RemovedToolCalls += removedCalls
			stats.RemovedToolResults += removedResults
			stats.DroppedMessages++
			continue
		}

		content, err := chatprompt.MarshalParts(kept)
		if err != nil {
			// Keep the original row rather than corrupting history.
			out = append(out, row)
			continue
		}
		row.Content = content
		row.ContentVersion = chatprompt.ContentVersionV1
		stats.RemovedToolCalls += removedCalls
		stats.RemovedToolResults += removedResults
		out = append(out, row)
	}
	return out, stats
}

// sanitizeForeignProviderExecutedToolRows strips provider-executed tool history
// produced by a provider other than the one targeted by modelConfigID. It
// resolves each row's provider via the model config cache and logs a single
// summary when anything is removed.
func (server *Server) sanitizeForeignProviderExecutedToolRows(
	ctx context.Context,
	logger slog.Logger,
	rows []database.ChatMessage,
	modelConfigID uuid.UUID,
) []database.ChatMessage {
	targetCfg, targetProvider, err := server.resolveModelConfigAndNormalizedProvider(ctx, modelConfigID)
	if err != nil || targetProvider == "" {
		// Without a known target provider we cannot classify history; leave it.
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
			// Unresolvable origin (e.g. a since-disabled or deleted config) is
			// treated as foreign so we fail closed rather than replay blocks the
			// target may reject.
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
		logger.Warn(ctx, "stripped foreign provider-executed tool history",
			slog.F("phase", "provider_switch"),
			slog.F("target_provider_identity", targetIdentity),
			slog.F("removed_tool_calls", stats.RemovedToolCalls),
			slog.F("removed_tool_results", stats.RemovedToolResults),
			slog.F("dropped_messages", stats.DroppedMessages),
		)
	}
	return sanitized
}
