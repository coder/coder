package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

func acpTestToolset(t *testing.T) (acpToolset, database.Chat, *agentconnmock.MockAgentConn, workspacesdk.ACPSession) {
	t.Helper()
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	session := workspacesdk.ACPSession{ID: workspacesdk.ACPSessionID{HarnessSlug: "test", WorkingDirectory: "/work", SessionID: "native/id?x=1 &雪"}, HarnessDisplayName: "Workspace Assistant"}
	chat := database.Chat{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, AgentID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	toolset := acpToolset{clock: quartz.NewMock(t), logger: slog.Make(), harnesses: []workspacesdk.ACPHarness{{Slug: "test", DisplayName: session.HarnessDisplayName}}, resolve: func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
		return conn, chat, chat.AgentID.UUID, nil
	}}
	return toolset, chat, conn, session
}

func acpTestArgs(session workspacesdk.ACPSession) acpSessionArgs {
	return acpSessionArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory}
}

func TestACPHarnessSchemaAndGating(t *testing.T) {
	t.Parallel()
	body, err := protojson.Marshal(&agentproto.ACPHarnessBody{Slug: "test", DisplayName: "Test", ConfigOptions: []*agentproto.ACPConfigOption{{Id: "model", Name: "Model", CurrentValue: "a", Values: []*agentproto.ACPConfigValue{{Id: "a", Name: "A"}, {Id: "b", Name: "B"}}}}})
	require.NoError(t, err)
	resources := []database.ChatContextResource{{BodyKind: database.WorkspaceAgentContextBodyKindAcpHarness, Status: database.WorkspaceAgentContextResourceStatusOk, Body: body}, {BodyKind: database.WorkspaceAgentContextBodyKindAcpHarness, Status: database.WorkspaceAgentContextResourceStatusInvalid, Body: body}, {BodyKind: database.WorkspaceAgentContextBodyKindAcpHarness, Status: database.WorkspaceAgentContextResourceStatusOk, Body: []byte("invalid")}}
	harnesses := acpHarnessesFromResources(resources)
	require.Len(t, harnesses, 1)
	props := acpSpawnProperties(harnesses)
	schema, err := json.Marshal(props)
	require.NoError(t, err)
	require.Contains(t, string(schema), `"enum":["a","b"]`)
	require.Contains(t, string(schema), "Default: a.")
	require.NoError(t, validateACPConfig(harnesses[0], map[string]string{"model": "b"}))
	require.Error(t, validateACPConfig(harnesses[0], map[string]string{"model": "c"}))
	require.Error(t, validateACPConfig(harnesses[0], map[string]string{"other": "a"}))
	for _, tc := range []struct {
		name        string
		experiments codersdk.Experiments
		plan, child bool
		noHarness   bool
		want        int
	}{
		{name: "Off"},
		{name: "Plan", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, plan: true},
		{name: "Child", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, child: true},
		{name: "ExecuteRoot", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, want: 3},
		{name: "NoHarness", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, noHarness: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			chat := database.Chat{ID: uuid.New()}
			if tc.child {
				chat.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			}
			if tc.want > 0 || tc.noHarness {
				pinned := resources
				if tc.noHarness {
					pinned = resources[1:]
				}
				db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).Return(pinned, nil)
			}
			server := &Server{db: db, experiments: tc.experiments}
			tools := server.acpTools(context.Background(), rootChatToolsOptions{chat: chat, isPlanModeTurn: tc.plan})
			require.Len(t, tools, tc.want)
		})
	}
	toolset := acpToolset{harnesses: harnesses}
	tools := newACPTools(toolset)
	require.Len(t, tools, 3)
	definitions := chatloop.BuildToolDefinitions(tools, nil, nil)
	spawn := definitions[0].(fantasy.FunctionTool)
	require.Equal(t, props, spawn.InputSchema["properties"])
	require.ElementsMatch(t, []string{"prompt", "working_directory", "agent"}, spawn.InputSchema["required"])
	for _, tool := range tools {
		require.True(t, tool.Info().Parallel)
	}
	for i, fields := range [][]string{{"harness_id", "session_id", "working_directory", "message"}, {"harness_id", "session_id", "working_directory"}} {
		definition := definitions[i+1].(fantasy.FunctionTool)
		require.ElementsMatch(t, fields, definition.InputSchema["required"])
		properties := definition.InputSchema["properties"].(map[string]any)
		require.Len(t, properties, len(fields))
		for _, field := range fields {
			require.Contains(t, properties, field)
			require.NotEmpty(t, properties[field].(map[string]any)["description"])
		}
	}
	toolset.harnesses = nil
	require.Empty(t, newACPTools(toolset))
}

func TestACPValidation(t *testing.T) {
	t.Parallel()
	ctx := workspacesdk.WithToolCallID(context.Background(), uuid.New())
	tools := newACPTools(acpToolset{harnesses: []workspacesdk.ACPHarness{{Slug: "test"}}})
	prepared := turnEnvironment{Tools: tools, BuiltinToolNames: map[string]bool{}}
	for _, tool := range tools {
		prepared.BuiltinToolNames[tool.Info().Name] = true
	}
	for _, input := range []string{
		`{}`, `{"harness_id":"test","session_id":"native"}`,
		`{"session_id":"native","working_directory":"/work"}`,
		`{"harness_id":"test","working_directory":"/work"}`,
		`{"harness_id":"","session_id":"native","working_directory":"/work"}`,
		`{"harness_id":"test","session_id":"","working_directory":"/work"}`,
		`{"harness_id":"test","session_id":"native","working_directory":""}`,
		`{"harness_id":"test","session_id":"native","working_directory":" "}`,
		`{"harness_id":"test","session_id":null,"working_directory":"/work"}`,
		`{"harness_id":"test","session_id":"native","session_id":"duplicate","working_directory":"/work"}`,
		`{"harness_id":"test","Session_id":"native","working_directory":"/work"}`,
		`null`, `{} {}`,
	} {
		call := fantasy.ToolCallContent{ToolName: tools[2].Info().Name, Input: input}
		allowed, _, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{call})
		if len(rejected) > 0 {
			require.Empty(t, allowed)
			continue
		}
		response, err := tools[2].Run(context.Background(), fantasy.ToolCall{Input: input})
		require.NoError(t, err)
		require.True(t, response.IsError, input)
	}
	for _, input := range []string{
		`{"prompt":"task","working_directory":"/work","agent":{"harness":"test"}}`,
		`{"prompt":"task","working_directory":"/work","agent":{}}`,
		`{"prompt":"task","working_directory":"/work","agent":{"harness_id":null}}`,
		`{"prompt":"task","working_directory":"/work","agent":{"harness_id":"unknown"}}`,
		`{"prompt":"task","working_directory":"/work","agent":{"harness_id":"test","config":{"unknown":"value"}}}`,
	} {
		response, err := tools[0].Run(ctx, fantasy.ToolCall{Input: input})
		require.NoError(t, err)
		require.True(t, response.IsError, input)
	}
	for _, input := range []string{
		`{"prompt":"task","working_directory":"/work","agent":{"Harness_id":"test"}}`,
		`{"prompt":"task","working_directory":"/work","agent":{"harness_id":"test","Config":{}}}`,
		`{"prompt":"task","working_directory":"/work","agent":{"harness_id":"test","harness_id":"other"}}`,
	} {
		call := fantasy.ToolCallContent{ToolName: tools[0].Info().Name, Input: input}
		allowed, _, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{call})
		require.Empty(t, allowed, input)
		require.Len(t, rejected, 1, input)
		result := rejected[0].Result.(fantasy.ToolResultOutputContentError)
		require.ErrorContains(t, result.Error, "input", input)
	}
}

func TestACPSpawnRetryAndInitialFailure(t *testing.T) {
	t.Parallel()
	toolset, _, conn, session := acpTestToolset(t)
	session.ID.WorkingDirectory = `C:\work`
	requestID := uuid.New()
	ctx := workspacesdk.WithToolCallID(context.Background(), requestID)
	input := `{"prompt":"task","working_directory":"C:\\work","agent":{"harness_id":"test"}}`
	create := workspacesdk.ACPCreateSessionRequest{HarnessSlug: session.ID.HarnessSlug, WorkingDirectory: session.ID.WorkingDirectory}
	message := workspacesdk.ACPMessageRequest{Text: "task"}
	promptContext := gomock.Cond(func(ctx context.Context) bool {
		id, ok := workspacesdk.ToolCallIDFromContext(ctx)
		return ok && id == uuid.NewSHA1(requestID, []byte("initial-prompt"))
	})
	gomock.InOrder(
		conn.EXPECT().CreateACPSession(gomock.Any(), create).Return(session, nil),
		conn.EXPECT().SendACPMessage(promptContext, session.ID, message).Return(workspacesdk.ACPMessageResponse{}, xerrors.New("submission failed")),
		conn.EXPECT().CreateACPSession(gomock.Any(), create).Return(session, nil),
		conn.EXPECT().SendACPMessage(promptContext, session.ID, message).Return(workspacesdk.ACPMessageResponse{}, nil),
	)
	tool := newACPTools(toolset)[0]
	first, err := tool.Run(ctx, fantasy.ToolCall{Input: input})
	require.NoError(t, err)
	require.True(t, first.IsError)
	var failed map[string]any
	require.NoError(t, json.Unmarshal([]byte(first.Content), &failed))
	require.Equal(t, session.ID.SessionID, failed["session_id"])
	require.Contains(t, first.Content, `"harness_display_name":"Workspace Assistant"`)
	retry, err := tool.Run(ctx, fantasy.ToolCall{Input: input})
	require.NoError(t, err)
	require.False(t, retry.IsError)
	require.JSONEq(t, `{"session_id":"native/id?x=1 &雪","harness_id":"test","harness_display_name":"Workspace Assistant","status":"running"}`, retry.Content)
}

func TestACPMessageAndInterrupt(t *testing.T) {
	t.Parallel()
	toolset, _, conn, session := acpTestToolset(t)
	session.ID.WorkingDirectory = `C:\work`
	id := uuid.New()
	ctx := workspacesdk.WithToolCallID(context.Background(), id)
	tools := newACPTools(toolset)
	conn.EXPECT().SendACPMessage(gomock.Any(), session.ID, workspacesdk.ACPMessageRequest{Text: "follow-up"}).Return(workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomePrompt}, nil)
	messageInput, err := json.Marshal(acpMessageArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory, Message: "follow-up"})
	require.NoError(t, err)
	response, err := tools[1].Run(ctx, fantasy.ToolCall{Input: string(messageInput)})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.Contains(t, response.Content, `"harness_display_name":"Workspace Assistant"`)
	conn.EXPECT().InterruptACPSession(gomock.Any(), session.ID).Return(workspacesdk.ACPSession{Status: workspacesdk.ACPSessionStatusInterrupting}, nil)
	interruptInput, err := json.Marshal(acpTestArgs(session))
	require.NoError(t, err)
	response, err = tools[2].Run(ctx, fantasy.ToolCall{Input: string(interruptInput)})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.Contains(t, response.Content, `"interrupted":true`)
	require.Contains(t, response.Content, `"harness_display_name":"Workspace Assistant"`)
}

func TestACPWorkspaceSessionAccess(t *testing.T) {
	t.Parallel()
	toolset, chat, conn, session := acpTestToolset(t)
	for range 2 {
		chat.ID = uuid.New()
		currentChat := chat
		toolset.resolve = func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
			return conn, currentChat, currentChat.AgentID.UUID, nil
		}
		requestID := uuid.New()
		ctx := workspacesdk.WithToolCallID(context.Background(), requestID)
		conn.EXPECT().SendACPMessage(gomock.Any(), session.ID, workspacesdk.ACPMessageRequest{Text: "continue"}).Return(workspacesdk.ACPMessageResponse{}, nil)
		input, err := json.Marshal(acpMessageArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory, Message: "continue"})
		require.NoError(t, err)
		response, err := newACPTools(toolset)[1].Run(ctx, fantasy.ToolCall{Input: string(input)})
		require.NoError(t, err)
		require.False(t, response.IsError)
		require.JSONEq(t, `{"session_id":"native/id?x=1 &雪","harness_id":"test","working_directory":"/work","harness_display_name":"Workspace Assistant","status":"running"}`, response.Content)
	}
	conn.EXPECT().InterruptACPSession(gomock.Any(), session.ID).Return(workspacesdk.ACPSession{}, codersdk.NewError(404, codersdk.Response{Message: "session missing"}))
	response, err := toolset.interrupt(context.Background(), acpTestArgs(session), fantasy.ToolCall{})
	require.NoError(t, err)
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "session missing")
}
