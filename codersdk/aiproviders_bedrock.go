package codersdk

// AIProviderSettingsTypeBedrock is the _type discriminator value for
// AIProviderBedrockSettings.
const AIProviderSettingsTypeBedrock = "bedrock"

// AIProviderBedrockSettingsVersion is the current schema version of
// AIProviderBedrockSettings.
const AIProviderBedrockSettingsVersion = 1

// AIProviderBedrockSettings configures providers that authenticate
// against AWS Bedrock. AccessKey and AccessKeySecret are write-only:
// servers strip them from GET and list responses.
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
	AccessKey string `json:"access_key,omitempty"`
	// AccessKeySecret is the AWS secret access key paired with
	// AccessKey. Write-only.
	AccessKeySecret string `json:"access_key_secret,omitempty"`
}

func (AIProviderBedrockSettings) settingsType() string {
	return AIProviderSettingsTypeBedrock
}

func (AIProviderBedrockSettings) settingsVersion() int {
	return AIProviderBedrockSettingsVersion
}
