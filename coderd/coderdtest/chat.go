package coderdtest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	// TestChatProviderOpenAICompat is the default provider for chat runtime tests.
	TestChatProviderOpenAICompat = "openai-compat"
	// TestChatProviderAPIKey is a non-secret API key for local chat providers.
	TestChatProviderAPIKey = "test-api-key"
	// TestChatModelOpenAICompat is the default model for chat runtime tests.
	TestChatModelOpenAICompat = "gpt-4o-mini"
)

// OpenAICompatProviderAPIKeys returns provider keys that route OpenAI-compatible
// chat calls to baseURL.
func OpenAICompatProviderAPIKeys(baseURL string) chatprovider.ProviderAPIKeys {
	return chatprovider.ProviderAPIKeys{
		ByProvider: map[string]string{
			TestChatProviderOpenAICompat: TestChatProviderAPIKey,
		},
		BaseURLByProvider: map[string]string{
			TestChatProviderOpenAICompat: baseURL,
		},
	}
}

// FakeOpenAICompatProviderAPIKeys starts a fake OpenAI-compatible provider and
// returns provider keys for coderdtest.Options.
func FakeOpenAICompatProviderAPIKeys(t testing.TB) chatprovider.ProviderAPIKeys {
	t.Helper()
	return OpenAICompatProviderAPIKeys(chattest.OpenAI(t))
}

// WithFakeOpenAICompatProvider starts a fake OpenAI-compatible provider and
// installs it into Options. Use this before creating the coderd test server so
// the fake provider outlives the chat daemon during test cleanup.
func WithFakeOpenAICompatProvider(t testing.TB) func(*Options) {
	t.Helper()
	keys := FakeOpenAICompatProviderAPIKeys(t)
	return func(options *Options) {
		options.ChatProviderAPIKeys = &keys
	}
}

// CreateOpenAICompatChatModelConfig creates the default provider and model
// config used by chat runtime tests. Tests that create chats should also use
// WithFakeOpenAICompatProvider, or otherwise set Options.ChatProviderAPIKeys,
// so background chat work routes to a local provider until coderd closes.
func CreateOpenAICompatChatModelConfig(t testing.TB, client *codersdk.ExperimentalClient) codersdk.ChatModelConfig {
	t.Helper()
	return provisionOpenAICompatChatModelConfig(t, client, "")
}

// CreateOpenAICompatChatModelConfigWithBaseURL creates the default provider and
// model config using the supplied OpenAI-compatible base URL.
func CreateOpenAICompatChatModelConfigWithBaseURL(
	t testing.TB,
	client *codersdk.ExperimentalClient,
	baseURL string,
) codersdk.ChatModelConfig {
	t.Helper()
	return provisionOpenAICompatChatModelConfig(t, client, baseURL)
}

func provisionOpenAICompatChatModelConfig(
	t testing.TB,
	client *codersdk.ExperimentalClient,
	baseURL string,
) codersdk.ChatModelConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: TestChatProviderOpenAICompat,
		APIKey:   TestChatProviderAPIKey,
		BaseURL:  baseURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     TestChatProviderOpenAICompat,
		Model:        TestChatModelOpenAICompat,
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)
	return modelConfig
}

// WaitForChatSettled waits for a chat to leave active processing and drains
// tracked chat daemon work before returning the final row.
func WaitForChatSettled(
	ctx context.Context,
	t testing.TB,
	api *coderd.API,
	chatID uuid.UUID,
) database.Chat {
	t.Helper()

	require.NotNil(t, api)
	// Test helper needs system scope to observe chatd-owned status changes.
	//nolint:gocritic
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	var settled database.Chat
	require.Eventually(t, func() bool {
		chat, err := api.Database.GetChatByID(systemCtx, chatID)
		if err != nil {
			return false
		}
		settled = chat
		return chat.Status != database.ChatStatusPending && chat.Status != database.ChatStatusRunning
	}, testutil.WaitLong, testutil.IntervalFast)

	server := api.ChatDaemonForTest()
	require.NotNil(t, server)
	chatd.WaitUntilIdleForTest(server)

	var err error
	settled, err = api.Database.GetChatByID(systemCtx, chatID)
	require.NoError(t, err)
	return settled
}
