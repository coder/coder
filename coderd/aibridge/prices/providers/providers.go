package providers

import "github.com/coder/coder/v2/coderd/database"

// Supported lists the providers a model price may be set for.
//
// openai-compat is excluded: it is a generic passthrough, so the upstream
// vendor is unknown and a price cannot be attributed to it. Listed explicitly
// rather than derived from ai_provider_type so a new provider is opt-in.
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

// SupportedStrings returns the supported providers as plain strings.
func SupportedStrings() []string {
	ids := make([]string, len(Supported))
	for i, provider := range Supported {
		ids[i] = string(provider)
	}
	return ids
}
