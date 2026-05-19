package coderd_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
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
		newURL := "https://api.anthropic.com/v1"
		disabled := false
		updated, err := client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
			DisplayName: &newDisplay,
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
	})

	t.Run("InvalidType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    "google",
			Name:    "google",
			Enabled: true,
			BaseURL: "https://api.example.com",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
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

		_, err = client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "bad-scheme",
			Enabled: true,
			BaseURL: "ftp://api.example.com",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
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

		err = client.DeleteAIProvider(ctx, "missing")
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
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

		err = client.DeleteAIProvider(ctx, "Bad_Name")
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create as owner.
		_, err := ownerClient.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "owner-only",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		// Member is not allowed to read or write providers.
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		_, err = memberClient.AIProviders(ctx)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		_, err = memberClient.AIProvider(ctx, "owner-only")
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		_, err = memberClient.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "member-attempt",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		err = memberClient.DeleteAIProvider(ctx, "owner-only")
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
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
		require.NotContains(t, body, "bedrock_access_key")
		require.NotContains(t, body, "bedrock_access_key_secret")
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

		// Provider's real key is left untouched.
		reread, err := client.AIProvider(ctx, provider.Name)
		require.NoError(t, err)
		require.Len(t, reread.APIKeys, 1)
		require.Equal(t, provider.APIKeys[0].ID, reread.APIKeys[0].ID)
	})
}
