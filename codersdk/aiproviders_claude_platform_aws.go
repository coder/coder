package codersdk

// AIProviderSettingsTypeClaudePlatformAWS is the _type discriminator value for
// AIProviderClaudePlatformAWSSettings.
const AIProviderSettingsTypeClaudePlatformAWS = "claude_platform_aws"

// AIProviderClaudePlatformAWSSettingsVersion is the current schema version of
// AIProviderClaudePlatformAWSSettings.
const AIProviderClaudePlatformAWSSettingsVersion = 1

// AIProviderClaudePlatformAWSSettings configures providers that authenticate
// against Claude Platform for AWS: Anthropic's native Messages API hosted on
// AWS. It speaks the standard Messages wire format with standard Anthropic
// model IDs, so it is an authentication and routing variant of
// AIProviderTypeAnthropic rather than a provider type of its own.
// Requests use a client or provider API key when available, otherwise the
// gateway signs with its ambient AWS credentials.
type AIProviderClaudePlatformAWSSettings struct {
	// Region selects the default regional endpoint and SigV4 signing scope.
	// Required even when BaseURL points at a proxy.
	Region string `json:"region"`
	// WorkspaceID is sent as the anthropic-workspace-id header on every
	// request. Required regardless of the credential used.
	WorkspaceID string `json:"workspace_id"`
}

// IsConfigured reports whether the settings carry the fields required to route
// a provider at Claude Platform for AWS.
func (c AIProviderClaudePlatformAWSSettings) IsConfigured() bool {
	return c.Region != "" && c.WorkspaceID != ""
}

func (AIProviderClaudePlatformAWSSettings) settingsType() string {
	return AIProviderSettingsTypeClaudePlatformAWS
}

func (AIProviderClaudePlatformAWSSettings) settingsVersion() int {
	return AIProviderClaudePlatformAWSSettingsVersion
}
