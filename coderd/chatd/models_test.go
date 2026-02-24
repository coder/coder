package chatd_test

import (
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMergeProviderAPIKeys(t *testing.T) {
	t.Parallel()

	merged := chatprovider.MergeProviderAPIKeys(
		chatprovider.ProviderAPIKeys{
			OpenAI:    " deployment-openai ",
			Anthropic: "deployment-anthropic",
			ByProvider: map[string]string{
				"openrouter": " deployment-openrouter ",
			},
			BaseURLByProvider: map[string]string{
				"openai": " https://openai.example.com/v1 ",
			},
		},
		[]chatprovider.ConfiguredProvider{
			{Provider: "openai", APIKey: "   ", BaseURL: "https://db-openai.example.com/v1"},
			{Provider: "anthropic", APIKey: " provider-anthropic "},
			{Provider: "openrouter", APIKey: "provider-openrouter"},
		},
	)

	require.Equal(t, "deployment-openai", merged.OpenAI)
	require.Equal(t, "provider-anthropic", merged.Anthropic)
	require.Equal(t, "provider-openrouter", merged.APIKey("openrouter"))
	require.Equal(t, "https://db-openai.example.com/v1", merged.BaseURL("openai"))
}

func TestSupportedProvidersNormalize(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{
		"anthropic",
		"azure",
		"bedrock",
		"google",
		"openai",
		"openai-compat",
		"openrouter",
		"vercel",
	}, chatprovider.SupportedProviders())

	for _, provider := range chatprovider.SupportedProviders() {
		require.Equal(t, provider, chatprovider.NormalizeProvider(provider))
		require.Equal(t, provider, chatprovider.NormalizeProvider(strings.ToUpper(provider)))
	}
}

func TestStreamManagerStopStreamDropsMessageParts(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	manager := chatd.NewStreamManager(testutil.Logger(t))
	_, events, cancel := manager.Subscribe(chatID)
	defer cancel()

	manager.StartStream(chatID)
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: string(fantasy.MessageRoleAssistant),
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: "before-stop",
			},
		},
	})

	select {
	case event, ok := <-events:
		require.True(t, ok)
		require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, event.Type)
		require.NotNil(t, event.MessagePart)
		require.Equal(t, "before-stop", event.MessagePart.Part.Text)
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for initial stream message part")
	}

	manager.StopStream(chatID)
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: string(fantasy.MessageRoleAssistant),
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: "after-stop",
			},
		},
	})
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{
			Status: codersdk.ChatStatusWaiting,
		},
	})

	select {
	case event, ok := <-events:
		require.True(t, ok)
		require.Equal(t, codersdk.ChatStreamEventTypeStatus, event.Type)
		require.NotNil(t, event.Status)
		require.Equal(t, codersdk.ChatStatusWaiting, event.Status.Status)
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for status event after stream stop")
	}

	require.Never(t, func() bool {
		select {
		case <-events:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 10*time.Millisecond)
}
