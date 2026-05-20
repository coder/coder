package codersdk

// AIProviderSettingsTypeBedrock is the _type discriminator value for
// AIProviderBedrockSettings.
const AIProviderSettingsTypeBedrock = "bedrock"

// AIProviderBedrockSettingsVersion is the current schema version of
// AIProviderBedrockSettings.
const AIProviderBedrockSettingsVersion = 1

// AIProviderBedrockSettings configures providers that authenticate
// against AWS Bedrock. AccessKey and AccessKeySecret are write-only:
// servers strip them from GET and list responses. Both secret fields
// use a pointer so a PATCH can distinguish "leave untouched" (omitted)
// from "explicitly clear" (empty string), e.g. when migrating to
// IAM role-based authentication.
type AIProviderBedrockSettings struct {
	// Region is the AWS region used to construct the Bedrock endpoint
	// URL when BaseURL is not set on the parent provider.
	Region string `json:"region,omitempty"`
	// Model is the AWS Bedrock model identifier used for primary
	// requests.
	Model string `json:"model,omitempty"`
	// SmallFastModel is the AWS Bedrock model identifier used for
	// background tasks (e.g. Claude Code's haiku-class model).
	SmallFastModel string `json:"small_fast_model,omitempty"`
	// AccessKey is the AWS access key ID used to authenticate against
	// Bedrock. Write-only.
	AccessKey *string `json:"access_key,omitempty"`
	// AccessKeySecret is the AWS secret access key paired with
	// AccessKey. Write-only.
	AccessKeySecret *string `json:"access_key_secret,omitempty"`
}

// IsConfigured reports whether any load-bearing Bedrock field is set,
// indicating that the operator wants the provider to authenticate via
// AWS Bedrock rather than as a bearer-token Anthropic provider.
//
// Model and SmallFastModel are intentionally excluded: they have
// deployment-level defaults declared in codersdk/deployment.go, so
// they're always non-empty in a real deployment and cannot serve as
// a detection signal. Region and credentials have no defaults and
// therefore reliably indicate operator intent. Credentials alone are
// not required because Bedrock can also authenticate via the AWS
// environment (instance profile, AWS_PROFILE, IRSA, etc.).
func (b AIProviderBedrockSettings) IsConfigured() bool {
	if b.Region != "" {
		return true
	}
	if b.AccessKey != nil && *b.AccessKey != "" {
		return true
	}
	if b.AccessKeySecret != nil && *b.AccessKeySecret != "" {
		return true
	}
	return false
}

func (AIProviderBedrockSettings) settingsType() string {
	return AIProviderSettingsTypeBedrock
}

func (AIProviderBedrockSettings) settingsVersion() int {
	return AIProviderBedrockSettingsVersion
}
