package coderd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// keyIDs extracts the IDs from a slice of AIProviderKey responses, in
// order, to make assertions on key-set membership easier to read.
func keyIDs(keys []codersdk.AIProviderKey) []uuid.UUID {
	out := make([]uuid.UUID, len(keys))
	for i, k := range keys {
		out[i] = k.ID
	}
	return out
}

func TestAIProvidersCRUD(t *testing.T) {
	t.Parallel()

	t.Run("EmptyList", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		got, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("CreatePreservesPresetProviderTypes", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		tests := []struct {
			providerType codersdk.AIProviderType
			baseURL      string
		}{
			{providerType: codersdk.AIProviderTypeAzure, baseURL: "https://example.openai.azure.com/openai/v1"},
			{providerType: codersdk.AIProviderTypeGoogle, baseURL: "https://generativelanguage.googleapis.com/v1beta/openai/"},
			{providerType: codersdk.AIProviderTypeOpenAICompat, baseURL: "https://compat.example.com/v1"},
			{providerType: codersdk.AIProviderTypeOpenrouter, baseURL: "https://openrouter.ai/api/v1"},
			{providerType: codersdk.AIProviderTypeVercel, baseURL: "https://ai-gateway.vercel.sh/v1"},
		}
		for _, tt := range tests {
			t.Run(string(tt.providerType), func(t *testing.T) {
				created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
					Type:    tt.providerType,
					Name:    "type-preserve-" + string(tt.providerType),
					Enabled: true,
					BaseURL: tt.baseURL,
					APIKeys: []string{"sk-test"},
				})
				require.NoError(t, err, tt.providerType)
				require.Equal(t, tt.providerType, created.Type)

				got, err := client.AIProvider(ctx, created.ID.String())
				require.NoError(t, err, tt.providerType)
				require.Equal(t, tt.providerType, got.Type)
			})
		}
	})

	t.Run("CreateGetUpdateDelete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create.
		req := codersdk.CreateAIProviderRequest{
			Type:        codersdk.AIProviderTypeAnthropic,
			Name:        "primary-anthropic",
			DisplayName: "Primary Anthropic",
			Icon:        "https://example.com/anthropic.svg",
			Enabled:     true,
			BaseURL:     "https://api.anthropic.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:         "us-east-1",
					Model:          "anthropic.claude-3-5-sonnet",
					SmallFastModel: "anthropic.claude-3-5-haiku",
				},
			},
		}
		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, req)
		require.NoError(t, err)
		require.NotEqual(t, [16]byte{}, created.ID)
		require.Equal(t, req.Type, created.Type)
		require.Equal(t, req.Name, created.Name)
		require.Equal(t, req.DisplayName, created.DisplayName)
		require.Equal(t, req.Icon, created.Icon)
		require.Equal(t, req.Enabled, created.Enabled)
		require.Equal(t, req.BaseURL, created.BaseURL)
		require.NotNil(t, created.Settings.Bedrock)
		require.Equal(t, req.Settings.Bedrock.Region, created.Settings.Bedrock.Region)

		// Get by ID.
		gotByID, err := client.AIProvider(ctx, created.ID.String())
		require.NoError(t, err)
		require.Equal(t, created.ID, gotByID.ID)

		// Get by name.
		gotByName, err := client.AIProvider(ctx, created.Name)
		require.NoError(t, err)
		require.Equal(t, created.ID, gotByName.ID)

		// List.
		list, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, created.ID, list[0].ID)

		// Update.
		newDisplay := "Updated Display"
		newIcon := "🦜"
		newURL := "https://api.anthropic.com/v1"
		disabled := false
		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			DisplayName: &newDisplay,
			Icon:        &newIcon,
			BaseURL:     &newURL,
			Enabled:     &disabled,
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:         "us-west-2",
					Model:          "anthropic.claude-3-5-sonnet",
					SmallFastModel: "anthropic.claude-3-5-haiku",
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, newDisplay, updated.DisplayName)
		require.Equal(t, newIcon, updated.Icon)
		require.Equal(t, newURL, updated.BaseURL)
		require.False(t, updated.Enabled)
		require.NotNil(t, updated.Settings.Bedrock)
		require.Equal(t, "us-west-2", updated.Settings.Bedrock.Region)
		require.Equal(t, "anthropic.claude-3-5-sonnet", updated.Settings.Bedrock.Model)

		// Delete.
		err = client.DeleteAIProvider(ctx, created.ID.String())
		require.NoError(t, err)

		// Subsequent get returns 404.
		_, err = client.AIProvider(ctx, created.ID.String())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Resource not found")

		// List excludes the deleted provider.
		list, err = client.AIProviders(ctx)
		require.NoError(t, err)
		require.Empty(t, list)

		// Soft-deleted rows do not block name reuse: the unique index
		// is partial on deleted = FALSE, so re-creating the same name
		// succeeds and produces a new row with a different id.
		recreated, err := client.CreateAIProvider(ctx, req)
		require.NoError(t, err)
		require.NotEqual(t, created.ID, recreated.ID)
		require.Equal(t, req.Name, recreated.Name)
	})

	t.Run("DefaultDisplayName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "no-display",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)
		// Server falls back to Name when DisplayName is empty.
		require.Equal(t, "no-display", created.DisplayName)
	})

	t.Run("RequiredBaseURL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "missing-base-url",
			Enabled: true,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid AI provider request.", sdkErr.Message)
		require.Contains(t, sdkErr.Validations, codersdk.ValidationError{Field: "base_url", Detail: "base_url is required"})

		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "required-base-url",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		baseURL := "https://proxy.example.com/v1"
		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			BaseURL: &baseURL,
		})
		require.NoError(t, err)
		require.Equal(t, baseURL, updated.BaseURL)

		baseURL = ""
		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			BaseURL: &baseURL,
		})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid AI provider request.", sdkErr.Message)
		require.Contains(t, sdkErr.Validations, codersdk.ValidationError{Field: "base_url", Detail: "base_url is required"})
	})

	t.Run("DuplicateNameConflict", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "duplicate",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		}
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, req)
		require.NoError(t, err)
		_, err = client.CreateAIProvider(ctx, req)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, `"duplicate"`)
		require.Contains(t, sdkErr.Message, "already exists")
	})

	t.Run("InvalidName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Invalid character in name.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "Bad_Name",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "name", sdkErr.Validations[0].Field)
	})

	t.Run("InvalidType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    "nope",
			Name:    "nope",
			Enabled: true,
			BaseURL: "https://api.example.com",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "type", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, `"nope"`)
	})

	t.Run("InvalidBaseURL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "bad-url",
			Enabled: true,
			BaseURL: "not-a-url",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "base_url", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "absolute URL")

		_, err = client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "bad-scheme",
			Enabled: true,
			BaseURL: "ftp://api.example.com",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "base_url", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "http or https")
	})

	t.Run("UpdateNoFields", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "patchable",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "At least one field must be provided")
	})

	t.Run("UpdateCannotMutateName", func(t *testing.T) {
		t.Parallel()
		// ai_providers.name is the stable key that aibridge_interceptions
		// snapshots into provider_name. Renames would silently desync
		// historical interceptions from their live row and break the
		// future FK backfill, so the PATCH endpoint must ignore any "name"
		// field in the payload. The SDK type intentionally has no Name
		// field; this test sends raw JSON to defend against a future
		// regression where someone adds one without thinking.
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "stable-name",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodPatch,
			"/api/v2/ai/providers/"+created.Name,
			json.RawMessage(`{"name":"renamed","display_name":"New Display"}`),
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		got, err := client.AIProvider(ctx, created.Name)
		require.NoError(t, err)
		require.Equal(t, "stable-name", got.Name, "name must not be mutable via PATCH")
		require.Equal(t, "New Display", got.DisplayName, "display_name should still update")

		// Confirm the original name still resolves and the attempted new
		// name does not exist as a separate row.
		_, err = client.AIProvider(ctx, "renamed")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("UpdateSettingsEmptyObjectRejected", func(t *testing.T) {
		t.Parallel()
		// "settings": {} cannot decode because the _type discriminator
		// is missing. The handler must reject with 400; nothing about
		// the provider should change.
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "patch-settings-empty",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodPatch,
			"/api/v2/ai/providers/"+created.Name,
			json.RawMessage(`{"settings":{}}`),
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var body codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		require.Contains(t, body.Message, "valid JSON")
		require.Contains(t, body.Detail, "_type discriminator")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.AIProvider(ctx, "missing")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Resource not found")

		err = client.DeleteAIProvider(ctx, "missing")
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Resource not found")
	})

	t.Run("ListExcludesDeletedProviderKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A soft-deleted provider's keys must not bleed into the list
		// response. Create one provider, delete it, then create a
		// second; the list should only contain the live one with its
		// own keys.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		deleted, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "list-deleted",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-deleted-qqqqqqqqqqqqqqqqqq"}, //nolint:gosec // test fixture
		})
		require.NoError(t, err)
		err = client.DeleteAIProvider(ctx, deleted.ID.String())
		require.NoError(t, err)

		live, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "list-live",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-live-rrrrrrrrrrrrrrrrrr"}, //nolint:gosec // test fixture
		})
		require.NoError(t, err)

		list, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, live.ID, list[0].ID)
		require.Len(t, list[0].APIKeys, 1)
		require.Equal(t, live.APIKeys[0].ID, list[0].APIKeys[0].ID)
	})

	t.Run("LookupInvalidName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A string that is neither a UUID nor a syntactically-valid
		// provider name must surface a 400, not a misleading 404.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.AIProvider(ctx, "Bad_Name")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid provider id or name")

		err = client.DeleteAIProvider(ctx, "Bad_Name")
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid provider id or name")
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		anon := codersdk.New(ownerClient.URL)
		_, err := anon.AIProviders(ctx)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
		require.NotEmpty(t, sdkErr.Message)
	})

	t.Run("BedrockSettingsRequireAnthropic", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create: OpenAI-typed provider with Bedrock settings is a type
		// mismatch and must be rejected so the runtime never silently
		// drops the operator's authentication intent.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "bedrock-on-openai",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new("AKIA-fixture"),    //nolint:gosec // test fixture
					AccessKeySecret: new("bedrock-fixture"), //nolint:gosec // test fixture
				},
			},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.NotEmpty(t, sdkErr.Validations)
		require.Equal(t, "settings", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "bedrock settings are only valid for type=anthropic")

		// Update: existing OpenAI provider patched with Bedrock settings
		// must also be rejected.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "openai-then-bedrock",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:         "us-east-1",
					Model:          "anthropic.claude-3-5-sonnet",
					SmallFastModel: "anthropic.claude-3-5-haiku",
				},
			},
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Bedrock settings are only valid for type=anthropic")
	})

	t.Run("BedrockRequiresModelsForInvokeModel", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// requireMissingModels asserts a 400 naming both model fields.
		requireMissingModels := func(t *testing.T, err error) {
			t.Helper()
			require.Error(t, err)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			require.Contains(t, sdkErr.Message, "Invalid AI provider request")
			fields := make([]string, 0, len(sdkErr.Validations))
			for _, v := range sdkErr.Validations {
				fields = append(fields, v.Field)
			}
			require.Contains(t, fields, "settings.model")
			require.Contains(t, fields, "settings.small_fast_model")
		}

		// The invoke-model protocol does not work without models.
		req := codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeBedrock,
			Name:    "bedrock-region-only",
			Enabled: true,
			BaseURL: "https://bedrock.us-east-2.amazonaws.com",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-2"},
			},
		}
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, req)
		requireMissingModels(t, err)

		// The same provider with both models is accepted.
		req.Settings.Bedrock.Model = "anthropic.claude-3-5-sonnet"
		req.Settings.Bedrock.SmallFastModel = "anthropic.claude-3-5-haiku"
		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, req)
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-3-5-sonnet", created.Settings.Bedrock.Model)
		require.Equal(t, "anthropic.claude-3-5-haiku", created.Settings.Bedrock.SmallFastModel)

		// A PATCH that drops the models is rejected the same way.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-west-2"},
			},
		})
		requireMissingModels(t, err)

		// The same PATCH with both models is accepted, and the stored provider
		// is untouched by the rejected one above.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:         "us-west-2",
					Model:          "anthropic.claude-3-7-sonnet",
					SmallFastModel: "anthropic.claude-3-5-haiku",
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, "us-west-2", updated.Settings.Bedrock.Region)
		require.Equal(t, "anthropic.claude-3-7-sonnet", updated.Settings.Bedrock.Model)
		require.Equal(t, "anthropic.claude-3-5-haiku", updated.Settings.Bedrock.SmallFastModel)
	})

	t.Run("CredentialsMustBePairedOnCreate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name             string
			provider         string
			accessKey        *string
			accessKeySecret  *string
			missingField     string
			detail           string
			secretNotInError string
		}{
			{
				name:             "MissingAccessKey",
				provider:         "bedrock-create-missing-access-key",
				accessKeySecret:  new("create-secret-only"),
				missingField:     "settings.access_key",
				detail:           "access_key_secret is set, but access_key is missing or empty",
				secretNotInError: "create-secret-only",
			},
			{
				name:             "MissingAccessKeySecret",
				provider:         "bedrock-create-missing-access-key-secret",
				accessKey:        new("AKIA-create-key-only"), //nolint:gosec // test fixture
				missingField:     "settings.access_key_secret",
				detail:           "access_key is set, but access_key_secret is missing or empty",
				secretNotInError: "AKIA-create-key-only",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
					Type:    codersdk.AIProviderTypeAnthropic,
					Name:    tt.provider,
					Enabled: true,
					BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
					Settings: codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:          "us-east-1",
							Model:           "anthropic.claude-3-5-sonnet",
							SmallFastModel:  "anthropic.claude-3-5-haiku",
							AccessKey:       tt.accessKey,
							AccessKeySecret: tt.accessKeySecret,
						},
					},
				})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid AI provider request.", sdkErr.Message)
				require.Contains(t, sdkErr.Validations, codersdk.ValidationError{
					Field:  tt.missingField,
					Detail: tt.detail,
				})
				body, marshalErr := json.Marshal(sdkErr)
				require.NoError(t, marshalErr)
				require.NotContains(t, string(body), tt.secretNotInError)
			})
		}
	})

	t.Run("BedrockSecretsHidden", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Bedrock providers carry their AWS access key + secret inside the
		// encrypted settings blob. The response never echoes those fields
		// back, so callers cannot recover them after creation.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "bedrock-secret-leak",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new("AKIA-leak"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: new("bedrock-supersecret"),
				},
			},
		})
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet, "/api/v2/ai/providers/bedrock-secret-leak", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		bodyBytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		body := string(bodyBytes)
		require.NotContains(t, body, "AKIA-leak")
		require.NotContains(t, body, "bedrock-supersecret")
		require.NotContains(t, body, `"access_key"`)
		require.NotContains(t, body, `"access_key_secret"`)
	})
}

func TestAIProvidersKeyManagement(t *testing.T) {
	t.Parallel()

	t.Run("CreateWithKeysReturnsMasked", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		const (
			primary   = "sk-openai-primary-fixture-aaaaaa"   //nolint:gosec // test fixture, not a real credential
			secondary = "sk-openai-secondary-fixture-bbbbbb" //nolint:gosec // test fixture, not a real credential
		)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-openai",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{
				primary, secondary,
				"sk-openai-third-fixture-cccccc",  //nolint:gosec // test fixture
				"sk-openai-fourth-fixture-dddddd", //nolint:gosec // test fixture
				"sk-openai-fifth-fixture-eeeeee",  //nolint:gosec // test fixture
			},
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 5)
		// Masked form preserves prefix and suffix while hiding the
		// middle, so it's enough for an operator to recognize the key
		// without recovering the plaintext.
		require.True(t, strings.HasPrefix(provider.APIKeys[0].Masked, "sk-o"))
		require.True(t, strings.HasSuffix(provider.APIKeys[0].Masked, "aaaa"))
		require.NotContains(t, provider.APIKeys[0].Masked, primary)
		require.NotContains(t, provider.APIKeys[1].Masked, secondary)
		for _, key := range provider.APIKeys {
			require.NotEqual(t, uuid.Nil, key.ID)
		}
	})

	t.Run("ResponseHidesPlaintext", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		const plaintext = "sk-openai-extra-secret-cccccccccccc" //nolint:gosec // test fixture, not a real credential

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-secret",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{plaintext},
		})
		require.NoError(t, err)

		// Inspect the raw HTTP body of the GET response. The masked
		// form must replace the plaintext entirely on the wire.
		res, err := client.Request(ctx, http.MethodGet, "/api/v2/ai/providers/keys-secret", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		bodyBytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NotContains(t, string(bodyBytes), plaintext)
	})

	t.Run("UpdateReplacesKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		const original = "sk-openai-original-ddddddddddddddd" //nolint:gosec // test fixture
		for _, tt := range []struct {
			name        string
			replacement []codersdk.AIProviderKeyMutation
		}{
			{
				name: "DifferentPlaintext",
				replacement: []codersdk.AIProviderKeyMutation{
					{APIKey: new("sk-openai-rotated-eeeeeeeeeeeeeeeeeee")},     //nolint:gosec // test fixture
					{APIKey: new("sk-openai-rotated-second-ffffffffffffffff")}, //nolint:gosec // test fixture
				},
			},
			{
				name:        "SamePlaintext",
				replacement: []codersdk.AIProviderKeyMutation{{APIKey: new(original)}},
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				//nolint:gocritic // Owner role is the audience for this endpoint.
				provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
					Type:    codersdk.AIProviderTypeOpenAI,
					Name:    "keys-replace-" + strings.ToLower(tt.name),
					Enabled: true,
					BaseURL: "https://api.openai.com/v1",
					APIKeys: []string{original},
				})
				require.NoError(t, err)
				require.Len(t, provider.APIKeys, 1)
				originalID := provider.APIKeys[0].ID

				// Omitting the original ID deletes it; plaintext entries create
				// fresh rows even when the secret is unchanged.
				updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
					APIKeys: &tt.replacement,
				})
				require.NoError(t, err)
				require.Len(t, updated.APIKeys, len(tt.replacement))
				for _, k := range updated.APIKeys {
					require.NotEqual(t, uuid.Nil, k.ID)
					require.NotEqual(t, originalID, k.ID)
				}
			})
		}
	})

	t.Run("UpdateKeepsExistingByID", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-keep-by-id",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{
				"sk-openai-keep-aaaaaaaaaaaaaaaaaaaaaa",  //nolint:gosec // test fixture
				"sk-openai-evict-bbbbbbbbbbbbbbbbbbbbbb", //nolint:gosec // test fixture
			},
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 2)
		keepID := provider.APIKeys[0].ID
		keepMasked := provider.APIKeys[0].Masked
		evictID := provider.APIKeys[1].ID

		// Reference only keepID and add four new keys, reaching the five-key
		// limit. evictID is implicitly removed.
		patch := []codersdk.AIProviderKeyMutation{
			{ID: &keepID},
			{APIKey: new("sk-openai-added-cccccccccccccccccccccc")}, //nolint:gosec // test fixture
			{APIKey: new("sk-openai-added-2-dddddddddddddddddddd")}, //nolint:gosec // test fixture
			{APIKey: new("sk-openai-added-3-eeeeeeeeeeeeeeeeeeee")}, //nolint:gosec // test fixture
			{APIKey: new("sk-openai-added-4-ffffffffffffffffffff")}, //nolint:gosec // test fixture
		}
		updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &patch,
		})
		require.NoError(t, err)
		require.Len(t, updated.APIKeys, 5)
		ids := keyIDs(updated.APIKeys)
		require.Contains(t, ids, keepID)
		require.NotContains(t, ids, evictID)
		// The kept key's masked value is unchanged.
		for _, k := range updated.APIKeys {
			if k.ID == keepID {
				require.Equal(t, keepMasked, k.Masked)
			}
		}
	})

	t.Run("UpdateClearsKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-clear",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-tobedeleted-gggggggggggggg"}, //nolint:gosec // test fixture, not a real credential
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 1)

		empty := []codersdk.AIProviderKeyMutation{}
		updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &empty,
		})
		require.NoError(t, err)
		require.Empty(t, updated.APIKeys)
	})

	t.Run("UpdateKeepOnlyIsNoOp", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-keeponly",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{
				"sk-openai-stay-1-iiiiiiiiiiiiiiiiiiii", //nolint:gosec // test fixture
				"sk-openai-stay-2-jjjjjjjjjjjjjjjjjjjj", //nolint:gosec // test fixture
			},
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 2)
		originalIDs := keyIDs(provider.APIKeys)

		mutations := []codersdk.AIProviderKeyMutation{
			{ID: &provider.APIKeys[0].ID},
			{ID: &provider.APIKeys[1].ID},
		}
		updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &mutations,
		})
		require.NoError(t, err)
		require.ElementsMatch(t, originalIDs, keyIDs(updated.APIKeys))
	})

	t.Run("UpdateWithoutKeysPreserves", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-preserve",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-keepme-hhhhhhhhhhhhhhhh"}, //nolint:gosec // test fixture, not a real credential
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 1)
		original := provider.APIKeys[0]

		// PATCH with no APIKeys field must leave keys untouched.
		newDisplay := "Keep Display"
		updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			DisplayName: &newDisplay,
		})
		require.NoError(t, err)
		require.Len(t, updated.APIKeys, 1)
		require.Equal(t, original.ID, updated.APIKeys[0].ID)
		require.Equal(t, original.Masked, updated.APIKeys[0].Masked)
	})

	t.Run("BedrockRejectsCreateWithKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Bedrock providers authenticate via the settings blob (AWS
		// access key + secret), so an api_keys list would be silently
		// unused.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "keys-bedrock-create",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			APIKeys: []string{"sk-should-be-rejected"}, //nolint:gosec // test fixture, not a real credential
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new("AKIA-test"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: new("bedrock-test-secret"),
				},
			},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Bedrock providers do not accept api_keys")
	})

	t.Run("BedrockRejectsUpdateWithKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "keys-bedrock-update",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new("AKIA-test"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: new("bedrock-test-secret"),
				},
			},
		})
		require.NoError(t, err)

		rejected := []codersdk.AIProviderKeyMutation{
			{APIKey: new("sk-bedrock-no")}, //nolint:gosec // test fixture, not a real credential
		}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &rejected,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Bedrock providers do not accept api_keys")
	})

	t.Run("CopilotCreateWithoutKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeCopilot,
			Name:    "keys-copilot",
			Enabled: true,
			BaseURL: "https://api.business.githubcopilot.com",
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.AIProviderTypeCopilot, provider.Type)
		require.Empty(t, provider.APIKeys)
	})

	t.Run("CopilotRejectsCreateWithKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeCopilot,
			Name:    "keys-copilot-create",
			Enabled: true,
			BaseURL: "https://api.business.githubcopilot.com",
			APIKeys: []string{"sk-should-be-rejected"}, //nolint:gosec // test fixture, not a real credential
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "api_keys", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "type=copilot does not accept api_keys")
	})

	t.Run("CopilotRejectsUpdateWithKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeCopilot,
			Name:    "keys-copilot-update",
			Enabled: true,
			BaseURL: "https://api.business.githubcopilot.com",
		})
		require.NoError(t, err)

		rejected := []codersdk.AIProviderKeyMutation{
			{APIKey: new("sk-copilot-no")}, //nolint:gosec // test fixture, not a real credential
		}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &rejected,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Copilot providers do not accept api_keys")
	})

	t.Run("CreateRejectsInvalidKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		const duplicateKey = "sk-openai-create-duplicate-vvvvvvvvvvvvvvvv" //nolint:gosec // test fixture
		for _, tt := range []struct {
			name       string
			keys       []string
			field      string
			detail     string
			notContain string
		}{
			{
				name:   "Empty",
				keys:   []string{""},
				field:  "api_keys[0]",
				detail: "must not be empty",
			},
			{
				// Surrounding whitespace would silently break upstream auth,
				// since credentials are stored verbatim.
				name:   "Whitespace",
				keys:   []string{" sk-openai-padded-nnnnnnnnnnnnnnnnnnnn "}, //nolint:gosec // test fixture
				field:  "api_keys[0]",
				detail: "must not contain leading or trailing whitespace",
			},
			{
				name:       "Duplicate",
				keys:       []string{duplicateKey, duplicateKey},
				field:      "api_keys[1]",
				detail:     "duplicate key already provided at api_keys[0]",
				notContain: duplicateKey,
			},
			{
				name: "TooMany",
				keys: []string{
					"sk-openai-create-max-1-vvvvvvvvvvvvvvvv", //nolint:gosec // test fixture
					"sk-openai-create-max-2-vvvvvvvvvvvvvvvv", //nolint:gosec // test fixture
					"sk-openai-create-max-3-vvvvvvvvvvvvvvvv", //nolint:gosec // test fixture
					"sk-openai-create-max-4-vvvvvvvvvvvvvvvv", //nolint:gosec // test fixture
					"sk-openai-create-max-5-vvvvvvvvvvvvvvvv", //nolint:gosec // test fixture
					"sk-openai-create-max-6-vvvvvvvvvvvvvvvv", //nolint:gosec // test fixture
				},
				field:  "api_keys",
				detail: "api_keys must contain at most 5 keys",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				//nolint:gocritic // Owner role is the audience for this endpoint.
				_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
					Type:    codersdk.AIProviderTypeOpenAI,
					Name:    "keys-create-invalid-" + strings.ToLower(tt.name),
					Enabled: true,
					BaseURL: "https://api.openai.com/v1",
					APIKeys: tt.keys,
				})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid AI provider request.", sdkErr.Message)
				require.Len(t, sdkErr.Validations, 1)
				require.Equal(t, tt.field, sdkErr.Validations[0].Field)
				require.Contains(t, sdkErr.Validations[0].Detail, tt.detail)
				if tt.notContain != "" {
					body, marshalErr := json.Marshal(sdkErr)
					require.NoError(t, marshalErr)
					require.NotContains(t, string(body), tt.notContain)
				}
			})
		}
	})

	t.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := ownerClient.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-owner-only",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		patch := []codersdk.AIProviderKeyMutation{
			{APIKey: new("sk-not-allowed")}, //nolint:gosec // test fixture, not a real credential
		}
		_, err = memberClient.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &patch,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("PATCHPropertiesAudited", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-props-audit",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		// Reset before the update so we look only at audits produced by
		// the PATCH (the create path emits its own AIProvider audit).
		auditor.ResetLogs()

		newDisplay := "Renamed"
		newURL := "https://api.openai.com/v2"
		disabled := false
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			DisplayName: &newDisplay,
			BaseURL:     &newURL,
			Enabled:     &disabled,
		})
		require.NoError(t, err)

		// The parent AIProvider audit entry fires for property-only
		// PATCHes; the enterprise auditor populates the diff with the
		// changed fields (display_name, base_url, enabled). The mock
		// auditor used here returns an empty diff so we only assert the
		// entry shape; the actual diff content is exercised by the
		// enterprise audit unit tests.
		var sawUpdate bool
		for _, lg := range auditor.AuditLogs() {
			if lg.Action == database.AuditActionWrite && lg.ResourceType == database.ResourceTypeAIProvider {
				require.Equal(t, provider.ID, lg.ResourceID)
				sawUpdate = true
			}
		}
		require.True(t, sawUpdate, "expected parent AIProvider audit for property-only PATCH")
	})

	t.Run("PATCHKeysSurfacesOpsInAudit", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Without surfacing per-op detail, a PATCH that only rotates
		// keys would produce an audit entry whose top-level diff is
		// empty: invisible key rotation in the log.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-audit-ops",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{
				"sk-openai-audit-1-ssssssssssssssssssss", //nolint:gosec // test fixture
				"sk-openai-audit-2-tttttttttttttttttttt", //nolint:gosec // test fixture
			},
		})
		require.NoError(t, err)
		keepID := provider.APIKeys[0].ID

		// Keep one, drop one, add one.
		mutations := []codersdk.AIProviderKeyMutation{
			{ID: &keepID},
			{APIKey: new("sk-openai-audit-3-uuuuuuuuuuuuuuuuuuuu")}, //nolint:gosec // test fixture
		}
		updatedProvider, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &mutations,
		})
		require.NoError(t, err)

		// The newly-inserted row's ID and masked rendering are dynamic;
		// pull them from the PATCH response so we can build the expected
		// audit payload without re-declaring the audit struct shape.
		var added codersdk.AIProviderKey
		for _, k := range updatedProvider.APIKeys {
			if k.ID != keepID {
				added = k
				break
			}
		}
		require.NotEqual(t, uuid.Nil, added.ID)
		require.NotEmpty(t, added.Masked)
		require.NotContains(t, added.Masked, "sk-openai-audit-3-uuuuuuuuuuuuuuuuuuuu")
		removed := provider.APIKeys[1]

		logs := auditor.AuditLogs()
		var updated *database.AuditLog
		for i := range logs {
			if logs[i].Action == database.AuditActionWrite && logs[i].ResourceType == database.ResourceTypeAIProvider {
				updated = &logs[i]
			}
		}
		require.NotNil(t, updated, "expected audit log for AI provider update")

		expected, err := json.Marshal(map[string]any{
			"added":   []map[string]any{{"id": added.ID, "masked": added.Masked}},
			"removed": []map[string]any{{"id": removed.ID, "masked": removed.Masked}},
			"kept":    1,
		})
		require.NoError(t, err)
		require.JSONEq(t, string(expected), string(updated.AdditionalFields))

		// Per-key audit entries surface the added/removed keys as their
		// own log lines, so a key-only PATCH is visible even without
		// frontend changes. The Create handler also emits per-key
		// audits for the initial two keys, so match by ResourceID.
		var sawCreate, sawDelete bool
		for _, lg := range logs {
			if lg.ResourceType != database.ResourceTypeAIProviderKey {
				continue
			}
			switch {
			case lg.Action == database.AuditActionCreate && lg.ResourceID == added.ID:
				sawCreate = true
			case lg.Action == database.AuditActionDelete && lg.ResourceID == removed.ID:
				sawDelete = true
			}
		}
		require.True(t, sawCreate, "expected create audit for added key")
		require.True(t, sawDelete, "expected delete audit for removed key")
	})

	t.Run("UpdateRejectsInvalidKeys", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		const (
			retainedKey = "sk-openai-update-retained-xxxxxxxxxxxxxxxx" //nolint:gosec // test fixture
			otherKey    = "sk-openai-update-other-yyyyyyyyyyyyyyyyyyy" //nolint:gosec // test fixture
			duplicate   = "sk-openai-update-duplicate-zzzzzzzzzzzzzzz" //nolint:gosec // test fixture
		)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:        codersdk.AIProviderTypeOpenAI,
			Name:        "keys-update-validation",
			DisplayName: "Original key provider",
			Enabled:     true,
			BaseURL:     "https://api.openai.com/v1",
			APIKeys:     []string{retainedKey, otherKey},
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 2)
		originalIDs := keyIDs(provider.APIKeys)
		retainedID := provider.APIKeys[0].ID

		tests := []struct {
			name       string
			mutations  []codersdk.AIProviderKeyMutation
			message    string
			field      string
			detail     string
			notContain string
		}{
			{
				name:      "BothFields",
				mutations: []codersdk.AIProviderKeyMutation{{ID: &retainedID, APIKey: new(duplicate)}},
				message:   "Invalid AI provider request.",
				field:     "api_keys[0]",
				detail:    "exactly one of id or api_key must be set",
			},
			{
				name:      "NeitherField",
				mutations: []codersdk.AIProviderKeyMutation{{}},
				message:   "Invalid AI provider request.",
				field:     "api_keys[0]",
				detail:    "exactly one of id or api_key must be set",
			},
			{
				name:      "DuplicateID",
				mutations: []codersdk.AIProviderKeyMutation{{ID: &retainedID}, {ID: &retainedID}},
				message:   "Invalid AI provider request.",
				field:     "api_keys[1].id",
				detail:    "already referenced",
			},
			{
				name:      "Whitespace",
				mutations: []codersdk.AIProviderKeyMutation{{APIKey: new(" sk-openai-padded-pppppppppppppppppppp ")}}, //nolint:gosec // test fixture
				message:   "Invalid AI provider request.",
				field:     "api_keys[0].api_key",
				detail:    "must not contain leading or trailing whitespace",
			},
			{
				name:      "UnknownID",
				mutations: []codersdk.AIProviderKeyMutation{{ID: new(uuid.New())}},
				message:   "api_keys references an unknown id for this provider",
			},
			{
				name: "DuplicateKey",
				mutations: []codersdk.AIProviderKeyMutation{
					{APIKey: new(duplicate)},
					{APIKey: new(duplicate)},
				},
				message:    "Invalid AI provider request.",
				field:      "api_keys[1].api_key",
				detail:     "duplicate key already provided at api_keys[0]",
				notContain: duplicate,
			},
			{
				name: "DuplicateKeyWithRetainedIDs",
				mutations: []codersdk.AIProviderKeyMutation{
					{ID: &retainedID},
					{APIKey: new(duplicate)},
					{ID: &provider.APIKeys[1].ID},
					{APIKey: new(duplicate)},
				},
				message:    "Invalid AI provider request.",
				field:      "api_keys[3].api_key",
				detail:     "duplicate key already provided at api_keys[1]",
				notContain: duplicate,
			},
			{
				name: "RetainedThenDuplicatePlaintext",
				mutations: []codersdk.AIProviderKeyMutation{
					{ID: &retainedID},
					{APIKey: new(retainedKey)},
				},
				message:    "Invalid AI provider request.",
				field:      "api_keys",
				detail:     "duplicate key already provided at api_keys[0]",
				notContain: retainedKey,
			},
			{
				name: "DuplicatePlaintextThenRetained",
				mutations: []codersdk.AIProviderKeyMutation{
					{APIKey: new(retainedKey)},
					{ID: &retainedID},
				},
				message:    "Invalid AI provider request.",
				field:      "api_keys",
				detail:     "duplicate key already provided at api_keys[0]",
				notContain: retainedKey,
			},
			{
				name: "TooManyFinalKeys",
				mutations: []codersdk.AIProviderKeyMutation{
					{ID: &provider.APIKeys[0].ID},
					{ID: &provider.APIKeys[1].ID},
					{APIKey: new("sk-openai-update-max-3-aaaaaaaaaaaaaaaa")}, //nolint:gosec // test fixture
					{APIKey: new("sk-openai-update-max-4-bbbbbbbbbbbbbbbb")}, //nolint:gosec // test fixture
					{APIKey: new("sk-openai-update-max-5-cccccccccccccccc")}, //nolint:gosec // test fixture
					{APIKey: new("sk-openai-update-max-6-dddddddddddddddd")}, //nolint:gosec // test fixture
				},
				message: "Invalid AI provider request.",
				field:   "api_keys",
				detail:  "api_keys must contain at most 5 keys",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				changedDisplay := "Must not be persisted by " + tt.name
				_, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
					DisplayName: &changedDisplay,
					APIKeys:     &tt.mutations,
				})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, tt.message, sdkErr.Message)
				if tt.field != "" {
					require.Len(t, sdkErr.Validations, 1)
					require.Equal(t, tt.field, sdkErr.Validations[0].Field)
					require.Contains(t, sdkErr.Validations[0].Detail, tt.detail)
				} else {
					require.Empty(t, sdkErr.Validations)
				}
				if tt.notContain != "" {
					body, marshalErr := json.Marshal(sdkErr)
					require.NoError(t, marshalErr)
					require.NotContains(t, string(body), tt.notContain)
				}

				reread, getErr := client.AIProvider(ctx, provider.Name)
				require.NoError(t, getErr)
				require.Equal(t, provider.DisplayName, reread.DisplayName)
				require.ElementsMatch(t, originalIDs, keyIDs(reread.APIKeys))
			})
		}
	})
}

// TestAIProviderSettingsMerge exercises the PATCH merge semantics for
// the write-only Bedrock secrets through a real HTTP client. Because
// the API never echoes AccessKey or AccessKeySecret back, each
// subtest reads the provider row directly from the database to
// confirm what the merge actually persisted.
func TestAIProviderSettingsMerge(t *testing.T) {
	t.Parallel()

	t.Run("RejectsUnpairedCredentialsWithoutStoredCredentials", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		for _, tt := range []struct {
			name             string
			accessKey        *string
			accessKeySecret  *string
			missingField     string
			detail           string
			secretNotInError string
		}{
			{
				name:             "AccessKeyOnly",
				accessKey:        new("AKIA-introduced"), //nolint:gosec // test fixture
				missingField:     "settings.access_key_secret",
				detail:           "access_key is set, but access_key_secret is missing or empty",
				secretNotInError: "AKIA-introduced",
			},
			{
				name:             "AccessKeySecretOnly",
				accessKeySecret:  new("introduced-secret"),
				missingField:     "settings.access_key",
				detail:           "access_key_secret is set, but access_key is missing or empty",
				secretNotInError: "introduced-secret",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
					Type:        codersdk.AIProviderTypeAnthropic,
					Name:        "bedrock-introduce-" + strings.ToLower(tt.name),
					DisplayName: "Original Bedrock provider",
					Enabled:     true,
					BaseURL:     "https://bedrock-runtime.us-east-1.amazonaws.com/",
					Settings: codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:         "us-east-1",
							Model:          "anthropic.claude-3-5-sonnet",
							SmallFastModel: "anthropic.claude-3-5-haiku",
						},
					},
				})
				require.NoError(t, err)
				_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
					DisplayName: new("Must not be persisted"),
					Settings: &codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:          "us-west-2",
							Model:           "anthropic.claude-3-7-sonnet",
							SmallFastModel:  "anthropic.claude-3-5-haiku",
							AccessKey:       tt.accessKey,
							AccessKeySecret: tt.accessKeySecret,
						},
					},
				})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid AI provider request.", sdkErr.Message)
				require.Contains(t, sdkErr.Validations, codersdk.ValidationError{
					Field:  tt.missingField,
					Detail: tt.detail,
				})
				body, err := json.Marshal(sdkErr)
				require.NoError(t, err)
				require.NotContains(t, string(body), tt.secretNotInError)

				reread, err := client.AIProvider(ctx, provider.Name)
				require.NoError(t, err)
				require.Equal(t, provider, reread)
				//nolint:gocritic // Test reads the row to verify write-only fields.
				row, err := db.GetAIProviderByID(dbauthz.AsSystemRestricted(ctx), provider.ID)
				require.NoError(t, err)
				persisted, err := db2sdk.AIProviderSettings(row.Settings)
				require.NoError(t, err)
				require.Equal(t, provider.Settings, persisted)
			})
		}
	})

	t.Run("OmittedSecretsPreserveExisting", func(t *testing.T) {
		t.Parallel()
		// A PATCH that only rotates non-secret fields must keep the
		// existing AccessKey and AccessKeySecret intact so the provider
		// keeps authenticating after the update.
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "merge-omit",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new("AKIA-old"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: new("secret-old"),
				},
			},
		})
		require.NoError(t, err)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:         "us-west-2",
					Model:          "anthropic.claude-3-5-haiku",
					SmallFastModel: "anthropic.claude-3-5-haiku",
				},
			},
		})
		require.NoError(t, err)

		//nolint:gocritic // Test reads the row to verify write-only fields.
		row, err := db.GetAIProviderByID(dbauthz.AsSystemRestricted(ctx), created.ID)
		require.NoError(t, err)
		persisted, err := db2sdk.AIProviderSettings(row.Settings)
		require.NoError(t, err)
		require.NotNil(t, persisted.Bedrock)
		require.Equal(t, "us-west-2", persisted.Bedrock.Region)
		require.Equal(t, "anthropic.claude-3-5-haiku", persisted.Bedrock.Model)
		require.NotNil(t, persisted.Bedrock.AccessKey)
		require.Equal(t, "AKIA-old", *persisted.Bedrock.AccessKey)
		require.NotNil(t, persisted.Bedrock.AccessKeySecret)
		require.Equal(t, "secret-old", *persisted.Bedrock.AccessKeySecret)
	})

	t.Run("ClearSecrets", func(t *testing.T) {
		t.Parallel()
		// Empty strings explicitly clear credentials; omitted fields retain
		// their stored values. Clearing just one half must be rejected.
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		for _, tt := range []struct {
			name            string
			accessKey       *string
			accessKeySecret *string
			missingField    string
			detail          string
		}{
			{
				name:            "Both",
				accessKey:       new(""),
				accessKeySecret: new(""),
			},
			{
				name:         "AccessKeyOnly",
				accessKey:    new(""),
				missingField: "settings.access_key",
				detail:       "access_key_secret is set, but access_key is missing or empty",
			},
			{
				name:            "AccessKeySecretOnly",
				accessKeySecret: new(""),
				missingField:    "settings.access_key_secret",
				detail:          "access_key is set, but access_key_secret is missing or empty",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				req := codersdk.CreateAIProviderRequest{
					Type:        codersdk.AIProviderTypeAnthropic,
					Name:        "merge-clear-" + strings.ToLower(tt.name),
					DisplayName: "Original Bedrock provider",
					Enabled:     true,
					BaseURL:     "https://bedrock-runtime.us-east-1.amazonaws.com/",
					Settings: codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:          "us-east-1",
							Model:           "anthropic.claude-3-5-sonnet",
							SmallFastModel:  "anthropic.claude-3-5-haiku",
							AccessKey:       new("AKIA-old"), //nolint:gosec // test fixture
							AccessKeySecret: new("secret-old"),
						},
					},
				}
				//nolint:gocritic // Owner role is the audience for this endpoint.
				created, err := client.CreateAIProvider(ctx, req)
				require.NoError(t, err)

				patch := codersdk.UpdateAIProviderRequest{
					DisplayName: new("Updated Bedrock provider"),
					Settings: &codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:          "us-west-2",
							Model:           "anthropic.claude-3-7-sonnet",
							SmallFastModel:  "anthropic.claude-3-5-haiku",
							AccessKey:       tt.accessKey,
							AccessKeySecret: tt.accessKeySecret,
						},
					},
				}
				_, err = client.UpdateAIProvider(ctx, created.Name, patch)
				expected := req.Settings
				if tt.missingField != "" {
					sdkErr := requireSDKError(t, err, http.StatusBadRequest)
					require.Equal(t, "Invalid AI provider request.", sdkErr.Message)
					require.Contains(t, sdkErr.Validations, codersdk.ValidationError{
						Field:  tt.missingField,
						Detail: tt.detail,
					})
					reread, err := client.AIProvider(ctx, created.Name)
					require.NoError(t, err)
					require.Equal(t, created, reread)
				} else {
					require.NoError(t, err)
					expected = *patch.Settings
				}

				//nolint:gocritic // Test reads the row to verify write-only fields.
				row, err := db.GetAIProviderByID(dbauthz.AsSystemRestricted(ctx), created.ID)
				require.NoError(t, err)
				persisted, err := db2sdk.AIProviderSettings(row.Settings)
				require.NoError(t, err)
				require.Equal(t, expected, persisted)
			})
		}
	})

	t.Run("ExplicitRotatesSecrets", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		for _, tt := range []struct {
			name                    string
			accessKey               *string
			accessKeySecret         *string
			expectedAccessKey       string
			expectedAccessKeySecret string
		}{
			{
				name:                    "Both",
				accessKey:               new("AKIA-new"), //nolint:gosec // test fixture
				accessKeySecret:         new("secret-new"),
				expectedAccessKey:       "AKIA-new",
				expectedAccessKeySecret: "secret-new",
			},
			{
				name:                    "AccessKeyOnly",
				accessKey:               new("AKIA-new"), //nolint:gosec // test fixture
				expectedAccessKey:       "AKIA-new",
				expectedAccessKeySecret: "secret-old",
			},
			{
				name:                    "AccessKeySecretOnly",
				accessKeySecret:         new("secret-new"),
				expectedAccessKey:       "AKIA-old",
				expectedAccessKeySecret: "secret-new",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				//nolint:gocritic // Owner role is the audience for this endpoint.
				created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
					Type:    codersdk.AIProviderTypeAnthropic,
					Name:    "merge-rotate-" + strings.ToLower(tt.name),
					Enabled: true,
					BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
					Settings: codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:          "us-east-1",
							Model:           "anthropic.claude-3-5-sonnet",
							SmallFastModel:  "anthropic.claude-3-5-haiku",
							AccessKey:       new("AKIA-old"), //nolint:gosec // test fixture, not a real credential
							AccessKeySecret: new("secret-old"),
						},
					},
				})
				require.NoError(t, err)

				_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
					Settings: &codersdk.AIProviderSettings{
						Bedrock: &codersdk.AIProviderBedrockSettings{
							Region:          "us-east-1",
							Model:           "anthropic.claude-3-5-sonnet",
							SmallFastModel:  "anthropic.claude-3-5-haiku",
							AccessKey:       tt.accessKey,
							AccessKeySecret: tt.accessKeySecret,
						},
					},
				})
				require.NoError(t, err)

				//nolint:gocritic // Test reads the row to verify write-only fields.
				row, err := db.GetAIProviderByID(dbauthz.AsSystemRestricted(ctx), created.ID)
				require.NoError(t, err)
				persisted, err := db2sdk.AIProviderSettings(row.Settings)
				require.NoError(t, err)
				require.NotNil(t, persisted.Bedrock)
				require.NotNil(t, persisted.Bedrock.AccessKey)
				require.Equal(t, tt.expectedAccessKey, *persisted.Bedrock.AccessKey)
				require.NotNil(t, persisted.Bedrock.AccessKeySecret)
				require.Equal(t, tt.expectedAccessKeySecret, *persisted.Bedrock.AccessKeySecret)
			})
		}
	})

	t.Run("MigrateStaticToRole", func(t *testing.T) {
		t.Parallel()
		// An admin migrating from static AWS credentials to IAM role assumption
		// clears the keys and sets a role ARN in a single PATCH.
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "merge-role",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new("AKIA-old"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: new("secret-old"),
				},
			},
		})
		require.NoError(t, err)

		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					SmallFastModel:  "anthropic.claude-3-5-haiku",
					AccessKey:       new(""),
					AccessKeySecret: new(""),
					RoleARN:         "arn:aws:iam::123456789012:role/target",
				},
			},
		})
		require.NoError(t, err)

		require.NotNil(t, updated.Settings.Bedrock)
		require.Equal(t, "arn:aws:iam::123456789012:role/target", updated.Settings.Bedrock.RoleARN)

		//nolint:gocritic // Test reads the row to verify write-only fields.
		row, err := db.GetAIProviderByID(dbauthz.AsSystemRestricted(ctx), created.ID)
		require.NoError(t, err)
		persisted, err := db2sdk.AIProviderSettings(row.Settings)
		require.NoError(t, err)
		require.NotNil(t, persisted.Bedrock)
		require.Equal(t, "arn:aws:iam::123456789012:role/target", persisted.Bedrock.RoleARN)
		require.NotNil(t, persisted.Bedrock.AccessKey)
		require.Equal(t, "", *persisted.Bedrock.AccessKey)
		require.NotNil(t, persisted.Bedrock.AccessKeySecret)
		require.Equal(t, "", *persisted.Bedrock.AccessKeySecret)
	})
}

// TestAIProvidersBedrockExternalID covers the server-owned STS external ID:
// it is generated when (and only when) the provider assumes a role, is
// rejected when a client tries to set or change it, and is stable across
// PATCHes that echo the stored value.
func TestAIProvidersBedrockExternalID(t *testing.T) {
	t.Parallel()

	const (
		roleARN               = "arn:aws:iam::123456789012:role/BedrockRole"
		externalIDReadOnlyMsg = "The STS external ID is server-generated and cannot be changed."
	)

	// withModels supplies the model identifiers the invoke-model protocol
	// requires, keeping the fixtures below focused on external ID behavior.
	withModels := func(b codersdk.AIProviderBedrockSettings) *codersdk.AIProviderBedrockSettings {
		b.Model = "anthropic.claude-3-5-sonnet"
		b.SmallFastModel = "anthropic.claude-3-5-haiku"
		return &b
	}

	createBedrock := func(t *testing.T, client *codersdk.Client, name string, b codersdk.AIProviderBedrockSettings) (codersdk.AIProvider, error) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		return client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:     codersdk.AIProviderTypeBedrock,
			Name:     name,
			Enabled:  true,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Settings: codersdk.AIProviderSettings{Bedrock: withModels(b)},
		})
	}

	t.Run("GeneratedWhenRoleSet", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := createBedrock(t, client, "bedrock-role", codersdk.AIProviderBedrockSettings{
			Region:  "us-east-1",
			RoleARN: roleARN,
		})
		require.NoError(t, err)
		require.NotNil(t, created.Settings.Bedrock)
		require.NotEmpty(t, created.Settings.Bedrock.ExternalID, "external ID must be generated when a role is set")

		// GET returns the same external ID.
		got, err := client.AIProvider(ctx, created.ID.String())
		require.NoError(t, err)
		require.Equal(t, created.Settings.Bedrock.ExternalID, got.Settings.Bedrock.ExternalID)
	})

	t.Run("AbsentWithoutRole", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := createBedrock(t, client, "bedrock-no-role", codersdk.AIProviderBedrockSettings{Region: "us-east-1"})
		require.NoError(t, err)
		require.NotNil(t, created.Settings.Bedrock)
		require.Empty(t, created.Settings.Bedrock.ExternalID, "no external ID without a role to assume")
	})

	t.Run("RejectsClientValueOnCreate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := createBedrock(t, client, "bedrock-client-id", codersdk.AIProviderBedrockSettings{
			Region:     "us-east-1",
			RoleARN:    roleARN,
			ExternalID: "client-supplied-value",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Validations, codersdk.ValidationError{
			Field:  "settings.external_id",
			Detail: "external_id is server-generated and cannot be set",
		})
	})

	t.Run("StableWhenPatchOmitsValue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := createBedrock(t, client, "bedrock-stable", codersdk.AIProviderBedrockSettings{
			Region:  "us-east-1",
			RoleARN: roleARN,
		})
		require.NoError(t, err)
		original := created.Settings.Bedrock.ExternalID
		require.NotEmpty(t, original)

		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-west-2", RoleARN: roleARN}),
			},
		})
		require.NoError(t, err)
		require.Equal(t, "us-west-2", updated.Settings.Bedrock.Region)
		require.Equal(t, original, updated.Settings.Bedrock.ExternalID, "external ID must be stable across PATCH")
	})

	t.Run("StableAcrossRoleRemovalAndReassignment", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		const roleB = "arn:aws:iam::123456789012:role/BedrockRoleB"

		created, err := createBedrock(t, client, "bedrock-toggle", codersdk.AIProviderBedrockSettings{
			Region:  "us-east-1",
			RoleARN: roleARN,
		})
		require.NoError(t, err)
		original := created.Settings.Bedrock.ExternalID
		require.NotEmpty(t, original)

		// Removing the role retains the external ID.
		cleared, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-east-1"}),
			},
		})
		require.NoError(t, err)
		require.Empty(t, cleared.Settings.Bedrock.RoleARN)
		require.Equal(t, original, cleared.Settings.Bedrock.ExternalID)

		// Adding a different role reuses the retained ID rather than
		// regenerating it, so a trust policy referencing it keeps working.
		readded, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleB}),
			},
		})
		require.NoError(t, err)
		require.Equal(t, roleB, readded.Settings.Bedrock.RoleARN)
		require.Equal(t, original, readded.Settings.Bedrock.ExternalID, "external ID must survive role removal and re-add")
	})

	t.Run("AllowsEchoedValueOnPatch", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := createBedrock(t, client, "bedrock-echo", codersdk.AIProviderBedrockSettings{
			Region:  "us-east-1",
			RoleARN: roleARN,
		})
		require.NoError(t, err)
		original := created.Settings.Bedrock.ExternalID
		require.NotEmpty(t, original)

		// Read-modify-write resends the whole settings, including the stored
		// external ID. Echoing the same value is allowed.
		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-west-2", RoleARN: roleARN, ExternalID: original}),
			},
		})
		require.NoError(t, err)
		require.Equal(t, "us-west-2", updated.Settings.Bedrock.Region)
		require.Equal(t, original, updated.Settings.Bedrock.ExternalID)
	})

	t.Run("RejectsChangedValueOnPatch", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := createBedrock(t, client, "bedrock-change", codersdk.AIProviderBedrockSettings{
			Region:  "us-east-1",
			RoleARN: roleARN,
		})
		require.NoError(t, err)
		require.NotEmpty(t, created.Settings.Bedrock.ExternalID)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleARN, ExternalID: "client-tries-to-change-it"}),
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, externalIDReadOnlyMsg, sdkErr.Message)
	})

	t.Run("GeneratedWhenRoleAddedByPatch", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := createBedrock(t, client, "bedrock-add-role", codersdk.AIProviderBedrockSettings{Region: "us-east-1"})
		require.NoError(t, err)
		require.Empty(t, created.Settings.Bedrock.ExternalID)

		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleARN}),
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, updated.Settings.Bedrock.ExternalID, "external ID must be generated when a role is added by PATCH")
	})

	t.Run("RejectsClientValueWhenRoleAddedByPatch", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := createBedrock(t, client, "bedrock-add-role-id", codersdk.AIProviderBedrockSettings{Region: "us-east-1"})
		require.NoError(t, err)
		require.Empty(t, created.Settings.Bedrock.ExternalID)

		// No value is stored yet, so any client value is a change and is rejected.
		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: withModels(codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleARN, ExternalID: "client-supplied-value"}),
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, externalIDReadOnlyMsg, sdkErr.Message)
	})
}

func TestAIProviderHostnameCollisionWarnings(t *testing.T) {
	t.Parallel()

	newClient := func(t *testing.T, proxyEnabled bool) *codersdk.Client {
		t.Helper()
		return coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
				values.AI.BridgeProxyConfig.Enabled = serpent.Bool(proxyEnabled)
			}),
		})
	}

	t.Run("CreateGetListReturnsWarning", func(t *testing.T) {
		t.Parallel()
		client := newClient(t, true)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		wantWarnings := []string{`Hostname "api.openai.com" is claimed by provider "first". AI Gateway Proxy excludes this provider from proxy routing. The hostname collision does not affect direct routing (/api/v2/ai-gateway/second/... endpoint).`}

		//nolint:gocritic // Owner role is the audience for this endpoint.
		first, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "first",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)
		require.Nil(t, first.Status, "first provider in database order should not get a warning")

		//nolint:gocritic // Owner role is the audience for this endpoint.
		second, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "second",
			Enabled: true,
			BaseURL: "https://api.openai.com/v2",
		})
		require.NoError(t, err)
		require.NotNil(t, second.Status)
		require.Equal(t, wantWarnings, second.Status.Warnings)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		got, err := client.AIProvider(ctx, second.ID.String())
		require.NoError(t, err)
		require.NotNil(t, got.Status)
		require.Equal(t, wantWarnings, got.Status.Warnings)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		winner, err := client.AIProvider(ctx, first.ID.String())
		require.NoError(t, err)
		require.Nil(t, winner.Status, "first provider in database order should not get a warning on get")

		//nolint:gocritic // Owner role is the audience for this endpoint.
		providers, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Len(t, providers, 2)
		var firstListed, secondListed codersdk.AIProvider
		for _, p := range providers {
			switch p.Name {
			case "first":
				firstListed = p
			case "second":
				secondListed = p
			}
		}
		require.Nil(t, firstListed.Status, "first provider in database order should not get a warning in list")
		require.NotNil(t, secondListed.Status)
		require.Equal(t, wantWarnings, secondListed.Status.Warnings)
	})

	t.Run("UpdateReturnsWarningOnEnable", func(t *testing.T) {
		t.Parallel()
		client := newClient(t, true)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		wantWarnings := []string{`Hostname "api.openai.com" is claimed by provider "first". AI Gateway Proxy excludes this provider from proxy routing. The hostname collision does not affect direct routing (/api/v2/ai-gateway/second/... endpoint).`}

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "first",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		second, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "second",
			Enabled: false,
			BaseURL: "https://api.openai.com/v2",
		})
		require.NoError(t, err)
		require.Nil(t, second.Status)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, second.ID.String(), codersdk.UpdateAIProviderRequest{
			Enabled: new(true),
		})
		require.NoError(t, err)
		require.NotNil(t, updated.Status)
		require.Equal(t, wantWarnings, updated.Status.Warnings)
	})

	t.Run("UpdateBaseURLReturnsWarning", func(t *testing.T) {
		t.Parallel()
		client := newClient(t, true)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		wantWarnings := []string{`Hostname "api.openai.com" is claimed by provider "first". AI Gateway Proxy excludes this provider from proxy routing. The hostname collision does not affect direct routing (/api/v2/ai-gateway/second/... endpoint).`}

		//nolint:gocritic // Owner role is the audience for this endpoint.
		first, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "first",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)
		require.Nil(t, first.Status)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		second, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "second",
			Enabled: true,
			BaseURL: "https://api.openai-compat.com/v1",
		})
		require.NoError(t, err)
		require.Nil(t, second.Status, "distinct hostnames should not collide")

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, second.ID.String(), codersdk.UpdateAIProviderRequest{
			BaseURL: new("https://api.openai.com/v2"),
		})
		require.NoError(t, err)
		require.NotNil(t, updated.Status)
		require.Equal(t, wantWarnings, updated.Status.Warnings)
	})

	t.Run("ProxyDisabledReturnsNoWarning", func(t *testing.T) {
		t.Parallel()
		client := newClient(t, false)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "first",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		second, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "second",
			Enabled: true,
			BaseURL: "https://api.openai.com/v2",
		})
		require.NoError(t, err)
		require.Nil(t, second.Status)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		providers, err := client.AIProviders(ctx)
		require.NoError(t, err)
		for _, provider := range providers {
			require.Nil(t, provider.Status)
		}
	})

	t.Run("NoWarningWhenNoCollision", func(t *testing.T) {
		t.Parallel()
		client := newClient(t, true)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		first, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "first",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)
		require.Nil(t, first.Status)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		second, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "second",
			Enabled: true,
			BaseURL: "https://api.anthropic.com/v1",
		})
		require.NoError(t, err)
		require.Nil(t, second.Status)
	})

	t.Run("UpdateSelfNoWarning", func(t *testing.T) {
		t.Parallel()
		client := newClient(t, true)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		first, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "first",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, first.ID.String(), codersdk.UpdateAIProviderRequest{
			BaseURL: new("https://api.openai.com/v2"),
		})
		require.NoError(t, err)
		require.Nil(t, updated.Status, "update-self should not trigger a warning")
	})
}

// TestAIProvidersClaudePlatformAWS covers the Claude Platform for AWS variant
// end to end through the API: it is an authentication method on
// type=anthropic, and accepts provider keys independently of its settings.
func TestAIProvidersClaudePlatformAWS(t *testing.T) {
	t.Parallel()

	claudePlatformSettings := func() codersdk.AIProviderSettings {
		return codersdk.AIProviderSettings{
			ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
			},
		}
	}

	t.Run("CreateWithoutSettingsKeys", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:     codersdk.AIProviderTypeAnthropic,
			Name:     "claude-platform-iam",
			Enabled:  true,
			BaseURL:  "https://aws-external-anthropic.us-east-1.api.aws/",
			Settings: claudePlatformSettings(),
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.AIProviderTypeAnthropic, created.Type)
		require.NotNil(t, created.Settings.ClaudePlatformAWS)
		require.Empty(t, created.APIKeys)
	})

	t.Run("CreateRejectsNonAnthropicType", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:     codersdk.AIProviderTypeOpenAI,
			Name:     "claude-platform-openai",
			Enabled:  true,
			BaseURL:  "https://aws-external-anthropic.us-east-1.api.aws/",
			Settings: claudePlatformSettings(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("CreateWithKeys", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "claude-platform-api-key",
			Enabled: true,
			BaseURL: "https://aws-external-anthropic.us-east-1.api.aws/",
			APIKeys: []string{"sk-workspace-key"},
			Settings: codersdk.AIProviderSettings{
				ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
					Region:      "us-east-1",
					WorkspaceID: "wrkspc_123",
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, created.APIKeys, 1)
		require.NotEmpty(t, created.APIKeys[0].Masked)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &[]codersdk.AIProviderKeyMutation{},
		})
		require.NoError(t, err)

		persisted, err := client.AIProvider(ctx, created.Name)
		require.NoError(t, err)
		require.Empty(t, persisted.APIKeys)
		require.Equal(t, created.Settings, persisted.Settings)

		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &[]codersdk.AIProviderKeyMutation{{APIKey: new("sk-replacement-key")}},
		})
		require.NoError(t, err)
		require.Len(t, updated.APIKeys, 1)
		require.Equal(t, created.Settings, updated.Settings)
	})

	t.Run("CreateWithoutKeys", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "claude-platform-api-key-empty",
			Enabled: true,
			BaseURL: "https://aws-external-anthropic.us-east-1.api.aws/",
			Settings: codersdk.AIProviderSettings{
				ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
					Region:      "us-east-1",
					WorkspaceID: "wrkspc_123",
				},
			},
		})
		require.NoError(t, err)
		require.Empty(t, created.APIKeys)
		require.NotNil(t, created.Settings.ClaudePlatformAWS)
	})

	t.Run("PatchAllowsStoredKeys", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "claude-platform-switch",
			Enabled: true,
			BaseURL: "https://aws-external-anthropic.us-east-1.api.aws/",
			APIKeys: []string{"sk-workspace-key"},
			Settings: codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
				Region: "us-east-1", WorkspaceID: "wrkspc_123",
			}},
		})
		require.NoError(t, err)
		settings := claudePlatformSettings()
		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{Settings: &settings})
		require.NoError(t, err)
	})

	t.Run("PatchAllowsNoKeys", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type: codersdk.AIProviderTypeAnthropic, Name: "claude-platform-to-api-key", Enabled: true,
			BaseURL: "https://aws-external-anthropic.us-east-1.api.aws/", Settings: claudePlatformSettings(),
		})
		require.NoError(t, err)
		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{Settings: &codersdk.AIProviderSettings{
			ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
				Region: "us-east-1", WorkspaceID: "wrkspc_123",
			},
		}})
		require.NoError(t, err)
	})
}
