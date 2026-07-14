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

func NewRecorder(logger slog.Logger, tracer trace.Tracer, clientFn func() (Recorder, error)) Recorder {
	return recorder.NewWrappedRecorder(logger, tracer, clientFn)
}
