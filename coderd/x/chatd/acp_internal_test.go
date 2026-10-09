package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

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
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func acpTestOptions(t *testing.T) (acpToolOptions, *dbmock.MockStore, *agentconnmock.MockAgentConn, database.AgentsAcpSession) {
	t.Helper()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	session := database.AgentsAcpSession{ID: uuid.New(), ChatID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), HarnessSlug: "test", HarnessDisplayName: "Workspace Assistant", WorkingDirectory: "/work", SessionID: "native"}
	chat := database.Chat{ID: session.ChatID, OrganizationID: session.OrganizationID, WorkspaceID: uuid.NullUUID{UUID: session.WorkspaceID, Valid: true}, AgentID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	opts := acpToolOptions{db: db, chatID: chat.ID, clock: quartz.NewMock(t), logger: slog.Make(), harnesses: []workspacesdk.ACPHarness{{Slug: "test", DisplayName: session.HarnessDisplayName}}, resolve: func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
		return conn, chat, chat.AgentID.UUID, nil
	}}
	return opts, db, conn, session
}

func acpExpectSession(db *dbmock.MockStore, session database.AgentsAcpSession) {
	db.EXPECT().GetAgentsACPSessionByIDAndChatID(gomock.Any(), database.GetAgentsACPSessionByIDAndChatIDParams{ID: session.ID, ChatID: session.ChatID}).Return(session, nil).AnyTimes()
}

func acpEvent(epoch uuid.UUID, seq uint64, kind workspacesdk.ACPEventKind, text string) workspacesdk.ACPEvent {
	event := workspacesdk.ACPEvent{Cursor: workspacesdk.ACPCursor{Epoch: epoch, Seq: seq}, Kind: kind, Text: text}
	if kind == workspacesdk.ACPEventKindUpdate {
		event.Text = ""
		event.Update = json.RawMessage(text)
	}
	return event
}

func acpSnapshot(epoch uuid.UUID, seq uint64, status workspacesdk.ACPSessionStatus) workspacesdk.ACPEvent {
	event := acpEvent(epoch, seq, workspacesdk.ACPEventKindSnapshot, "")
	event.Session = &workspacesdk.ACPSession{Status: status, HistoryComplete: true, Cursor: event.Cursor}
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
		want        int
	}{
		{name: "Off"}, {name: "Plan", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, plan: true}, {name: "Child", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, child: true}, {name: "ExecuteRoot", experiments: codersdk.Experiments{codersdk.ExperimentChatACPSubagents}, want: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			chat := database.Chat{ID: uuid.New()}
			if tc.child {
				chat.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			}
			if tc.want > 0 {
				db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).Return(resources, nil)
			}
			server := &Server{db: db, experiments: tc.experiments}
			tools := server.acpTools(context.Background(), rootChatToolsOptions{chat: chat, isPlanModeTurn: tc.plan})
			require.Len(t, tools, tc.want)
		})
	}
	opts := acpToolOptions{harnesses: harnesses}
	require.Len(t, newACPTools(opts), 5)
	definitions := chatloop.BuildToolDefinitions(newACPTools(opts), nil, nil)
	spawn := definitions[0].(fantasy.FunctionTool)
	require.Equal(t, false, spawn.InputSchema["additionalProperties"])
	for _, input := range []string{`{"prompt":"task","working_directory":"/work","agent":{"Harness":"test"}}`, `{"prompt":"task","working_directory":"/work","agent":{"harness":"test","unknown":true}}`} {
		response, err := newACPTools(opts)[0].Run(context.Background(), fantasy.ToolCall{Input: input})
		require.NoError(t, err)
		require.True(t, response.IsError)
	}
	opts.harnesses = nil
	require.Len(t, newACPTools(opts), 4)
}

func TestACPValidationAndChatIsolation(t *testing.T) {
	t.Parallel()
	opts, db, _, session := acpTestOptions(t)
	tools := newACPTools(opts)
	for _, input := range []string{`{"limit":0}`, `{"limit":51}`, `{"offset":-1}`, `{"offset":2147483648}`, `{"Limit":10}`, `{"limit":1,"limit":2}`, `{"extra":true}`, `null`, `{} {}`} {
		response, err := tools[1].Run(context.Background(), fantasy.ToolCall{Input: input})
		require.NoError(t, err)
		require.True(t, response.IsError, input)
	}
	db.EXPECT().GetAgentsACPSessionByIDAndChatID(gomock.Any(), database.GetAgentsACPSessionByIDAndChatIDParams{ID: session.ID, ChatID: opts.chatID}).Return(database.AgentsAcpSession{}, sql.ErrNoRows)
	response, err := opts.message(context.Background(), acpMessageArgs{SessionID: session.ID.String(), Message: "steer"}, fantasy.ToolCall{})
	require.NoError(t, err)
	require.True(t, response.IsError)
}

func TestACPSpawnRetryAndInitialFailure(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	ctx := workspacesdk.WithToolCallID(context.Background(), session.ID)
	args := acpSpawnArgs{Prompt: "task", WorkingDirectory: session.WorkingDirectory}
	args.Agent.Harness = session.HarnessSlug
	gomock.InOrder(
		db.EXPECT().GetAgentsACPSessionByIDAndChatID(gomock.Any(), gomock.Any()).Return(database.AgentsAcpSession{}, sql.ErrNoRows),
		conn.EXPECT().CreateACPSession(gomock.Any(), workspacesdk.ACPCreateSessionRequest{RequestID: session.ID, HarnessSlug: session.HarnessSlug, WorkingDirectory: session.WorkingDirectory}).Return(workspacesdk.ACPSession{ID: acpNativeID(session), HarnessDisplayName: session.HarnessDisplayName}, nil),
		db.EXPECT().InsertAgentsACPSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params database.InsertAgentsACPSessionParams) (database.AgentsAcpSession, error) {
			require.Equal(t, session.HarnessDisplayName, params.HarnessDisplayName)
			return session, nil
		}),
	)
	db.EXPECT().GetAgentsACPSessionByIDAndChatID(gomock.Any(), gomock.Any()).Return(session, nil)
	db.EXPECT().UpdateAgentsACPSessionUpdatedAt(gomock.Any(), gomock.Any()).Return(session.ID, nil).Times(2)
	req := workspacesdk.ACPMessageRequest{ID: uuid.NewSHA1(session.ID, []byte("initial-prompt")), Text: "task"}
	gomock.InOrder(conn.EXPECT().SendACPMessage(gomock.Any(), acpNativeID(session), req).Return(workspacesdk.ACPMessageResponse{}, xerrors.New("submission failed")), conn.EXPECT().SendACPMessage(gomock.Any(), acpNativeID(session), req).Return(workspacesdk.ACPMessageResponse{}, nil))
	first, err := opts.spawn(ctx, args, fantasy.ToolCall{})
	require.NoError(t, err)
	require.True(t, first.IsError)
	require.Contains(t, first.Content, session.ID.String())
	require.Contains(t, first.Content, `"harness_display_name":"Workspace Assistant"`)
	retry, err := opts.spawn(ctx, args, fantasy.ToolCall{})
	require.NoError(t, err)
	require.False(t, retry.IsError)
	require.Contains(t, retry.Content, `"harness_display_name":"Workspace Assistant"`)
}

func TestACPMessageAndInterrupt(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	acpExpectSession(db, session)
	db.EXPECT().UpdateAgentsACPSessionUpdatedAt(gomock.Any(), gomock.Any()).Return(session.ID, nil).AnyTimes()
	id := uuid.New()
	ctx := workspacesdk.WithToolCallID(context.Background(), id)
	conn.EXPECT().SendACPMessage(gomock.Any(), acpNativeID(session), workspacesdk.ACPMessageRequest{ID: id, Text: "steer"}).Return(workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomeInjected}, nil)
	response, err := opts.message(ctx, acpMessageArgs{SessionID: session.ID.String(), Message: "steer"}, fantasy.ToolCall{})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.Contains(t, response.Content, `"harness_display_name":"Workspace Assistant"`)
	conn.EXPECT().InterruptACPSession(gomock.Any(), acpNativeID(session)).Return(workspacesdk.ACPSession{Status: workspacesdk.ACPSessionStatusRunning}, nil)
	response, err = opts.interrupt(ctx, acpSessionArgs{SessionID: session.ID.String()}, fantasy.ToolCall{})
	require.NoError(t, err)
	require.Contains(t, response.Content, `"interrupted":true`)
	require.Contains(t, response.Content, `"harness_display_name":"Workspace Assistant"`)
}

func TestACPWaitBaselineAndLive(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	acpExpectSession(db, session)
	epoch := uuid.New()
	events := make(chan workspacesdk.ACPEvent, 16)
	conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), acpNativeID(session), workspacesdk.ACPReadOptions{}).Return((<-chan workspacesdk.ACPEvent)(events), io.NopCloser(strings.NewReader("")), nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	published := make(chan codersdk.ChatMessagePart, 32)
	ctx = chatloop.WithMessagePartPublisher(ctx, func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) { published <- part })
	type outcome struct {
		response fantasy.ToolResponse
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		response, err := opts.wait(ctx, acpWaitArgs{SessionID: session.ID.String()}, fantasy.ToolCall{ID: "outer"})
		done <- outcome{response, err}
	}()
	events <- acpEvent(epoch, 1, workspacesdk.ACPEventKindUserMessage, "old")
	events <- acpEvent(epoch, 2, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"old answer"}}`)
	historical := acpSnapshot(epoch, 3, workspacesdk.ACPSessionStatusIdle)
	historical.Kind = workspacesdk.ACPEventKindStatus
	events <- historical
	events <- acpEvent(epoch, 4, workspacesdk.ACPEventKindUserMessage, "current")
	events <- acpEvent(epoch, 5, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"help","description":"List commands"}]}`)
	events <- acpSnapshot(epoch, 5, workspacesdk.ACPSessionStatusRunning)
	baseline := testutil.RequireReceive(ctx, t, published).ResultDelta
	var preview struct {
		Messages []codersdk.ChatACPTranscriptMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(baseline), &preview))
	require.Len(t, preview.Messages, 1)
	require.Equal(t, "current", preview.Messages[0].Content[0].Text)
	require.Contains(t, baseline, "current")
	require.NotContains(t, baseline, "availableCommands")
	events <- acpEvent(epoch, 6, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"private reasoning"}}`)
	reasoning := testutil.RequireReceive(ctx, t, published)
	require.True(t, reasoning.ResultReset)
	require.NoError(t, json.Unmarshal([]byte(reasoning.ResultDelta), &preview))
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, preview.Messages[1].Content[0].Type)
	require.Equal(t, "private reasoning", preview.Messages[1].Content[0].Text)
	events <- acpEvent(epoch, 7, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"usage_update","used":18722,"size":1000000}`)
	events <- acpEvent(epoch, 8, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer\n\"quoted\""}}`)
	require.Contains(t, testutil.RequireReceive(ctx, t, published).ResultDelta, `answer\n\"quoted\"`)
	events <- acpEvent(epoch, 9, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"tool_call","toolCallId":"read","title":"Read","status":"pending","rawInput":{"path":"/work/file"}}`)
	pending := testutil.RequireReceive(ctx, t, published)
	require.NoError(t, json.Unmarshal([]byte(pending.ResultDelta), &preview))
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, preview.Messages[1].Content[2].Type)
	require.Contains(t, string(preview.Messages[2].Content[0].Result), `"status":"pending"`)
	events <- acpEvent(epoch, 10, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"tool_call_update","toolCallId":"read","status":"completed","rawOutput":"contents"}`)
	completed := testutil.RequireReceive(ctx, t, published)
	require.NoError(t, json.Unmarshal([]byte(completed.ResultDelta), &preview))
	require.Len(t, preview.Messages, 3)
	require.Contains(t, string(preview.Messages[2].Content[0].Result), `"status":"completed"`)
	require.NotContains(t, completed.ResultDelta, `"status":"pending"`)
	terminal := acpSnapshot(epoch, 11, workspacesdk.ACPSessionStatusIdle)
	terminal.Kind = workspacesdk.ACPEventKindStatus
	events <- terminal
	result := testutil.RequireReceive(ctx, t, done)
	require.NoError(t, result.err)
	require.NotContains(t, result.response.Content, "private reasoning")
	require.NotContains(t, result.response.Content, "old answer")
	require.Contains(t, result.response.Content, "answer")
	require.NotContains(t, result.response.Content, "availableCommands")
	require.NotContains(t, result.response.Content, "usage_update")
	user := acpUserResult(t, result.response)
	require.Equal(t, session.HarnessDisplayName, user["harness_display_name"])
	encoded, _ := json.Marshal(user)
	require.Contains(t, string(encoded), "private reasoning")
	require.NotContains(t, string(encoded), "availableCommands")
	require.NotContains(t, string(encoded), "usage_update")
	require.Equal(t, false, user["timed_out"])
}

func TestACPWaitTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	for _, cancelRoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "Timeout", true: "Cancellation"}[cancelRoot], func(t *testing.T) {
			t.Parallel()
			opts, db, conn, session := acpTestOptions(t)
			acpExpectSession(db, session)
			clock := opts.clock.(*quartz.Mock)
			ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
			ctx = chatloop.WithMessagePartPublisher(ctx, func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
			defer cancel()
			ready := make(chan struct{})
			epoch := uuid.New()
			events := make(chan workspacesdk.ACPEvent, 2)
			events <- acpEvent(epoch, 1, workspacesdk.ACPEventKindUserMessage, "task")
			events <- acpSnapshot(epoch, 1, workspacesdk.ACPSessionStatusRunning)
			conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, slog.Logger, workspacesdk.ACPSessionID, workspacesdk.ACPReadOptions) (<-chan workspacesdk.ACPEvent, io.Closer, error) {
				close(ready)
				return events, io.NopCloser(strings.NewReader("")), nil
			})
			type outcome struct {
				response fantasy.ToolResponse
				err      error
			}
			done := make(chan outcome, 1)
			seconds := 1
			go func() {
				r, e := opts.wait(ctx, acpWaitArgs{SessionID: session.ID.String(), TimeoutSeconds: &seconds}, fantasy.ToolCall{ID: "outer"})
				done <- outcome{r, e}
			}()
			testutil.TryReceive(ctx, t, ready)
			if cancelRoot {
				cancel()
			} else {
				clock.Advance(time.Second).MustWait(ctx)
			}
			result := testutil.RequireReceive(testutil.Context(t, testutil.WaitLong), t, done)
			if cancelRoot {
				require.ErrorIs(t, result.err, context.Canceled)
			} else {
				require.NoError(t, result.err)
				require.Contains(t, result.response.Content, `"timed_out":true`)
			}
		})
	}
}

func TestACPWaitTerminalFailure(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	acpExpectSession(db, session)
	epoch := uuid.New()
	snapshot := acpSnapshot(epoch, 4, workspacesdk.ACPSessionStatusError)
	snapshot.Session.Error = "adapter died"
	snapshot.Session.HistoryComplete = false
	events := acpEventStream(acpEvent(epoch, 1, workspacesdk.ACPEventKindUserMessage, "task"), acpEvent(epoch, 2, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial"}}`), acpEvent(epoch, 3, workspacesdk.ACPEventKindError, "adapter died"), snapshot)
	conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(events, io.NopCloser(strings.NewReader("")), nil)
	ctx := chatloop.WithMessagePartPublisher(context.Background(), func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
	result, err := opts.wait(ctx, acpWaitArgs{SessionID: session.ID.String()}, fantasy.ToolCall{ID: "outer"})
	require.NoError(t, err)
	require.True(t, result.IsError)
	user := acpUserResult(t, result)
	require.Equal(t, false, user["history_complete"])
	require.Equal(t, "adapter died", user["error"])
	require.Contains(t, result.Content, "partial")
	require.Len(t, user["messages"], 2)
}

func TestACPWaitReconnect(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"Cursor", "EpochReset", "AgentReplacement", "AgentReplacementFailure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			opts, db, conn, session := acpTestOptions(t)
			acpExpectSession(db, session)
			clock := opts.clock.(*quartz.Mock)
			trap := clock.Trap().NewTimer("acp-reconnect")
			defer trap.Close()
			epoch := uuid.New()
			cursor := workspacesdk.ACPCursor{Epoch: epoch, Seq: 2}
			first := acpEventStream(acpEvent(epoch, 1, workspacesdk.ACPEventKindUserMessage, "first task"), acpEvent(epoch, 2, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"before"}}`), acpSnapshot(epoch, 2, workspacesdk.ACPSessionStatusRunning))
			conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), acpNativeID(session), workspacesdk.ACPReadOptions{}).Return(first, io.NopCloser(strings.NewReader("")), nil)
			nextConn := conn
			after := &cursor
			if strings.HasPrefix(scenario, "AgentReplacement") {
				nextConn = agentconnmock.NewMockAgentConn(gomock.NewController(t))
				agentID := uuid.New()
				originalAgentID := uuid.New()
				calls := 0
				opts.resolve = func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
					calls++
					chat := database.Chat{ID: session.ChatID, OrganizationID: session.OrganizationID, WorkspaceID: uuid.NullUUID{UUID: session.WorkspaceID, Valid: true}, AgentID: uuid.NullUUID{UUID: originalAgentID, Valid: true}}
					if calls == 1 {
						return conn, chat, originalAgentID, nil
					}
					chat.AgentID = uuid.NullUUID{UUID: agentID, Valid: true}
					return nextConn, chat, agentID, nil
				}
				after = nil
				epoch = uuid.New()
			}
			if scenario == "EpochReset" {
				conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), acpNativeID(session), workspacesdk.ACPReadOptions{After: after}).Return(nil, nil, codersdk.NewError(409, codersdk.Response{Message: "epoch changed"}))
				after = nil
				epoch = uuid.New()
			}
			var second <-chan workspacesdk.ACPEvent
			if after != nil {
				second = acpEventStream(acpEvent(epoch, 3, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"after"}}`), acpSnapshot(epoch, 3, workspacesdk.ACPSessionStatusIdle))
			} else {
				snapshot := acpSnapshot(epoch, 2, workspacesdk.ACPSessionStatusIdle)
				snapshot.Session.HistoryComplete = false
				second = acpEventStream(acpEvent(epoch, 1, workspacesdk.ACPEventKindUserMessage, "restored task"), acpEvent(epoch, 2, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"after"}}`), snapshot)
			}
			if scenario == "AgentReplacementFailure" {
				nextConn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), acpNativeID(session), workspacesdk.ACPReadOptions{After: after}).Return(nil, nil, codersdk.NewError(503, codersdk.Response{Message: "cannot restore native session"}))
			} else {
				nextConn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), acpNativeID(session), workspacesdk.ACPReadOptions{After: after}).Return(second, io.NopCloser(strings.NewReader("")), nil)
			}
			ctx := testutil.Context(t, testutil.WaitLong)
			var parts []codersdk.ChatMessagePart
			ctx = chatloop.WithMessagePartPublisher(ctx, func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
				parts = append(parts, part)
			})
			type outcome struct {
				response fantasy.ToolResponse
				err      error
			}
			done := make(chan outcome, 1)
			go func() {
				r, e := opts.wait(ctx, acpWaitArgs{SessionID: session.ID.String()}, fantasy.ToolCall{ID: "outer"})
				done <- outcome{r, e}
			}()
			trap.MustWait(ctx).MustRelease(ctx)
			clock.Advance(250 * time.Millisecond).MustWait(ctx)
			result := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, result.err)
			if scenario == "AgentReplacementFailure" {
				require.True(t, result.response.IsError)
				require.Contains(t, result.response.Content, "cannot restore native session")
				require.NotContains(t, result.response.Content, "before")
				require.Equal(t, false, acpUserResult(t, result.response)["history_complete"])
				return
			}
			require.False(t, result.response.IsError)
			if scenario == "Cursor" {
				require.Contains(t, result.response.Content, "beforeafter")
			} else {
				require.NotContains(t, result.response.Content, "before")
				require.Contains(t, result.response.Content, "after")
				require.Equal(t, false, acpUserResult(t, result.response)["history_complete"])
			}
			require.True(t, slices.ContainsFunc(parts, func(part codersdk.ChatMessagePart) bool { return part.ResultReset }))
			var preview struct {
				Messages []codersdk.ChatACPTranscriptMessage `json:"messages"`
			}
			require.NoError(t, json.Unmarshal([]byte(parts[len(parts)-1].ResultDelta), &preview))
			encoded, err := json.Marshal(preview.Messages)
			require.NoError(t, err)
			var messages any
			require.NoError(t, json.Unmarshal(encoded, &messages))
			require.Equal(t, acpUserResult(t, result.response)["messages"], messages)
		})
	}
}

func TestACPTranscriptReduction(t *testing.T) {
	t.Parallel()
	epoch := uuid.New()
	var transcript acpTranscript
	updates := []string{
		`{"sessionUpdate":"user_message_chunk","messageId":"u1","content":{"type":"text","text":"ta"}}`,
		`{"sessionUpdate":"user_message_chunk","messageId":"u1","content":{"type":"text","text":"sk"}}`,
		`{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"help","description":"List commands"}]}`,
		`{"sessionUpdate":"agent_thought_chunk","messageId":"a1","content":{"type":"text","text":"think"}}`,
		`{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Read","status":"pending","rawInput":{"path":"/work/file"},"_meta":{"private":"tool protocol metadata"}}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"tool","status":"completed","rawOutput":"contents","_meta":{"private":"updated tool protocol metadata"}}`,
		`{"sessionUpdate":"agent_message_chunk","messageId":"a2","content":{"type":"text","text":"an"}}`,
		`{"sessionUpdate":"usage_update","used":18722,"size":1000000}`,
		`{"sessionUpdate":"agent_message_chunk","messageId":"a2","content":{"type":"text","text":"swer"}}`,
		`{"sessionUpdate":"future_variant","content":{"type":"text","text":"future content"}}`,
		`{"sessionUpdate":"agent_message_chunk","content":{"type":"future_media","data":"binary"}}`,
	}
	var seq uint64
	for _, update := range updates {
		seq++
		_, _, err := transcript.consume(acpEvent(epoch, seq, workspacesdk.ACPEventKindUpdate, update))
		require.NoError(t, err)
	}
	require.Equal(t, "answer", transcript.response.String())
	require.Equal(t, "task", transcript.messages[0].Content[0].Text)
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
	_, reset, err := transcript.consume(acpEvent(epoch, 9, workspacesdk.ACPEventKindUserMessage, "steer"))
	require.NoError(t, err)
	require.True(t, reset)
	require.Empty(t, transcript.response.String())
	require.Len(t, transcript.messages, 1)
	_, _, err = transcript.consume(acpEvent(epoch, 10, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"tool_call_update","toolCallId":"new","status":"failed"}`))
	require.NoError(t, err)
	require.Len(t, transcript.tools, 1)
}

func TestACPListPagination(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	missing := session
	missing.ID, missing.SessionID = uuid.New(), "missing"
	otherWorkspace := session
	otherWorkspace.ID, otherWorkspace.WorkspaceID = uuid.New(), uuid.New()
	otherWorkspace.SessionID = "other-workspace"
	db.EXPECT().ListAgentsACPSessionsByChatID(gomock.Any(), database.ListAgentsACPSessionsByChatIDParams{ChatID: opts.chatID, LimitValue: 10, OffsetValue: 2}).Return([]database.AgentsAcpSession{session, missing, otherWorkspace}, nil)
	db.EXPECT().CountAgentsACPSessionsByChatID(gomock.Any(), opts.chatID).Return(int64(6), nil)
	resolve := opts.resolve
	resolutions := 0
	opts.resolve = func(ctx context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
		resolutions++
		return resolve(ctx)
	}
	conn.EXPECT().ListACPSessions(gomock.Any()).Return([]workspacesdk.ACPSession{
		{ID: acpNativeID(session), Status: workspacesdk.ACPSessionStatusRunning},
		{ID: acpNativeID(otherWorkspace), Status: workspacesdk.ACPSessionStatusIdle},
		{ID: workspacesdk.ACPSessionID{SessionID: "unrelated"}, Status: workspacesdk.ACPSessionStatusIdle},
	}, nil)
	result, err := opts.list(context.Background(), acpListArgs{Offset: 2}, fantasy.ToolCall{})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, 1, resolutions)
	var output struct {
		Agents []struct {
			SessionID          string `json:"session_id"`
			HarnessDisplayName string `json:"harness_display_name"`
			Status             string `json:"status"`
			Error              string `json:"error"`
		} `json:"agents"`
		Total    int  `json:"total"`
		Returned int  `json:"returned"`
		Offset   int  `json:"offset"`
		HasMore  bool `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Content), &output))
	require.Equal(t, 6, output.Total)
	require.Equal(t, 3, output.Returned)
	require.Equal(t, 2, output.Offset)
	require.True(t, output.HasMore)
	require.Len(t, output.Agents, 3)
	require.Equal(t, session.ID.String(), output.Agents[0].SessionID)
	require.Equal(t, session.HarnessDisplayName, output.Agents[0].HarnessDisplayName)
	require.Equal(t, "running", output.Agents[0].Status)
	require.Equal(t, missing.ID.String(), output.Agents[1].SessionID)
	require.Equal(t, "unknown", output.Agents[1].Status)
	require.Equal(t, "error", output.Agents[2].Status)
	require.Equal(t, errACPWorkspaceChanged.Error(), output.Agents[2].Error)
}

func TestACPWaitLargeTranscript(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	acpExpectSession(db, session)
	epoch := uuid.New()
	reasoning := strings.Repeat("r", (1<<20)+1)
	chunk, err := json.Marshal(map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]string{"type": "text", "text": reasoning}})
	require.NoError(t, err)
	events := acpEventStream(acpEvent(epoch, 1, workspacesdk.ACPEventKindUserMessage, "task"), acpEvent(epoch, 2, workspacesdk.ACPEventKindUpdate, string(chunk)), acpEvent(epoch, 3, workspacesdk.ACPEventKindUpdate, `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer"}}`), acpSnapshot(epoch, 3, workspacesdk.ACPSessionStatusIdle))
	conn.EXPECT().WatchACPSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(events, io.NopCloser(strings.NewReader("")), nil)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := messagepartbuffer.Key{ChatID: opts.chatID}
	require.NoError(t, buffer.CreateEpisode(key))
	ctx := chatloop.WithMessagePartPublisher(context.Background(), func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		_ = buffer.AddPart(key, role, part)
	})
	result, err := opts.wait(ctx, acpWaitArgs{SessionID: session.ID.String()}, fantasy.ToolCall{ID: "outer"})
	require.NoError(t, err)
	require.False(t, result.IsError)
	user := acpUserResult(t, result)
	encoded, err := json.Marshal(user)
	require.NoError(t, err)
	require.Contains(t, string(encoded), reasoning)
	require.NotContains(t, result.Content, reasoning)
	require.Contains(t, result.Content, "answer")
}

func TestACPListPreservesAdapterFailure(t *testing.T) {
	t.Parallel()
	opts, db, conn, session := acpTestOptions(t)
	db.EXPECT().ListAgentsACPSessionsByChatID(gomock.Any(), gomock.Any()).Return([]database.AgentsAcpSession{session}, nil)
	db.EXPECT().CountAgentsACPSessionsByChatID(gomock.Any(), opts.chatID).Return(int64(1), nil)
	conn.EXPECT().ListACPSessions(gomock.Any()).Return([]workspacesdk.ACPSession{{ID: acpNativeID(session), Status: workspacesdk.ACPSessionStatusError, Error: "adapter failed"}}, nil)
	response, err := opts.list(context.Background(), acpListArgs{}, fantasy.ToolCall{})
	require.NoError(t, err)
	require.Contains(t, response.Content, "adapter failed")
	require.Contains(t, response.Content, `"status":"error"`)
}
