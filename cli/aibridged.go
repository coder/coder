//go:build !slim

package cli

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// errUnsupportedProviderType is returned by buildProviderFromRow when an
// ai_providers row has a type that the aibridge runtime has no constructor
// for yet. BuildProvidersFromDB treats this as a skip-with-warning so that
// admin-created rows with future types do not block server startup.
var errUnsupportedProviderType = xerrors.New("unsupported provider type")

func newAIBridgeDaemon(coderAPI *coderd.API, providers []aibridge.Provider) (*aibridged.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")

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

// BuildProvidersFromDB constructs aibridge providers from the ai_providers
// table. Each Provider carries the row's UUID via Provider.ID() so that
// interception records can persist provider_id alongside the legacy
// provider/provider_name snapshot columns.
//
// Provider-instance configuration (BaseURL, keys, Bedrock settings) comes
// from the row; cross-cutting knobs (circuit breaker, send-actor-headers)
// come from cfg since they are not modeled per-provider in the DB schema.
// The db handle must be the dbcrypt-wrapped store so that settings and API
// keys are returned decrypted.
func BuildProvidersFromDB(ctx context.Context, logger slog.Logger, db database.Store, cfg codersdk.AIBridgeConfig) ([]aibridge.Provider, error) {
	//nolint:gocritic // aibridged daemon load, no user actor available
	ctx = dbauthz.AsAIBridged(ctx)
	rows, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
		IncludeDeleted:  false,
		IncludeDisabled: false,
	})
	if err != nil {
		return nil, xerrors.Errorf("list ai_providers: %w", err)
	}

	var cbConfig *config.CircuitBreaker
	if cfg.CircuitBreakerEnabled.Value() {
		cbConfig = &config.CircuitBreaker{
			FailureThreshold: uint32(cfg.CircuitBreakerFailureThreshold.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
			Interval:         cfg.CircuitBreakerInterval.Value(),
			Timeout:          cfg.CircuitBreakerTimeout.Value(),
			MaxRequests:      uint32(cfg.CircuitBreakerMaxRequests.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
		}
	}

	// Batch-fetch keys for all providers in one query rather than N+1.
	providerIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		providerIDs = append(providerIDs, row.ID)
	}
	keysByProvider := map[uuid.UUID][]string{}
	if len(providerIDs) > 0 {
		keyRows, err := db.GetAIProviderKeysByProviderIDs(ctx, providerIDs)
		if err != nil {
			return nil, xerrors.Errorf("load ai_provider keys: %w", err)
		}
		for _, k := range keyRows {
			if k.APIKey == "" {
				continue
			}
			keysByProvider[k.ProviderID] = append(keysByProvider[k.ProviderID], k.APIKey)
		}
	}

	providers := make([]aibridge.Provider, 0, len(rows))
	for _, row := range rows {
		p, err := buildProviderFromRow(row, keysByProvider[row.ID], cbConfig, cfg.SendActorHeaders.Value())
		if errors.Is(err, errUnsupportedProviderType) {
			// Admin-created rows with provider types the aibridge runtime
			// has no constructor for must not block server startup; the
			// remaining supported providers continue to work.
			logger.Warn(ctx, "skipping ai provider with unsupported type",
				slog.F("provider_id", row.ID),
				slog.F("name", row.Name),
				slog.F("type", row.Type),
			)
			continue
		}
		if err != nil {
			return nil, xerrors.Errorf("build provider %q (%s): %w", row.Name, row.ID, err)
		}
		providers = append(providers, p)
	}
	return providers, nil
}

func buildProviderFromRow(row database.AIProvider, apiKeys []string, cbConfig *config.CircuitBreaker, sendActorHeaders bool) (aibridge.Provider, error) {
	var settings codersdk.AIProviderSettings
	if row.Settings.Valid && row.Settings.String != "" {
		if err := json.Unmarshal([]byte(row.Settings.String), &settings); err != nil {
			return nil, xerrors.Errorf("decode settings: %w", err)
		}
	}

	switch row.Type {
	case database.AiProviderTypeOpenai:
		var pool *keypool.Pool
		if len(apiKeys) > 0 {
			var err error
			pool, err = keypool.New(apiKeys, quartz.NewReal())
			if err != nil {
				return nil, xerrors.Errorf("create openai key pool: %w", err)
			}
		}
		return aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			ID:               row.ID,
			Name:             row.Name,
			BaseURL:          row.BaseUrl,
			KeyPool:          pool,
			CircuitBreaker:   cbConfig,
			SendActorHeaders: sendActorHeaders,
		}), nil

	case database.AiProviderTypeAnthropic:
		var pool *keypool.Pool
		if len(apiKeys) > 0 {
			var err error
			pool, err = keypool.New(apiKeys, quartz.NewReal())
			if err != nil {
				return nil, xerrors.Errorf("create anthropic key pool: %w", err)
			}
		}
		var (
			bedrockCfg    *aibridge.AWSBedrockConfig
			anthropicBase = row.BaseUrl
		)
		if settings.Bedrock != nil {
			// Bedrock providers reuse the row's BaseUrl as the Bedrock
			// endpoint, mirroring the legacy env-driven seed. The Anthropic
			// API URL is left empty in that case so the provider routes
			// through Bedrock instead of api.anthropic.com.
			anthropicBase = ""
			var accessKey, accessKeySecret string
			if settings.Bedrock.AccessKey != nil {
				accessKey = *settings.Bedrock.AccessKey
			}
			if settings.Bedrock.AccessKeySecret != nil {
				accessKeySecret = *settings.Bedrock.AccessKeySecret
			}
			bedrockCfg = &aibridge.AWSBedrockConfig{
				BaseURL:         row.BaseUrl,
				Region:          settings.Bedrock.Region,
				AccessKey:       accessKey,
				AccessKeySecret: accessKeySecret,
				Model:           settings.Bedrock.Model,
				SmallFastModel:  settings.Bedrock.SmallFastModel,
			}
		}
		return aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
			ID:               row.ID,
			Name:             row.Name,
			BaseURL:          anthropicBase,
			KeyPool:          pool,
			CircuitBreaker:   cbConfig,
			SendActorHeaders: sendActorHeaders,
		}, bedrockCfg), nil

	case database.AiProviderTypeCopilot:
		// Copilot authenticates via request-time GitHub OAuth tokens, so any
		// keys persisted alongside the provider row are intentionally
		// ignored at construction time.
		return aibridge.NewCopilotProvider(aibridge.CopilotConfig{
			ID:             row.ID,
			Name:           row.Name,
			BaseURL:        row.BaseUrl,
			CircuitBreaker: cbConfig,
		}), nil

	default:
		// Other ai_provider_type values (azure, google, openai-compat,
		// openrouter, vercel) are not yet supported by aibridge runtime
		// constructors. Return the sentinel so the caller can skip the row
		// with a warning rather than aborting startup.
		return nil, xerrors.Errorf("type %q: %w", row.Type, errUnsupportedProviderType)
	}
}

// BuildProviders constructs the list of AI providers from config.
// It merges legacy single-provider env vars and indexed provider configs:
//  1. Legacy providers (from CODER_AI_GATEWAY_OPENAI_KEY, etc.) are added first.
//     If a legacy name conflicts with an indexed provider, startup fails with
//     a clear error asking the admin to remove one or the other.
//  2. Indexed providers (from CODER_AI_GATEWAY_PROVIDER_<N>_*) are added next.
func BuildProviders(cfg codersdk.AIBridgeConfig) ([]aibridge.Provider, error) {
	var cbConfig *config.CircuitBreaker
	if cfg.CircuitBreakerEnabled.Value() {
		cbConfig = &config.CircuitBreaker{
			FailureThreshold: uint32(cfg.CircuitBreakerFailureThreshold.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
			Interval:         cfg.CircuitBreakerInterval.Value(),
			Timeout:          cfg.CircuitBreakerTimeout.Value(),
			MaxRequests:      uint32(cfg.CircuitBreakerMaxRequests.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
		}
	}

	var providers []aibridge.Provider
	usedNames := make(map[string]struct{})

	// Collect names from indexed providers so we can detect conflicts
	// with legacy providers.
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
			return nil, xerrors.Errorf("legacy CODER_AI_GATEWAY_OPENAI_KEY (or CODER_AIBRIDGE_OPENAI_KEY) conflicts with indexed provider named %q; remove one or the other", aibridge.ProviderOpenAI)
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

	// Add legacy Anthropic provider if configured. Bedrock credentials
	// alone are sufficient, an Anthropic API key is not required when
	// using AWS Bedrock.
	if cfg.LegacyAnthropic.Key.String() != "" || getBedrockConfig(cfg.LegacyBedrock) != nil {
		if _, conflict := usedNames[aibridge.ProviderAnthropic]; conflict {
			return nil, xerrors.Errorf("legacy CODER_AI_GATEWAY_ANTHROPIC_KEY (or CODER_AIBRIDGE_ANTHROPIC_KEY) conflicts with indexed provider named %q; remove one or the other", aibridge.ProviderAnthropic)
		}
		var pool *keypool.Pool
		if key := cfg.LegacyAnthropic.Key.String(); key != "" {
			var err error
			pool, err = keypool.New([]string{key}, quartz.NewReal())
			if err != nil {
				return nil, xerrors.Errorf("create legacy anthropic key pool: %w", err)
			}
		}
		providers = append(providers, aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
			Name:             aibridge.ProviderAnthropic,
			BaseURL:          cfg.LegacyAnthropic.BaseURL.String(),
			KeyPool:          pool,
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
			var pool *keypool.Pool
			if len(p.Keys) > 0 {
				var err error
				pool, err = keypool.New(p.Keys, quartz.NewReal())
				if err != nil {
					return nil, xerrors.Errorf("create openai key pool for provider %q: %w", name, err)
				}
			}
			providers = append(providers, aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
				Name:             name,
				BaseURL:          p.BaseURL,
				KeyPool:          pool,
				APIDumpDir:       p.DumpDir,
				CircuitBreaker:   cbConfig,
				SendActorHeaders: cfg.SendActorHeaders.Value(),
			}))
		case aibridge.ProviderAnthropic:
			var pool *keypool.Pool
			if len(p.Keys) > 0 {
				var err error
				pool, err = keypool.New(p.Keys, quartz.NewReal())
				if err != nil {
					return nil, xerrors.Errorf("create anthropic key pool for provider %q: %w", name, err)
				}
			}
			providers = append(providers, aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
				Name:             name,
				BaseURL:          p.BaseURL,
				KeyPool:          pool,
				APIDumpDir:       p.DumpDir,
				CircuitBreaker:   cbConfig,
				SendActorHeaders: cfg.SendActorHeaders.Value(),
			}, bedrockConfigFromProvider(p)))
		case aibridge.ProviderCopilot:
			providers = append(providers, aibridge.NewCopilotProvider(aibridge.CopilotConfig{
				Name:           name,
				BaseURL:        p.BaseURL,
				APIDumpDir:     p.DumpDir,
				CircuitBreaker: cbConfig,
			}))
		default:
			return nil, xerrors.Errorf("unknown provider type %q for provider %q", p.Type, name)
		}
	}

	return providers, nil
}

// bedrockConfigFromProvider converts Bedrock fields from an indexed
// AIProviderConfig into an aibridge AWSBedrockConfig.
// Returns nil if no Bedrock fields are set.
func bedrockConfigFromProvider(p codersdk.AIProviderConfig) *aibridge.AWSBedrockConfig {
	// Currently, only the first key pair is used, if any.
	// TODO(ssncferreira): pass a keypool.Pool instead.
	var accessKey, accessKeySecret string
	if len(p.BedrockAccessKeys) > 0 {
		accessKey = p.BedrockAccessKeys[0]
	}
	if len(p.BedrockAccessKeySecrets) > 0 {
		accessKeySecret = p.BedrockAccessKeySecrets[0]
	}
	settings := codersdk.NewAIProviderBedrockSettings(
		p.BedrockRegion, accessKey, accessKeySecret,
		p.BedrockModel, p.BedrockSmallFastModel,
	)
	if !codersdk.IsBedrockConfigured(p.BedrockBaseURL, settings) {
		return nil
	}
	return &aibridge.AWSBedrockConfig{
		BaseURL:         p.BedrockBaseURL,
		Region:          p.BedrockRegion,
		AccessKey:       accessKey,
		AccessKeySecret: accessKeySecret,
		Model:           p.BedrockModel,
		SmallFastModel:  p.BedrockSmallFastModel,
	}
}

func getBedrockConfig(cfg codersdk.AIBridgeBedrockConfig) *aibridge.AWSBedrockConfig {
	// codersdk.IsBedrockConfigured decides what counts as Bedrock; when
	// it returns false, the AWS SDK default credential chain (env vars,
	// shared config, IAM roles, etc.) is left to resolve credentials.
	settings := codersdk.NewAIProviderBedrockSettings(
		cfg.Region.String(),
		cfg.AccessKey.String(),
		cfg.AccessKeySecret.String(),
		cfg.Model.String(),
		cfg.SmallFastModel.String(),
	)
	if !codersdk.IsBedrockConfigured(cfg.BaseURL.String(), settings) {
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
