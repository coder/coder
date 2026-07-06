package codersdk

// AIProviderSettingsTypeWIF is the _type discriminator value for
// AIProviderWIFSettings.
const AIProviderSettingsTypeWIF = "wif"

// AIProviderWIFSettingsVersion is the current schema version of
// AIProviderWIFSettings.
const AIProviderWIFSettingsVersion = 1

// AIProviderWIFSettings configures providers that authenticate via
// Anthropic Workload Identity Federation. The gateway exchanges an
// OIDC identity token for a short-lived Anthropic access token
// instead of using static API keys.
type AIProviderWIFSettings struct {
	// FederationRuleID is the tagged ID (fdrl_...) of the Anthropic
	// federation rule governing this exchange. Required.
	FederationRuleID string `json:"federation_rule_id"`
	// OrganizationID is the UUID of the Anthropic organization.
	// Required.
	OrganizationID string `json:"organization_id"`
	// IdentityTokenFile is the path to a file containing the OIDC
	// identity token (JWT). The file is re-read on every exchange.
	// Required.
	IdentityTokenFile string `json:"identity_token_file"`
	// ServiceAccountID is the optional svac_... tagged ID for
	// target_type=SERVICE_ACCOUNT federation rules.
	ServiceAccountID string `json:"service_account_id,omitempty"`
	// WorkspaceID is the optional wrkspc_... tagged ID or "default".
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// IsConfigured reports whether the minimum required WIF fields are set.
func (w AIProviderWIFSettings) IsConfigured() bool {
	return w.FederationRuleID != "" && w.OrganizationID != "" && w.IdentityTokenFile != ""
}

func (AIProviderWIFSettings) settingsType() string {
	return AIProviderSettingsTypeWIF
}

func (AIProviderWIFSettings) settingsVersion() int {
	return AIProviderWIFSettingsVersion
}
