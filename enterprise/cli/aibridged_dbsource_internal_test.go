//go:build !slim

package cli

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// TestBuildProvidersFromDB_ChatdTypes locks in the runtime mapping for
// the chatd-side ai_provider_type values. Native gateway-side support
// is future work; until then the new non-Bedrock types route through
// aibridge's OpenAI client, and 'bedrock' routes through the Anthropic
// client with the Bedrock discriminator in Settings.
func TestBuildProvidersFromDB_ChatdTypes(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	cfg := codersdk.AIBridgeConfig{}

	// openaiCompatTypes covers every type that should resolve to the
	// OpenAI fantasy client today. Each row must carry a bearer key in
	// ai_provider_keys; without one the provider is skipped because
	// the OpenAI client cannot authenticate against an empty key.
	openaiCompatTypes := []struct {
		dbType  database.AIProviderType
		name    string
		baseURL string
	}{
		{database.AiProviderTypeOpenai, "openai-direct", "https://api.openai.com/v1"},
		{database.AiProviderTypeAzure, "azure-east", "https://east.openai.azure.com/v1"},
		{database.AiProviderTypeGoogle, "google-gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
		{database.AiProviderTypeOpenaiCompat, "openai-compat-custom", "https://llm.internal.example.com/v1"},
		{database.AiProviderTypeOpenrouter, "openrouter-default", "https://openrouter.ai/api/v1"},
		{database.AiProviderTypeVercel, "vercel-default", "https://api.v0.dev/v1"},
	}

	for _, tc := range openaiCompatTypes {
		t.Run("OpenAIFamily/"+string(tc.dbType), func(t *testing.T) {
			t.Parallel()

			id := uuid.New()
			rows := []database.AIProvider{{
				ID:      id,
				Type:    tc.dbType,
				Name:    tc.name,
				BaseUrl: tc.baseURL,
				Enabled: true,
			}}
			keys := map[string][]database.AIProviderKey{
				id.String(): {{APIKey: "sk-test"}},
			}

			providers, err := buildProvidersFromDB(rows, keys, cfg, logger)
			require.NoError(t, err)
			require.Len(t, providers, 1)
			assert.Equal(t, aibridge.ProviderOpenAI, providers[0].Type(),
				"type %q must route through the OpenAI fantasy client", tc.dbType)
			assert.Equal(t, tc.name, providers[0].Name())
			assert.Equal(t, tc.baseURL, providers[0].BaseURL())
		})

		t.Run("OpenAIFamily/"+string(tc.dbType)+"/SkippedWhenNoKey", func(t *testing.T) {
			t.Parallel()

			rows := []database.AIProvider{{
				ID:      uuid.New(),
				Type:    tc.dbType,
				Name:    tc.name,
				BaseUrl: tc.baseURL,
				Enabled: true,
			}}
			// No ai_provider_keys entry for the provider; OpenAI-family
			// providers must have at least one bearer key.
			providers, err := buildProvidersFromDB(rows, nil, cfg, logger)
			require.NoError(t, err)
			assert.Empty(t, providers, "type %q with no key must be skipped, not errored", tc.dbType)
		})
	}

	t.Run("Bedrock/WithSettingsRoutesThroughAnthropic", func(t *testing.T) {
		t.Parallel()

		settings := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-west-2",
				Model:           "anthropic.claude-sonnet-4",
				SmallFastModel:  "anthropic.claude-haiku-4",
				AccessKey:       ptr.Ref("AKIATEST"), //nolint:gosec // fixture
				AccessKeySecret: ptr.Ref("secret"),   //nolint:gosec // fixture
			},
		}
		raw, err := json.Marshal(settings)
		require.NoError(t, err)

		id := uuid.New()
		rows := []database.AIProvider{{
			ID:       id,
			Type:     database.AiProviderTypeBedrock,
			Name:     "bedrock-prod",
			BaseUrl:  "https://bedrock-runtime.us-west-2.amazonaws.com",
			Enabled:  true,
			Settings: sql.NullString{String: string(raw), Valid: true},
		}}
		// Bedrock providers authenticate via settings; no
		// ai_provider_keys row is required.
		providers, err := buildProvidersFromDB(rows, nil, cfg, logger)
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, aibridge.ProviderAnthropic, providers[0].Type(),
			"bedrock must route through the Anthropic fantasy client until native support lands")
		assert.Equal(t, "bedrock-prod", providers[0].Name())
	})

	t.Run("Bedrock/WithoutSettingsIsSkipped", func(t *testing.T) {
		t.Parallel()

		rows := []database.AIProvider{{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeBedrock,
			Name:    "bedrock-pending",
			BaseUrl: "https://bedrock-runtime.us-west-2.amazonaws.com",
			Enabled: true,
		}}
		// No Bedrock settings and no API keys: the runtime cannot
		// build a usable provider, so it must skip the row with a
		// warning instead of failing the entire reload.
		providers, err := buildProvidersFromDB(rows, nil, cfg, logger)
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("AnthropicWithBedrockSettingsStillWorks", func(t *testing.T) {
		t.Parallel()

		// The pre-existing shape (type=anthropic, settings.bedrock
		// set) must continue to route through the Anthropic client
		// with bedrock config. We do not flip these rows over on the
		// chatd cutover; only the new 'bedrock' typed rows are
		// special.
		settings := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-west-2",
				AccessKey:       ptr.Ref("AKIATEST"), //nolint:gosec // fixture
				AccessKeySecret: ptr.Ref("secret"),   //nolint:gosec // fixture
			},
		}
		raw, err := json.Marshal(settings)
		require.NoError(t, err)

		id := uuid.New()
		rows := []database.AIProvider{{
			ID:       id,
			Type:     database.AiProviderTypeAnthropic,
			Name:     "anthropic-bedrock-legacy",
			BaseUrl:  "https://bedrock-runtime.us-west-2.amazonaws.com",
			Enabled:  true,
			Settings: sql.NullString{String: string(raw), Valid: true},
		}}
		providers, err := buildProvidersFromDB(rows, nil, cfg, logger)
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, aibridge.ProviderAnthropic, providers[0].Type())
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()

		// A future enum value the runtime has not been taught about
		// yet must surface a clear error rather than being silently
		// dropped, so operators learn they need to upgrade.
		rows := []database.AIProvider{{
			ID:      uuid.New(),
			Type:    database.AIProviderType("future-protocol"),
			Name:    "future",
			BaseUrl: "https://example.com",
			Enabled: true,
		}}
		_, err := buildProvidersFromDB(rows, nil, cfg, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider type")
	})
}
