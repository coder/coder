package providers

import "github.com/coder/coder/v2/coderd/database"

// Supported lists the provider IDs a model price may be set for.
//
// openai-compat is excluded: it is a generic passthrough, so the upstream
// vendor is unknown and a price cannot be attributed to it. Listed explicitly
// rather than derived from ai_provider_type so a new provider is opt-in.
var Supported = []string{
	string(database.AIProviderTypeAnthropic),
	string(database.AIProviderTypeAzure),
	string(database.AIProviderTypeBedrock),
	string(database.AIProviderTypeCopilot),
	string(database.AIProviderTypeGoogle),
	string(database.AIProviderTypeOpenai),
	string(database.AIProviderTypeOpenrouter),
	string(database.AIProviderTypeVercel),
}
