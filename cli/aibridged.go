//go:build !slim

package cli

import (
	"context"
	"path"

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
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

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

// BuildProviders loads every enabled ai_providers row, attaches its
// keys, and constructs the equivalent [aibridge.Provider] instances.
// The database is the single source of truth for runtime provider
// configuration.
//
// Per-provider construction errors are logged and the offending row is
// excluded from the returned snapshot; only a failure of the DB query
// itself is propagated. This keeps a single misconfigured row from
// taking the whole daemon down.
func BuildProviders(ctx context.Context, db database.Store, cfg codersdk.AIBridgeConfig, logger slog.Logger) ([]aibridge.Provider, error) {
	//nolint:gocritic // AsAIBridged has a minimal permission set for this purpose.
	authCtx := dbauthz.AsAIBridged(ctx)

	rows, err := db.GetAIProviders(authCtx, database.GetAIProvidersParams{
		IncludeDisabled: false,
	})
	if err != nil {
		return nil, xerrors.Errorf("load ai providers: %w", err)
	}

	// Group keys by provider in a single query to avoid N+1.
	keyRows, err := db.GetAIProviderKeys(authCtx, false)
	if err != nil {
		return nil, xerrors.Errorf("load ai provider keys: %w", err)
	}
	keysByProvider := make(map[uuid.UUID][]database.AIProviderKey, len(keyRows))
	for _, k := range keyRows {
		keysByProvider[k.ProviderID] = append(keysByProvider[k.ProviderID], k)
	}

	out := make([]aibridge.Provider, 0, len(rows))
	for _, row := range rows {
		prov, err := buildAIProviderFromRow(row, keysByProvider[row.ID], cfg)
		if err != nil {
			logger.Error(ctx, "skip ai provider, failed to build",
				slog.F("provider_id", row.ID),
				slog.F("provider_name", row.Name),
				slog.F("provider_type", string(row.Type)),
				slog.Error(err),
			)
			continue
		}
		if prov == nil {
			continue
		}
		out = append(out, prov)
	}
	return out, nil
}

// buildAIProviderFromRow constructs the appropriate [aibridge.Provider]
// for a single ai_providers row. The row's settings blob is already
// decrypted by the dbcrypt-wrapped store; this function only decodes
// the JSON shape on top of it.
func buildAIProviderFromRow(
	row database.AIProvider,
	keys []database.AIProviderKey,
	cfg codersdk.AIBridgeConfig,
) (aibridge.Provider, error) {
	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return nil, xerrors.Errorf("decode settings: %w", err)
	}

	cbCfg := circuitBreakerConfig(cfg)
	sendActorHeaders := cfg.SendActorHeaders.Value()
	dumpDir := cfg.APIDumpDir.Value()
	if dumpDir != "" {
		dumpDir = path.Join(dumpDir, row.Name)
	}

	switch row.Type {
	case database.AiProviderTypeOpenai:
		if len(keys) == 0 {
			return nil, xerrors.New("openai provider has no api keys configured")
		}
		pool, err := buildAIProviderKeyPool(keys)
		if err != nil {
			return nil, xerrors.Errorf("openai key pool: %w", err)
		}
		return aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:             row.Name,
			BaseURL:          row.BaseUrl,
			KeyPool:          pool,
			APIDumpDir:       dumpDir,
			CircuitBreaker:   cbCfg,
			SendActorHeaders: sendActorHeaders,
		}), nil

	case database.AiProviderTypeAnthropic:
		bedrock := bedrockConfigFromRow(row, settings)
		// Bedrock-backed Anthropic authenticates via AWS credentials in
		// the settings blob, not the api_keys table. A bearer-token
		// Anthropic without any key cannot make upstream calls.
		if bedrock == nil && len(keys) == 0 {
			return nil, xerrors.New("anthropic provider has no api keys and no bedrock credentials")
		}
		var pool *keypool.Pool
		if len(keys) > 0 {
			var err error
			pool, err = buildAIProviderKeyPool(keys)
			if err != nil {
				return nil, xerrors.Errorf("anthropic key pool: %w", err)
			}
		}
		return aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
			Name:             row.Name,
			BaseURL:          row.BaseUrl,
			KeyPool:          pool,
			APIDumpDir:       dumpDir,
			CircuitBreaker:   cbCfg,
			SendActorHeaders: sendActorHeaders,
		}, bedrock), nil

	case database.AiProviderTypeCopilot:
		// Copilot is always BYOK; the per-user token is supplied on each
		// request via the Authorization header, so no keypool is built.
		return aibridge.NewCopilotProvider(aibridge.CopilotConfig{
			Name:           row.Name,
			BaseURL:        row.BaseUrl,
			APIDumpDir:     dumpDir,
			CircuitBreaker: cbCfg,
		}), nil

	default:
		return nil, xerrors.Errorf("unsupported provider type: %q", row.Type)
	}
}

// buildAIProviderKeyPool constructs a [keypool.Pool] from the keys
// attached to a provider row. Callers must check len(keys) before
// invoking: keypool.New rejects an empty input.
func buildAIProviderKeyPool(keys []database.AIProviderKey) (*keypool.Pool, error) {
	raw := make([]string, 0, len(keys))
	for _, k := range keys {
		raw = append(raw, k.APIKey)
	}
	return keypool.New(raw, quartz.NewReal())
}

// bedrockConfigFromRow returns the Bedrock config for an Anthropic
// row, or nil for a native Anthropic row.
func bedrockConfigFromRow(row database.AIProvider, settings codersdk.AIProviderSettings) *aibridge.AWSBedrockConfig {
	if settings.Bedrock == nil {
		return nil
	}
	bedrockSettings := *settings.Bedrock
	var accessKey, accessKeySecret string
	if bedrockSettings.AccessKey != nil {
		accessKey = *bedrockSettings.AccessKey
	}
	if bedrockSettings.AccessKeySecret != nil {
		accessKeySecret = *bedrockSettings.AccessKeySecret
	}
	return &aibridge.AWSBedrockConfig{
		BaseURL:         row.BaseUrl,
		Region:          bedrockSettings.Region,
		AccessKey:       accessKey,
		AccessKeySecret: accessKeySecret,
		Model:           bedrockSettings.Model,
		SmallFastModel:  bedrockSettings.SmallFastModel,
	}
}

// circuitBreakerConfig translates the deployment-level circuit-breaker
// settings into the [config.CircuitBreaker] consumed by providers.
// Returns nil when the breaker is disabled so callers can pass the
// result straight through without branching on Enabled.
func circuitBreakerConfig(cfg codersdk.AIBridgeConfig) *config.CircuitBreaker {
	if !cfg.CircuitBreakerEnabled.Value() {
		return nil
	}
	return &config.CircuitBreaker{
		FailureThreshold: uint32(cfg.CircuitBreakerFailureThreshold.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
		Interval:         cfg.CircuitBreakerInterval.Value(),
		Timeout:          cfg.CircuitBreakerTimeout.Value(),
		MaxRequests:      uint32(cfg.CircuitBreakerMaxRequests.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
	}
}
