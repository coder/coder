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

// ResolvedProtocol returns the configured protocol, mapping the empty value to
// the legacy InvokeModel protocol so existing providers keep the legacy
// behavior.
func (c AWSBedrock) ResolvedProtocol() BedrockProtocol {
	if c.Protocol == "" {
		return BedrockProtocolInvokeModel
	}
	return c.Protocol
}

// FieldError is a single failed validation rule scoped to a settings field.
// Field is the settings JSON tag name (region, model, small_fast_model,
// base_url) without the "settings." prefix; callers mapping to an API
// response add that prefix.
type FieldError struct {
	Field  string
	Detail string
}

func (e FieldError) Error() string { return e.Detail }

// ValidationErrors returns the field-scoped validation errors for the bedrock
// config. It encodes the same required-field rules as Validate() but as a
// slice so callers can map each to an API field-level error. Returns nil when
// the config is valid. It returns nil for an unknown protocol; Validate()
// handles that case as a hard non-field-scoped error.
func (c AWSBedrock) ValidationErrors() []FieldError {
	var errs []FieldError
	switch c.ResolvedProtocol() {
	case BedrockProtocolInvokeModel:
		if c.Region == "" && c.BaseURL == "" {
			errs = append(errs, FieldError{Field: "region", Detail: "region or base url required"})
		}
		if c.Model == "" {
			errs = append(errs, FieldError{Field: "model", Detail: "model required"})
		}
		if c.SmallFastModel == "" {
			errs = append(errs, FieldError{Field: "small_fast_model", Detail: "small fast model required"})
		}
	case BedrockProtocolMantle:
		if c.Region == "" {
			errs = append(errs, FieldError{Field: "region", Detail: "region required"})
		}
		if c.BaseURL == "" {
			errs = append(errs, FieldError{Field: "base_url", Detail: "base_url required"})
		}
	}
	return errs
}

// Validate verifies protocol-specific Bedrock configuration.
func (c AWSBedrock) Validate() error {
	if errs := c.ValidationErrors(); len(errs) > 0 {
		// Preserve the single-error behavior callers expect: return the first.
		return errs[0]
	}
	// Unknown protocol is still a hard error (not field-scoped);
	// ValidationErrors() returns nil for unknown protocols (the switch falls
	// through), so handle it here to preserve behavior.
	if c.ResolvedProtocol() != BedrockProtocolInvokeModel && c.ResolvedProtocol() != BedrockProtocolMantle {
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
