package providers

import "github.com/coder/coder/v2/coderd/database"

// Supported lists the providers included in the generated model price book.
//
// openai-compat is excluded because it is a generic passthrough and does not
// identify an upstream vendor. Listed explicitly rather than derived from
// ai_provider_type so a new provider is opt-in.
var Supported = []database.AIProviderType{
	database.AIProviderTypeAnthropic,
	database.AIProviderTypeAzure,
	database.AIProviderTypeBedrock,
	database.AIProviderTypeCopilot,
	database.AIProviderTypeGoogle,
	database.AIProviderTypeOpenai,
	database.AIProviderTypeOpenrouter,
	database.AIProviderTypeVercel,
}

// SupportedStrings returns the generated price book providers as strings.
func SupportedStrings() []string {
	ids := make([]string, len(Supported))
	for i, provider := range Supported {
		ids[i] = string(provider)
	}
	return ids
}
