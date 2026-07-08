//go:build !slim

package cli

import (
	"context"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// newAIBridgeDaemon constructs the in-memory aibridge daemon and wires
// up a subscription that hot-reloads the provider pool over the in-memory
// RPC on every ai_providers change event. The returned unsubscribe
// function tears down the subscription; callers must invoke it
// alongside Server.Close on shutdown.
//
// Reloads fetch the provider set from coderd over the in-memory DRPC
// (GetAIProviders) rather than reading the database directly, so embedded and
// standalone gateways construct providers identically. Pubsub remains the
// hot-reload trigger.
//
// SubscribeProviderReload performs a best-effort initial reload synchronously,
// so the pool is populated before this returns whenever the fetch succeeds.
// That reload blocks on srv.Client(), but the embedded daemon's connection is
// an in-memory pipe that comes up immediately, and the env seed (which holds
// the seed lock) has already completed earlier in startup, so the wait is
// negligible.
func newAIBridgeDaemon(coderAPI *coderd.API, cfg codersdk.AIBridgeConfig, reg prometheus.Registerer, metrics *aibridge.Metrics) (*aibridged.Server, func(), error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")

	providerMetrics := aibridged.NewMetrics(reg)
	tracer := coderAPI.TracerProvider.Tracer(tracing.TracerName)

	// Create an empty pool for reusable stateful [aibridge.RequestBridge]
	// instances (one per user). The reloader populates it via the initial
	// reload below.
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger.Named("pool"), metrics, tracer) // TODO: configurable size.
	if err != nil {
		return nil, nil, xerrors.Errorf("create request pool: %w", err)
	}

	// Report current key pool state per provider at scrape time.
	reg.MustRegister(keypool.NewStateCollector(pool.KeyPools))

	// Create daemon. Construct it before subscribing so the reloader can use
	// srv.Client() to fetch providers over the in-memory RPC.
	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return coderAPI.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	if err != nil {
		return nil, nil, xerrors.Errorf("start in-memory aibridge daemon: %w", err)
	}

	// Subscribe to ai_providers change events so the pool tracks the database
	// without a restart, and perform the initial reload. The reload data path
	// is the in-memory RPC.
	reloader := NewPoolRPCReloader(pool, srv.Client, cfg, logger.Named("provider-loader"), metrics, providerMetrics)
	unsubscribe, err := aibridged.SubscribeProviderReload(ctx, coderAPI.Pubsub, reloader, logger.Named("provider-reload"))
	if err != nil {
		// Without the subscription the pool can never track provider changes,
		// so fail startup rather than serve a permanently stale snapshot.
		_ = srv.Close()
		return nil, nil, xerrors.Errorf("subscribe to ai providers change channel: %w", err)
	}

	return srv, unsubscribe, nil
}

// poolRPCReloader implements [aibridged.ProviderReloader] by fetching the
// live provider set from coderd over a DRPC client and forwarding it to the
// pool. It is shared by the embedded daemon (in-memory RPC, pubsub-triggered)
// and the standalone gateway (WebSocket RPC, retried at startup) so the fetch,
// build, replace, and reload-metric accounting live in one place.
type poolRPCReloader struct {
	pool            *aibridged.CachedBridgePool
	client          func() (aibridged.DRPCClient, error)
	cfg             codersdk.AIBridgeConfig
	logger          slog.Logger
	aibridgeMetrics *aibridge.Metrics
	providerMetrics *aibridged.Metrics
}

// NewPoolRPCReloader builds an [aibridged.ProviderReloader] that fetches the
// provider set over the DRPC client returned by client and replaces pool's
// providers, recording reload metrics against providerMetrics.
func NewPoolRPCReloader(
	pool *aibridged.CachedBridgePool,
	client func() (aibridged.DRPCClient, error),
	cfg codersdk.AIBridgeConfig,
	logger slog.Logger,
	aibridgeMetrics *aibridge.Metrics,
	providerMetrics *aibridged.Metrics,
) aibridged.ProviderReloader {
	return &poolRPCReloader{
		pool:            pool,
		client:          client,
		cfg:             cfg,
		logger:          logger,
		aibridgeMetrics: aibridgeMetrics,
		providerMetrics: providerMetrics,
	}
}

func (r *poolRPCReloader) Reload(ctx context.Context) error {
	r.providerMetrics.RecordReloadAttempt()
	// r.client() blocks until the daemon is connected to coderd.
	client, err := r.client()
	if err != nil {
		return xerrors.Errorf("get ai-gateway client: %w", err)
	}
	resp, err := client.GetAIProviders(ctx, &proto.GetAIProvidersRequest{})
	if err != nil {
		// Keep the previous snapshot in place: dropping all providers
		// because the fetch failed would compound the visible failure mode
		// beyond the operator's actual misconfiguration.
		return xerrors.Errorf("fetch ai providers: %w", err)
	}
	providers, outcomes := BuildProvidersFromProto(ctx, resp.GetProviders(), r.cfg, r.logger, r.aibridgeMetrics)
	r.pool.ReplaceProviders(providers)
	r.providerMetrics.RecordReloadSuccess(outcomes)
	return nil
}

// BuildProvidersFromProto constructs the runtime [aibridge.Provider] set from
// proto provider configuration.
//
// Disabled entries produce a Provider stub with Enabled() == false so the
// bridge can answer requests targeting them with a 503 sentinel.
//
// Per-provider construction errors are logged and the offending entry is
// excluded from the returned snapshot; this keeps a single misconfigured
// provider from taking the whole daemon down. The returned outcomes mirror the
// per-provider status for metrics reporting.
func BuildProvidersFromProto(ctx context.Context, protoProviders []*proto.AIProvider, cfg codersdk.AIBridgeConfig, logger slog.Logger, metrics *aibridge.Metrics) ([]aibridge.Provider, []aibridged.ProviderOutcome) {
	providers := make([]aibridge.Provider, 0, len(protoProviders))
	outcomes := make([]aibridged.ProviderOutcome, 0, len(protoProviders))
	enabledCount := 0
	for _, pp := range protoProviders {
		spec := protoToProviderSpec(pp)
		outcome := aibridged.ProviderOutcome{
			Name: spec.Name,
			Type: string(spec.Type),
		}
		if spec.Enabled {
			enabledCount++
		}
		prov, err := buildProvider(ctx, spec, cfg, metrics)
		if err != nil {
			outcome.Status = aibridged.ProviderStatusError
			outcome.Err = err
			outcomes = append(outcomes, outcome)
			logger.Error(ctx, "skipping misconfigured ai provider",
				slog.F("provider_name", spec.Name),
				slog.F("provider_type", string(spec.Type)),
				slog.Error(err),
			)
			continue
		}
		if spec.Enabled {
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

	return providers, outcomes
}

// protoToProviderSpec maps a proto [proto.AIProvider] into the database-neutral
// [aiProviderSpec] consumed by [buildProvider]. Keys and Bedrock settings are
// only meaningful for enabled providers; disabled providers carry neither over
// the wire.
func protoToProviderSpec(pp *proto.AIProvider) aiProviderSpec {
	spec := aiProviderSpec{
		Type:    database.AIProviderType(pp.GetType()),
		Name:    pp.GetName(),
		Enabled: pp.GetEnabled(),
		BaseURL: pp.GetBaseUrl(),
		Keys:    pp.GetKeys(),
	}
	if b := pp.GetBedrock(); b != nil {
		bedrock := codersdk.NewAIProviderBedrockSettings(
			b.GetRegion(),
			b.GetAccessKey(),
			b.GetAccessKeySecret(),
			b.GetModel(),
			b.GetSmallFastModel(),
		)
		bedrock.RoleARN = b.GetRoleArn()
		bedrock.ExternalID = b.GetExternalId()
		spec.Bedrock = ptr.Ref(bedrock)
	}
	return spec
}

// aiProviderSpec is a database-neutral description of a single provider,
// carrying exactly the inputs [buildProvider] needs. The RPC path
// ([protoToProviderSpec]) maps the proto provider into this shape so the
// per-type construction logic stays in one place.
type aiProviderSpec struct {
	Type    database.AIProviderType
	Name    string
	Enabled bool
	BaseURL string
	// Keys holds bearer API keys for non-Bedrock providers.
	Keys []string
	// Bedrock holds Bedrock-specific settings when the provider targets
	// AWS Bedrock; nil otherwise.
	Bedrock *codersdk.AIProviderBedrockSettings
}

// buildProvider constructs the appropriate [aibridge.Provider] for a
// single provider spec, independent of where the spec was sourced from.
func buildProvider(ctx context.Context, spec aiProviderSpec, cfg codersdk.AIBridgeConfig, metrics *aibridge.Metrics) (aibridge.Provider, error) {
	if !spec.Enabled {
		return aibridge.NewDisabledProviderStub(spec.Name, string(spec.Type)), nil
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
	switch spec.Type {
	case database.AIProviderTypeOpenai,
		database.AIProviderTypeAzure,
		database.AIProviderTypeGoogle,
		database.AIProviderTypeOpenaiCompat,
		database.AIProviderTypeOpenrouter,
		database.AIProviderTypeVercel:
		if len(spec.Keys) == 0 && !cfg.AllowBYOK.Value() {
			return nil, xerrors.Errorf("%s provider has no api keys configured and BYOK is not enabled", spec.Type)
		}
		var pool *keypool.Pool
		if len(spec.Keys) > 0 {
			var err error
			pool, err = buildAIProviderKeyPool(spec.Name, spec.Keys, metrics)
			if err != nil {
				return nil, xerrors.Errorf("%s key pool: %w", spec.Type, err)
			}
		}
		return aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:             spec.Name,
			BaseURL:          spec.BaseURL,
			KeyPool:          pool,
			APIDumpDir:       dumpDir,
			CircuitBreaker:   cbCfg,
			SendActorHeaders: sendActorHeaders,
		}), nil

	case database.AIProviderTypeAnthropic, database.AIProviderTypeBedrock:
		bedrock := bedrockConfig(spec.BaseURL, spec.Bedrock)
		// A spec typed 'bedrock' authenticates exclusively via settings;
		// without populated Bedrock credentials it cannot make upstream
		// calls, so refuse rather than falling back to an unsigned
		// Anthropic client.
		if spec.Type == database.AIProviderTypeBedrock && bedrock == nil {
			return nil, xerrors.New("bedrock provider has no bedrock credentials configured")
		}
		// Bedrock-backed Anthropic authenticates via AWS credentials in
		// the settings blob, not bearer keys. A bearer-token Anthropic
		// without any key cannot make upstream calls.
		if bedrock == nil && len(spec.Keys) == 0 && !cfg.AllowBYOK.Value() {
			return nil, xerrors.New("anthropic provider has no api keys, no bedrock credentials, and BYOK is not enabled")
		}
		var pool *keypool.Pool
		if len(spec.Keys) > 0 {
			var err error
			pool, err = buildAIProviderKeyPool(spec.Name, spec.Keys, metrics)
			if err != nil {
				return nil, xerrors.Errorf("anthropic key pool: %w", err)
			}
		}
		return aibridge.NewAnthropicProvider(ctx, aibridge.AnthropicConfig{
			Name:             spec.Name,
			BaseURL:          spec.BaseURL,
			KeyPool:          pool,
			APIDumpDir:       dumpDir,
			CircuitBreaker:   cbCfg,
			SendActorHeaders: sendActorHeaders,
		}, bedrock)

	case database.AIProviderTypeCopilot:
		// Copilot is always BYOK; the per-user token is supplied on each
		// request via the Authorization header, so no keypool is built.
		return aibridge.NewCopilotProvider(aibridge.CopilotConfig{
			Name:           spec.Name,
			BaseURL:        spec.BaseURL,
			APIDumpDir:     dumpDir,
			CircuitBreaker: cbCfg,
		}), nil

	default:
		return nil, xerrors.Errorf("unsupported provider type: %q", spec.Type)
	}
}

// buildAIProviderKeyPool builds a [keypool.Pool]. Callers must check
// len(keys) > 0 first; keypool.New rejects empty input.
func buildAIProviderKeyPool(providerName string, keys []string, metrics *aibridge.Metrics) (*keypool.Pool, error) {
	return keypool.New(providerName, keys, quartz.NewReal(), metrics)
}

// bedrockConfig returns nil when the settings are absent or when the
// Bedrock fields are not actually configured. The provider's BaseURL is
// the generic upstream endpoint and is always non-empty, so it cannot
// serve as a Bedrock detection signal; gate on the settings alone via
// [codersdk.AIProviderBedrockSettings.IsConfigured].
func bedrockConfig(baseURL string, bedrock *codersdk.AIProviderBedrockSettings) *aibridge.AWSBedrockConfig {
	if bedrock == nil {
		return nil
	}
	bedrockSettings := *bedrock
	if !bedrockSettings.IsConfigured() {
		return nil
	}
	accessKey := ptr.NilToEmpty(bedrockSettings.AccessKey)
	accessKeySecret := ptr.NilToEmpty(bedrockSettings.AccessKeySecret)
	return &aibridge.AWSBedrockConfig{
		BaseURL:         baseURL,
		Region:          bedrockSettings.Region,
		AccessKey:       accessKey,
		AccessKeySecret: accessKeySecret,
		Model:           bedrockSettings.Model,
		SmallFastModel:  bedrockSettings.SmallFastModel,
		RoleARN:         bedrockSettings.RoleARN,
		ExternalID:      bedrockSettings.ExternalID,
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
