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

// stripForeignProviderExecutedToolRows removes provider-executed tool blocks
// (both calls and results) from assistant history rows whose producing provider
// differs from targetProvider. Provider-executed tool blocks are only valid for
// the provider that produced them: a provider sharing another's wire format can
// still reject them (e.g. Bedrock rejects Anthropic web_search_tool_result with
// HTTP 400), so switching providers mid-chat must drop the foreign blocks.
//
// originProvider resolves a row's ModelConfigID to a normalized provider name;
// ok is false when the origin cannot be determined, in which case the row is
// treated as foreign (fail closed). Rows from the target provider, non-assistant
// rows, rows with no provider-executed parts, and rows that fail to parse or
// re-marshal are returned unchanged. Rows emptied by stripping are dropped.
//
// Provenance is the model config provider (derived from the AI provider type),
// not anything fantasy reports, so it stays correct when requests route through
// aibridged, which serializes both Anthropic and Bedrock as the Anthropic wire
// format.
func stripForeignProviderExecutedToolRows(
	rows []database.ChatMessage,
	targetProvider string,
	originProvider func(uuid.NullUUID) (string, bool),
) ([]database.ChatMessage, providerSwitchStripStats) {
	var stats providerSwitchStripStats
	if targetProvider == "" || len(rows) == 0 {
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
		if origin, ok := originProvider(row.ModelConfigID); ok && origin == targetProvider {
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
	_, target, err := server.resolveModelConfigAndNormalizedProvider(ctx, modelConfigID)
	if err != nil || target == "" {
		// Without a known target provider we cannot classify history; leave it.
		logger.Debug(ctx, "skipping provider-switch sanitization: target provider unresolved",
			slog.F("model_config_id", modelConfigID),
			slog.Error(err),
		)
		return rows
	}

	cache := make(map[uuid.UUID]string)
	originProvider := func(id uuid.NullUUID) (string, bool) {
		if !id.Valid {
			return "", false
		}
		if provider, seen := cache[id.UUID]; seen {
			return provider, provider != ""
		}
		_, provider, rErr := server.resolveModelConfigAndNormalizedProvider(ctx, id.UUID)
		if rErr != nil {
			provider = ""
		}
		cache[id.UUID] = provider
		return provider, provider != ""
	}

	sanitized, stats := stripForeignProviderExecutedToolRows(rows, target, originProvider)
	if stats != (providerSwitchStripStats{}) {
		logger.Warn(ctx, "stripped foreign provider-executed tool history",
			slog.F("phase", "provider_switch"),
			slog.F("target_provider", target),
			slog.F("removed_tool_calls", stats.RemovedToolCalls),
			slog.F("removed_tool_results", stats.RemovedToolResults),
			slog.F("dropped_messages", stats.DroppedMessages),
		)
	}
	return sanitized
}
