package chatopenai

import (
	"maps"
	"slices"
	"strings"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// ChainModeInfo holds the information needed to determine whether a follow-up turn
// can use OpenAI's previous_response_id chaining instead of replaying full
// conversation history.
type ChainModeInfo struct {
	// previousResponseID is the provider response ID from the last assistant
	// message, if any.
	previousResponseID string
	// modelConfigID is the model configuration used to produce the assistant
	// message referenced by previousResponseID.
	modelConfigID uuid.UUID
	// contributingTrailingUserCount counts the trailing user messages that
	// materially change the provider input.
	contributingTrailingUserCount int
	// hasUnresolvedLocalToolCalls is true when previousResponseID points at an
	// assistant message with pending local tool calls.
	hasUnresolvedLocalToolCalls bool
	// providerMissingToolResults is true when the assistant message has local
	// tool calls with local results, but no follow-up assistant message exists to
	// confirm the results were sent back to the provider. This happens when
	// StopAfterTool terminates a turn before the results are round-tripped.
	providerMissingToolResults bool
}

// PreviousResponseID returns the provider response ID from the last assistant
// message, if any.
func (c ChainModeInfo) PreviousResponseID() string {
	return c.previousResponseID
}

// ModelConfigID returns the model configuration used to produce the assistant
// message referenced by PreviousResponseID.
func (c ChainModeInfo) ModelConfigID() uuid.UUID {
	return c.modelConfigID
}

// ContributingTrailingUserCount returns the number of trailing user messages
// that materially change the provider input.
func (c ChainModeInfo) ContributingTrailingUserCount() int {
	return c.contributingTrailingUserCount
}

// HasUnresolvedLocalToolCalls reports whether PreviousResponseID points at an
// assistant message with pending local tool calls.
func (c ChainModeInfo) HasUnresolvedLocalToolCalls() bool {
	return c.hasUnresolvedLocalToolCalls
}

// ProviderMissingToolResults reports whether PreviousResponseID points at an
// assistant message with local tool results, but no follow-up assistant message
// confirms those tool results were sent to the provider (not just persisted
// locally).
func (c ChainModeInfo) ProviderMissingToolResults() bool {
	return c.providerMissingToolResults
}

// IsResponsesStoreEnabled checks if the OpenAI Responses provider options are
// present and have Store set to true. When true, the provider stores
// conversation history server-side, enabling follow-up chaining via
// PreviousResponseID.
func IsResponsesStoreEnabled(opts fantasy.ProviderOptions) bool {
	if opts == nil {
		return false
	}
	raw, ok := opts[fantasyopenai.Name]
	if !ok {
		return false
	}
	respOpts, ok := raw.(*fantasyopenai.ResponsesProviderOptions)
	if !ok || respOpts == nil {
		return false
	}
	return respOpts.Store != nil && *respOpts.Store
}

// WithPreviousResponseID shallow-clones the provider options map and the OpenAI
// Responses entry, setting PreviousResponseID on the clone. The original map
// and entry are not mutated.
func WithPreviousResponseID(
	opts fantasy.ProviderOptions,
	previousResponseID string,
) fantasy.ProviderOptions {
	cloned := maps.Clone(opts)
	if cloned == nil {
		cloned = fantasy.ProviderOptions{}
	}
	if raw, ok := cloned[fantasyopenai.Name]; ok {
		if respOpts, ok := raw.(*fantasyopenai.ResponsesProviderOptions); ok && respOpts != nil {
			clone := *respOpts
			clone.PreviousResponseID = &previousResponseID
			cloned[fantasyopenai.Name] = &clone
		}
	}
	return cloned
}

// HasPreviousResponseID checks whether the provider options contain an OpenAI
// Responses entry with a non-empty PreviousResponseID.
func HasPreviousResponseID(providerOptions fantasy.ProviderOptions) bool {
	if len(providerOptions) == 0 {
		return false
	}

	entry, ok := providerOptions[fantasyopenai.Name]
	if !ok {
		return false
	}
	options, ok := entry.(*fantasyopenai.ResponsesProviderOptions)
	return ok && options != nil && options.PreviousResponseID != nil &&
		*options.PreviousResponseID != ""
}

// ClearPreviousResponseID returns a clone of providerOptions with
// PreviousResponseID cleared on the OpenAI Responses options. The original
// providerOptions is not modified.
func ClearPreviousResponseID(providerOptions fantasy.ProviderOptions) fantasy.ProviderOptions {
	cloned := maps.Clone(providerOptions)
	if cloned == nil {
		return fantasy.ProviderOptions{}
	}

	entry, ok := cloned[fantasyopenai.Name]
	if !ok {
		return cloned
	}
	options, ok := entry.(*fantasyopenai.ResponsesProviderOptions)
	if !ok || options == nil {
		return cloned
	}
	optionsClone := *options
	optionsClone.PreviousResponseID = nil
	cloned[fantasyopenai.Name] = &optionsClone
	return cloned
}

// extractResponseID extracts the OpenAI Responses API response ID from provider
// metadata. Returns an empty string if no OpenAI Responses metadata is present.
func extractResponseID(metadata fantasy.ProviderMetadata) string {
	if len(metadata) == 0 {
		return ""
	}

	entry, ok := metadata[fantasyopenai.Name]
	if !ok {
		return ""
	}
	providerMetadata, ok := entry.(*fantasyopenai.ResponsesProviderMetadata)
	if !ok || providerMetadata == nil {
		return ""
	}
	return providerMetadata.ResponseID
}

// ExtractResponseIDIfStored returns the OpenAI response ID only when the
// provider options indicate store=true. Response IDs from store=false turns are
// not persisted server-side and cannot be used for chaining.
func ExtractResponseIDIfStored(
	providerOptions fantasy.ProviderOptions,
	metadata fantasy.ProviderMetadata,
) string {
	if !IsResponsesStoreEnabled(providerOptions) {
		return ""
	}

	return extractResponseID(metadata)
}

// ShouldActivateChainMode reports whether a follow-up turn can use
// previous_response_id instead of replaying history. It requires store=true, a
// matching model config, meaningful trailing user input, non-plan mode,
// complete local tool state, and confirmation that tool results were sent to
// the provider.
func ShouldActivateChainMode(
	providerOptions fantasy.ProviderOptions,
	info ChainModeInfo,
	modelConfigID uuid.UUID,
	isPlanModeTurn bool,
) bool {
	return IsResponsesStoreEnabled(providerOptions) &&
		info.previousResponseID != "" &&
		info.contributingTrailingUserCount > 0 &&
		info.modelConfigID == modelConfigID &&
		!isPlanModeTurn &&
		!info.hasUnresolvedLocalToolCalls &&
		!info.providerMissingToolResults
}

// ResolveChainMode scans DB messages from the end to inspect the current
// trailing user turn and detect whether the immediately preceding assistant/tool
// block can chain from a provider response ID.
func ResolveChainMode(messages []database.ChatMessage) ChainModeInfo {
	var info ChainModeInfo
	i := len(messages) - 1
	for ; i >= 0; i-- {
		if messages[i].Role != database.ChatMessageRoleUser {
			break
		}
		if userMessageContributesToChainMode(messages[i]) {
			info.contributingTrailingUserCount++
		}
	}
	for ; i >= 0; i-- {
		switch messages[i].Role {
		case database.ChatMessageRoleAssistant:
			if messages[i].ProviderResponseID.Valid &&
				messages[i].ProviderResponseID.String != "" {
				info.previousResponseID = messages[i].ProviderResponseID.String
				if messages[i].ModelConfigID.Valid {
					info.modelConfigID = messages[i].ModelConfigID.UUID
				}
				info.hasUnresolvedLocalToolCalls = assistantHasUnresolvedLocalToolCalls(messages, i)
				if !info.hasUnresolvedLocalToolCalls {
					info.providerMissingToolResults = providerHasMissingToolResults(messages, i)
				}
				return info
			}
			return info
		case database.ChatMessageRoleTool:
			continue
		default:
			return info
		}
	}
	return info
}

// FilterPromptForChainMode keeps only system messages and the trailing user
// messages that still contribute model-visible content to the current turn.
// Assistant and tool messages are dropped because the provider already has
// them via the previous_response_id chain.
func FilterPromptForChainMode(
	prompt []fantasy.Message,
	info ChainModeInfo,
) []fantasy.Message {
	if info.contributingTrailingUserCount <= 0 {
		return prompt
	}

	totalUsers := 0
	for _, msg := range prompt {
		if msg.Role == "user" {
			totalUsers++
		}
	}

	// Prompt construction already drops user turns with no model-visible
	// content, such as skill-only sentinel messages. That means the user
	// count here stays aligned with contributingTrailingUserCount even
	// when non-contributing DB turns are interleaved in the trailing
	// block.
	usersToSkip := totalUsers - info.contributingTrailingUserCount
	if usersToSkip < 0 {
		usersToSkip = 0
	}

	filtered := make([]fantasy.Message, 0, len(prompt))
	usersSeen := 0
	for _, msg := range prompt {
		switch msg.Role {
		case "system":
			filtered = append(filtered, msg)
		case "user":
			usersSeen++
			if usersSeen > usersToSkip {
				filtered = append(filtered, msg)
			}
		}
	}

	return filtered
}

func userMessageContributesToChainMode(msg database.ChatMessage) bool {
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return false
	}
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeText,
			codersdk.ChatMessagePartTypeReasoning:
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		case codersdk.ChatMessagePartTypeFile,
			codersdk.ChatMessagePartTypeFileReference:
			return true
		case codersdk.ChatMessagePartTypeContextFile:
			if part.ContextFileContent != "" {
				return true
			}
		}
	}
	return false
}

// assistantHasUnresolvedLocalToolCalls reports whether the assistant message
// at assistantIdx contains local tool calls that lack matching tool results. It
// returns true when content parsing fails because full-history replay is safer
// than chaining from state that cannot be inspected.
func assistantHasUnresolvedLocalToolCalls(
	messages []database.ChatMessage,
	assistantIdx int,
) bool {
	if assistantIdx < 0 || assistantIdx >= len(messages) {
		return false
	}

	parts, err := chatprompt.ParseContent(messages[assistantIdx])
	if err != nil {
		// Use full replay when persisted assistant content cannot be parsed.
		return true
	}

	localCallIDs := make(map[string]struct{})
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeToolCall ||
			part.ProviderExecuted {
			continue
		}
		localCallIDs[part.ToolCallID] = struct{}{}
	}
	if len(localCallIDs) == 0 {
		return false
	}

	resolvedCallIDs := make(map[string]struct{})
	for i := assistantIdx + 1; i < len(messages); i++ {
		if messages[i].Role != database.ChatMessageRoleTool {
			break
		}
		parts, err := chatprompt.ParseContent(messages[i])
		if err != nil {
			// Use full replay when persisted tool content cannot be parsed.
			return true
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult {
				continue
			}
			if _, ok := localCallIDs[part.ToolCallID]; ok {
				resolvedCallIDs[part.ToolCallID] = struct{}{}
			}
		}
	}

	return len(resolvedCallIDs) != len(localCallIDs)
}

// providerHasMissingToolResults reports whether the assistant message at
// assistantIdx has local tool calls whose results exist in the database but
// were never sent back to the provider. This is detected by the absence of a
// follow-up assistant message after the tool results. In normal flow the LLM
// processes tool results and produces a follow-up response, but StopAfterTool
// skips that round-trip.
func providerHasMissingToolResults(
	messages []database.ChatMessage,
	assistantIdx int,
) bool {
	if assistantIdx < 0 || assistantIdx >= len(messages) {
		return false
	}

	parts, err := chatprompt.ParseContent(messages[assistantIdx])
	if err != nil {
		// Parsing errors are already handled by
		// assistantHasUnresolvedLocalToolCalls.
		return false
	}

	if !slices.ContainsFunc(parts, func(p codersdk.ChatMessagePart) bool {
		return p.Type == codersdk.ChatMessagePartTypeToolCall && !p.ProviderExecuted
	}) {
		return false
	}

	// Scan forward past tool messages. If the first non-tool message is not an
	// assistant, the tool results were never round-tripped to the provider.
	for i := assistantIdx + 1; i < len(messages); i++ {
		switch messages[i].Role {
		case database.ChatMessageRoleTool:
			continue
		case database.ChatMessageRoleAssistant:
			// A follow-up assistant exists, so results were sent.
			return false
		default:
			// User or system message with no follow-up assistant.
			return true
		}
	}

	// Reached end of messages without a follow-up assistant.
	return true
}
