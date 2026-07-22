package chatd

import (
	"context"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
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
	ProtectedRows      int
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
// When protectSignedReasoningRun is non-nil and reports signed reasoning in
// the trailing run of consecutive assistant rows, no row in that run is
// stripped: those rows serialize into the latest assistant message on the
// wire, and Anthropic rejects any request whose latest assistant message
// differs from the original response. A row in that run that fails to parse
// also protects the run, since its content cannot be ruled out as signed.
//
// See modelConfigProviderIdentity for how identity is derived.
func stripForeignProviderExecutedToolRows(
	rows []database.ChatMessage,
	targetIdentity string,
	originIdentity func(uuid.NullUUID) (string, bool),
	protectSignedReasoningRun func([]codersdk.ChatMessagePart) bool,
) ([]database.ChatMessage, providerSwitchStripStats) {
	var stats providerSwitchStripStats
	if targetIdentity == "" || len(rows) == 0 {
		return rows, stats
	}

	runStart, runEnd := latestAssistantRunBounds(rows)
	protectedRun := false
	if protectSignedReasoningRun != nil && runEnd >= 0 {
		for i := runStart; i <= runEnd; i++ {
			parts, err := chatprompt.ParseContent(rows[i])
			if err != nil || protectSignedReasoningRun(parts) {
				protectedRun = true
				break
			}
		}
	}

	out := make([]database.ChatMessage, 0, len(rows))
	for rowIndex, row := range rows {
		if row.Role != database.ChatMessageRoleAssistant {
			out = append(out, row)
			continue
		}
		if origin, ok := originIdentity(row.ModelConfigID); ok && origin == targetIdentity {
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
		if protectedRun && rowIndex >= runStart && rowIndex <= runEnd {
			stats.ProtectedRows++
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

// originProviderIdentity derives the producing provider identity for a
// historical assistant row's config. Unlike target resolution, it ignores
// enabled/deleted state: a soft-deleted config still records which provider
// instance produced the row. Legacy configs without a provider FK stay
// unresolved (fail closed).
func originProviderIdentity(cfg database.ChatModelConfig) string {
	if cfg.AIProviderID.Valid {
		return cfg.AIProviderID.UUID.String()
	}
	return ""
}

// latestAssistantRunBounds returns the inclusive bounds of the trailing run
// of consecutive assistant rows containing the last assistant row, or
// (-1, -1) when no assistant row exists. Consecutive assistant rows merge
// into a single assistant message when serialized for Anthropic, so the
// whole run forms the latest assistant message on the wire.
func latestAssistantRunBounds(rows []database.ChatMessage) (start, end int) {
	runEnd := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Role == database.ChatMessageRoleAssistant {
			runEnd = i
			break
		}
	}
	if runEnd < 0 {
		return -1, -1
	}
	runStart := runEnd
	for runStart > 0 && rows[runStart-1].Role == database.ChatMessageRoleAssistant {
		runStart--
	}
	return runStart, runEnd
}

// signedReasoningRunProtection returns the latest-assistant-run protection
// predicate for Anthropic wire targets and nil for every other provider,
// where Anthropic replay validation does not apply.
func signedReasoningRunProtection(wireProvider string, logger slog.Logger) func([]codersdk.ChatMessagePart) bool {
	if wireProvider != fantasyanthropic.Name {
		return nil
	}
	return func(parts []codersdk.ChatMessagePart) bool {
		return chatprompt.PartsHaveAnthropicSignedReasoning(logger, parts)
	}
}

func (server *Server) sanitizeForeignProviderExecutedToolRows(
	ctx context.Context,
	logger slog.Logger,
	rows []database.ChatMessage,
	modelConfigID uuid.UUID,
	wireProvider string,
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
	originIdentity := func(id uuid.NullUUID) (string, bool) {
		if !id.Valid {
			return "", false
		}
		if identity, seen := cache[id.UUID]; seen {
			return identity, identity != ""
		}
		originCfg, rErr := server.db.GetChatModelConfigByIDIncludeDeleted(ctx, id.UUID)
		if rErr != nil {
			logger.Debug(ctx, "provider-switch sanitization: origin provider unresolved, treating as foreign",
				slog.F("model_config_id", id.UUID),
				slog.Error(rErr),
			)
			cache[id.UUID] = ""
			return "", false
		}
		identity := originProviderIdentity(originCfg)
		cache[id.UUID] = identity
		return identity, identity != ""
	}

	protectSignedReasoningRun := signedReasoningRunProtection(wireProvider, logger)

	sanitized, stats := stripForeignProviderExecutedToolRows(rows, targetIdentity, originIdentity, protectSignedReasoningRun)
	if stats.ProtectedRows > 0 {
		logger.Warn(ctx, "kept foreign provider-executed tool history in latest signed assistant run",
			slog.F("phase", "provider_switch"),
			slog.F("target_provider_identity", targetIdentity),
			slog.F("protected_rows", stats.ProtectedRows),
		)
	}
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
