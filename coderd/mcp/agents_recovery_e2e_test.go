package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestMCPHTTP_AcceptedChatReplayAfterPolicyChanges(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	var hookCalls, providerCalls atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hookCalls.Add(1); _, _ = w.Write([]byte("{}")) }))
	defer consumer.Close()
	provider := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		providerCalls.Add(1)
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"replay"}`)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	keys := coderdtest.OpenAICompatProviderAPIKeys(provider)
	values := mcpDeploymentValues(t)
	values.Experiments = append(values.Experiments, string(codersdk.ExperimentAgentLifecycleHooks))
	require.NoError(t, values.AI.Chat.HookURL.Set(consumer.URL))
	values.AI.Chat.HookEnabled = serpent.Bool(true)
	values.AI.Chat.HookAllowInsecure = serpent.Bool(true)
	values.AI.Chat.HookSecret = serpent.String("test-hook-secret-32-bytes-minimum!!")
	values.AI.Chat.HookTimeout = serpent.Duration(testutil.WaitLong)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: values, ChatProviderAPIKeys: &keys})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	exp := codersdk.NewExperimentalClient(client)
	model := coderdtest.CreateOpenAICompatChatModel(t, exp, provider)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	connect := func(c *codersdk.Client) *mcp.ClientSession {
		session, err := newIsolatedMCPClient(ctx, api.AccessURL.String()+mcpserver.MCPEndpoint, uuid.NewString(), map[string]string{"Authorization": "Bearer " + c.SessionToken()})
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	session := connect(client)
	create := toolsdk.CreateChatArgs{RequestID: uuid.NewString(), OrganizationID: user.OrganizationID.String(), ModelConfigID: model.ID.String(), Prompt: "first"}
	first := callChatTool[toolsdk.ChatToolStatus](ctx, t, session, toolsdk.ToolNameCreateChat, create)
	wait := func() {
		out := callChatTool[toolsdk.AwaitChatResponse](ctx, t, session, toolsdk.ToolNameAwaitChat, toolsdk.AwaitChatArgs{ChatID: first.ID, WaitSecs: 10})
		require.False(t, out.TimedOut)
		require.Equal(t, codersdk.ChatStatusWaiting, out.Chat.Status)
	}
	wait()
	send := toolsdk.SendChatMessageArgs{RequestID: uuid.NewString(), ChatID: first.ID, ModelConfigID: model.ID.String(), Text: "followup"}
	sent := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, session, toolsdk.ToolNameSendChatMessage, send)
	wait()
	callChatTool[codersdk.Response](ctx, t, session, toolsdk.ToolNameArchiveChat, toolsdk.ArchiveChatArgs{ChatID: first.ID})
	beforeHooks, beforeProvider := hookCalls.Load(), providerCalls.Load()
	require.Positive(t, beforeHooks)
	require.Positive(t, beforeProvider)
	for _, disabled := range []bool{false, true} {
		if disabled {
			_, err := exp.UpdateChatModel(ctx, user.OrganizationID, model.ID, codersdk.UpdateChatModelRequest{Enabled: new(false)})
			require.NoError(t, err)
		}
		fresh := connect(client)
		replay := callChatTool[toolsdk.ChatToolStatus](ctx, t, fresh, toolsdk.ToolNameCreateChat, create)
		require.Equal(t, first.ID, replay.ID)
		require.True(t, replay.Archived)
		require.Equal(t, first.Submission, replay.Submission)
		recovered := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, fresh, toolsdk.ToolNameSendChatMessage, send)
		require.Equal(t, sent.Submission, recovered.Submission)
		changed := create
		changed.Prompt = "different"
		conflict, err := fresh.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameCreateChat, Arguments: changed})
		require.NoError(t, err)
		require.True(t, conflict.IsError)
		require.Contains(t, conflict.Content[0].(*mcp.TextContent).Text, "different input")
		changedSend := send
		changedSend.Text = "different"
		conflict, err = fresh.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: changedSend})
		require.NoError(t, err)
		require.True(t, conflict.IsError)
		require.Contains(t, conflict.Content[0].(*mcp.TextContent).Text, "different input")
	}
	require.Equal(t, beforeHooks, hookCalls.Load())
	require.Equal(t, beforeProvider, providerCalls.Load())
	other, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	denied, err := connect(other).CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: send})
	require.NoError(t, err)
	require.True(t, denied.IsError)
}
