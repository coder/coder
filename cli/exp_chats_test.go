package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func expChatsDeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	t.Helper()

	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentAgents)}
	return values
}

func newExpChatsClient(t testing.TB) *codersdk.Client {
	t.Helper()

	return coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: expChatsDeploymentValues(t),
	})
}

func createExpChatModelConfig(t testing.TB, client *codersdk.Client) codersdk.ChatModelConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	return modelConfig
}

func createExpChat(t testing.TB, client *codersdk.Client, prompt string) codersdk.Chat {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: prompt,
		}},
	})
	require.NoError(t, err)

	return chat
}

func runExpChatsCommand(
	t *testing.T,
	client *codersdk.Client,
	args ...string,
) (stdout string, stderr string, err error) {
	t.Helper()

	inv, root := clitest.New(t, args...)
	if client != nil {
		clitest.SetupConfig(t, client, root)
	}

	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	inv.Stdout = stdoutBuf
	inv.Stderr = stderrBuf

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	err = inv.WithContext(ctx).Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

func TestExpChatsList(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)
	_ = createExpChatModelConfig(t, client)

	activeChatA := createExpChat(t, client, "alpha list chat")
	activeChatB := createExpChat(t, client, "beta list chat")
	archivedChat := createExpChat(t, client, "archived list chat")

	ctx := testutil.Context(t, testutil.WaitLong)
	err := client.UpdateChat(ctx, archivedChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
	require.NoError(t, err)

	t.Run("Table", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "list")
		require.NoError(t, err)
		require.Empty(t, stderr)

		require.Contains(t, stdout, "id")
		require.Contains(t, stdout, "title")
		require.Contains(t, stdout, "status")
		require.Contains(t, stdout, activeChatA.ID.String())
		require.Contains(t, stdout, activeChatB.ID.String())
		require.Contains(t, stdout, activeChatA.Title)
		require.Contains(t, stdout, activeChatB.Title)
		require.NotContains(t, stdout, archivedChat.ID.String())
		require.NotContains(t, stdout, archivedChat.Title)
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "list", "--output", "json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		var chats []codersdk.Chat
		require.NoError(t, json.Unmarshal([]byte(stdout), &chats))
		require.Len(t, chats, 2)

		chatIDs := make([]uuid.UUID, 0, len(chats))
		for _, chat := range chats {
			chatIDs = append(chatIDs, chat.ID)
		}
		require.Contains(t, chatIDs, activeChatA.ID)
		require.Contains(t, chatIDs, activeChatB.ID)
		require.NotContains(t, chatIDs, archivedChat.ID)
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "list", "--limit", "1", "--output", "json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		var chats []codersdk.Chat
		require.NoError(t, json.Unmarshal([]byte(stdout), &chats))
		require.Len(t, chats, 1)
		require.NotEqual(t, archivedChat.ID, chats[0].ID)
	})

	t.Run("Archived", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "list", "--archived")
		require.NoError(t, err)
		require.Empty(t, stderr)

		require.Contains(t, stdout, archivedChat.ID.String())
		require.Contains(t, stdout, archivedChat.Title)
		require.NotContains(t, stdout, activeChatA.ID.String())
		require.NotContains(t, stdout, activeChatB.ID.String())
	})
}

func TestExpChatsListSearch(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)
	_ = createExpChatModelConfig(t, client)

	matchingChat := createExpChat(t, client, "searchable-needle chat")
	_ = createExpChat(t, client, "different-haystack chat")

	stdout, stderr, err := runExpChatsCommand(
		t,
		client,
		"exp",
		"chats",
		"list",
		"--search",
		"searchable-needle",
		"--output",
		"json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var chats []codersdk.Chat
	require.NoError(t, json.Unmarshal([]byte(stdout), &chats))
	require.Len(t, chats, 1)
	require.Equal(t, matchingChat.ID, chats[0].ID)
	require.Equal(t, matchingChat.Title, chats[0].Title)
}

func TestExpChatsShow(t *testing.T) {
	t.Parallel()

	t.Run("Table", func(t *testing.T) {
		t.Parallel()

		client := newExpChatsClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createExpChatModelConfig(t, client)

		chat := createExpChat(t, client, "show me this chat")

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "show", chat.ID.String())
		require.NoError(t, err)
		require.Empty(t, stderr)

		require.Contains(t, stdout, "workspace id")
		require.Contains(t, stdout, "last error")
		require.Contains(t, stdout, chat.ID.String())
		require.Contains(t, stdout, chat.Title)
		require.Contains(t, stdout, string(chat.Status))
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client := newExpChatsClient(t)
		_ = coderdtest.CreateFirstUser(t, client)
		_ = createExpChatModelConfig(t, client)

		chat := createExpChat(t, client, "show json chat")

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "show", chat.ID.String(), "--output", "json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		var got codersdk.Chat
		require.NoError(t, json.Unmarshal([]byte(stdout), &got))
		require.Equal(t, chat.ID, got.ID)
		require.Equal(t, chat.Title, got.Title)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()

		_, _, err := runExpChatsCommand(t, nil, "exp", "chats", "show", "not-a-uuid")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid chat ID \"not-a-uuid\"")
	})
}

func TestExpChatsModels(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)
	_ = createExpChatModelConfig(t, client)

	t.Run("Table", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "models")
		require.NoError(t, err)
		require.Empty(t, stderr)

		require.Contains(t, stdout, "display name")
		require.Contains(t, stdout, "openai")
		require.Contains(t, stdout, "gpt-4o-mini")
		require.Contains(t, stdout, "true")
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "models", "--output", "json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		var catalog codersdk.ChatModelsResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &catalog))
		require.NotEmpty(t, catalog.Providers)

		found := false
		for _, provider := range catalog.Providers {
			if provider.Provider != "openai" {
				continue
			}
			require.True(t, provider.Available)
			for _, model := range provider.Models {
				if model.Model == "gpt-4o-mini" {
					found = true
					break
				}
			}
		}
		require.True(t, found)
	})
}
