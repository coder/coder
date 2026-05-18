package coderd_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestAIProvidersCRUD(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				// No aibridge feature.
				Features: license.Features{},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.AIProviders(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("EmptyList", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is the audience for this endpoint.
		got, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("CreateGetUpdateDelete", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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

	t.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()
		ownerClient, _, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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
		ownerClient, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		anon := codersdk.New(ownerClient.URL)
		_, err := anon.AIProviders(ctx)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})

	t.Run("BedrockSecretsHidden", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
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
					AccessKey:       "AKIA-leak", //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: "bedrock-supersecret",
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

func TestAIProviderKeysCRUD(t *testing.T) {
	t.Parallel()

	t.Run("CreateDelete", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-openai",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		// Create two keys to exercise multi-key failover seeding.
		first, err := client.CreateAIProviderKey(ctx, provider.Name, codersdk.CreateAIProviderKeyRequest{
			APIKey: "sk-openai-primary", //nolint:gosec // test fixture, not a real credential
		})
		require.NoError(t, err)
		require.NotEqual(t, [16]byte{}, first.ID)
		require.Equal(t, provider.ID, first.ProviderID)

		second, err := client.CreateAIProviderKey(ctx, provider.Name, codersdk.CreateAIProviderKeyRequest{
			APIKey: "sk-openai-secondary", //nolint:gosec // test fixture, not a real credential
		})
		require.NoError(t, err)
		require.NotEqual(t, first.ID, second.ID)

		// Delete the first key; the second remains and the request
		// succeeds with 204.
		err = client.DeleteAIProviderKey(ctx, provider.Name, first.ID)
		require.NoError(t, err)
	})

	t.Run("BedrockProviderRejected", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		// Bedrock providers authenticate via the settings blob (AWS
		// access key + secret), so adding a key would be silently unused.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeAnthropic,
			Name:    "keys-bedrock",
			Enabled: true,
			BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-3-5-sonnet",
					AccessKey:       "AKIA-test", //nolint:gosec // test fixture, not a real credential
					AccessKeySecret: "bedrock-test-secret",
				},
			},
		})
		require.NoError(t, err)

		_, err = client.CreateAIProviderKey(ctx, provider.Name, codersdk.CreateAIProviderKeyRequest{
			APIKey: "sk-should-be-rejected", //nolint:gosec // test fixture, not a real credential
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("RequiresAPIKey", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-empty-body",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		_, err = client.CreateAIProviderKey(ctx, provider.Name, codersdk.CreateAIProviderKeyRequest{
			APIKey: "   ",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("DeleteCrossProviderForbidden", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		// Two distinct providers, each with their own key.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		providerA, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-a",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		providerB, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-b",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		key, err := client.CreateAIProviderKey(ctx, providerA.Name, codersdk.CreateAIProviderKeyRequest{
			APIKey: "sk-openai-a", //nolint:gosec // test fixture, not a real credential
		})
		require.NoError(t, err)

		// Attempting to delete A's key under B's path returns 404. The
		// handler intentionally hides the key's existence to avoid
		// leaking IDs across providers.
		err = client.DeleteAIProviderKey(ctx, providerB.Name, key.ID)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		// Deleting under provider A succeeds, confirming the key was
		// still attached to A all along.
		require.NoError(t, client.DeleteAIProviderKey(ctx, providerA.Name, key.ID))
	})

	t.Run("KeyResponseHidesSecret", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "keys-secret",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
		})
		require.NoError(t, err)

		// Inspect the raw HTTP body of the create response. The
		// api_key field is intentionally omitted from the response
		// shape, so the plaintext should never appear over the wire.
		res, err := client.Request(ctx, http.MethodPost,
			fmt.Sprintf("/api/v2/ai/providers/%s/keys", provider.Name),
			codersdk.CreateAIProviderKeyRequest{
				APIKey: "sk-openai-extra-secret", //nolint:gosec // test fixture, not a real credential
			})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		bodyBytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		body := string(bodyBytes)
		require.NotContains(t, body, "sk-openai-extra-secret")
		require.NotContains(t, body, "api_key")
	})

	t.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()
		ownerClient, _, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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

		_, err = memberClient.CreateAIProviderKey(ctx, provider.Name, codersdk.CreateAIProviderKeyRequest{
			APIKey: "sk-not-allowed", //nolint:gosec // test fixture, not a real credential
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("ProviderNotFound", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProviderKey(ctx, "missing", codersdk.CreateAIProviderKeyRequest{
			APIKey: "sk-noop", //nolint:gosec // test fixture, not a real credential
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
