package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	aibridgeutils "github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
	auditor audit.Auditor,
	logger slog.Logger,
) error {
	desired, err := providersFromEnv(cfg)
	if err != nil {
		return xerrors.Errorf("compute providers from env: %w", err)
	}
	if len(desired) == 0 {
		return nil
	}

	// All of the work runs as the system actor so that audit entries
	// are attributed to the deployment rather than a user, and so
	// that dbauthz allows the writes. There is no user-driven request
	// here; this only runs at server startup before the API is
	// serving traffic.
	//nolint:gocritic // server startup, no user actor available
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	return db.InTx(func(tx database.Store) error {
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
		byName := make(map[string]database.AIProvider, len(all))
		for _, row := range all {
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
				existingDP := desiredAIProvider{
					Type:    existing.Type,
					BaseURL: existing.BaseUrl,
					Bedrock: existingSettings.Bedrock,
				}
				existingHash := computeProviderHash(existingDP.canonical())
				if existingHash == dp.Hash {
					continue
				}
				return xerrors.Errorf("AI provider %q is managed via the database and no longer reads from environment variables; remove the corresponding CODER_AIBRIDGE_* env vars and manage the provider through the API", dp.Name)
			}

			row, err := tx.InsertAIProvider(sysCtx, database.InsertAIProviderParams{
				ID:            uuid.New(),
				Type:          dp.Type,
				Name:          dp.Name,
				DisplayName:   sql.NullString{String: dp.Name, Valid: true},
				Enabled:       true,
				BaseUrl:       dp.BaseURL,
				Settings:      settings,
				SettingsKeyID: sql.NullString{},
			})
			if err != nil {
				return xerrors.Errorf("insert ai provider %q: %w", dp.Name, err)
			}

			audit.BackgroundAudit(sysCtx, &audit.BackgroundAuditParams[database.AIProvider]{
				Audit:  auditor,
				Log:    logger,
				Action: database.AuditActionCreate,
				New:    row,
			})

			// Insert one ai_provider_keys row per env-supplied key.
			now := time.Now().UTC()
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
				// Mask the plaintext key before it enters the audit
				// pipeline; the audit policy on api_key relies on the
				// masked rendering so plaintext never reaches a backend.
				auditRow := keyRow
				auditRow.APIKey = aibridgeutils.MaskSecret(auditRow.APIKey)
				audit.BackgroundAudit(sysCtx, &audit.BackgroundAuditParams[database.AIProviderKey]{
					Audit:  auditor,
					Log:    logger,
					Action: database.AuditActionCreate,
					New:    auditRow,
				})
			}

			logger.Info(sysCtx, "seeded ai provider from environment",
				slog.F("name", dp.Name),
				slog.F("type", string(dp.Type)),
				slog.F("key_count", len(dp.Keys)),
			)
		}
		return nil
	}, nil)
}

// canonicalAIProvider is the shape we hash to detect drift between the
// configured environment and the row stored in the database. The fields
// we hash are exactly the operator-controllable inputs that affect
// runtime behavior. Credentials are intentionally NOT part of the hash
// so operators can rotate them via the API without forcing a server
// restart. This applies to both bearer API keys (stored in
// ai_provider_keys) and to Bedrock access key/secret pairs (stored in
// the settings blob because Bedrock authenticates via settings rather
// than a bearer token).
type canonicalAIProvider struct {
	Type                  string `json:"type"`
	BaseURL               string `json:"base_url"`
	BedrockRegion         string `json:"bedrock_region"`
	BedrockModel          string `json:"bedrock_model"`
	BedrockSmallFastModel string `json:"bedrock_small_fast_model"`
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
		c.BedrockModel = d.Bedrock.Model
		c.BedrockSmallFastModel = d.Bedrock.SmallFastModel
	}
	return c
}

func computeProviderHash(c canonicalAIProvider) string {
	// json.Marshal is deterministic for structs because field order is
	// fixed, but we still sort the resulting JSON via canonical struct
	// shape rather than maps to keep it deterministic and explicit.
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// providersFromEnv normalizes the deployment-values AI Bridge config
// (legacy single-provider env vars and indexed CODER_AIBRIDGE_PROVIDER_<N>_*
// env vars) into the deduplicated set of providers we want present in
// the database. Conflicts between legacy and indexed providers under
// the same canonical name are surfaced as errors.
func providersFromEnv(cfg codersdk.AIBridgeConfig) ([]desiredAIProvider, error) {
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
			Type:    database.AiProviderTypeOpenai,
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
	bedrock := codersdk.AIProviderBedrockSettings{
		Region:         cfg.LegacyBedrock.Region.String(),
		Model:          cfg.LegacyBedrock.Model.String(),
		SmallFastModel: cfg.LegacyBedrock.SmallFastModel.String(),
	}
	if key := cfg.LegacyBedrock.AccessKey.String(); key != "" {
		bedrock.AccessKey = ptr.Ref(key)
	}
	if secret := cfg.LegacyBedrock.AccessKeySecret.String(); secret != "" {
		bedrock.AccessKeySecret = ptr.Ref(secret)
	}
	hasAnthropicKey := cfg.LegacyAnthropic.Key.String() != ""
	hasLegacyBedrock := bedrock.IsConfigured()
	if hasAnthropicKey || hasLegacyBedrock {
		dp := desiredAIProvider{
			Name:    aibridge.ProviderAnthropic,
			Type:    database.AiProviderTypeAnthropic,
			BaseURL: cfg.LegacyAnthropic.BaseURL.String(),
		}
		if hasLegacyBedrock {
			dp.Bedrock = &bedrock
		} else {
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
		switch p.Type {
		case aibridge.ProviderOpenAI:
			dp.Type = database.AiProviderTypeOpenai
		case aibridge.ProviderAnthropic:
			dp.Type = database.AiProviderTypeAnthropic
		default:
			// Skip other types (e.g. copilot) until they are added
			// to the database enum.
			continue
		}

		dp.BaseURL = p.BaseURL
		// Bedrock fields only apply to Anthropic. Detection goes
		// through AIProviderBedrockSettings.IsConfigured() so the
		// legacy and indexed paths agree on what counts as a Bedrock
		// provider.
		isBedrock := false
		if dp.Type == database.AiProviderTypeAnthropic {
			bedrock := codersdk.AIProviderBedrockSettings{
				Region:         p.BedrockRegion,
				Model:          p.BedrockModel,
				SmallFastModel: p.BedrockSmallFastModel,
			}
			if len(p.BedrockAccessKeys) > 0 && p.BedrockAccessKeys[0] != "" {
				bedrock.AccessKey = ptr.Ref(p.BedrockAccessKeys[0])
			}
			if len(p.BedrockAccessKeySecrets) > 0 && p.BedrockAccessKeySecrets[0] != "" {
				bedrock.AccessKeySecret = ptr.Ref(p.BedrockAccessKeySecrets[0])
			}
			isBedrock = bedrock.IsConfigured()
			if isBedrock {
				dp.Bedrock = &bedrock
			}
		}
		// Non-Bedrock providers carry their bearer keys in
		// ai_provider_keys. Bedrock providers authenticate via the
		// settings blob and have no keys; cli/server.go rejects
		// configs that set both before we get here.
		if !isBedrock {
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
	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	sort.Strings(names)
	res := make([]desiredAIProvider, 0, len(out))
	for _, name := range names {
		res = append(res, out[name])
	}
	return res, nil
}
