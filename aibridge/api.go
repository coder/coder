package aibridge

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Const + Type + function aliases for backwards compatibility.
const (
	ProviderAnthropic = config.ProviderAnthropic
	ProviderBedrock   = config.ProviderBedrock
	ProviderOpenAI    = config.ProviderOpenAI
	ProviderCopilot   = config.ProviderCopilot
)

type (
	Metrics = metrics.Metrics

	Provider = provider.Provider

	InterceptionRecord      = recorder.InterceptionRecord
	InterceptionRecordEnded = recorder.InterceptionRecordEnded
	TokenUsageRecord        = recorder.TokenUsageRecord
	PromptUsageRecord       = recorder.PromptUsageRecord
	ToolUsageRecord         = recorder.ToolUsageRecord
	ModelThoughtRecord      = recorder.ModelThoughtRecord
	Recorder                = recorder.Recorder
	Metadata                = recorder.Metadata
	ErrorType               = recorder.ErrorType

	AnthropicConfig  = config.Anthropic
	AWSBedrockConfig = config.AWSBedrock
	OpenAIConfig     = config.OpenAI
	CopilotConfig    = config.Copilot
)

func AsActor(ctx context.Context, actorID string, metadata recorder.Metadata) context.Context {
	return aibcontext.AsActor(ctx, actorID, metadata)
}

func NewAnthropicProvider(ctx context.Context, cfg config.Anthropic, bedrockCfg *config.AWSBedrock) (provider.Provider, error) {
	return provider.NewAnthropic(ctx, cfg, bedrockCfg)
}

func NewBedrockProvider(ctx context.Context, cfg config.Anthropic, bedrockCfg config.AWSBedrock) (provider.Provider, error) {
	return provider.NewBedrock(ctx, cfg, bedrockCfg)
}

func NewOpenAIProvider(cfg config.OpenAI) provider.Provider {
	return provider.NewOpenAI(cfg)
}

func NewCopilotProvider(cfg config.Copilot) provider.Provider {
	return provider.NewCopilot(cfg)
}

// NewDisabledProviderStub returns a Provider that reports Enabled() ==
// false and has no-op implementations for all other methods. Use this
// instead of constructing a concrete provider for disabled rows so that
// adding a new provider type does not require updating a switch here.
func NewDisabledProviderStub(name, providerType string) provider.Provider {
	return provider.NewDisabledStub(name, providerType)
}

func NewMetrics(reg prometheus.Registerer) *metrics.Metrics {
	return metrics.NewMetrics(reg)
}

// NewRecorder creates a [Recorder] which logs each record and acquires a client
// per call. clientFn receives the context of the call it serves.
//
// middleware is inserted directly below the logging middleware, so that every
// record is logged before any of it runs, and above the tracing middleware,
// which must stay immediately above the recorder its spans measure. Policy
// that drops records, such as [recorder.WithoutRecords], belongs here: it
// keeps NewRecorder to its own concerns and leaves the choice to the caller.
func NewRecorder(logger slog.Logger, tracer trace.Tracer, apiKeyID string, structured bool, clientFn func(context.Context) (Recorder, error), middleware ...recorder.Middleware) Recorder {
	chain := make([]recorder.Middleware, 0, len(middleware)+2)
	chain = append(chain, recorder.WithLogging(logger, apiKeyID, structured))
	chain = append(chain, middleware...)
	chain = append(chain, recorder.WithTracing(tracer))

	return recorder.ChainMiddleware(chain...)(recorder.NewWrappedRecorder(clientFn))
}
