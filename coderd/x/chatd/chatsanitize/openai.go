package chatsanitize

import (
	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// HasOpenAIReasoningOptions reports whether options carry finalized encrypted
// reasoning that can be replayed without server-side storage.
func HasOpenAIReasoningOptions(options fantasy.ProviderOptions) bool {
	metadata := fantasyopenai.GetReasoningMetadata(options)
	return metadata != nil && metadata.ItemID != "" && metadata.Finalized &&
		metadata.EncryptedContent != nil && *metadata.EncryptedContent != ""
}

// HasOpenAIReasoningState reports whether options carry an OpenAI reasoning
// item. Its item ID and encrypted content only resolve on the provider
// instance that issued them.
func HasOpenAIReasoningState(options fantasy.ProviderOptions) bool {
	metadata := fantasyopenai.GetReasoningMetadata(options)
	return metadata != nil && metadata.ItemID != ""
}
