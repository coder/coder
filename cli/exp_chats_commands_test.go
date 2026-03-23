package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func expChatsDeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	t.Helper()

	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentAgents)}
	return values
}

func createTestChatModelConfig(t *testing.T, client *codersdk.Client) codersdk.ChatModelConfig {
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

type expChatsTestClients struct {
	adminClient *codersdk.Client
	userClient  *codersdk.Client
	owner       codersdk.CreateFirstUserResponse
	user        codersdk.User
}

func newExpChatsCLIClient(t testing.TB) expChatsTestClients {
	t.Helper()

	adminClient := coderdtest.New(t, &coderdtest.Options{DeploymentValues: expChatsDeploymentValues(t)})
	owner := coderdtest.CreateFirstUser(t, adminClient)
	userClient, user := coderdtest.CreateAnotherUser(t, adminClient, owner.OrganizationID)
	return expChatsTestClients{
		adminClient: adminClient,
		userClient:  userClient,
		owner:       owner,
		user:        user,
	}
}

func containsTextPart(parts []codersdk.ChatMessagePart, want string) bool {
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText && part.Text == want {
			return true
		}
	}
	return false
}

func responseContainsPrompt(resp codersdk.CreateChatMessageResponse, want string) bool {
	switch {
	case resp.Message != nil:
		return containsTextPart(resp.Message.Content, want)
	case resp.QueuedMessage != nil:
		return containsTextPart(resp.QueuedMessage.Content, want)
	default:
		return false
	}
}

func TestExpChatsStart_NoPrompt(t *testing.T) {
	t.Parallel()

	clients := newExpChatsCLIClient(t)

	inv, root := clitest.New(t, "exp", "chats", "start")
	inv.Stdin = strings.NewReader(" \n\t ")
	clitest.SetupConfig(t, clients.userClient, root)

	err := inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "prompt is required")
}

func TestExpChatsStart_WithPrompt(t *testing.T) {
	t.Parallel()

	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: expChatsDeploymentValues(t)})
	owner := coderdtest.CreateFirstUser(t, adminClient)
	userClient, user := coderdtest.CreateAnotherUser(t, adminClient, owner.OrganizationID)
	modelConfig := createTestChatModelConfig(t, adminClient)
	workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        user.ID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitLong)
	inv, root := clitest.New(
		t,
		"exp", "chats", "start",
		"--workspace", workspaceBuild.Workspace.ID.String(),
		"--model", modelConfig.ID.String(),
		"--output", "json",
		"Hello", "world",
	)
	clitest.SetupConfig(t, userClient, root)

	var stdout bytes.Buffer
	inv.Stdout = &stdout

	err := inv.WithContext(ctx).Run()
	require.NoError(t, err)

	var chat codersdk.Chat
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &chat))
	require.NotEqual(t, uuid.Nil, chat.ID)
	require.NotNil(t, chat.WorkspaceID)
	require.Equal(t, workspaceBuild.Workspace.ID, *chat.WorkspaceID)
	require.Equal(t, modelConfig.ID, chat.LastModelConfigID)

	require.Eventually(t, func() bool {
		messages, err := userClient.GetChatMessages(ctx, chat.ID, nil)
		if err != nil {
			return false
		}

		for _, message := range messages.Messages {
			if message.Role == codersdk.ChatMessageRoleUser && containsTextPart(message.Content, "Hello world") {
				return true
			}
		}
		for _, queued := range messages.QueuedMessages {
			if containsTextPart(queued.Content, "Hello world") {
				return true
			}
		}
		return false
	}, testutil.WaitLong, testutil.IntervalFast, "expected initial user message to contain prompt text")
}

func TestExpChatsSend_InvalidChatID(t *testing.T) {
	t.Parallel()

	clients := newExpChatsCLIClient(t)

	inv, root := clitest.New(t, "exp", "chats", "send", "not-a-uuid", "hello")
	clitest.SetupConfig(t, clients.userClient, root)

	err := inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid chat ID")
}

func TestExpChatsSend_NoPrompt(t *testing.T) {
	t.Parallel()

	clients := newExpChatsCLIClient(t)

	inv, root := clitest.New(t, "exp", "chats", "send", uuid.NewString())
	inv.Stdin = strings.NewReader(" \n\t ")
	clitest.SetupConfig(t, clients.userClient, root)

	err := inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "prompt is required")
}

func TestExpChatsSend_WithPromptArgs(t *testing.T) {
	t.Parallel()

	clients := newExpChatsCLIClient(t)
	modelConfig := createTestChatModelConfig(t, clients.adminClient)

	ctx := testutil.Context(t, testutil.WaitLong)
	chat, err := clients.userClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "initial prompt",
		}},
		ModelConfigID: &modelConfig.ID,
	})
	require.NoError(t, err)

	inv, root := clitest.New(t, "exp", "chats", "send", "--output", "json", chat.ID.String(), "follow", "up", "prompt")
	clitest.SetupConfig(t, clients.userClient, root)

	var stdout bytes.Buffer
	inv.Stdout = &stdout

	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)

	var resp codersdk.CreateChatMessageResponse
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &resp))
	require.True(t, responseContainsPrompt(resp, "follow up prompt"), "expected created message to contain prompt text")
}

func TestExpChatsWatch_InvalidChatID(t *testing.T) {
	t.Parallel()

	clients := newExpChatsCLIClient(t)

	inv, root := clitest.New(t, "exp", "chats", "watch", "not-a-uuid")
	clitest.SetupConfig(t, clients.userClient, root)

	err := inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid chat ID")
}

func TestExpChatsWatch_OutputJSON(t *testing.T) {
	t.Parallel()

	clients := newExpChatsCLIClient(t)

	inv, root := clitest.New(t, "exp", "chats", "watch", "--output", "json", "not-a-uuid")
	clitest.SetupConfig(t, clients.userClient, root)

	err := inv.WithContext(context.Background()).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid chat ID")
}
