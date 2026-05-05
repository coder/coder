package chatopenai

import (
	"maps"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
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
