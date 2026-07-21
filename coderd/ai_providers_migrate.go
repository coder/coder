package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	aibridgeutils "github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

// SeedAIProvidersFromEnv reconciles the deployment's environment-
// derived AI provider configuration with rows in the ai_providers
// table at server startup. Concurrent server starts are serialized via a
// Postgres advisory lock; rows that already exist with a matching
// canonical hash are left alone, missing rows are inserted, and rows
// whose hash differs from the env-derived value cause startup to fail
// with a descriptive error.
//
// API keys derived from env vars are inserted into ai_provider_keys at
// the time the provider row is first created. We do NOT add env-sourced
// keys to a provider that already has keys, because operators may have
// added or rotated keys via the API after the initial seed and we do
// not want to clobber that state on every restart.
//
// Only env-sourced providers participate in the seed; rows created via
// the HTTP CRUD endpoints are not affected.
//
// Audit entries are recorded via the system actor for any inserts.
func SeedAIProvidersFromEnv(
	ctx context.Context,
	db database.Store,
	cfg codersdk.AIBridgeConfig,
	logger slog.Logger,
) error {
	desired, err := providersFromEnv(ctx, cfg, logger)
	if err != nil {
		return xerrors.Errorf("compute providers from env: %w", err)
	}
	if len(desired) == 0 {
		return nil
	}

	// Audit entries are attributed to the deployment rather than a user.
	//nolint:gocritic // server startup, no user actor available
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	// Collect inserted rows inside the transaction and emit audit
	// entries only after the transaction commits. The auditor writes
	// through the outer db handle, so emitting inside InTx would leave
	// phantom audit rows if the transaction later rolls back.
	var (
		insertedProviders []database.AIProvider
		insertedKeys      []database.AIProviderKey
	)

	err = db.InTx(func(tx database.Store) error {
		insertedProviders = insertedProviders[:0]
		insertedKeys = insertedKeys[:0]

		// Acquire the advisory lock. The lock is released when the
		// transaction ends.
		if err := tx.AcquireLock(sysCtx, database.LockIDAIProvidersEnvSeed); err != nil {
			return xerrors.Errorf("acquire ai providers env seed lock: %w", err)
		}

		// Load every provider (including soft-deleted and disabled rows)
		// once so we can decide insert vs. skip vs. drift per desired
		// row without a query per name.
		all, err := tx.GetAIProviders(sysCtx, database.GetAIProvidersParams{
			IncludeDeleted:  true,
			IncludeDisabled: true,
		})
		if err != nil {
			return xerrors.Errorf("load ai providers: %w", err)
		}
		// Prefer the live row when a soft-deleted row shares its name.
		byName := make(map[string]database.AIProvider, len(all))
		for _, row := range all {
			if existing, ok := byName[row.Name]; ok && !existing.Deleted && row.Deleted {
				continue
			}
			byName[row.Name] = row
		}

		for _, dp := range desired {
			settings, err := encodeAIProviderSettings(codersdk.AIProviderSettings{Bedrock: dp.Bedrock})
			if err != nil {
				return xerrors.Errorf("encode settings for %q: %w", dp.Name, err)
			}

			existing, found := byName[dp.Name]
			switch {
			case found && existing.Deleted:
				// The provider was created here, then explicitly
				// deleted by an operator. We do NOT re-create it
				// from env; the operator's deletion is sticky.
				logger.Warn(sysCtx, "skipping env-seeded ai provider that was previously soft-deleted",
					slog.F("name", dp.Name))
				continue
			case found:
				existingSettings, err := db2sdk.AIProviderSettings(existing.Settings)
				if err != nil {
					return xerrors.Errorf("decode existing settings for %q: %w", dp.Name, err)
				}
				// Load existing bearer keys so the canonical hash
				// includes credentials for comparison.
				existingKeyRows, err := tx.GetAIProviderKeysByProviderID(sysCtx, existing.ID)
				if err != nil {
					return xerrors.Errorf("load existing keys for %q: %w", dp.Name, err)
				}
				existingKeys := make([]string, 0, len(existingKeyRows))
				for _, k := range existingKeyRows {
					existingKeys = append(existingKeys, k.APIKey)
				}
				// Use the canonical type so that a row promoted from
				// type=anthropic to type=bedrock by the startup backfill
				// is not mistaken for drift on the next startup.
				existingType := existing.Type
				if existingSettings.Bedrock != nil && existing.Type == database.AIProviderTypeAnthropic {
					existingType = database.AIProviderTypeBedrock
				}
				existingDP := desiredAIProvider{
					Type:    existingType,
					BaseURL: existing.BaseUrl,
					Bedrock: existingSettings.Bedrock,
					Keys:    existingKeys,
				}
				existingHash := computeProviderHash(existingDP.canonical())
				if existingHash == dp.Hash {
					continue
				}
				return xerrors.Errorf("AI provider %q already exists in the database and differs from the current environment configuration; update the provider through the API or remove the CODER_AIBRIDGE_* (legacy) / CODER_AI_GATEWAY_* env vars to stop seeding it", dp.Name)
			}

			row, err := tx.InsertAIProvider(sysCtx, database.InsertAIProviderParams{
				ID:            uuid.New(),
				Type:          dp.Type,
				Name:          dp.Name,
				DisplayName:   sql.NullString{String: dp.Name, Valid: true},
				Icon:          "",
				Enabled:       true,
				BaseUrl:       dp.BaseURL,
				Settings:      settings,
				SettingsKeyID: sql.NullString{},
			})
			if err != nil {
				return xerrors.Errorf("insert ai provider %q: %w", dp.Name, err)
			}
			insertedProviders = append(insertedProviders, row)

			// Insert one ai_provider_keys row per env-supplied key.
			now := dbtime.Now()
			for _, key := range dp.Keys {
				if key == "" {
					continue
				}
				keyRow, err := tx.InsertAIProviderKey(sysCtx, database.InsertAIProviderKeyParams{
					ID:          uuid.New(),
					ProviderID:  row.ID,
					APIKey:      key,
					ApiKeyKeyID: sql.NullString{},
					CreatedAt:   now,
					UpdatedAt:   now,
				})
				if err != nil {
					return xerrors.Errorf("insert ai provider key for %q: %w", dp.Name, err)
				}
				insertedKeys = append(insertedKeys, keyRow)
			}

			logger.Info(sysCtx, "seeded ai provider from environment",
				slog.F("name", dp.Name),
				slog.F("type", string(dp.Type)),
				slog.F("key_count", len(dp.Keys)),
			)
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}

	for _, row := range insertedProviders {
		logger.Info(sysCtx, "env-seeded ai provider",
			slog.F("provider_id", row.ID),
			slog.F("name", row.Name),
			slog.F("type", row.Type),
			slog.F("base_url", row.BaseUrl),
		)
	}
	for _, keyRow := range insertedKeys {
		logger.Info(sysCtx, "env-seeded ai provider key",
			slog.F("key_id", keyRow.ID),
			slog.F("provider_id", keyRow.ProviderID),
			slog.F("api_key", aibridgeutils.MaskSecret(keyRow.APIKey)),
		)
	}
	return nil
}

// canonicalAIProvider is the shape we hash to detect drift between the
// configured environment and the row stored in the database. The fields
// we hash are exactly the operator-controllable inputs that affect
// runtime behavior, including credentials.
//
// Model and SmallFastModel are excluded: they're tunables, and their
// serpent defaults shift across releases.
type canonicalAIProvider struct {
	Type          string `json:"type"`
	BaseURL       string `json:"base_url"`
	BedrockRegion string `json:"bedrock_region"`
	KeysHash      string `json:"keys_hash"`
}

// desiredAIProvider is a normalized provider description sourced from
// environment configuration that we want to materialize as a row.
type desiredAIProvider struct {
	Name string
	Type database.AIProviderType
	// BaseURL is the upstream provider's HTTP endpoint.
	BaseURL string
	// Keys is the list of API keys to seed into ai_provider_keys for
	// non-Bedrock providers. Bedrock providers have no entries here
	// because they authenticate via the encrypted settings blob.
	Keys []string
	// Bedrock holds the Bedrock-specific settings when the provider
	// targets AWS Bedrock; nil otherwise.
	Bedrock *codersdk.AIProviderBedrockSettings
	Hash    string
}

func (d desiredAIProvider) canonical() canonicalAIProvider {
	c := canonicalAIProvider{
		Type:    string(d.Type),
		BaseURL: d.BaseURL,
	}
	if d.Bedrock != nil {
		c.BedrockRegion = d.Bedrock.Region
	}
	c.KeysHash = computeKeysHash(d.Keys, d.Bedrock)
	return c
}

// computeKeysHash produces a deterministic hash over the bearer API
// keys and, for Bedrock providers, the access key and secret.
func computeKeysHash(bearerKeys []string, bedrock *codersdk.AIProviderBedrockSettings) string {
	// Collect all credential material in a deterministic order.
	// Bearer keys are sorted so reordering in env vars does not
	// trigger a false-positive drift.
	sorted := make([]string, len(bearerKeys))
	copy(sorted, bearerKeys)
	slices.Sort(sorted)

	h := sha256.New()
	for _, k := range sorted {
		_, _ = h.Write([]byte(k))
		// Separator so "ab"+"c" != "a"+"bc".
		_, _ = h.Write([]byte{0})
	}
	if bedrock != nil {
		if bedrock.AccessKey != nil {
			_, _ = h.Write([]byte(*bedrock.AccessKey))
		}
		_, _ = h.Write([]byte{0})
		if bedrock.AccessKeySecret != nil {
			_, _ = h.Write([]byte(*bedrock.AccessKeySecret))
		}
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func computeProviderHash(c canonicalAIProvider) string {
	// json.Marshal is deterministic for structs because field order is
	// fixed by the struct definition.
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// providersFromEnv normalizes the deployment-values AI Bridge config
// (legacy single-provider env vars and indexed CODER_AIBRIDGE_PROVIDER_<N>_*
// env vars) into the deduplicated set of providers we want present in
// the database. Conflicts between legacy and indexed providers under
// the same canonical name are surfaced as errors.
func providersFromEnv(ctx context.Context, cfg codersdk.AIBridgeConfig, logger slog.Logger) ([]desiredAIProvider, error) {
	out := make(map[string]desiredAIProvider)
	legacyNames := make(map[string]bool)

	addLegacy := func(name string, p desiredAIProvider) {
		out[name] = p
		legacyNames[name] = true
	}

	// Legacy OpenAI.
	if cfg.LegacyOpenAI.Key.String() != "" {
		dp := desiredAIProvider{
			Name:    aibridge.ProviderOpenAI,
			Type:    database.AIProviderTypeOpenai,
			BaseURL: cfg.LegacyOpenAI.BaseURL.String(),
			Keys:    []string{cfg.LegacyOpenAI.Key.String()},
		}
		dp.Hash = computeProviderHash(dp.canonical())
		addLegacy(aibridge.ProviderOpenAI, dp)
	}

	// Legacy Anthropic + Bedrock. Anthropic is enabled if either an
	// Anthropic key OR any Bedrock setting is explicitly configured.
	// Detection goes through AIProviderBedrockSettings.IsConfigured()
	// so the legacy and indexed paths agree on what counts as a
	// Bedrock provider.
	bedrock := codersdk.NewAIProviderBedrockSettings(
		cfg.LegacyBedrock.Region.String(),
		cfg.LegacyBedrock.AccessKey.String(),
		cfg.LegacyBedrock.AccessKeySecret.String(),
		cfg.LegacyBedrock.Model.String(),
		cfg.LegacyBedrock.SmallFastModel.String(),
	)
	hasAnthropicKey := cfg.LegacyAnthropic.Key.String() != ""
	hasLegacyBedrock := codersdk.IsBedrockConfigured(cfg.LegacyBedrock.BaseURL.String(), bedrock)
	if hasAnthropicKey || hasLegacyBedrock {
		dp := desiredAIProvider{
			Name: aibridge.ProviderAnthropic,
			Type: database.AIProviderTypeAnthropic,
		}
		if hasLegacyBedrock {
			dp.Type = database.AIProviderTypeBedrock
			if hasAnthropicKey {
				logger.Warn(ctx, "ignoring legacy Anthropic API key because Bedrock credentials are configured; Bedrock authenticates via access keys or credential chain",
					slog.F("provider", aibridge.ProviderAnthropic),
				)
			}
			// Bedrock-only deployments use CODER_AIBRIDGE_BEDROCK_BASE_URL
			// for custom VPC, FIPS, or proxy endpoints.
			dp.BaseURL = cfg.LegacyBedrock.BaseURL.String()
			dp.Bedrock = &bedrock
		} else {
			dp.BaseURL = cfg.LegacyAnthropic.BaseURL.String()
			dp.Keys = []string{cfg.LegacyAnthropic.Key.String()}
		}
		dp.Hash = computeProviderHash(dp.canonical())
		addLegacy(aibridge.ProviderAnthropic, dp)
	}

	// Indexed providers.
	for _, p := range cfg.Providers {
		name := p.Name
		if name == "" {
			name = p.Type
		}
		if name == "" {
			return nil, xerrors.Errorf("indexed AI provider must have a name or type")
		}
		// Reject invalid characters here so that bad env values
		// fail startup rather than producing a hidden runtime row.
		if !codersdk.AIProviderNameRegex.MatchString(name) {
			return nil, xerrors.Errorf("invalid AI provider name %q: must match %s", name, codersdk.AIProviderNameRegex)
		}

		dp := desiredAIProvider{
			Name: name,
		}
		providerType := database.AIProviderType(p.Type)
		if !providerType.Valid() {
			logger.Warn(ctx, "skipping indexed AI provider with unsupported type",
				slog.F("name", name),
				slog.F("type", p.Type),
			)
			continue
		}
		dp.Type = providerType

		dp.BaseURL = p.BaseURL
		// Bedrock fields apply to Anthropic and the dedicated Bedrock
		// type. Detection goes through
		// AIProviderBedrockSettings.IsConfigured() so the legacy and
		// indexed paths agree on what counts as a Bedrock provider.
		isBedrock := false
		if dp.Type == database.AIProviderTypeAnthropic || dp.Type == database.AIProviderTypeBedrock {
			var accessKey, accessKeySecret string
			if len(p.BedrockAccessKeys) > 0 {
				accessKey = p.BedrockAccessKeys[0]
			}
			if len(p.BedrockAccessKeySecrets) > 0 {
				accessKeySecret = p.BedrockAccessKeySecrets[0]
			}
			bedrock := codersdk.NewAIProviderBedrockSettings(
				p.BedrockRegion,
				accessKey,
				accessKeySecret,
				p.BedrockModel,
				p.BedrockSmallFastModel,
			)
			isBedrock = codersdk.IsBedrockConfigured(p.BedrockBaseURL, bedrock)
			if isBedrock {
				dp.Bedrock = &bedrock
				// Always overwrite the generic BaseURL so removing
				// BASE_URL later doesn't trigger drift. Empty is fine:
				// the runtime derives the endpoint from the region.
				dp.BaseURL = p.BedrockBaseURL
			}
		}
		// Non-Bedrock, non-Copilot providers carry their bearer keys in
		// ai_provider_keys. Bedrock providers authenticate via the
		// settings blob; Copilot providers use request-time GitHub
		// OAuth tokens. cli/server.go rejects configs that set Bedrock
		// alongside bearer keys before we get here.
		switch {
		case isBedrock:
			if len(p.Keys) > 0 {
				logger.Warn(ctx, "ignoring bearer keys configured on Bedrock AI provider; Bedrock authenticates via access keys or credential chain",
					slog.F("name", name),
					slog.F("ignored_key_count", len(p.Keys)),
				)
			}
		case dp.Type == database.AIProviderTypeCopilot:
			if len(p.Keys) > 0 {
				logger.Warn(ctx, "ignoring bearer keys configured on Copilot AI provider; Copilot authenticates via request-time GitHub OAuth tokens",
					slog.F("name", name),
					slog.F("ignored_key_count", len(p.Keys)),
				)
			}
		default:
			dp.Keys = append(dp.Keys, p.Keys...)
		}

		dp.Hash = computeProviderHash(dp.canonical())
		if legacyNames[name] {
			return nil, xerrors.Errorf("indexed AI provider %q conflicts with the legacy env var of the same name; remove one or the other", name)
		}
		if existing, ok := out[name]; ok {
			if existing.Hash != dp.Hash {
				return nil, xerrors.Errorf("duplicate AI provider name %q with conflicting fields", name)
			}
			continue
		}
		out[name] = dp
	}

	// Stable order so audit log entries are deterministic across
	// restarts, which makes comparison in tests trivial.
	res := make([]desiredAIProvider, 0, len(out))
	for _, name := range slices.Sorted(maps.Keys(out)) {
		res = append(res, out[name])
	}
	return res, nil
}
