package aibridge

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge/config"
	aibcontext "github.com/coder/aibridge/context"
	"github.com/coder/aibridge/metrics"
	"github.com/coder/aibridge/provider"
	"github.com/coder/aibridge/recorder"
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

	AnthropicConfig  = config.Anthropic
	AWSBedrockConfig = config.AWSBedrock
	OpenAIConfig     = config.OpenAI
	CopilotConfig    = config.Copilot
)

func AsActor(ctx context.Context, actorID string, metadata recorder.Metadata) context.Context {
	return aibcontext.AsActor(ctx, actorID, metadata)
}

func NewAnthropicProvider(cfg config.Anthropic, bedrockCfg *config.AWSBedrock) provider.Provider {
	return provider.NewAnthropic(cfg, bedrockCfg)
}

func NewOpenAIProvider(cfg config.OpenAI) provider.Provider {
	return provider.NewOpenAI(cfg)
}

func NewCopilotProvider(cfg config.Copilot) provider.Provider {
	return provider.NewCopilot(cfg)
}

func NewMetrics(reg prometheus.Registerer) *metrics.Metrics {
	return metrics.NewMetrics(reg)
}

func NewRecorder(logger slog.Logger, tracer trace.Tracer, clientFn func() (Recorder, error)) Recorder {
	return recorder.NewWrappedRecorder(logger, tracer, clientFn)
}
