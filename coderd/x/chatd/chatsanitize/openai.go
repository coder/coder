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
