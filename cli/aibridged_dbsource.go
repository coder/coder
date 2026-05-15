//go:build !slim

package cli

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// buildProvidersFromDB constructs the list of aibridge providers from
// rows in the ai_providers table along with their associated keys.
// The deployment-values config is still consulted for circuit-breaker
// and BYOK-style settings that are global rather than per-provider.
//
// keysByProvider maps each provider's UUID to its ordered list of
// bearer keys (oldest first). Bedrock providers must map to an empty
// slice; non-Bedrock providers with an empty slice are skipped with a
// warning so the deployment can still serve the rest of the pool.
func buildProvidersFromDB(
	rows []database.AIProvider,
	keysByProvider map[string][]database.AIProviderKey,
	cfg codersdk.AIBridgeConfig,
	logger slog.Logger,
) ([]aibridge.Provider, error) {
	var cbConfig *config.CircuitBreaker
	if cfg.CircuitBreakerEnabled.Value() {
		cbConfig = &config.CircuitBreaker{
			FailureThreshold: uint32(cfg.CircuitBreakerFailureThreshold.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
			Interval:         cfg.CircuitBreakerInterval.Value(),
			Timeout:          cfg.CircuitBreakerTimeout.Value(),
			MaxRequests:      uint32(cfg.CircuitBreakerMaxRequests.Value()), //nolint:gosec // Validated by serpent.Validate in deployment options.
		}
	}

	out := make([]aibridge.Provider, 0, len(rows))
	for _, row := range rows {
		var settings codersdk.AIProviderSettings
		if row.Settings.Valid && row.Settings.String != "" {
			if err := json.Unmarshal([]byte(row.Settings.String), &settings); err != nil {
				return nil, xerrors.Errorf("decode settings for %q: %w", row.Name, err)
			}
		}

		keys := keysByProvider[row.ID.String()]
		// firstKey holds the operator-preferred primary. The seed
		// and CRUD layers preserve insertion order via monotonically
		// increasing created_at, and GetAIProviderKeysByProviderID
		// returns keys ordered by created_at ASC, so the first
		// element is the primary.
		firstKey := ""
		if len(keys) > 0 {
			firstKey = keys[0].APIKey
		}

		// aibridge currently has native support for OpenAI and
		// Anthropic only. The other ai_provider_type values
		// (azure, google, openai-compat, openrouter, vercel) are
		// routed through the OpenAI fantasy client because chatd
		// configures these providers against their OpenAI-compatible
		// endpoints. Bedrock providers route through the Anthropic
		// fantasy client with a Bedrock discriminator in Settings;
		// native gateway-side support for any of these arrives later.
		switch row.Type {
		case database.AiProviderTypeOpenai,
			database.AiProviderTypeAzure,
			database.AiProviderTypeGoogle,
			database.AiProviderTypeOpenaiCompat,
			database.AiProviderTypeOpenrouter,
			database.AiProviderTypeVercel:
			if firstKey == "" {
				logger.Warn(context.Background(),
					"skipping enabled AI Bridge provider with no API keys; add one via /api/v2/aibridge/providers/{name}/keys",
					slog.F("name", row.Name),
					slog.F("type", string(row.Type)),
				)
				continue
			}
			out = append(out, aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
				Name:             row.Name,
				BaseURL:          row.BaseUrl,
				Key:              firstKey,
				CircuitBreaker:   cbConfig,
				SendActorHeaders: cfg.SendActorHeaders.Value(),
			}))
		case database.AiProviderTypeAnthropic, database.AiProviderTypeBedrock:
			var bedrock *aibridge.AWSBedrockConfig
			if settings.Bedrock != nil {
				bedrock = &aibridge.AWSBedrockConfig{
					Region:          settings.Bedrock.Region,
					AccessKey:       ptr.NilToEmpty(settings.Bedrock.AccessKey),
					AccessKeySecret: ptr.NilToEmpty(settings.Bedrock.AccessKeySecret),
					Model:           settings.Bedrock.Model,
					SmallFastModel:  settings.Bedrock.SmallFastModel,
				}
			}
			// A row typed 'bedrock' authenticates exclusively via
			// settings; if those have not been populated yet (e.g.
			// the row was just migrated and the operator has not
			// entered Bedrock credentials), skip it rather than
			// silently fall back to an unsigned Anthropic client.
			if row.Type == database.AiProviderTypeBedrock && bedrock == nil {
				logger.Warn(context.Background(),
					"skipping enabled bedrock AI Bridge provider with no Bedrock settings; configure access credentials via the provider settings",
					slog.F("name", row.Name),
				)
				continue
			}
			// Bedrock providers authenticate via settings and may
			// have zero ai_provider_keys; that is the expected
			// configuration. Non-Bedrock Anthropic providers must
			// have at least one bearer key to be usable.
			if bedrock == nil && firstKey == "" {
				logger.Warn(context.Background(),
					"skipping enabled AI Bridge provider with no API keys; add one via /api/v2/aibridge/providers/{name}/keys",
					slog.F("name", row.Name),
					slog.F("type", string(row.Type)),
				)
				continue
			}
			out = append(out, aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
				Name:             row.Name,
				BaseURL:          row.BaseUrl,
				Key:              firstKey,
				CircuitBreaker:   cbConfig,
				SendActorHeaders: cfg.SendActorHeaders.Value(),
			}, bedrock))
		default:
			return nil, xerrors.Errorf("unknown provider type %q for provider %q", row.Type, row.Name)
		}
	}
	return out, nil
}

// loadProvidersFromDB returns the current set of enabled, non-deleted
// AI Bridge providers from the database, converted into aibridge
// Provider instances. Each provider's API keys are loaded from
// ai_provider_keys in created_at order so the first key per provider
// is the operator-preferred primary.
func loadProvidersFromDB(
	ctx context.Context,
	db database.Store,
	cfg codersdk.AIBridgeConfig,
	logger slog.Logger,
) ([]aibridge.Provider, error) {
	// The queries are authorized via dbauthz on ResourceAibridgeProvider.
	// At reload time we run as the system actor because there is no
	// user-facing request driving the reload.
	//nolint:gocritic // server-side reload, no user actor available
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	rows, err := db.GetAIProviders(sysCtx, database.GetAIProvidersParams{})
	if err != nil {
		return nil, xerrors.Errorf("get enabled ai providers: %w", err)
	}
	keysByProvider := make(map[string][]database.AIProviderKey, len(rows))
	for _, row := range rows {
		keys, err := db.GetAIProviderKeysByProviderID(sysCtx, row.ID)
		if err != nil {
			return nil, xerrors.Errorf("get ai provider keys for %q: %w", row.Name, err)
		}
		keysByProvider[row.ID.String()] = keys
	}
	return buildProvidersFromDB(rows, keysByProvider, cfg, logger)
}
