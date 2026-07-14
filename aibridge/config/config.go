package config

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/keypool"
)

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderCopilot   = "copilot"
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

// BedrockProtocol selects which AWS Bedrock wire protocol a provider targets.
type BedrockProtocol string

const (
	// BedrockProtocolInvokeModel is the legacy InvokeModel protocol
	// (bedrock-runtime.{region}.amazonaws.com), which translates the native
	// Messages request into Bedrock's InvokeModel format. It is the default
	// for the zero value.
	BedrockProtocolInvokeModel BedrockProtocol = "invoke-model"
	// BedrockProtocolMantle is the mantle protocol
	// (bedrock-mantle.{region}.api.aws/anthropic/v1/messages). It is a
	// passthrough: the gateway forwards the native Messages request body
	// unchanged and only applies AWS SigV4 signing (service bedrock-mantle).
	BedrockProtocolMantle BedrockProtocol = "mantle"
)

type AWSBedrock struct {
	Region                     string
	AccessKey, AccessKeySecret string
	Model, SmallFastModel      string
	// BaseURL configures the upstream Bedrock endpoint.
	//
	// For InvokeModel, it is optional. When empty, requests use the default
	// https://bedrock-runtime.{region}.amazonaws.com endpoint. Set it to route
	// InvokeModel requests through a proxy or test server.
	//
	// For mantle, it is required and must be the Messages API prefix without
	// /v1/messages, e.g. https://bedrock-mantle.{region}.api.aws/anthropic.
	BaseURL string
	// RoleARN, when set, is assumed via STS before calling Bedrock. The base
	// identity (static keys or the AWS SDK default credential chain, e.g.
	// IRSA / EKS Pod Identity / EC2 Instance Profile) signs the AssumeRole
	// call, and the resulting temporary credentials sign Bedrock requests.
	RoleARN string
	// ExternalID is sent as the STS external ID on the AssumeRole call.
	// It is meaningful only alongside RoleARN and must match the
	// sts:ExternalId condition on the target role's trust policy.
	ExternalID string
	// Protocol selects the Bedrock wire protocol. The zero value behaves as
	// BedrockProtocolInvokeModel.
	Protocol BedrockProtocol
}

// Validate verifies protocol-specific Bedrock configuration.
func (c AWSBedrock) Validate() error {
	switch c.Protocol {
	case "", BedrockProtocolInvokeModel:
		if c.Region == "" && c.BaseURL == "" {
			return xerrors.New("region or base url required")
		}
		if c.Model == "" {
			return xerrors.New("model required")
		}
		if c.SmallFastModel == "" {
			return xerrors.New("small fast model required")
		}
	case BedrockProtocolMantle:
		if c.Region == "" {
			return xerrors.New("region required")
		}
		if c.BaseURL == "" {
			return xerrors.New("base_url required")
		}
	default:
		return xerrors.Errorf("unknown bedrock protocol: %q", c.Protocol)
	}
	return nil
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
