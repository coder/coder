package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatModelReasoningModePersistence(t *testing.T) {
	t.Parallel()

	for _, model := range []string{
		"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-astra",
		"gpt-5.6-2026-06-01", "gpt-5.6-sol-2026-06-01", "gpt-6-astra-2026-06-01",
	} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			provider := createAIProviderForTest(t, client, "openai", "test-api-key")
			config := chatModelReasoningModeConfig("pro", nil)
			created, err := client.CreateChatModel(ctx, firstUser.OrganizationID, codersdk.CreateChatModelRequest{
				AIProviderID: &provider.ID,
				Model:        model,
				ContextLimit: ptr.Ref(int64(4096)),
				ModelConfig:  config,
			})
			require.NoError(t, err)
			require.Equal(t, config, created.ModelConfig)
			stored, err := client.ChatModel(ctx, created.OrganizationID, created.ID)
			require.NoError(t, err)
			require.Equal(t, config, stored.ModelConfig)

			updated, err := client.UpdateChatModel(ctx, created.OrganizationID, created.ID, codersdk.UpdateChatModelRequest{DisplayName: "renamed"})
			require.NoError(t, err)
			require.Equal(t, config, updated.ModelConfig)

			config = chatModelReasoningModeConfig("standard", ptr.Ref(true))
			updated, err = client.UpdateChatModel(ctx, created.OrganizationID, created.ID, codersdk.UpdateChatModelRequest{ModelConfig: config})
			require.NoError(t, err)
			require.Equal(t, config, updated.ModelConfig)
			stored, err = client.ChatModel(ctx, created.OrganizationID, created.ID)
			require.NoError(t, err)
			require.Equal(t, config, stored.ModelConfig)

			config = &codersdk.ChatModelCallConfig{
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningSummary: ptr.Ref("detailed")},
				},
			}
			updated, err = client.UpdateChatModel(ctx, created.OrganizationID, created.ID, codersdk.UpdateChatModelRequest{ModelConfig: config})
			require.NoError(t, err)
			require.Equal(t, config, updated.ModelConfig)
			stored, err = client.ChatModel(ctx, created.OrganizationID, created.ID)
			require.NoError(t, err)
			require.Equal(t, config, stored.ModelConfig)

			updated, err = client.UpdateChatModel(ctx, created.OrganizationID, created.ID, codersdk.UpdateChatModelRequest{
				Model:       "gpt-4o",
				ModelConfig: &codersdk.ChatModelCallConfig{},
			})
			require.NoError(t, err)
			require.Nil(t, updated.ModelConfig)
			stored, err = client.ChatModel(ctx, created.OrganizationID, created.ID)
			require.NoError(t, err)
			require.Nil(t, stored.ModelConfig)
			require.Equal(t, "gpt-4o", stored.Model)
		})
	}
}

func TestChatModelReasoningModeInvalid(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		provider  string
		model     string
		mode      string
		responses *bool
	}{
		{name: "UnknownMode", provider: "openai", model: "gpt-5.6", mode: "turbo"},
		{name: "EmptyMode", provider: "openai", model: "gpt-5.6", mode: ""},
		{name: "UppercaseMode", provider: "openai", model: "gpt-5.6", mode: "PRO"},
		{name: "WhitespaceMode", provider: "openai", model: "gpt-5.6", mode: " pro "},
		{name: "UnsupportedModel", provider: "openai", model: "gpt-4o", mode: "pro", responses: ptr.Ref(true)},
		{name: "UnsupportedStandard", provider: "openai", model: "gpt-4o", mode: "standard", responses: ptr.Ref(true)},
		{name: "LookalikeModel", provider: "openai", model: "gpt-5.6-mini", mode: "pro", responses: ptr.Ref(true)},
		{name: "ChatCompletions", provider: "openai", model: "gpt-5.6", mode: "pro", responses: ptr.Ref(false)},
		{name: "Anthropic", provider: "anthropic", model: "gpt-5.6", mode: "pro", responses: ptr.Ref(true)},
		{name: "Azure", provider: "azure", model: "gpt-5.6", mode: "pro", responses: ptr.Ref(true)},
		{name: "OpenAICompat", provider: "openai-compat", model: "gpt-5.6", mode: "pro", responses: ptr.Ref(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			provider := createAIProviderForTest(t, client, tc.provider, "test-api-key")
			request := codersdk.CreateChatModelRequest{
				AIProviderID: &provider.ID,
				Model:        tc.model,
				ContextLimit: ptr.Ref(int64(4096)),
			}
			created, err := client.CreateChatModel(ctx, firstUser.OrganizationID, request)
			require.NoError(t, err)

			config := chatModelReasoningModeConfig(tc.mode, tc.responses)
			_, err = client.UpdateChatModel(ctx, created.OrganizationID, created.ID, codersdk.UpdateChatModelRequest{ModelConfig: config})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Contains(t, sdkErr.Detail, "reasoning_mode")
			stored, err := client.ChatModel(ctx, created.OrganizationID, created.ID)
			require.NoError(t, err)
			require.Nil(t, stored.ModelConfig)

			require.NoError(t, client.DeleteChatModel(ctx, created.OrganizationID, created.ID))
			request.ModelConfig = config
			_, err = client.CreateChatModel(ctx, firstUser.OrganizationID, request)
			sdkErr = requireSDKError(t, err, http.StatusBadRequest)
			require.Contains(t, sdkErr.Detail, "reasoning_mode")
		})
	}
}

func TestChatModelReasoningModePartialUpdate(t *testing.T) {
	t.Parallel()

	for _, change := range []string{"Model", "Provider", "AzureProvider", "Transport"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			provider := createAIProviderForTest(t, client, "openai", "test-api-key")
			config := chatModelReasoningModeConfig("pro", nil)
			created, err := client.CreateChatModel(ctx, firstUser.OrganizationID, codersdk.CreateChatModelRequest{
				AIProviderID: &provider.ID,
				Model:        "gpt-5.6",
				ContextLimit: ptr.Ref(int64(4096)),
				ModelConfig:  config,
			})
			require.NoError(t, err)

			var request codersdk.UpdateChatModelRequest
			switch change {
			case "Model":
				request.Model = "gpt-4o"
			case "Provider", "AzureProvider":
				providerType := "openai-compat"
				if change == "AzureProvider" {
					providerType = "azure"
				}
				other := createAIProviderForTest(t, client, providerType, "test-api-key")
				request.AIProviderID = &other.ID
			case "Transport":
				request.ModelConfig = chatModelReasoningModeConfig("pro", ptr.Ref(false))
			}
			_, err = client.UpdateChatModel(ctx, created.OrganizationID, created.ID, request)
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Contains(t, sdkErr.Detail, "reasoning_mode")
			stored, err := client.ChatModel(ctx, created.OrganizationID, created.ID)
			require.NoError(t, err)
			require.Equal(t, created, stored)

			request.ModelConfig = &codersdk.ChatModelCallConfig{}
			updated, err := client.UpdateChatModel(ctx, created.OrganizationID, created.ID, request)
			require.NoError(t, err)
			require.Nil(t, updated.ModelConfig)
		})
	}
}

func chatModelReasoningModeConfig(mode string, responses *bool) *codersdk.ChatModelCallConfig {
	config := &codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningMode: &mode},
		},
	}
	if responses != nil {
		config.OpenAIConfig = &codersdk.ChatModelOpenAIConfig{UseResponsesAPI: responses}
	}
	return config
}
