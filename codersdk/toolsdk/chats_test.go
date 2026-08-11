package toolsdk_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

// Chat tools need a chat-enabled coderd (provider keys, a default model
// config, and an AI bridge daemon), so they are tested separately from
// TestTools. Subtests run sequentially and share the deployment.
// nolint:tparallel,paralleltest
func TestChatTools(t *testing.T) {
	providerKeys := coderdtest.FakeOpenAICompatProviderAPIKeys(t)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:    coderdtest.DeploymentValues(t),
		ChatProviderAPIKeys: &providerKeys,
	})
	coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)
	defaultModelConfig := coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, "")
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)

	tb, err := toolsdk.NewDeps(client)
	require.NoError(t, err)

	t.Run("ListChatModelConfigs", func(t *testing.T) {
		result, err := testTool(t, toolsdk.ListChatModelConfigs, tb, toolsdk.NoArgs{})
		require.NoError(t, err)
		require.Len(t, result.ModelConfigs, 1)
		require.Equal(t, defaultModelConfig.ID.String(), result.ModelConfigs[0].ID)
		require.Equal(t, coderdtest.TestChatModelOpenAICompat, result.ModelConfigs[0].Model)
		require.True(t, result.ModelConfigs[0].IsDefault)
	})

	t.Run("Lifecycle", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{
			Prompt: "Say hello.",
			Labels: map[string]string{"purpose": "toolsdk-test"},
		})
		require.NoError(t, err)
		chatID, err := uuid.Parse(created.ID)
		require.NoError(t, err)
		require.Equal(t, client.URL.String()+"/agents/"+created.ID, created.URL)

		coderdtest.WaitForChatSettled(ctx, t, api, chatID)

		got, err := testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.Equal(t, created.ID, got.ID)
		require.Equal(t, codersdk.ChatStatusWaiting, got.Status)
		require.Nil(t, got.LastError)
		require.False(t, got.Archived)

		sent, err := testTool(t, toolsdk.SendChatMessage, tb, toolsdk.SendChatMessageArgs{
			ChatID: created.ID,
			Text:   "Say hello again.",
		})
		require.NoError(t, err)
		require.False(t, sent.Queued)

		coderdtest.WaitForChatSettled(ctx, t, api, chatID)

		messages, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.False(t, messages.HasMore)
		var texts []string
		for _, msg := range messages.Messages {
			texts = append(texts, string(msg.Role)+": "+msg.Text)
		}
		require.Contains(t, texts, "user: Say hello.")
		require.Contains(t, texts, "user: Say hello again.")
		require.Contains(t, texts, "assistant: Hello from test server.")
		// Chronological order: the initial prompt comes first.
		require.Equal(t, "user: Say hello.", texts[0])

		archived, err := testTool(t, toolsdk.ArchiveChat, tb, toolsdk.ArchiveChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.NotEmpty(t, archived.Message)

		got, err = testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.True(t, got.Archived)
	})

	t.Run("Interrupt", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		// A provider that never answers the chat turn keeps the chat
		// running so the interrupt has a deterministic target.
		release := make(chan struct{})
		blockingURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if req.Stream {
				select {
				case <-release:
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Released.")...)
			}
			return chattest.OpenAINonStreamingResponse(`{"title": "Interrupt Test"}`)
		})
		blockingModelConfig := coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, blockingURL)

		created, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{
			Prompt:        "Block forever.",
			ModelConfigID: blockingModelConfig.ID.String(),
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusRunning, created.Status)

		interrupted, err := testTool(t, toolsdk.InterruptChat, tb, toolsdk.InterruptChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusInterrupting, interrupted.Status)

		close(release)
		coderdtest.WaitForChatSettled(ctx, t, api, uuid.MustParse(created.ID))
	})

	t.Run("Validation", func(t *testing.T) {
		_, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{})
		require.ErrorContains(t, err, "prompt is required")

		_, err = testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: "not-a-uuid"})
		require.ErrorContains(t, err, "chat_id must be a valid UUID")

		_, err = testTool(t, toolsdk.SendChatMessage, tb, toolsdk.SendChatMessageArgs{
			ChatID:       uuid.NewString(),
			Text:         "hi",
			BusyBehavior: "bogus",
		})
		require.ErrorContains(t, err, "busy_behavior")
	})
}
