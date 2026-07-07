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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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
					Region: "us-east-1",
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
					Region: "us-west-2",
					Model:  "anthropic.claude-3-5-sonnet",
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
					AccessKey:       ptr.Ref("AKIA-fixture"),    //nolint:gosec // test fixture
					AccessKeySecret: ptr.Ref("bedrock-fixture"), //nolint:gosec // test fixture
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
			},
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Bedrock settings are only valid for type=anthropic")
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
					AccessKey:       ptr.Ref("AKIA-leak"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("bedrock-supersecret"),
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
			APIKeys: []string{primary, secondary},
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 2)
		// Masked form preserves prefix and suffix while hiding the
		// middle, so it's enough for an operator to recognize the key
		// without recovering the plaintext.
		require.True(t, strings.HasPrefix(provider.APIKeys[0].Masked, "sk-o"))
		require.True(t, strings.HasSuffix(provider.APIKeys[0].Masked, "aaaa"))
		require.NotContains(t, provider.APIKeys[0].Masked, primary)
		require.NotContains(t, provider.APIKeys[1].Masked, secondary)
		require.NotEqual(t, uuid.Nil, provider.APIKeys[0].ID)
		require.NotEqual(t, uuid.Nil, provider.APIKeys[1].ID)
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
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-replace",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-original-ddddddddddddddd"}, //nolint:gosec // test fixture, not a real credential
		})
		require.NoError(t, err)
		require.Len(t, provider.APIKeys, 1)
		originalID := provider.APIKeys[0].ID

		// Omitting the original ID from the mutation list deletes it;
		// the two APIKey-bearing entries add fresh rows.
		replacement := []codersdk.AIProviderKeyMutation{
			{APIKey: ptr.Ref("sk-openai-rotated-eeeeeeeeeeeeeeeeeee")},     //nolint:gosec // test fixture
			{APIKey: ptr.Ref("sk-openai-rotated-second-ffffffffffffffff")}, //nolint:gosec // test fixture
		}
		updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &replacement,
		})
		require.NoError(t, err)
		require.Len(t, updated.APIKeys, 2)
		for _, k := range updated.APIKeys {
			require.NotEqual(t, originalID, k.ID)
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

		// Reference only keepID and add one new plaintext: evictID is
		// implicitly removed.
		patch := []codersdk.AIProviderKeyMutation{
			{ID: &keepID},
			{APIKey: ptr.Ref("sk-openai-added-cccccccccccccccccccccc")}, //nolint:gosec // test fixture
		}
		updated, err := client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &patch,
		})
		require.NoError(t, err)
		require.Len(t, updated.APIKeys, 2)
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
					AccessKey:       ptr.Ref("AKIA-test"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("bedrock-test-secret"),
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
					AccessKey:       ptr.Ref("AKIA-test"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("bedrock-test-secret"),
				},
			},
		})
		require.NoError(t, err)

		rejected := []codersdk.AIProviderKeyMutation{
			{APIKey: ptr.Ref("sk-bedrock-no")}, //nolint:gosec // test fixture, not a real credential
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
			{APIKey: ptr.Ref("sk-copilot-no")}, //nolint:gosec // test fixture, not a real credential
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

	t.Run("EmptyKeyRejected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-empty-element",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{""},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "api_keys[0]", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "must not be empty")
	})

	t.Run("WhitespaceKeyRejected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Surrounding whitespace would silently break upstream auth,
		// since the server stores credentials verbatim. Reject up-front
		// so the operator gets a clear signal instead of a 401 later.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-whitespace-create",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{" sk-openai-padded-nnnnnnnnnnnnnnnnnnnn "}, //nolint:gosec // test fixture
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-whitespace-update",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-clean-oooooooooooooooooooo"}, //nolint:gosec // test fixture
		})
		require.NoError(t, err)
		padded := " sk-openai-padded-pppppppppppppppppppp "
		muts := []codersdk.AIProviderKeyMutation{{APIKey: &padded}}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &muts,
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
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
			{APIKey: ptr.Ref("sk-not-allowed")}, //nolint:gosec // test fixture, not a real credential
		}
		_, err = memberClient.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &patch,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("MutationBothFieldsRejected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-mut-both",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-existing-kkkkkkkkkkkkkkkk"}, //nolint:gosec // test fixture
		})
		require.NoError(t, err)
		existingID := provider.APIKeys[0].ID

		muts := []codersdk.AIProviderKeyMutation{
			{ID: &existingID, APIKey: ptr.Ref("sk-conflict")}, //nolint:gosec // test fixture
		}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &muts,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "api_keys[0]", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "exactly one of id or api_key must be set")
	})

	t.Run("MutationNeitherFieldRejected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-mut-empty",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		muts := []codersdk.AIProviderKeyMutation{{}}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &muts,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "api_keys[0]", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "exactly one of id or api_key must be set")
	})

	t.Run("MutationDuplicateIDRejected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-mut-dup",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-dup-llllllllllllllllllll"}, //nolint:gosec // test fixture
		})
		require.NoError(t, err)
		id := provider.APIKeys[0].ID

		muts := []codersdk.AIProviderKeyMutation{
			{ID: &id},
			{ID: &id},
		}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &muts,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid AI provider request")
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "api_keys[1].id", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "already referenced")
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
			{APIKey: ptr.Ref("sk-openai-audit-3-uuuuuuuuuuuuuuuuuuuu")}, //nolint:gosec // test fixture
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

	t.Run("MutationUnknownIDRejected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-mut-unknown",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			APIKeys: []string{"sk-openai-real-mmmmmmmmmmmmmmmmmmmm"}, //nolint:gosec // test fixture
		})
		require.NoError(t, err)

		bogus := uuid.New()
		muts := []codersdk.AIProviderKeyMutation{{ID: &bogus}}
		_, err = client.UpdateAIProvider(ctx, provider.Name, codersdk.UpdateAIProviderRequest{
			APIKeys: &muts,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "api_keys references an unknown id for this provider")

		// Provider's real key is left untouched.
		reread, err := client.AIProvider(ctx, provider.Name)
		require.NoError(t, err)
		require.Len(t, reread.APIKeys, 1)
		require.Equal(t, provider.APIKeys[0].ID, reread.APIKeys[0].ID)
	})
}

// TestAIProviderSettingsMerge exercises the PATCH merge semantics for
// the write-only Bedrock secrets through a real HTTP client. Because
// the API never echoes AccessKey or AccessKeySecret back, each
// subtest reads the provider row directly from the database to
// confirm what the merge actually persisted.
func TestAIProviderSettingsMerge(t *testing.T) {
	t.Parallel()

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
					AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("secret-old"),
				},
			},
		})
		require.NoError(t, err)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region: "us-west-2",
					Model:  "anthropic.claude-3-5-haiku",
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

	t.Run("ExplicitEmptyClearsSecrets", func(t *testing.T) {
		t.Parallel()
		// An admin migrating from static AWS credentials to IAM
		// role-based auth needs to clear AccessKey and AccessKeySecret
		// in a single PATCH. Sending the field with an empty string is
		// the explicit clear signal; the *string field distinguishes
		// "omitted" (nil) from "set to empty" (pointer to "").
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "merge-clear",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("secret-old"),
				},
			},
		})
		require.NoError(t, err)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					AccessKey:       ptr.Ref(""),
					AccessKeySecret: ptr.Ref(""),
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
		require.Equal(t, "", *persisted.Bedrock.AccessKey)
		require.NotNil(t, persisted.Bedrock.AccessKeySecret)
		require.Equal(t, "", *persisted.Bedrock.AccessKeySecret)
	})

	t.Run("ExplicitRotatesSecrets", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "merge-rotate",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("secret-old"),
				},
			},
		})
		require.NoError(t, err)

		_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					AccessKey:       ptr.Ref("AKIA-new"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("secret-new"),
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
		require.Equal(t, "AKIA-new", *persisted.Bedrock.AccessKey)
		require.NotNil(t, persisted.Bedrock.AccessKeySecret)
		require.Equal(t, "secret-new", *persisted.Bedrock.AccessKeySecret)
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
					AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: ptr.Ref("secret-old"),
				},
			},
		})
		require.NoError(t, err)

		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					AccessKey:       ptr.Ref(""),
					AccessKeySecret: ptr.Ref(""),
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
		externalIDReadOnlyMsg = "The Bedrock external ID is server-generated and cannot be changed."
	)

	createBedrock := func(t *testing.T, client *codersdk.Client, name string, b codersdk.AIProviderBedrockSettings) (codersdk.AIProvider, error) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		return client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:     codersdk.AIProviderTypeBedrock,
			Name:     name,
			Enabled:  true,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Settings: codersdk.AIProviderSettings{Bedrock: &b},
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-west-2", RoleARN: roleARN},
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
			},
		})
		require.NoError(t, err)
		require.Empty(t, cleared.Settings.Bedrock.RoleARN)
		require.Equal(t, original, cleared.Settings.Bedrock.ExternalID)

		// Adding a different role reuses the retained ID rather than
		// regenerating it, so a trust policy referencing it keeps working.
		readded, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleB},
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-west-2", RoleARN: roleARN, ExternalID: original},
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleARN, ExternalID: "client-tries-to-change-it"},
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleARN},
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
				Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1", RoleARN: roleARN, ExternalID: "client-supplied-value"},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, externalIDReadOnlyMsg, sdkErr.Message)
	})
}
