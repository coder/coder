package config

import "time"

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderCopilot   = "copilot"
)

type Anthropic struct {
	// Name is the provider instance name. If empty, defaults to "anthropic".
	Name             string
	BaseURL          string
	Key              string
	APIDumpDir       string
	CircuitBreaker   *CircuitBreaker
	SendActorHeaders bool
	ExtraHeaders     map[string]string
	// BYOKBearerToken is set in BYOK mode when the user authenticates
	// with a access token. When set, the access token is used for upstream
	// LLM requests instead of the API key.
	BYOKBearerToken string
	// MaxRetries controls the number of automatic retries the SDK will perform
	// on transient errors. If nil, the SDK default (2) is used.
	// Set to 0 to disable retries entirely.
	MaxRetries *int
}

type AWSBedrock struct {
	Region                     string
	AccessKey, AccessKeySecret string
	Model, SmallFastModel      string
	// If set, requests will be sent to this URL instead of the default AWS Bedrock endpoint
	// (https://bedrock-runtime.{region}.amazonaws.com).
	// This is useful for routing requests through a proxy or for testing.
	BaseURL string
}

type OpenAI struct {
	// Name is the provider instance name. If empty, defaults to "openai".
	Name             string
	BaseURL          string
	Key              string
	APIDumpDir       string
	CircuitBreaker   *CircuitBreaker
	SendActorHeaders bool
	ExtraHeaders     map[string]string
	// MaxRetries controls the number of automatic retries the SDK will perform
	// on transient errors. If nil, the SDK default (2) is used.
	// Set to 0 to disable retries entirely.
	MaxRetries *int
}

type Copilot struct {
	// Name is the provider instance name. If empty, defaults to "copilot".
	Name           string
	BaseURL        string
	APIDumpDir     string
	CircuitBreaker *CircuitBreaker
	// MaxRetries controls the number of automatic retries the SDK will perform
	// on transient errors. If nil, the SDK default (2) is used.
	// Set to 0 to disable retries entirely.
	MaxRetries *int
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
