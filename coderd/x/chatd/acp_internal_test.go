package chatd

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/go-playground/validator/v10"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func acpTestToolset(t *testing.T) (acpToolset, database.Chat, *agentconnmock.MockAgentConn, workspacesdk.ACPSession) {
	t.Helper()
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	session := workspacesdk.ACPSession{ID: workspacesdk.ACPSessionID{HarnessSlug: "test", WorkingDirectory: "/work", SessionID: "native/id?x=1 &雪"}, HarnessDisplayName: "Workspace Assistant"}
	chat := database.Chat{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, AgentID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	conn.EXPECT().AgentID().Return(chat.AgentID.UUID).AnyTimes()
	toolset := acpToolset{clock: quartz.NewMock(t), logger: slog.Make(), harnesses: []workspacesdk.ACPHarness{{Slug: "test", DisplayName: session.HarnessDisplayName}}, resolve: func(context.Context) (workspacesdk.AgentConn, error) {
		return conn, nil
	}}
	return toolset, chat, conn, session
}

func acpTestArgs(session workspacesdk.ACPSession) acpSessionArgs {
	return acpSessionArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory}
}

func acpEvent(generationID uuid.UUID, seq uint64, kind workspacesdk.ACPEventKind, text string) workspacesdk.ACPEvent {
	event := workspacesdk.ACPEvent{Cursor: workspacesdk.ACPCursor{GenerationID: generationID, Seq: seq}, Kind: kind, Text: text}
	if kind == workspacesdk.ACPEventKindUpdate {
		event.Text = ""
		event.Update = json.RawMessage(text)
	}
	return event
}

func acpSnapshot(generationID uuid.UUID, seq uint64, status workspacesdk.ACPSessionStatus) workspacesdk.ACPEvent {
	event := acpEvent(generationID, seq, workspacesdk.ACPEventKindSnapshot, "")
	event.Session = &workspacesdk.ACPSession{Status: status, Cursor: event.Cursor}
	return event
}

func acpEventStream(events ...workspacesdk.ACPEvent) <-chan workspacesdk.ACPEvent {
	ch := make(chan workspacesdk.ACPEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

func acpUserResult(t *testing.T, response fantasy.ToolResponse) map[string]any {
	t.Helper()
	data, err := chattool.UserResultFromMetadata(response.Metadata)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

type acpWaitOutcome struct {
	response fantasy.ToolResponse
	err      error
}

func acpWaitAsync(ctx context.Context, toolset acpToolset, session workspacesdk.ACPSession, timeout *int) <-chan acpWaitOutcome {
	done := make(chan acpWaitOutcome, 1)
	go func() {
		response, err := toolset.wait(ctx, acpWaitArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory, TimeoutSeconds: timeout}, fantasy.ToolCall{ID: "outer"})
		done <- acpWaitOutcome{response, err}
	}()
	return done
}

type acpStreamPart struct {
	MessageIndex int                      `json:"message_index"`
	Role         codersdk.ChatMessageRole `json:"role"`
	Part         codersdk.ChatMessagePart `json:"part"`
}

func acpStreamParts(t *testing.T, part codersdk.ChatMessagePart) []acpStreamPart {
	t.Helper()
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, part.Type)
	require.Equal(t, "acp_wait_agent", part.ToolName)
	require.Equal(t, "outer", part.ToolCallID)
	require.False(t, part.ResultReset)
	require.Empty(t, part.Result)
	require.True(t, strings.HasSuffix(part.ResultDelta, "\n"))
	var parts []acpStreamPart
	for line := range strings.SplitSeq(strings.TrimSuffix(part.ResultDelta, "\n"), "\n") {
		var record acpStreamPart
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.NotEmpty(t, record.Part.Type)
		parts = append(parts, record)
	}
	return parts
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
		{name: "ExecuteRoot", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, want: 4},
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
	require.Len(t, tools, 4)
	definitions := chatloop.BuildToolDefinitions(tools, nil, nil)
	spawn := definitions[0].(fantasy.FunctionTool)
	require.Equal(t, props, spawn.InputSchema["properties"])
	require.ElementsMatch(t, []string{"prompt", "working_directory", "agent"}, spawn.InputSchema["required"])
	for _, tool := range tools {
		require.True(t, tool.Info().Parallel)
	}
	wait := definitions[1].(fantasy.FunctionTool)
	require.Equal(t, "acp_wait_agent", wait.Name)
	require.ElementsMatch(t, []string{"harness_id", "session_id", "working_directory"}, wait.InputSchema["required"])
	waitProperties := wait.InputSchema["properties"].(map[string]any)
	require.Len(t, waitProperties, 4)
	for _, field := range []string{"harness_id", "session_id", "working_directory", "timeout_seconds"} {
		require.Contains(t, waitProperties, field)
		require.NotEmpty(t, waitProperties[field].(map[string]any)["description"])
	}
	for i, fields := range [][]string{{"harness_id", "session_id", "working_directory", "message"}, {"harness_id", "session_id", "working_directory"}} {
		definition := definitions[i+2].(fantasy.FunctionTool)
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
		call := fantasy.ToolCallContent{ToolName: tools[1].Info().Name, Input: input}
		allowed, _, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{call})
		if len(rejected) > 0 {
			require.Empty(t, allowed)
			continue
		}
		response, err := tools[1].Run(context.Background(), fantasy.ToolCall{Input: input})
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
	response, err := tools[2].Run(ctx, fantasy.ToolCall{Input: string(messageInput)})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.Contains(t, response.Content, `"harness_display_name":"Workspace Assistant"`)
	conn.EXPECT().InterruptACPSession(gomock.Any(), session.ID).Return(workspacesdk.ACPSession{Status: workspacesdk.ACPSessionStatusInterrupting}, nil)
	interruptInput, err := json.Marshal(acpTestArgs(session))
	require.NoError(t, err)
	response, err = tools[3].Run(ctx, fantasy.ToolCall{Input: string(interruptInput)})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.Contains(t, response.Content, `"interrupted":true`)
	require.Contains(t, response.Content, `"harness_display_name":"Workspace Assistant"`)
}

func TestACPWaitBaselineAndLive(t *testing.T) {
	t.Parallel()
	opts, _, conn, session := acpTestToolset(t)
	generationID := uuid.New()
	events := make(chan workspacesdk.ACPEvent, 16)
	conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), session.ID, workspacesdk.ACPReadOptions{}).Return((<-chan workspacesdk.ACPEvent)(events), io.NopCloser(strings.NewReader("")), nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	published := make(chan codersdk.ChatMessagePart, 32)
	ctx = chatloop.WithMessagePartPublisher(ctx, func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) { published <- part })
	done := acpWaitAsync(ctx, opts, session, nil)
	historical := acpSnapshot(generationID, 1, workspacesdk.ACPSessionStatusIdle)
	historical.Kind = workspacesdk.ACPEventKindStatus
	events <- historical
	events <- acpEvent(generationID, 2, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"earlier"}}`)
	baseline := acpStreamParts(t, testutil.RequireReceive(ctx, t, published))
	require.Equal(t, []acpStreamPart{{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("earlier")}}, baseline)
	events <- acpSnapshot(generationID, 2, workspacesdk.ACPSessionStatusInterrupting)
	reasoningText := strings.Repeat("private reasoning", (1<<20)/len("private reasoning")+1)
	events <- acpEvent(generationID, 3, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"`+reasoningText+`"}}`)
	reasoning := acpStreamParts(t, testutil.RequireReceive(ctx, t, published))
	require.Equal(t, []acpStreamPart{{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageReasoning(reasoningText)}}, reasoning)
	events <- acpEvent(generationID, 4, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer\n\"quoted\""}}`)
	answer := acpStreamParts(t, testutil.RequireReceive(ctx, t, published))
	require.Equal(t, []acpStreamPart{{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("answer\n\"quoted\"")}}, answer)
	terminal := acpSnapshot(generationID, 5, workspacesdk.ACPSessionStatusIdle)
	terminal.Kind = workspacesdk.ACPEventKindStatus
	events <- terminal
	result := testutil.RequireReceive(ctx, t, done)
	require.NoError(t, result.err)
	require.False(t, result.response.IsError)
	require.Empty(t, published)
	require.NotContains(t, result.response.Content, "private reasoning")
	require.Contains(t, result.response.Content, "answer")
	user := acpUserResult(t, result.response)
	require.Equal(t, session.HarnessDisplayName, user["harness_display_name"])
	require.Equal(t, session.ID.WorkingDirectory, user["working_directory"])
	require.Equal(t, session.ID.SessionID, user["session_id"])
	require.Equal(t, session.ID.HarnessSlug, user["harness_id"])
	require.NotContains(t, result.response.Content, "working_directory")
	encoded, err := json.Marshal(user)
	require.NoError(t, err)
	require.Contains(t, string(encoded), reasoningText)
	require.Equal(t, false, user["timed_out"])
}

func TestACPWaitTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		seconds    *int
		duration   time.Duration
		cancelRoot bool
	}{
		{name: "Default", duration: 5 * time.Minute},
		{name: "Timeout", seconds: new(1), duration: time.Second},
		{name: "LongTimeout", seconds: new(600), duration: 10 * time.Minute},
		{name: "ZeroTimeout", seconds: new(0)},
		{name: "NegativeTimeout", seconds: new(-1), duration: -time.Second},
		{name: "Cancellation", seconds: new(600), duration: 10 * time.Minute, cancelRoot: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, _, conn, session := acpTestToolset(t)
			clock := opts.clock.(*quartz.Mock)
			ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
			ctx = chatloop.WithMessagePartPublisher(ctx, func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
			defer cancel()
			ready := make(chan context.Context, 1)
			generationID := uuid.New()
			events := make(chan workspacesdk.ACPEvent, 2)
			events <- acpSnapshot(generationID, 1, workspacesdk.ACPSessionStatusRunning)
			watch := conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(waitCtx context.Context, _ slog.Logger, _ workspacesdk.ACPSessionID, _ workspacesdk.ACPReadOptions) (<-chan workspacesdk.ACPEvent, io.Closer, error) {
				ready <- waitCtx
				return events, io.NopCloser(strings.NewReader("")), nil
			})
			if tc.duration <= 0 {
				watch.MaxTimes(1)
			}
			done := acpWaitAsync(ctx, opts, session, tc.seconds)
			if tc.duration > 0 {
				waitCtx := testutil.RequireReceive(ctx, t, ready)
				if tc.cancelRoot {
					cancel()
				} else {
					clock.Advance(tc.duration - time.Second).MustWait(ctx)
					require.NoError(t, waitCtx.Err())
					clock.Advance(time.Second).MustWait(ctx)
				}
			}
			result := testutil.RequireReceive(testutil.Context(t, testutil.WaitLong), t, done)
			if tc.cancelRoot {
				require.ErrorIs(t, result.err, context.Canceled)
			} else {
				require.NoError(t, result.err)
				require.False(t, result.response.IsError)
				require.Contains(t, result.response.Content, `"timed_out":true`)
			}
		})
	}
}

func TestACPWaitTerminalFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, error string
	}{
		{"SessionError", "adapter died"},
		{"MissingSnapshotSession", "ACP snapshot has no session"},
		{"InvalidUpdate", "decode ACP update: unexpected end of JSON input"},
		{"MissingToolCallID", "invalid ACP tool_call_update: Key: 'toolCallId' Error:Field validation for 'toolCallId' failed on the 'required' tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, _, conn, session := acpTestToolset(t)
			generationID := uuid.New()
			failure := acpSnapshot(generationID, 3, workspacesdk.ACPSessionStatusError)
			failure.Session.Error = "adapter died"
			switch tc.name {
			case "MissingSnapshotSession":
				failure.Session = nil
			case "InvalidUpdate":
				failure = acpEvent(generationID, 3, workspacesdk.ACPEventKindUpdate, `{`)
			case "MissingToolCallID":
				failure = acpEvent(generationID, 3, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"tool_call_update","status":"completed"}`)
			}
			events := acpEventStream(acpEvent(generationID, 1, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial"}}`), acpEvent(generationID, 2, workspacesdk.ACPEventKindError, "adapter died"), failure)
			conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(events, io.NopCloser(strings.NewReader("")), nil)
			ctx := chatloop.WithMessagePartPublisher(testutil.Context(t, testutil.WaitLong), func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
			result, err := opts.wait(ctx, acpWaitArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory}, fantasy.ToolCall{ID: "outer"})
			require.NoError(t, err)
			require.True(t, result.IsError)
			user := acpUserResult(t, result)
			require.Equal(t, tc.error, user["error"])
			require.Equal(t, false, user["timed_out"])
			require.Contains(t, result.Content, "partial")
			require.Len(t, user["messages"], 1)
		})
	}
}

func TestACPWaitReconnect(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, response, error string
	}{
		{"Cursor", "beforeafter", ""},
		{"BeforeSnapshot", "beforeafter", ""},
		{"SequenceGap", "beforeafter", ""},
		{"NoUpdates", "before", ""},
		{"GenerationIDChanged", "before", "generation changed"},
		{"AgentReplacement", "before", "ACP workspace agent changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, _, conn, session := acpTestToolset(t)
			clock := opts.clock.(*quartz.Mock)
			trap := clock.Trap().NewTimer("acp-retry")
			defer trap.Close()
			generationID := uuid.New()
			cursor := workspacesdk.ACPCursor{GenerationID: generationID, Seq: 1}
			before := acpEvent(generationID, 1, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"before"}}`)
			first := acpEventStream(before, acpSnapshot(generationID, 1, workspacesdk.ACPSessionStatusRunning))
			if tc.name == "BeforeSnapshot" {
				first = acpEventStream(before)
			}
			if tc.name == "SequenceGap" {
				first = acpEventStream(before, acpSnapshot(generationID, 1, workspacesdk.ACPSessionStatusRunning), acpEvent(generationID, 3, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"skipped"}}`))
			}
			conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), session.ID, workspacesdk.ACPReadOptions{}).Return(first, io.NopCloser(strings.NewReader("")), nil)
			if tc.name == "AgentReplacement" {
				replacement := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				replacement.EXPECT().AgentID().Return(uuid.New())
				calls := 0
				opts.resolve = func(context.Context) (workspacesdk.AgentConn, error) {
					calls++
					if calls > 1 {
						return replacement, nil
					}
					return conn, nil
				}
			}
			switch tc.name {
			case "GenerationIDChanged":
				conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), session.ID, workspacesdk.ACPReadOptions{After: &cursor}).Return(nil, nil, codersdk.NewError(409, codersdk.Response{Message: tc.error}))
			case "AgentReplacement":
			default:
				second := acpEventStream(before, acpEvent(generationID, 2, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"after"}}`), acpSnapshot(generationID, 2, workspacesdk.ACPSessionStatusIdle))
				if tc.name == "NoUpdates" {
					terminal := acpSnapshot(generationID, 2, workspacesdk.ACPSessionStatusIdle)
					terminal.Kind = workspacesdk.ACPEventKindStatus
					second = acpEventStream(acpSnapshot(generationID, 1, workspacesdk.ACPSessionStatusRunning), terminal)
				}
				conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), session.ID, workspacesdk.ACPReadOptions{After: &cursor}).Return(second, io.NopCloser(strings.NewReader("")), nil)
			}
			ctx := testutil.Context(t, testutil.WaitLong)
			var parts []codersdk.ChatMessagePart
			ctx = chatloop.WithMessagePartPublisher(ctx, func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
				parts = append(parts, part)
			})
			done := acpWaitAsync(ctx, opts, session, nil)
			trap.MustWait(ctx).MustRelease(ctx)
			clock.Advance(250 * time.Millisecond).MustWait(ctx)
			result := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, result.err)
			require.Equal(t, tc.error != "", result.response.IsError)
			if tc.error != "" {
				require.Contains(t, result.response.Content, tc.error)
			}
			require.Contains(t, result.response.Content, `"response":"`+tc.response+`"`)
			require.Equal(t, false, acpUserResult(t, result.response)["timed_out"])
			var streamed []acpStreamPart
			for _, part := range parts {
				streamed = append(streamed, acpStreamParts(t, part)...)
			}
			expected := []acpStreamPart{{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("before")}}
			if tc.response == "beforeafter" {
				expected = append(expected, acpStreamPart{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("after")})
			}
			require.Equal(t, expected, streamed)
			encoded, err := json.Marshal(acpUserResult(t, result.response)["messages"])
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"text":"`+tc.response+`"`)
		})
	}
}

func TestACPTranscriptReduction(t *testing.T) {
	t.Parallel()
	transcript := newACPTranscript()
	updates := []string{
		`{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"help","description":"List commands"}]}`,
		`{"sessionUpdate":"agent_thought_chunk","messageId":"a1","content":{"type":"text","text":"think"}}`,
		`{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","status":"pending","rawInput":{"path":"/work/file"},"_meta":{"private":"tool protocol metadata"}}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"tool","status":"in_progress","rawInput":{"path":"/work/other"}}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"tool","status":"completed","rawOutput":"contents","_meta":{"private":"updated tool protocol metadata"}}`,
		`{"sessionUpdate":"agent_message_chunk","messageId":"a2","content":{"type":"text","text":"an"}}`,
		`{"sessionUpdate":"usage_update","used":18722,"size":1000000}`,
		`{"sessionUpdate":"agent_message_chunk","messageId":"a2","content":{"type":"text","text":"swer"}}`,
		`{"sessionUpdate":"future_variant","content":{"type":"text","text":"future content"}}`,
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"future_media","data":"binary"}}`,
	}
	var streamed []acpTranscriptPart
	for _, update := range updates {
		parts, err := transcript.consume(workspacesdk.ACPEvent{Kind: workspacesdk.ACPEventKindUpdate, Update: json.RawMessage(update)})
		require.NoError(t, err)
		streamed = append(streamed, parts...)
	}
	require.Len(t, streamed, 7)
	require.Equal(t, acpTranscriptPart{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageReasoning("think")}, streamed[0])
	require.Equal(t, acpTranscriptPart{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageToolCall("tool", "Read", json.RawMessage(`{"path":"/work/file"}`))}, streamed[1])
	require.Equal(t, acpTranscriptPart{MessageIndex: 0, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageToolCall("tool", "Read", json.RawMessage(`{"path":"/work/other"}`))}, streamed[2])
	require.Equal(t, 1, streamed[3].MessageIndex)
	require.Equal(t, codersdk.ChatMessageRoleTool, streamed[3].Role)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, streamed[3].Part.Type)
	require.Equal(t, "tool", streamed[3].Part.ToolCallID)
	require.Contains(t, string(streamed[3].Part.Result), `"status":"completed"`)
	require.Contains(t, string(streamed[3].Part.Result), `"output":"contents"`)
	require.Equal(t, acpTranscriptPart{MessageIndex: 2, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("an")}, streamed[4])
	require.Equal(t, acpTranscriptPart{MessageIndex: 2, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("swer")}, streamed[5])
	require.Equal(t, "answer", transcript.response.String())
	require.Equal(t, codersdk.ChatMessageRoleAssistant, transcript.messages[0].Role)
	require.Len(t, transcript.tools, 1)
	tool := transcript.tools["tool"]
	require.Contains(t, string(transcript.messages[tool.resultMessage].Content[0].Result), "contents")
	encoded, err := json.Marshal(transcript.messages)
	require.NoError(t, err)
	preview := string(encoded)
	require.Contains(t, preview, "think")
	require.NotContains(t, preview, "availableCommands")
	require.NotContains(t, preview, "usage_update")
	require.NotContains(t, preview, "future_variant")
	require.NotContains(t, preview, "meta")
	require.Contains(t, preview, `"text":"answer`)
	parts, err := transcript.consume(workspacesdk.ACPEvent{Kind: workspacesdk.ACPEventKindUpdate, Update: json.RawMessage(`{"sessionUpdate":"tool_call_update","toolCallId":"new","status":"failed"}`)})
	require.NoError(t, err)
	require.Len(t, transcript.tools, 2)
	require.Len(t, parts, 2)
	require.Equal(t, "new", parts[0].Part.ToolCallID)
	require.Equal(t, "new", parts[1].Part.ToolCallID)
	require.True(t, parts[1].Part.IsError)
}

func TestACPTranscriptValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, update, field, tag string
	}{
		{"MissingUpdateKind", `{}`, "sessionUpdate", "required"},
		{"NullUpdateKind", `{"sessionUpdate":null}`, "sessionUpdate", "required"},
		{"EmptyUpdateKind", `{"sessionUpdate":""}`, "sessionUpdate", "required"},
		{"MissingContent", `{"sessionUpdate":"agent_message_chunk"}`, "content", "required"},
		{"NullContent", `{"sessionUpdate":"agent_message_chunk","content":null}`, "content", "required"},
		{"MissingContentType", `{"sessionUpdate":"agent_message_chunk","content":{"text":"answer"}}`, "type", "required"},
		{"NullContentType", `{"sessionUpdate":"agent_message_chunk","content":{"type":null,"text":"answer"}}`, "type", "required"},
		{"MissingText", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text"}}`, "text", "required_if"},
		{"NullText", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":null}}`, "text", "required_if"},
		{"WrongTextType", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":42}}`, "", ""},
		{"MissingThoughtContent", `{"sessionUpdate":"agent_thought_chunk"}`, "content", "required"},
		{"MissingThoughtText", `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text"}}`, "text", "required_if"},
		{"MissingToolCallID", `{"sessionUpdate":"tool_call","title":"Read"}`, "toolCallId", "required"},
		{"EmptyToolCallID", `{"sessionUpdate":"tool_call","toolCallId":"","title":"Read"}`, "toolCallId", "required"},
		{"NullToolCallID", `{"sessionUpdate":"tool_call","toolCallId":null,"title":"Read"}`, "toolCallId", "required"},
		{"MissingToolTitle", `{"sessionUpdate":"tool_call","toolCallId":"tool"}`, "title", "required_if"},
		{"NullToolTitle", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":null}`, "title", "required_if"},
		{"InvalidToolStatus", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","status":"invalid"}`, "status", "oneof"},
		{"EmptyToolStatus", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","status":""}`, "status", "oneof"},
		{"InvalidToolKind", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","kind":"invalid"}`, "kind", "oneof"},
		{"EmptyToolKind", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","kind":""}`, "kind", "oneof"},
		{"MissingToolUpdateID", `{"sessionUpdate":"tool_call_update","status":"completed"}`, "toolCallId", "required"},
		{"EmptyToolUpdateID", `{"sessionUpdate":"tool_call_update","toolCallId":"","status":"completed"}`, "toolCallId", "required"},
		{"InvalidToolUpdateStatus", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","status":"invalid"}`, "status", "oneof"},
		{"EmptyToolUpdateStatus", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","status":""}`, "status", "oneof"},
		{"InvalidToolUpdateKind", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","kind":"invalid"}`, "kind", "oneof"},
		{"EmptyToolUpdateKind", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","kind":""}`, "kind", "oneof"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transcript := newACPTranscript()
			parts, err := transcript.consume(workspacesdk.ACPEvent{Kind: workspacesdk.ACPEventKindUpdate, Update: json.RawMessage(tc.update)})
			if tc.field == "" {
				var decodeError *json.UnmarshalTypeError
				require.ErrorAs(t, err, &decodeError)
			} else {
				var validationErrors validator.ValidationErrors
				require.ErrorAs(t, err, &validationErrors)
				require.Len(t, validationErrors, 1)
				require.Equal(t, tc.field, validationErrors[0].Field())
				require.Equal(t, tc.tag, validationErrors[0].Tag())
			}
			require.Empty(t, parts)
			require.Empty(t, transcript.messages)
			require.Empty(t, transcript.tools)
			require.Empty(t, transcript.response.String())
		})
	}
}

func TestACPTranscriptExtensions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, update, response string
		parts                  int
	}{
		{"EmptyText", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":""}}`, "", 1},
		{"EmptyThought", `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":""}}`, "", 1},
		{"MessageExtensions", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer","extension":true},"_meta":{"key":"value"},"extension":true}`, "answer", 1},
		{"MessageWithToolFields", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer"},"toolCallId":"tool","title":"Read","status":"completed"}`, "answer", 1},
		{"ImageWithTextField", `{"sessionUpdate":"agent_message_chunk","content":{"type":"image","data":"binary","mimeType":"image/png","text":"hidden"}}`, "", 1},
		{"ImageWithNumericTextField", `{"sessionUpdate":"agent_message_chunk","content":{"type":"image","data":"binary","mimeType":"image/png","text":42}}`, "", 1},
		{"UnknownContentWithTextField", `{"sessionUpdate":"agent_message_chunk","content":{"type":"future_media","text":"hidden"}}`, "", 1},
		{"ToolDefaults", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":""}`, "", 1},
		{"ToolKind", `{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","kind":"read","status":"pending"}`, "", 1},
		{"ToolUpdateWithoutStatus", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","title":"Read","extension":true}`, "", 1},
		{"ToolUpdateWithNullFields", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","title":null,"kind":null,"status":null}`, "", 1},
		{"ToolUpdateKind", `{"sessionUpdate":"tool_call_update","toolCallId":"tool","kind":"execute","status":"completed"}`, "", 2},
		{"UnknownUpdate", `{"sessionUpdate":"future_variant","content":{"text":"ignored"}}`, "", 0},
		{"UnrelatedUpdate", `{"sessionUpdate":"usage_update","used":42,"size":100}`, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transcript := newACPTranscript()
			parts, err := transcript.consume(workspacesdk.ACPEvent{Kind: workspacesdk.ACPEventKindUpdate, Update: json.RawMessage(tc.update)})
			require.NoError(t, err)
			require.Len(t, parts, tc.parts)
			require.Equal(t, tc.response, transcript.response.String())
		})
	}
}

func TestACPWaitIncrementalBuffer(t *testing.T) {
	t.Parallel()
	opts, chat, conn, session := acpTestToolset(t)
	generationID := uuid.New()
	const chunks = 256
	text := strings.Repeat("x", 1024)
	update, err := json.Marshal(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}})
	require.NoError(t, err)
	events := []workspacesdk.ACPEvent{acpSnapshot(generationID, 0, workspacesdk.ACPSessionStatusRunning)}
	for seq := range uint64(chunks) {
		events = append(events, acpEvent(generationID, seq+1, workspacesdk.ACPEventKindUpdate, string(update)))
	}
	terminal := acpSnapshot(generationID, chunks+1, workspacesdk.ACPSessionStatusIdle)
	terminal.Kind = workspacesdk.ACPEventKindStatus
	events = append(events, terminal)
	conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), session.ID, workspacesdk.ACPReadOptions{}).Return(acpEventStream(events...), io.NopCloser(strings.NewReader("")), nil)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := messagepartbuffer.Key{ChatID: chat.ID}
	require.NoError(t, buffer.CreateEpisode(key))
	var publishErrors []error
	var published int
	ctx := chatloop.WithMessagePartPublisher(testutil.Context(t, testutil.WaitLong), func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		published++
		if err := buffer.AddPart(key, role, part); err != nil {
			publishErrors = append(publishErrors, err)
		}
	})
	result, err := opts.wait(ctx, acpWaitArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory}, fantasy.ToolCall{ID: "outer"})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, chunks, published)
	require.Empty(t, publishErrors)
	require.Contains(t, result.Content, strings.Repeat(text, chunks))
}

func TestACPWorkspaceSessionAccess(t *testing.T) {
	t.Parallel()
	toolset, _, conn, session := acpTestToolset(t)
	for range 2 {
		requestID := uuid.New()
		ctx := workspacesdk.WithToolCallID(context.Background(), requestID)
		conn.EXPECT().SendACPMessage(gomock.Any(), session.ID, workspacesdk.ACPMessageRequest{Text: "continue"}).Return(workspacesdk.ACPMessageResponse{}, nil)
		input, err := json.Marshal(acpMessageArgs{HarnessID: session.ID.HarnessSlug, SessionID: session.ID.SessionID, WorkingDirectory: session.ID.WorkingDirectory, Message: "continue"})
		require.NoError(t, err)
		response, err := newACPTools(toolset)[2].Run(ctx, fantasy.ToolCall{Input: string(input)})
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
