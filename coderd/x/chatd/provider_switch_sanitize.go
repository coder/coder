package chatd

import (
	"bytes"
	"context"
	"encoding/json"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

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

const (
	reasoningCredentialDeployment = "deployment"
	reasoningCredentialUser       = "user"
)

// reasoningProvenance identifies where OpenAI reasoning state was produced.
// Reasoning item IDs only resolve on the issuing provider instance, and
// encrypted content can be rejected by a different model or by the
// organization behind a different credential. Epoch counts rejections of
// the chat's encrypted reasoning from the same provider, model, and
// credential, so reasoning replayed into a rejected call is never replayed
// again.
type reasoningProvenance struct {
	ProviderIdentity string
	Model            string
	Credential       string
	Epoch            int
}

// sameSource reports whether p and o name the same provider, model, and
// credential, regardless of epoch.
func (p reasoningProvenance) sameSource(o reasoningProvenance) bool {
	return p.ProviderIdentity == o.ProviderIdentity && p.Model == o.Model && p.Credential == o.Credential
}

// stampedReasoningProvenance returns the provenance recorded on part.
func stampedReasoningProvenance(part codersdk.ChatMessagePart) reasoningProvenance {
	return reasoningProvenance{
		ProviderIdentity: part.ProviderIdentity,
		Model:            part.ProviderModel,
		Credential:       part.ProviderCredential,
		Epoch:            part.ReasoningEpoch,
	}
}

// reasoningCredential names the credential an AI Gateway route sends.
func reasoningCredential(auth aiGatewayProviderAuth) string {
	if len(auth.Headers) > 0 {
		return reasoningCredentialUser
	}
	return reasoningCredentialDeployment
}

// unstampedReasoningProvenance is the provenance assumed for reasoning saved
// without a stamp: the row's model config, the target's credential, and the
// first epoch.
func unstampedReasoningProvenance(rowOrigin, target reasoningProvenance) reasoningProvenance {
	return reasoningProvenance{
		ProviderIdentity: rowOrigin.ProviderIdentity,
		Model:            rowOrigin.Model,
		Credential:       target.Credential,
	}
}

// currentReasoningEpoch returns the highest epoch recorded in rows for the
// provider, model, and credential of target. A rejected request only
// replayed reasoning from that source, so the rejection says nothing about
// reasoning from other models or credentials.
func currentReasoningEpoch(rows []database.ChatMessage, target reasoningProvenance) int {
	epoch := 0
	for _, row := range rows {
		if row.Role != database.ChatMessageRoleAssistant || !bytes.Contains(row.Content.RawMessage, []byte(`"reasoning_epoch"`)) {
			continue
		}
		parts, err := chatprompt.ParseContent(row)
		if err != nil {
			continue
		}
		for _, part := range parts {
			if source := stampedReasoningProvenance(part); source.sameSource(target) {
				epoch = max(epoch, source.Epoch)
			}
		}
	}
	return epoch
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
		if unstampedReasoningProvenance(origin, target) == target && !bytes.Contains(row.Content.RawMessage, []byte(`"provider_identity"`)) {
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
	source := unstampedReasoningProvenance(rowOrigin, target)
	if part.ProviderIdentity != "" {
		source = stampedReasoningProvenance(part)
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
) ([]database.ChatMessage, error) {
	if target.ProviderIdentity == "" {
		return rows, nil
	}

	// An unavailable origin config maps to the zero provenance, so its rows
	// are foreign.
	origins := make(map[uuid.UUID]reasoningProvenance)
	for _, row := range rows {
		id := row.ModelConfigID
		if row.Role != database.ChatMessageRoleAssistant || !id.Valid {
			continue
		}
		if _, seen := origins[id.UUID]; seen {
			continue
		}
		originCfg, provider, err := server.resolveModelConfigAndNormalizedProvider(ctx, ownerID, id.UUID)
		switch {
		case err == nil:
			origins[id.UUID] = reasoningProvenance{
				ProviderIdentity: modelConfigProviderIdentity(originCfg, provider),
				Model:            originCfg.Model,
			}
		case modelConfigUnavailable(err):
			origins[id.UUID] = reasoningProvenance{}
		default:
			return nil, xerrors.Errorf("resolve origin model config %s: %w", id.UUID, err)
		}
	}
	originOf := func(id uuid.NullUUID) reasoningProvenance {
		return origins[id.UUID]
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
	return sanitized, nil
}
