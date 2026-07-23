package codersdk

// AIProviderSettingsTypeBedrock is the _type discriminator value for
// AIProviderBedrockSettings.
const AIProviderSettingsTypeBedrock = "bedrock"

// AIProviderBedrockSettingsVersion is the current schema version of
// AIProviderBedrockSettings.
const AIProviderBedrockSettingsVersion = 1

// AIProviderBedrockProtocol selects which AWS Bedrock wire protocol a provider
// targets.
type AIProviderBedrockProtocol string

const (
	// AIProviderBedrockProtocolInvokeModel is the legacy InvokeModel protocol
	// (bedrock-runtime.{region}.amazonaws.com), which translates the native
	// Messages request into Bedrock's InvokeModel format. It is the default
	// for the zero value.
	AIProviderBedrockProtocolInvokeModel AIProviderBedrockProtocol = "invoke-model"
	// AIProviderBedrockProtocolMantle is the mantle protocol
	// (bedrock-mantle.{region}.api.aws/anthropic/v1/messages). It is a
	// passthrough: the gateway forwards the native Messages request body
	// unchanged and only applies AWS SigV4 signing (service bedrock-mantle).
	AIProviderBedrockProtocolMantle AIProviderBedrockProtocol = "mantle"
)

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
	// RoleARN, when set, is the IAM role assumed via STS before calling
	// Bedrock. The base identity (static keys or the AWS environment, e.g.
	// IRSA / EKS Pod Identity / EC2 Instance Profile) signs the AssumeRole
	// call, and the resulting temporary credentials sign Bedrock requests.
	RoleARN string `json:"role_arn,omitempty"`
	// ExternalID is the STS external ID sent on the AssumeRole call when
	// RoleARN is set. The server generates and owns it: create and update
	// reject any client-supplied value that differs from the stored one (an
	// update may echo the stored value back).
	ExternalID string `json:"external_id,omitempty"`
	// Protocol selects the Bedrock wire protocol. An empty value resolves to
	// AIProviderBedrockProtocolInvokeModel, so existing rows keep the legacy
	// behavior.
	Protocol AIProviderBedrockProtocol `json:"protocol,omitempty"`
}

// ResolvedProtocol returns the configured protocol, mapping the empty value to
// the legacy InvokeModel protocol.
func (b AIProviderBedrockSettings) ResolvedProtocol() AIProviderBedrockProtocol {
	if b.Protocol == "" {
		return AIProviderBedrockProtocolInvokeModel
	}
	return b.Protocol
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
	if b.RoleARN != "" {
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

// NewAIProviderBedrockSettings builds an AIProviderBedrockSettings,
// promoting non-empty credential strings to pointers so callers don't
// have to repeat the "set field iff non-empty" boilerplate. Empty
// credentials are left nil, matching the PATCH-omit semantics of the
// pointer-typed fields.
func NewAIProviderBedrockSettings(region, accessKey, accessKeySecret, model, smallFastModel string) AIProviderBedrockSettings {
	s := AIProviderBedrockSettings{
		Region:         region,
		Model:          model,
		SmallFastModel: smallFastModel,
	}
	if accessKey != "" {
		s.AccessKey = &accessKey
	}
	if accessKeySecret != "" {
		s.AccessKeySecret = &accessKeySecret
	}
	return s
}

// IsBedrockConfigured reports whether the combination of the parent
// provider's BaseURL and AIProviderBedrockSettings indicates a Bedrock
// provider. BaseURL alone (e.g. a custom VPC or FIPS endpoint with
// credentials resolved via the AWS environment) is sufficient.
//
// Use this rather than AIProviderBedrockSettings.IsConfigured() when
// BaseURL is available; the seed, the runtime config builder, and the
// legacy validator must all agree on what counts as a Bedrock provider.
func IsBedrockConfigured(baseURL string, b AIProviderBedrockSettings) bool {
	return baseURL != "" || b.IsConfigured()
}

func (AIProviderBedrockSettings) settingsType() string {
	return AIProviderSettingsTypeBedrock
}

func (AIProviderBedrockSettings) settingsVersion() int {
	return AIProviderBedrockSettingsVersion
}
