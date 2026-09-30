package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func callChatTool[T any](ctx context.Context, t *testing.T, session *mcp.ClientSession, name string, args any) T {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result.Content)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	var out T
	require.NoError(t, json.Unmarshal([]byte(content.Text), &out))
	return out
}

func TestMCPHTTP_AgentsExactSubmission(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	type wireCall struct {
		model, effort string
		release       chan struct{}
		canceled      chan struct{}
	}
	requests := make(chan wireCall, 8)
	providerURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"Exact selection"}`)
		}
		var body struct {
			Model  string `json:"model"`
			Effort string `json:"reasoning_effort"`
		}
		require.NoError(t, json.Unmarshal(req.RawBody, &body))
		call := wireCall{model: body.Model, effort: body.Effort, release: make(chan struct{}), canceled: make(chan struct{})}
		select {
		case requests <- call:
		case <-ctx.Done():
		}
		select {
		case <-call.release:
		case <-req.Context().Done():
			close(call.canceled)
		case <-ctx.Done():
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	keys := coderdtest.OpenAICompatProviderAPIKeys(providerURL)
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: mcpDeploymentValues(t), ChatProviderAPIKeys: &keys,
	})
	defer closer.Close()
	user := coderdtest.CreateFirstUser(t, client)
	exp := codersdk.NewExperimentalClient(client)
	model := coderdtest.CreateOpenAICompatChatModel(t, exp, providerURL)
	config := &codersdk.ChatModelCallConfig{ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Default: new("low"), Max: new("high")}}
	model, err := exp.UpdateChatModel(ctx, user.OrganizationID, model.ID, codersdk.UpdateChatModelRequest{ModelConfig: config})
	require.NoError(t, err)
	secondModel, err := exp.CreateChatModel(ctx, user.OrganizationID, codersdk.CreateChatModelRequest{
		AIProviderID: &model.AIProviderID, Model: "exact-followup-model", ModelConfig: config, ContextLimit: new(int64(4096)),
	})
	require.NoError(t, err)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	workspace := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{OwnerID: user.UserID, OrganizationID: user.OrganizationID}).WithAgent().Do()
	_ = agenttest.New(t, client.URL, workspace.AgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.Workspace.ID).Wait()

	connect := func(drop bool) (*mcp.ClientSession, *lostChatResponseTransport) {
		httpClient := coderdtest.NewIsolatedHTTPClient(nil)
		lost := &lostChatResponseTransport{base: &headerRoundTripper{base: httpClient.Transport, headers: map[string]string{"Authorization": "Bearer " + client.SessionToken()}}}
		lost.drop.Store(drop)
		httpClient.Transport = lost
		mcpClient := mcp.NewClient(&mcp.Implementation{Name: "exact-agents", Version: "1"}, nil)
		session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: api.AccessURL.String() + mcpserver.MCPEndpoint, HTTPClient: httpClient}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		return session, lost
	}
	session, lost := connect(true)
	models := callChatTool[toolsdk.ListChatModelConfigsResponse](ctx, t, session, toolsdk.ToolNameListChatModelConfigs, toolsdk.ListChatModelConfigsArgs{})
	require.Len(t, models.ModelConfigs, 2)
	require.Equal(t, []string{"none", "minimal", "low", "medium", "high"}, models.ModelConfigs[0].SupportedReasoningEfforts)
	create := toolsdk.CreateChatArgs{
		RequestID: uuid.NewString(), WorkspaceID: workspace.Workspace.ID.String(), Prompt: "first",
		OrganizationID: user.OrganizationID.String(), ModelConfigID: model.ID.String(), ReasoningEffort: new("high"),
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameCreateChat, Arguments: create})
	require.Error(t, err)
	require.True(t, lost.dropped.Load())
	createdReceipt := lostChatReceipt(t, lost)
	// The transport consumed and discarded a successful tool response. A new
	// session recovers its durable identity rather than submitting another chat.
	require.NoError(t, session.Close())
	session, lost = connect(false)
	first := callChatTool[toolsdk.ChatToolStatus](ctx, t, session, toolsdk.ToolNameCreateChat, create)
	require.Equal(t, createdReceipt, first.Submission)
	stored, err := api.Database.GetChatSubmission(dbauthz.AsSystemRestricted(ctx), database.GetChatSubmissionParams{
		OrganizationID: user.OrganizationID, ActorID: user.UserID, RequestID: uuid.MustParse(create.RequestID),
	})
	require.NoError(t, err)
	require.Equal(t, stored.ChatID.String(), first.ID)
	require.Equal(t, stored.ID, first.Submission.ID)
	require.Equal(t, workspace.Workspace.ID.String(), first.WorkspaceID)
	require.Equal(t, "accepted", first.Submission.State)
	require.Equal(t, "high", *first.Submission.Settings.ReasoningEffort)

	var initial wireCall
	select {
	case initial = <-requests:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Equal(t, model.Model, initial.model)
	require.Equal(t, "high", initial.effort)
	// Submit a follow-up while the first turn is blocked at the real provider.
	followup := toolsdk.SendChatMessageArgs{RequestID: uuid.NewString(), ChatID: first.ID, Text: "follow up", ModelConfigID: secondModel.ID.String(), ReasoningEffort: new("low")}
	lost.drop.Store(true)
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: followup})
	require.Error(t, err)
	require.True(t, lost.dropped.Load())
	queuedReceipt := lostChatReceipt(t, lost)
	require.NoError(t, session.Close())
	session, _ = connect(false)
	// Recover the accepted queued message after losing its successful response.
	queued := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, session, toolsdk.ToolNameSendChatMessage, followup)
	require.True(t, queued.Queued)
	require.Equal(t, queuedReceipt, queued.Submission)
	require.NotZero(t, queued.Submission.QueuedMessageID)
	changed := followup
	changed.Text = "different side effect"
	conflict, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: changed})
	require.NoError(t, err)
	require.True(t, conflict.IsError)
	retry := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, session, toolsdk.ToolNameSendChatMessage, followup)
	require.Equal(t, queued.Submission.ID, retry.Submission.ID)
	invalid := followup
	invalid.RequestID = uuid.NewString()
	invalid.ReasoningEffort = new("max")
	rejected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: invalid})
	require.NoError(t, err)
	require.True(t, rejected.IsError)
	close(initial.release)
	var second wireCall
	select {
	case second = <-requests:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Equal(t, "low", second.effort)
	require.Equal(t, secondModel.Model, second.model)
	// The queue row has been promoted, but replay still finds the same receipt.
	promoted := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, session, toolsdk.ToolNameSendChatMessage, followup)
	require.Equal(t, queued.Submission.ID, promoted.Submission.ID)
	require.Equal(t, queued.Submission.QueuedMessageID, promoted.Submission.QueuedMessageID)
	status := callChatTool[toolsdk.ChatToolStatus](ctx, t, session, toolsdk.ToolNameGetChat, toolsdk.GetChatArgs{ChatID: first.ID})
	require.NotNil(t, status.ExactSettings)
	require.Equal(t, "low", *status.ExactSettings.ReasoningEffort)
	close(second.release)
	settled := callChatTool[toolsdk.AwaitChatResponse](ctx, t, session, toolsdk.ToolNameAwaitChat, toolsdk.AwaitChatArgs{ChatID: first.ID, WaitSecs: 10})
	require.False(t, settled.TimedOut)
	require.Equal(t, codersdk.ChatStatusWaiting, settled.Chat.Status)
	messages := callChatTool[toolsdk.GetChatMessagesResponse](ctx, t, session, toolsdk.ToolNameGetChatMessages, toolsdk.GetChatMessagesArgs{ChatID: first.ID})
	userMessages := 0
	for _, message := range messages.Messages {
		if message.Role == codersdk.ChatMessageRoleUser {
			userMessages++
		}
	}
	require.Equal(t, 2, userMessages)
	select {
	case unexpected := <-requests:
		close(unexpected.release)
		t.Fatalf("unexpected provider turn: %+v", unexpected)
	default:
	}

	running := callChatTool[toolsdk.SendChatMessageResponse](ctx, t, session, toolsdk.ToolNameSendChatMessage, toolsdk.SendChatMessageArgs{
		RequestID: uuid.NewString(), ChatID: first.ID, Text: "interrupt this turn", ExactSettings: true,
	})
	require.NotNil(t, running.Submission)
	var interrupted wireCall
	select {
	case interrupted = <-requests:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Equal(t, "low", interrupted.effort)
	callChatTool[toolsdk.ChatToolStatus](ctx, t, session, toolsdk.ToolNameInterruptChat, toolsdk.InterruptChatArgs{ChatID: first.ID})
	select {
	case <-interrupted.canceled:
	case <-ctx.Done():
		t.Fatal("interrupt did not cancel the provider HTTP request")
	}
	interruptedStatus := callChatTool[toolsdk.AwaitChatResponse](ctx, t, session, toolsdk.ToolNameAwaitChat, toolsdk.AwaitChatArgs{ChatID: first.ID, WaitSecs: 10})
	require.False(t, interruptedStatus.TimedOut)
	require.Equal(t, codersdk.ChatStatusWaiting, interruptedStatus.Chat.Status)
	// Another actor cannot use a known chat or workspace to replay its work.
	other, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	otherSession, err := newIsolatedMCPClient(ctx, api.AccessURL.String()+mcpserver.MCPEndpoint, "other-actor", map[string]string{"Authorization": "Bearer " + other.SessionToken()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = otherSession.Close() })
	forbidden, err := otherSession.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameSendChatMessage, Arguments: followup})
	require.NoError(t, err)
	require.True(t, forbidden.IsError)
	forbidden, err = otherSession.CallTool(ctx, &mcp.CallToolParams{Name: toolsdk.ToolNameCreateChat, Arguments: create})
	require.NoError(t, err)
	require.True(t, forbidden.IsError)

	callChatTool[codersdk.Response](ctx, t, session, toolsdk.ToolNameArchiveChat, toolsdk.ArchiveChatArgs{ChatID: first.ID})
	stillExists, err := client.Workspace(ctx, workspace.Workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.Workspace.ID, stillExists.ID)
}

func lostChatReceipt(t *testing.T, transport *lostChatResponseTransport) *codersdk.ChatSubmissionReceipt {
	t.Helper()
	result := transport.accepted.Load()
	require.NotNil(t, result)
	require.False(t, result.IsError, "%+v", result.Content)
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var out struct {
		Submission *codersdk.ChatSubmissionReceipt `json:"submission"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotNil(t, out.Submission)
	require.NotEqual(t, uuid.Nil, out.Submission.ID)
	require.Equal(t, "accepted", out.Submission.State)
	return out.Submission
}

// lostChatResponseTransport loses a successful submission response after the
// server commits it. Every subsequent request uses the real HTTP transport.
type lostChatResponseTransport struct {
	base     http.RoundTripper
	drop     atomic.Bool
	dropped  atomic.Bool
	accepted atomic.Pointer[mcp.CallToolResult]
}

func (t *lostChatResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	lose := false
	if req.Body != nil && t.drop.Load() {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
		var call struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &call) == nil && call.Method == "tools/call" && (call.Params.Name == toolsdk.ToolNameCreateChat || call.Params.Name == toolsdk.ToolNameSendChatMessage) {
			lose = t.drop.CompareAndSwap(true, false)
		}
	}
	response, err := t.base.RoundTrip(req)
	if err != nil || !lose {
		return response, err
	}
	var envelope struct {
		Result *mcp.CallToolResult `json:"result"`
	}
	err = json.NewDecoder(response.Body).Decode(&envelope)
	_ = response.Body.Close()
	if err != nil {
		return nil, err
	}
	t.accepted.Store(envelope.Result)
	t.dropped.Store(true)
	return nil, io.ErrUnexpectedEOF
}
