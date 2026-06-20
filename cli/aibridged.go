//go:build !slim

package cli

import (
	"context"
	"slices"

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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// newAIBridgeDaemon constructs the in-memory aibridge daemon and wires
// up a subscription that hot-reloads the provider pool from the
// database on every ai_providers change event. The returned unsubscribe
// function tears down the subscription; callers must invoke it
// alongside Server.Close on shutdown.
func newAIBridgeDaemon(coderAPI *coderd.API, providers []aibridge.Provider, cfg codersdk.AIBridgeConfig, reg prometheus.Registerer, metrics *aibridge.Metrics) (*aibridged.Server, func(), error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")

	providerMetrics := aibridged.NewMetrics(reg)
	tracer := coderAPI.TracerProvider.Tracer(tracing.TracerName)

	// Create pool for reusable stateful [aibridge.RequestBridge] instances (one per user).
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool"), metrics, tracer) // TODO: configurable size.
	if err != nil {
		return nil, nil, xerrors.Errorf("create request pool: %w", err)
	}

	// Report current key pool state per provider at scrape time.
	reg.MustRegister(keypool.NewStateCollector(pool.KeyPools))

	// Subscribe to ai_providers change events so the pool tracks the
	// database without a restart. The boot-time `providers` snapshot
	// derives from env config and serves as a fallback if the database
	// load fails inside the reloader.
	reloader := &poolDBReloader{
		pool:            pool,
		db:              coderAPI.Database,
		cfg:             cfg,
		logger:          logger.Named("provider-loader"),
		aibridgeMetrics: metrics,
		providerMetrics: providerMetrics,
	}
	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, coderAPI.Pubsub, reloader, logger.Named("provider-reload"))
	if err != nil {
		// Pool is still usable with the boot-time snapshot; subscription
		// failure is logged but not fatal so the daemon still serves.
		logger.Warn(ctx, "subscribe to ai providers change channel", slog.Error(err))
		unsubscribe = func() {}
	}

	// Create daemon.
	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return coderAPI.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	if err != nil {
		unsubscribe()
		return nil, nil, xerrors.Errorf("start in-memory aibridge daemon: %w", err)
	}
	return srv, unsubscribe, nil
}

// poolDBReloader implements [aibridged.ProviderReloader] by loading
// the live provider set from the database and forwarding it to the
// pool.
type poolDBReloader struct {
	pool            *aibridged.CachedBridgePool
	db              database.Store
	cfg             codersdk.AIBridgeConfig
	logger          slog.Logger
	aibridgeMetrics *aibridge.Metrics
	providerMetrics *aibridged.Metrics
}

func (r *poolDBReloader) Reload(ctx context.Context) error {
	r.providerMetrics.RecordReloadAttempt()
	providers, outcomes, err := BuildProviders(ctx, r.db, r.cfg, r.logger, r.aibridgeMetrics)
	if err != nil {
		// Keep the previous snapshot in place: dropping all providers
		// because the DB read failed would compound the visible failure
		// mode beyond the operator's actual misconfiguration.
		return xerrors.Errorf("load ai providers from database: %w", err)
	}
	r.pool.ReplaceProviders(providers)
	r.providerMetrics.RecordReloadSuccess(outcomes)
	return nil
}

// BuildProviders loads all ai_providers rows (enabled and disabled),
// attaches keys to enabled rows, and constructs the equivalent
// [aibridge.Provider] instances. The database is the single source of
// truth for runtime provider configuration.
//
// Disabled rows produce a Provider stub with Enabled() == false so the
// bridge can answer requests targeting them with a 503 sentinel.
//
// Per-provider construction errors are logged and the offending row is
// excluded from the returned snapshot; only a failure of the DB query
// itself is propagated. This keeps a single misconfigured row from
// taking the whole daemon down.
func BuildProviders(ctx context.Context, db database.Store, cfg codersdk.AIBridgeConfig, logger slog.Logger, metrics *aibridge.Metrics) ([]aibridge.Provider, []aibridged.ProviderOutcome, error) {
	//nolint:gocritic // AsAIBridged has a minimal permission set for this purpose.
	authCtx := dbauthz.AsAIBridged(ctx)

	var rows []database.AIProvider
	keysByProvider := make(map[uuid.UUID][]database.AIProviderKey)

	// Wrap both queries in a read-only transaction so the provider list
	// and the key list are consistent with each other.
	err := db.InTx(func(tx database.Store) error {
		var err error
		rows, err = tx.GetAIProviders(authCtx, database.GetAIProvidersParams{
			IncludeDisabled: true,
		})
		if err != nil {
			return xerrors.Errorf("load ai providers: %w", err)
		}

		if len(rows) == 0 {
			return nil
		}

		// Load keys only for the enabled providers to avoid materializing
		// secrets for disabled rows.
		ids := make([]uuid.UUID, 0, len(rows))
		for _, r := range rows {
			if !r.Enabled {
				continue
			}
			ids = append(ids, r.ID)
		}
		if len(ids) == 0 {
			return nil
		}
		keyRows, err := tx.GetAIProviderKeysByProviderIDs(authCtx, ids)
		if err != nil {
			return xerrors.Errorf("load ai provider keys: %w", err)
		}
		for _, k := range keyRows {
			keysByProvider[k.ProviderID] = append(keysByProvider[k.ProviderID], k)
		}
		return nil
	}, &database.TxOptions{ReadOnly: true, TxIdentifier: "build_ai_providers"})
	if err != nil {
		return nil, nil, err
	}

	providers := make([]aibridge.Provider, 0, len(rows))
	outcomes := make([]aibridged.ProviderOutcome, 0, len(rows))
	enabledCount := 0
	for _, row := range rows {
		outcome := aibridged.ProviderOutcome{
			Name: row.Name,
			Type: string(row.Type),
		}
		if row.Enabled {
			enabledCount++
		}
		prov, err := buildAIProviderFromRow(ctx, row, keysByProvider[row.ID], cfg, metrics)
		if err != nil {
			outcome.Status = aibridged.ProviderStatusError
			outcome.Err = err
			outcomes = append(outcomes, outcome)
			logger.Error(ctx, "skipping misconfigured ai provider",
				slog.F("provider_id", row.ID),
				slog.F("provider_name", row.Name),
				slog.F("provider_type", string(row.Type)),
				slog.Error(err),
			)
			continue
		}
		if row.Enabled {
			outcome.Status = aibridged.ProviderStatusEnabled
		} else {
			outcome.Status = aibridged.ProviderStatusDisabled
		}
		outcomes = append(outcomes, outcome)
		providers = append(providers, prov)
	}

	if enabledCount > 0 && !slices.ContainsFunc(providers, func(p aibridge.Provider) bool { return p.Enabled() }) {
		logger.Warn(ctx, "all enabled ai providers failed to build; only disabled providers remain")
	}

	return providers, outcomes, nil
}

// buildAIProviderFromRow decodes the settings blob and constructs the
// appropriate [aibridge.Provider] for a single ai_providers row.
// Disabled rows return a Provider stub carrying only Name and
// Disabled: true; settings decode, key loading, and credential checks
// are skipped because the provider will never call upstream.
func buildAIProviderFromRow(
	ctx context.Context,
	row database.AIProvider,
	keys []database.AIProviderKey,
	cfg codersdk.AIBridgeConfig,
	metrics *aibridge.Metrics,
) (aibridge.Provider, error) {
	if !row.Enabled {
		return disabledProviderFromRow(row)
	}

	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return nil, xerrors.Errorf("decode settings: %w", err)
	}

	cbCfg := circuitBreakerConfig(cfg)
	sendActorHeaders := cfg.SendActorHeaders.Value()
	dumpDir := cfg.APIDumpDir.Value()

	// aibridge currently has native support for OpenAI and Anthropic
	// only. The other ai_provider_type values (azure, google,
	// openai-compat, openrouter, vercel) route through the OpenAI
	// provider because chatd configures them against their
	// OpenAI-compatible endpoints. Bedrock routes through the Anthropic
	// provider with a Bedrock discriminator in Settings.
	switch row.Type {
	case database.AIProviderTypeOpenai,
		database.AIProviderTypeAzure,
		database.AIProviderTypeGoogle,
		database.AIProviderTypeOpenaiCompat,
		database.AIProviderTypeOpenrouter,
		database.AIProviderTypeVercel:
		if len(keys) == 0 && !cfg.AllowBYOK.Value() {
			return nil, xerrors.Errorf("%s provider has no api keys configured and BYOK is not enabled", row.Type)
		}
		var pool *keypool.Pool
		if len(keys) > 0 {
			var err error
			pool, err = buildAIProviderKeyPool(row.Name, keys, metrics)
			if err != nil {
				return nil, xerrors.Errorf("%s key pool: %w", row.Type, err)
			}
		}
		return aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:             row.Name,
			BaseURL:          row.BaseUrl,
			KeyPool:          pool,
			APIDumpDir:       dumpDir,
			CircuitBreaker:   cbCfg,
			SendActorHeaders: sendActorHeaders,
		}), nil

	case database.AIProviderTypeAnthropic, database.AIProviderTypeBedrock:
		bedrock := bedrockConfigFromRow(row, settings)
		// A row typed 'bedrock' authenticates exclusively via settings;
		// without populated Bedrock credentials it cannot make upstream
		// calls, so refuse rather than falling back to an unsigned
		// Anthropic client.
		if row.Type == database.AIProviderTypeBedrock && bedrock == nil {
			return nil, xerrors.New("bedrock provider has no bedrock credentials configured")
		}
		// Bedrock-backed Anthropic authenticates via AWS credentials in
		// the settings blob, not the api_keys table. A bearer-token
		// Anthropic without any key cannot make upstream calls.
		if bedrock == nil && len(keys) == 0 && !cfg.AllowBYOK.Value() {
			return nil, xerrors.New("anthropic provider has no api keys, no bedrock credentials, and BYOK is not enabled")
		}
		var pool *keypool.Pool
		if len(keys) > 0 {
			var err error
			pool, err = buildAIProviderKeyPool(row.Name, keys, metrics)
			if err != nil {
				return nil, xerrors.Errorf("anthropic key pool: %w", err)
			}
		}
		return aibridge.NewAnthropicProvider(ctx, aibridge.AnthropicConfig{
			Name:             row.Name,
			BaseURL:          row.BaseUrl,
			KeyPool:          pool,
			APIDumpDir:       dumpDir,
			CircuitBreaker:   cbCfg,
			SendActorHeaders: sendActorHeaders,
		}, bedrock)

	case database.AIProviderTypeCopilot:
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

// disabledProviderFromRow builds a Provider stub for a disabled row.
// Using provider.DisabledStub rather than a concrete provider avoids
// duplicating the row.Type switch and ensures that a new AIProviderType
// value is automatically handled without requiring a matching case here.
func disabledProviderFromRow(row database.AIProvider) (aibridge.Provider, error) {
	return aibridge.NewDisabledProviderStub(row.Name, string(row.Type)), nil
}

// buildAIProviderKeyPool builds a [keypool.Pool]. Callers must check
// len(keys) > 0 first; keypool.New rejects empty input.
func buildAIProviderKeyPool(providerName string, keys []database.AIProviderKey, metrics *aibridge.Metrics) (*keypool.Pool, error) {
	raw := make([]string, 0, len(keys))
	for _, k := range keys {
		raw = append(raw, k.APIKey)
	}
	return keypool.New(providerName, raw, quartz.NewReal(), metrics)
}

// bedrockConfigFromRow returns nil when the settings have no Bedrock
// discriminator or when the Bedrock fields are not actually configured.
// The provider row's BaseUrl is the generic upstream endpoint and is
// always non-empty, so it cannot serve as a Bedrock detection signal;
// gate on the settings blob alone via [codersdk.AIProviderBedrockSettings.IsConfigured].
func bedrockConfigFromRow(row database.AIProvider, settings codersdk.AIProviderSettings) *aibridge.AWSBedrockConfig {
	if settings.Bedrock == nil {
		return nil
	}
	bedrockSettings := *settings.Bedrock
	if !bedrockSettings.IsConfigured() {
		return nil
	}
	accessKey := ptr.NilToEmpty(bedrockSettings.AccessKey)
	accessKeySecret := ptr.NilToEmpty(bedrockSettings.AccessKeySecret)
	return &aibridge.AWSBedrockConfig{
		BaseURL:         row.BaseUrl,
		Region:          bedrockSettings.Region,
		AccessKey:       accessKey,
		AccessKeySecret: accessKeySecret,
		Model:           bedrockSettings.Model,
		SmallFastModel:  bedrockSettings.SmallFastModel,
		RoleARN:         bedrockSettings.RoleARN,
	}
}

// circuitBreakerConfig returns nil when the breaker is disabled.
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
