package codersdk

// AIProviderSettingsTypeClaudePlatformAWS is the _type discriminator value for
// AIProviderClaudePlatformAWSSettings.
const AIProviderSettingsTypeClaudePlatformAWS = "claude_platform_aws"

// AIProviderClaudePlatformAWSSettingsVersion is the current schema version of
// AIProviderClaudePlatformAWSSettings.
const AIProviderClaudePlatformAWSSettingsVersion = 1

// AIProviderClaudePlatformAWSAuthMode selects how the gateway authenticates to
// Anthropic's AWS-hosted Messages API.
type AIProviderClaudePlatformAWSAuthMode string

const (
	// AIProviderClaudePlatformAWSAuthModeIAM signs requests with AWS SigV4
	// (service aws-external-anthropic).
	AIProviderClaudePlatformAWSAuthModeIAM AIProviderClaudePlatformAWSAuthMode = "iam"
	// AIProviderClaudePlatformAWSAuthModeAPIKey sends a workspace API key in
	// x-api-key, drawn from the provider's api_keys.
	AIProviderClaudePlatformAWSAuthModeAPIKey AIProviderClaudePlatformAWSAuthMode = "api_key"
)

// AIProviderClaudePlatformAWSSettings configures providers that authenticate
// against Claude Platform for AWS: Anthropic's native Messages API hosted on
// AWS. It speaks the standard Messages wire format with standard Anthropic
// model IDs, so it is an authentication and routing variant of
// AIProviderTypeAnthropic rather than a provider type of its own.
//
// AccessKey and AccessKeySecret are write-only: servers strip them from GET and
// list responses. Both use a pointer so a PATCH can distinguish "leave
// untouched" (omitted) from "explicitly clear" (empty string), e.g. when
// migrating from static keys to the ambient AWS credential chain.
type AIProviderClaudePlatformAWSSettings struct {
	// AuthMode selects SigV4 signing or a workspace API key. Required, and
	// explicit rather than inferred: the gateway prefers a configured api_keys
	// pool over signing, so an IAM provider that also carried keys would
	// silently authenticate with the keys instead of the role.
	AuthMode AIProviderClaudePlatformAWSAuthMode `json:"auth_mode"`
	// Region is the AWS region. It is required in both auth modes: it selects
	// the default regional endpoint, and SigV4 signatures are region-scoped, so
	// it must stay explicit even when BaseURL points at a proxy.
	Region string `json:"region"`
	// WorkspaceID is sent as the anthropic-workspace-id header on every
	// request. Required in both auth modes.
	WorkspaceID string `json:"workspace_id"`
	// AccessKey is the AWS access key ID used to sign requests. IAM mode only.
	// When unset, the ambient AWS credential chain (instance profile,
	// AWS_PROFILE, IRSA, etc.) resolves the base identity. Write-only.
	AccessKey *string `json:"access_key,omitempty"`
	// AccessKeySecret is the AWS secret access key paired with AccessKey. IAM
	// mode only. Write-only.
	AccessKeySecret *string `json:"access_key_secret,omitempty"`
	// RoleARN, when set, is the IAM role assumed via STS before signing. IAM
	// mode only. The base identity signs the AssumeRole call, and the resulting
	// temporary credentials sign Claude Platform requests.
	RoleARN string `json:"role_arn,omitempty"`
	// ExternalID is the STS external ID sent on the AssumeRole call when
	// RoleARN is set. The server generates and owns it: create and update
	// reject any client-supplied value that differs from the stored one (an
	// update may echo the stored value back).
	ExternalID string `json:"external_id,omitempty"`
}

// ResolvedAuthMode returns the configured authentication mode. Unlike Bedrock's
// protocol there is no legacy default to fall back to, so an empty value stays
// empty and validation rejects it.
func (c AIProviderClaudePlatformAWSSettings) ResolvedAuthMode() AIProviderClaudePlatformAWSAuthMode {
	return c.AuthMode
}

// IsConfigured reports whether the settings carry the fields required to route
// a provider at Claude Platform for AWS. Unlike Bedrock there is no field with
// a usable default, so all three required fields must be present.
func (c AIProviderClaudePlatformAWSSettings) IsConfigured() bool {
	return c.AuthMode != "" && c.Region != "" && c.WorkspaceID != ""
}

func (AIProviderClaudePlatformAWSSettings) settingsType() string {
	return AIProviderSettingsTypeClaudePlatformAWS
}

func (AIProviderClaudePlatformAWSSettings) settingsVersion() int {
	return AIProviderClaudePlatformAWSSettingsVersion
}
