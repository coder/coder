package config

import (
	"time"

	"github.com/coder/coder/v2/aibridge/keypool"
)

const (
	ProviderAnthropic         = "anthropic"
	ProviderOpenAI            = "openai"
	ProviderCopilot           = "copilot"
	ProviderClaudePlatformAWS = "claude-platform-aws"
)

// Anthropic carries configuration for an Anthropic provider.
type Anthropic struct {
	// Name is the provider instance name. If empty, defaults to "anthropic".
	Name    string
	BaseURL string
	// KeyPool holds the centralized keys, with automatic key failover. BYOK
	// credentials are resolved per request from the incoming headers.
	KeyPool          *keypool.Pool
	APIDumpDir       string
	CircuitBreaker   *CircuitBreaker
	SendActorHeaders bool
}

type AWSBedrock struct {
	Region                     string
	AccessKey, AccessKeySecret string
	Model, SmallFastModel      string
	// If set, requests will be sent to this URL instead of the default AWS Bedrock endpoint
	// (https://bedrock-runtime.{region}.amazonaws.com).
	// This is useful for routing requests through a proxy or for testing.
	BaseURL string
	// RoleARN, when set, is assumed via STS before calling Bedrock. The base
	// identity (static keys or the AWS SDK default credential chain, e.g.
	// IRSA / EKS Pod Identity / EC2 Instance Profile) signs the AssumeRole
	// call, and the resulting temporary credentials sign Bedrock requests.
	RoleARN string
}

// AWSClaudePlatform carries configuration for a Claude Platform for AWS
// provider. Unlike AWSBedrock, this targets Anthropic's native Messages API
// hosted on AWS (https://aws-external-anthropic.<region>.api.aws), signed with
// the SigV4 service name "aws-external-anthropic" and carrying the
// anthropic-workspace-id header. Model IDs are standard Anthropic IDs and pass
// through unchanged.
type AWSClaudePlatform struct {
	// Region is the AWS region used for SigV4 signing and, when BaseURL is not
	// set, to construct the regional endpoint.
	Region string
	// WorkspaceID is sent in the anthropic-workspace-id header on every
	// request. Required.
	WorkspaceID string
	// AccessKey and AccessKeySecret are static AWS credentials for SigV4. When
	// unset, credentials resolve via the AWS default credential chain.
	AccessKey, AccessKeySecret string
	// RoleARN, when set, is assumed via STS before signing requests. The base
	// identity (static keys or the AWS default credential chain) signs the
	// AssumeRole call, and the resulting temporary credentials sign requests.
	RoleARN string
	// ExternalID is the STS external ID passed when assuming RoleARN. Only
	// meaningful when RoleARN is set.
	ExternalID string
	// APIKey, when set, authenticates with a Claude Platform workspace API key
	// sent as x-api-key, taking precedence over SigV4.
	APIKey string
	// BaseURL overrides the default regional endpoint. Useful for routing
	// through a proxy or for testing.
	BaseURL string
}

// OpenAI carries configuration for an OpenAI provider.
type OpenAI struct {
	// Name is the provider instance name. If empty, defaults to "openai".
	Name    string
	BaseURL string
	// KeyPool holds the centralized keys, with automatic key failover. BYOK
	// credentials are resolved per request from the incoming headers.
	KeyPool          *keypool.Pool
	APIDumpDir       string
	CircuitBreaker   *CircuitBreaker
	SendActorHeaders bool
}

type Copilot struct {
	// Name is the provider instance name. If empty, defaults to "copilot".
	Name           string
	BaseURL        string
	APIDumpDir     string
	CircuitBreaker *CircuitBreaker
}

// CircuitBreaker holds configuration for circuit breakers.
type CircuitBreaker struct {
	// MaxRequests is the maximum number of requests allowed in half-open state.
	MaxRequests uint32
	// Interval is the cyclic period of the closed state for clearing internal counts.
	Interval time.Duration
	// Timeout is how long the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// FailureThreshold is the number of consecutive failures that triggers the circuit to open.
	FailureThreshold uint32
	// IsFailure determines if a status code should count as a failure.
	// If nil, defaults to DefaultIsFailure.
	IsFailure func(statusCode int) bool
	// OpenErrorResponse returns the response body when the circuit is open.
	// This should match the provider's error format.
	OpenErrorResponse func() []byte
}

// DefaultCircuitBreaker returns sensible defaults for circuit breaker configuration.
func DefaultCircuitBreaker() CircuitBreaker {
	return CircuitBreaker{
		FailureThreshold: 5,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		MaxRequests:      3,
	}
}
