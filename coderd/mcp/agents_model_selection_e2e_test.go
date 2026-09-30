package mcp_test

import (
	"encoding/json"
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
)

func TestMCPHTTP_DelegateModelSelectionWithoutIdentity(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	models := make(chan string, 8)
	providerURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"Delegate"}`)
		}
		var body struct {
			Model string `json:"model"`
		}
		require.NoError(t, json.Unmarshal(req.RawBody, &body))
		models <- body.Model
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	keys := coderdtest.OpenAICompatProviderAPIKeys(providerURL)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: mcpDeploymentValues(t), ChatProviderAPIKeys: &keys})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	exp := codersdk.NewExperimentalClient(client)
	defaultModel := coderdtest.CreateOpenAICompatChatModel(t, exp, providerURL)
	createModel := func(name string) codersdk.ChatModel {
		model, err := exp.CreateChatModel(ctx, user.OrganizationID, codersdk.CreateChatModelRequest{
			AIProviderID: &defaultModel.AIProviderID, Model: name, IsDefault: new(false), ContextLimit: new(int64(4096)),
		})
		require.NoError(t, err)
		return model
	}
	selected, followup := createModel("delegate-selected-model"), createModel("followup-selected-model")
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	session, err := newIsolatedMCPClient(ctx, api.AccessURL.String()+mcpserver.MCPEndpoint, uuid.NewString(), map[string]string{"Authorization": "Bearer " + client.SessionToken()})
	require.NoError(t, err)
	defer session.Close()
	prompt, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: toolsdk.PromptNameAgentsDelegate, Arguments: map[string]string{
		"task": "Say hello.", "organization_id": user.OrganizationID.String(), "model_config_id": selected.ID.String(),
	}})
	require.NoError(t, err)
	require.Len(t, prompt.Messages, 1)
	text, ok := prompt.Messages[0].Content.(*mcp.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, toolsdk.ToolNameCreateChat)
	require.Contains(t, text.Text, `model_config_id "`+selected.ID.String()+`"`)
	require.NotContains(t, text.Text, "request_id")
	// These are precisely the arguments supplied by the shipped delegate prompt.
	create := toolsdk.CreateChatArgs{Prompt: "Say hello.", OrganizationID: user.OrganizationID.String(), ModelConfigID: selected.ID.String()}
	chat := callChatTool[toolsdk.ChatToolStatus](ctx, t, session, toolsdk.ToolNameCreateChat, create)
	require.Nil(t, chat.Submission)
	wait := func(want codersdk.ChatModel) {
		out := callChatTool[toolsdk.AwaitChatResponse](ctx, t, session, toolsdk.ToolNameAwaitChat, toolsdk.AwaitChatArgs{ChatID: chat.ID, WaitSecs: 10})
		require.False(t, out.TimedOut)
		require.Equal(t, codersdk.ChatStatusWaiting, out.Chat.Status)
		stored, err := exp.GetChat(ctx, uuid.MustParse(chat.ID))
		require.NoError(t, err)
		require.Equal(t, want.ID, stored.LastModelConfigID)
		require.Equal(t, want.Model, testutil.RequireReceive(ctx, t, models))
		require.NotEqual(t, defaultModel.ID, stored.LastModelConfigID)
	}
	wait(selected)
	send := toolsdk.SendChatMessageArgs{ChatID: chat.ID, Text: "Say hello again.", ModelConfigID: followup.ID.String()}
	sent := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, session, toolsdk.ToolNameSendChatMessage, send)
	require.Nil(t, sent.Submission)
	wait(followup)
	// Explicit reasoning or strict-default guarantees still require durable identity.
	for _, strict := range []bool{false, true} {
		strictCreate, strictSend := create, send
		if strict {
			strictCreate.ExactSettings, strictSend.ExactSettings = true, true
		} else {
			strictCreate.ReasoningEffort, strictSend.ReasoningEffort = new("high"), new("high")
		}
		for _, call := range []struct {
			name string
			args any
		}{{toolsdk.ToolNameCreateChat, strictCreate}, {toolsdk.ToolNameSendChatMessage, strictSend}} {
			out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
			require.NoError(t, err)
			require.True(t, out.IsError)
			require.Contains(t, out.Content[0].(*mcp.TextContent).Text, "request_id")
		}
	}
	_, err = exp.UpdateChatModel(ctx, user.OrganizationID, selected.ID, codersdk.UpdateChatModelRequest{Enabled: new(false)})
	require.NoError(t, err)
	for _, modelID := range []string{selected.ID.String(), uuid.NewString()} {
		create.ModelConfigID, send.ModelConfigID = modelID, modelID
		for _, call := range []struct {
			name string
			args any
		}{{toolsdk.ToolNameCreateChat, create}, {toolsdk.ToolNameSendChatMessage, send}} {
			out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
			require.NoError(t, err)
			require.True(t, out.IsError, "an unavailable explicit model must not fall back")
		}
	}
	select {
	case unexpected := <-models:
		t.Fatalf("rejected call dispatched model %q", unexpected)
	default:
	}
}
