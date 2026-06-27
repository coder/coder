package codersdk

// AIProviderSettingsTypeClaudePlatformAWS is the _type discriminator value for
// AIProviderClaudePlatformAWSSettings.
const AIProviderSettingsTypeClaudePlatformAWS = "claude-platform-aws"

// AIProviderClaudePlatformAWSSettingsVersion is the current schema version of
// AIProviderClaudePlatformAWSSettings.
const AIProviderClaudePlatformAWSSettingsVersion = 1

// AIProviderClaudePlatformAWSSettings configures providers that authenticate
// against Claude Platform for AWS (https://aws-external-anthropic.<region>.api.aws).
//
// Unlike Amazon Bedrock, Claude Platform for AWS is Anthropic's native Messages
// API hosted on AWS infrastructure: it uses standard Anthropic model IDs, signs
// requests with the SigV4 service name "aws-external-anthropic", and requires
// the anthropic-workspace-id header on every request. Authentication is either
// AWS SigV4 (static keys, the AWS default credential chain, or an assumed IAM
// role) or a workspace API key sent as x-api-key.
//
// AccessKey, AccessKeySecret, and APIKey are write-only: servers strip them
// from GET and list responses. They use pointers so a PATCH can distinguish
// "leave untouched" (omitted) from "explicitly clear" (empty string), e.g. when
// migrating from static credentials to an IAM role or API key.
type AIProviderClaudePlatformAWSSettings struct {
	// Region is the AWS region used both to construct the Claude Platform
	// endpoint (when BaseURL is not set on the parent provider) and as the
	// SigV4 signing region. It must match the region in the endpoint URL.
	Region string `json:"region,omitempty"`
	// WorkspaceID is the Claude Platform workspace ID sent in the
	// anthropic-workspace-id header on every request. Required.
	WorkspaceID string `json:"workspace_id,omitempty"`
	// AccessKey is the AWS access key ID used for SigV4 authentication.
	// Write-only.
	AccessKey *string `json:"access_key,omitempty"`
	// AccessKeySecret is the AWS secret access key paired with AccessKey.
	// Write-only.
	AccessKeySecret *string `json:"access_key_secret,omitempty"`
	// RoleARN, when set, is the IAM role assumed via STS before signing
	// requests. The base identity (static keys or the AWS default credential
	// chain, e.g. IRSA / EKS Pod Identity / EC2 Instance Profile) signs the
	// AssumeRole call, and the resulting temporary credentials sign requests.
	RoleARN string `json:"role_arn,omitempty"`
	// ExternalID is the STS external ID passed when assuming RoleARN. It
	// mitigates the confused-deputy problem for cross-account role assumption
	// and is only meaningful when RoleARN is set.
	ExternalID string `json:"external_id,omitempty"`
	// APIKey is a Claude Platform workspace API key sent as x-api-key. When
	// set, it takes precedence over SigV4 credentials. Write-only.
	APIKey *string `json:"api_key,omitempty"`
}

// IsConfigured reports whether any load-bearing Claude Platform field is set.
// WorkspaceID alone is sufficient: a SigV4 provider can authenticate via the
// AWS environment (instance profile, AWS_PROFILE, IRSA, etc.) with no explicit
// credentials, so the workspace ID is the reliable signal of operator intent.
func (c AIProviderClaudePlatformAWSSettings) IsConfigured() bool {
	if c.Region != "" {
		return true
	}
	if c.WorkspaceID != "" {
		return true
	}
	if c.RoleARN != "" {
		return true
	}
	if c.AccessKey != nil && *c.AccessKey != "" {
		return true
	}
	if c.AccessKeySecret != nil && *c.AccessKeySecret != "" {
		return true
	}
	if c.APIKey != nil && *c.APIKey != "" {
		return true
	}
	return false
}

func (AIProviderClaudePlatformAWSSettings) settingsType() string {
	return AIProviderSettingsTypeClaudePlatformAWS
}

func (AIProviderClaudePlatformAWSSettings) settingsVersion() int {
	return AIProviderClaudePlatformAWSSettingsVersion
}
