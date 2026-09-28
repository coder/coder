package chattool_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fileTool is edit_files or write_file with what a test needs to run it
// against a mock agent connection.
type fileTool struct {
	name   string
	change string
	input  string
	// build returns the tool.
	build func(getConn func(context.Context) (workspacesdk.AgentConn, error), clock quartz.Clock, isPlanTurn bool, resolvePlanPath func(context.Context) (string, string, error)) fantasy.AgentTool
	// expect expects one request and returns the answer from answer.
	// The request's context is passed to answer.
	expect func(t *testing.T, conn *agentconnmock.MockAgentConn, answer func(ctx context.Context) error) *gomock.Call
}

func fileTools() []fileTool {
	return []fileTool{
		{
			name:   chattool.EditFilesToolName,
			change: "edit",
			input:  `{"files":[{"path":"/a.txt","edits":[{"old_text":"old","new_text":"new"}]}]}`,
			build: func(getConn func(context.Context) (workspacesdk.AgentConn, error), clock quartz.Clock, isPlanTurn bool, resolvePlanPath func(context.Context) (string, string, error)) fantasy.AgentTool {
				return chattool.EditFiles(chattool.EditFilesOptions{GetWorkspaceConn: getConn, Clock: clock, IsPlanTurn: isPlanTurn, ResolvePlanPath: resolvePlanPath})
			},
			expect: func(t *testing.T, conn *agentconnmock.MockAgentConn, answer func(ctx context.Context) error) *gomock.Call {
				return conn.EXPECT().EditFiles(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req workspacesdk.FileEditRequest) (workspacesdk.FileEditResponse, error) {
						assert.Equal(t, workspacesdk.FileEditRequest{
							Files:       []workspacesdk.FileEdits{{Path: "/a.txt", Edits: []workspacesdk.FileEdit{{OldText: "old", NewText: "new"}}}},
							IncludeDiff: true,
						}, req)
						return workspacesdk.FileEditResponse{}, answer(ctx)
					})
			},
		},
		{
			name:   chattool.WriteFileToolName,
			change: "write",
			input:  `{"path":"/a.txt","content":"content"}`,
			build: func(getConn func(context.Context) (workspacesdk.AgentConn, error), clock quartz.Clock, isPlanTurn bool, resolvePlanPath func(context.Context) (string, string, error)) fantasy.AgentTool {
				return chattool.WriteFile(chattool.WriteFileOptions{GetWorkspaceConn: getConn, Clock: clock, IsPlanTurn: isPlanTurn, ResolvePlanPath: resolvePlanPath})
			},
			expect: func(t *testing.T, conn *agentconnmock.MockAgentConn, answer func(ctx context.Context) error) *gomock.Call {
				return conn.EXPECT().WriteFile(gomock.Any(), "/a.txt", gomock.Any()).
					DoAndReturn(func(ctx context.Context, _ string, body io.Reader) error {
						data, err := io.ReadAll(body)
						assert.NoError(t, err)
						assert.Equal(t, "content", string(data), "every send streams the whole content")
						return answer(ctx)
					})
			},
		},
	}
}

func newFileToolCallIdentity(toolName string) chattool.ToolCallIdentity {
	return chattool.ToolCallIdentity{
		ChatID:     uuid.New(),
		MessageID:  42,
		ToolCallID: "call-1",
		ToolName:   toolName,
	}
}

// runFileTool runs tool with its input. A zero id runs it without a tool
// call identity.
func runFileTool(ctx context.Context, t *testing.T, tool fantasy.AgentTool, ft fileTool, id chattool.ToolCallIdentity) fantasy.ToolResponse {
	t.Helper()
	if id.ChatID != uuid.Nil {
		ctx = chattool.WithToolCallIdentity(ctx, id)
	}
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: ft.name, Input: ft.input})
	require.NoError(t, err)
	return resp
}

// TestFileToolCall covers edit_files and write_file when the dispatch
// identifies the tool call: the request carries the tool call headers and
// every agent answer maps to one result.
func TestFileToolCall(t *testing.T) {
	t.Parallel()

	recordedErr := codersdk.NewTestError(http.StatusNotFound, http.MethodPost, "http://agent/api/v0/edit-files")
	recordedErr.Message = "open /a.txt: file does not exist"
	toolCallError := func(code workspacesdk.ToolCallErrorCode) error {
		return &workspacesdk.ToolCallError{Response: codersdk.Response{Message: "refused"}, Code: code}
	}

	for _, ft := range fileTools() {
		t.Run(ft.name, func(t *testing.T) {
			t.Parallel()

			answers := []struct {
				name        string
				noIdentity  bool
				err         error
				wantIsError bool
				// wantContent lists substrings of the result, wantAbsent
				// substrings it must not contain.
				wantContent []string
				wantAbsent  []string
			}{
				{name: "NoIdentity", noIdentity: true, wantContent: []string{`"ok":true`}},
				{name: "Applied", wantContent: []string{`"ok":true`}},
				{
					name:        "RecordedError",
					err:         xerrors.Errorf("do request: %w", recordedErr),
					wantIsError: true,
					wantContent: []string{"file does not exist"},
					wantAbsent:  []string{"outcome unknown"},
				},
				{
					name:        "Canceled",
					err:         toolCallError(workspacesdk.ToolCallErrorCanceled),
					wantIsError: true,
					wantContent: []string{ft.change + " not applied: the tool call was canceled before the workspace agent received it."},
				},
				{
					name:        "Unknown",
					err:         toolCallError(workspacesdk.ToolCallErrorUnknown),
					wantIsError: true,
					wantContent: []string{"outcome unknown: the workspace agent cannot tell whether this tool call ran", "the " + ft.change + " may have been applied. Check the file before changing it again."},
				},
			}
			for _, tt := range answers {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					ctx := testutil.Context(t, testutil.WaitShort)
					id := newFileToolCallIdentity(ft.name)
					conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
					ft.expect(t, conn, func(ctx context.Context) error {
						tc, ok := workspacesdk.ToolCallFromContext(ctx)
						if tt.noIdentity {
							assert.False(t, ok, "request must not carry a tool call")
						} else {
							assert.True(t, ok, "request must carry the tool call")
							assert.Equal(t, id.AgentToolCall(), tc)
						}
						return tt.err
					})
					if tt.noIdentity {
						id = chattool.ToolCallIdentity{}
					}
					tool := ft.build(func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil }, quartz.NewMock(t), false, nil)

					resp := runFileTool(ctx, t, tool, ft, id)
					assert.Equal(t, tt.wantIsError, resp.IsError, resp.Content)
					for _, want := range tt.wantContent {
						assert.Contains(t, resp.Content, want)
					}
					for _, absent := range tt.wantAbsent {
						assert.NotContains(t, resp.Content, absent)
					}
				})
			}

			t.Run("RetriedUntilAnswered", func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				id := newFileToolCallIdentity(ft.name)
				clock := quartz.NewMock(t)
				retryTrap := clock.Trap().NewTimer("chattool", "agent-retry")
				defer retryTrap.Close()
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				gomock.InOrder(
					ft.expect(t, conn, func(context.Context) error {
						return xerrors.Errorf("do request: %w", &url.Error{Op: "Post", URL: "http://agent", Err: xerrors.New("connection reset by peer")})
					}),
					ft.expect(t, conn, func(context.Context) error { return nil }),
				)
				tool := ft.build(func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil }, clock, false, nil)

				done := make(chan fantasy.ToolResponse, 1)
				go func() { done <- runFileTool(ctx, t, tool, ft, id) }()
				retryTrap.MustWait(ctx).MustRelease(ctx)
				clock.Advance(time.Second).MustWait(ctx)

				resp := testutil.RequireReceive(ctx, t, done)
				assert.False(t, resp.IsError, resp.Content)
				assert.Contains(t, resp.Content, `"ok":true`)
			})

			t.Run("NoAnswerIsUnknown", func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				id := newFileToolCallIdentity(ft.name)
				clock := quartz.NewMock(t)
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				sent := make(chan struct{}, 1)
				ft.expect(t, conn, func(ctx context.Context) error {
					sent <- struct{}{}
					<-ctx.Done()
					return &url.Error{Op: "Post", URL: "http://agent", Err: ctx.Err()}
				})
				tool := ft.build(func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil }, clock, false, nil)

				done := make(chan fantasy.ToolResponse, 1)
				go func() { done <- runFileTool(ctx, t, tool, ft, id) }()
				testutil.RequireReceive(ctx, t, sent)
				clock.Advance(chattool.AgentAnswerTimeout).MustWait(ctx)

				resp := testutil.RequireReceive(ctx, t, done)
				assert.True(t, resp.IsError)
				assert.Contains(t, resp.Content, "outcome unknown: the workspace agent did not answer within 1m0s")
				assert.Contains(t, resp.Content, "the "+ft.change+" may have been applied")
				assert.Contains(t, resp.Content, "tool call "+id.UUID())
			})

			t.Run("ConnErrors", func(t *testing.T) {
				t.Parallel()

				tests := []struct {
					name        string
					noIdentity  bool
					err         error
					wantUnknown bool
				}{
					{name: "NoWorkspace", err: chattool.ErrChatHasNoWorkspace},
					{name: "WorkspaceDeleted", err: chattool.ErrWorkspaceDeleted},
					// A stopped workspace keeps its disk, so an earlier
					// attempt's change may be there.
					{name: "NoRunningAgent", err: xerrors.Errorf("resolve: %w", chattool.ErrWorkspaceHasNoAgent), wantUnknown: true},
					{name: "DialFailed", err: xerrors.New("connection to the workspace agent timed out"), wantUnknown: true},
					{name: "DialFailedNoIdentity", noIdentity: true, err: xerrors.New("connection to the workspace agent timed out")},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						t.Parallel()
						id := newFileToolCallIdentity(ft.name)
						if tt.noIdentity {
							id = chattool.ToolCallIdentity{}
						}
						tool := ft.build(func(context.Context) (workspacesdk.AgentConn, error) { return nil, tt.err }, quartz.NewMock(t), false, nil)

						resp := runFileTool(testutil.Context(t, testutil.WaitShort), t, tool, ft, id)
						assert.True(t, resp.IsError)
						if !tt.wantUnknown {
							assert.Equal(t, tt.err.Error(), resp.Content)
							return
						}
						assert.Contains(t, resp.Content, "outcome unknown: the workspace agent could not be reached ("+tt.err.Error()+")")
						assert.Contains(t, resp.Content, "the "+ft.change+" may have been applied")
						assert.Contains(t, resp.Content, "tool call "+id.UUID())
					})
				}
			})
		})
	}
}
