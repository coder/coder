package chatsanitize

import (
	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// HasOpenAIReasoningOptions reports whether provider options carry OpenAI
// Responses reasoning state that the provider can replay even when the
// reasoning has no visible summary text.
func HasOpenAIReasoningOptions(options fantasy.ProviderOptions) bool {
	metadata := fantasyopenai.GetReasoningMetadata(options)
	return metadata != nil && metadata.ItemID != ""
}
