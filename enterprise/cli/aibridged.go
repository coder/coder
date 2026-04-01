//go:build !slim

package cli

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
	"github.com/coder/aibridge/config"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIBridgeDaemon(coderAPI *coderd.API) (*aibridged.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")
	cfg := coderAPI.DeploymentValues.AI.BridgeConfig

	// Build circuit breaker config if enabled.
	var cbConfig *config.CircuitBreaker
	if cfg.CircuitBreakerEnabled.Value() {
		cbConfig = &config.CircuitBreaker{
			FailureThreshold: uint32(cfg.CircuitBreakerFailureThreshold.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
			Interval:         cfg.CircuitBreakerInterval.Value(),
			Timeout:          cfg.CircuitBreakerTimeout.Value(),
			MaxRequests:      uint32(cfg.CircuitBreakerMaxRequests.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
		}
	}

	providers, err := buildProviders(cfg, cbConfig)
	if err != nil {
		return nil, xerrors.Errorf("build providers: %w", err)
	}

	reg := prometheus.WrapRegistererWithPrefix("coder_aibridged_", coderAPI.PrometheusRegistry)
	metrics := aibridge.NewMetrics(reg)
	tracer := coderAPI.TracerProvider.Tracer(tracing.TracerName)

	// Create pool for reusable stateful [aibridge.RequestBridge] instances (one per user).
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool"), metrics, tracer) // TODO: configurable size.
	if err != nil {
		return nil, xerrors.Errorf("create request pool: %w", err)
	}

	// Create daemon.
	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return coderAPI.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	if err != nil {
		return nil, xerrors.Errorf("start in-memory aibridge daemon: %w", err)
	}
	return srv, nil
}

// buildProviders constructs the list of aibridge providers from config.
// It merges legacy single-provider env vars, indexed provider configs, and
// built-in providers (copilot variants, chatgpt):
//  1. Legacy providers (from CODER_AIBRIDGE_OPENAI_KEY, etc.) are added first.
//     If a legacy name conflicts with an indexed provider, startup fails with
//     a clear error asking the admin to remove one or the other.
//  2. Indexed providers (from CODER_AIBRIDGE_PROVIDER_<N>_*) are added next.
//  3. Built-in providers are always added unless their name is already claimed.
func buildProviders(cfg codersdk.AIBridgeConfig, cbConfig *config.CircuitBreaker) ([]aibridge.Provider, error) {
	var providers []aibridge.Provider
	usedNames := make(map[string]struct{})

	// Collect names from indexed providers so we know which legacy/builtin
	// providers to skip.
	for _, p := range cfg.Providers {
		name := p.Name
		if name == "" {
			name = p.Type
		}
		usedNames[name] = struct{}{}
	}

	// Add legacy OpenAI provider if configured.
	if cfg.LegacyOpenAI.Key.String() != "" {
		if _, conflict := usedNames[aibridge.ProviderOpenAI]; conflict {
			return nil, xerrors.Errorf("legacy CODER_AIBRIDGE_OPENAI_KEY conflicts with indexed provider named %q; remove one or the other", aibridge.ProviderOpenAI)
		}
		providers = append(providers, aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:             aibridge.ProviderOpenAI,
			BaseURL:          cfg.LegacyOpenAI.BaseURL.String(),
			Key:              cfg.LegacyOpenAI.Key.String(),
			CircuitBreaker:   cbConfig,
			SendActorHeaders: cfg.SendActorHeaders.Value(),
		}))
		usedNames[aibridge.ProviderOpenAI] = struct{}{}
	}

	// Add legacy Anthropic provider if configured.
	if cfg.LegacyAnthropic.Key.String() != "" {
		if _, conflict := usedNames[aibridge.ProviderAnthropic]; conflict {
			return nil, xerrors.Errorf("legacy CODER_AIBRIDGE_ANTHROPIC_KEY conflicts with indexed provider named %q; remove one or the other", aibridge.ProviderAnthropic)
		}
		providers = append(providers, aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
			Name:             aibridge.ProviderAnthropic,
			BaseURL:          cfg.LegacyAnthropic.BaseURL.String(),
			Key:              cfg.LegacyAnthropic.Key.String(),
			CircuitBreaker:   cbConfig,
			SendActorHeaders: cfg.SendActorHeaders.Value(),
		}, getBedrockConfig(cfg.LegacyBedrock)))
		usedNames[aibridge.ProviderAnthropic] = struct{}{}
	}

	// Add indexed providers.
	for _, p := range cfg.Providers {
		name := p.Name
		if name == "" {
			name = p.Type
		}
		switch p.Type {
		case aibridge.ProviderOpenAI:
			providers = append(providers, aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
				Name:             name,
				BaseURL:          p.BaseURL,
				Key:              p.Key,
				CircuitBreaker:   cbConfig,
				SendActorHeaders: cfg.SendActorHeaders.Value(),
			}))
		case aibridge.ProviderAnthropic:
			providers = append(providers, aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
				Name:             name,
				BaseURL:          p.BaseURL,
				Key:              p.Key,
				CircuitBreaker:   cbConfig,
				SendActorHeaders: cfg.SendActorHeaders.Value(),
			}, bedrockConfigFromProvider(p)))
		case aibridge.ProviderCopilot:
			providers = append(providers, aibridge.NewCopilotProvider(aibridge.CopilotConfig{
				Name:           name,
				BaseURL:        p.BaseURL,
				CircuitBreaker: cbConfig,
			}))
		default:
			return nil, xerrors.Errorf("unknown provider type %q for provider %q", p.Type, name)
		}
	}

	// Always add built-in providers unless their name is already claimed.
	builtins := []struct {
		name    string
		baseURL string
		factory func(name, baseURL string) aibridge.Provider
	}{
		{
			name: aibridge.ProviderCopilot,
			factory: func(name, baseURL string) aibridge.Provider {
				return aibridge.NewCopilotProvider(aibridge.CopilotConfig{
					Name: name, BaseURL: baseURL, CircuitBreaker: cbConfig,
				})
			},
		},
		{
			name:    agplaibridge.ProviderCopilotBusiness,
			baseURL: "https://" + agplaibridge.HostCopilotBusiness,
			factory: func(name, baseURL string) aibridge.Provider {
				return aibridge.NewCopilotProvider(aibridge.CopilotConfig{
					Name: name, BaseURL: baseURL, CircuitBreaker: cbConfig,
				})
			},
		},
		{
			name:    agplaibridge.ProviderCopilotEnterprise,
			baseURL: "https://" + agplaibridge.HostCopilotEnterprise,
			factory: func(name, baseURL string) aibridge.Provider {
				return aibridge.NewCopilotProvider(aibridge.CopilotConfig{
					Name: name, BaseURL: baseURL, CircuitBreaker: cbConfig,
				})
			},
		},
		{
			name:    agplaibridge.ProviderChatGPT,
			baseURL: agplaibridge.BaseURLChatGPT,
			factory: func(name, baseURL string) aibridge.Provider {
				return aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
					Name: name, BaseURL: baseURL, CircuitBreaker: cbConfig,
					SendActorHeaders: cfg.SendActorHeaders.Value(),
				})
			},
		},
	}
	for _, b := range builtins {
		if _, exists := usedNames[b.name]; !exists {
			providers = append(providers, b.factory(b.name, b.baseURL))
			usedNames[b.name] = struct{}{}
		}
	}

	return providers, nil
}

// bedrockConfigFromProvider converts Bedrock fields from an indexed
// AIBridgeProviderConfig into an aibridge AWSBedrockConfig.
// Returns nil if no Bedrock fields are set.
func bedrockConfigFromProvider(p codersdk.AIBridgeProviderConfig) *aibridge.AWSBedrockConfig {
	if p.BedrockRegion == "" && p.BedrockBaseURL == "" && p.BedrockAccessKey == "" && p.BedrockAccessKeySecret == "" {
		return nil
	}
	return &aibridge.AWSBedrockConfig{
		BaseURL:         p.BedrockBaseURL,
		Region:          p.BedrockRegion,
		AccessKey:       p.BedrockAccessKey,
		AccessKeySecret: p.BedrockAccessKeySecret,
		Model:           p.BedrockModel,
		SmallFastModel:  p.BedrockSmallFastModel,
	}
}

func getBedrockConfig(cfg codersdk.AIBridgeBedrockConfig) *aibridge.AWSBedrockConfig {
	if cfg.Region.String() == "" && cfg.BaseURL.String() == "" && cfg.AccessKey.String() == "" && cfg.AccessKeySecret.String() == "" {
		return nil
	}

	return &aibridge.AWSBedrockConfig{
		BaseURL:         cfg.BaseURL.String(),
		Region:          cfg.Region.String(),
		AccessKey:       cfg.AccessKey.String(),
		AccessKeySecret: cfg.AccessKeySecret.String(),
		Model:           cfg.Model.String(),
		SmallFastModel:  cfg.SmallFastModel.String(),
	}
}
