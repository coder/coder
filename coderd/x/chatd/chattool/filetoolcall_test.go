package chattool_test

import (
	"context"
	"encoding/json"
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

// fileToolCallCase is one agent answer to an edit_files or write_file
// request and the tool result it must produce.
type fileToolCallCase struct {
	name string
	// noIdentity runs the tool without a tool call identity.
	noIdentity bool
	// canceled runs the tool with a canceled context.
	canceled bool
	// err is the error the agent request returns.
	err error
	// connErr is the error obtaining the workspace connection returns;
	// no agent request is made.
	connErr     error
	wantIsError bool
	// wantContent lists substrings of the tool result, and wantAbsent
	// substrings it must not contain.
	wantContent []string
	wantAbsent  []string
}

func fileToolCallCases() []fileToolCallCase {
	toolCallError := func(code workspacesdk.ToolCallErrorCode) error {
		return &workspacesdk.ToolCallError{
			Response: codersdk.Response{Message: "refused"},
			Code:     code,
		}
	}
	sdkErr := codersdk.NewTestError(http.StatusNotFound, http.MethodPost, "http://agent/api/v0/files")
	sdkErr.Message = "open /a.txt: file does not exist"
	transportErr := xerrors.Errorf("do request: %w", &url.Error{Op: http.MethodPost, URL: "http://agent/api/v0/edit-files", Err: xerrors.New("connection reset by peer")})
	return []fileToolCallCase{
		{name: "NoIdentity", noIdentity: true, wantContent: []string{`"ok":true`}},
		{name: "Applied", wantContent: []string{`"ok":true`}},
		{name: "RecordedError", err: xerrors.Errorf("do request: %w", sdkErr), wantIsError: true, wantContent: []string{"open /a.txt: file does not exist"}, wantAbsent: []string{"outcome unknown"}},
		{
			// The request may have reached the agent, so the edit may have
			// been applied.
			name:        "TransportError",
			err:         transportErr,
			wantIsError: true,
			wantContent: []string{"outcome unknown", "could not be reached", "may have been applied", "connection reset by peer", "Check the file"},
		},
		{
			// The agent answered, but its answer could not be read.
			name:        "UnreadableAnswer",
			err:         xerrors.Errorf("decode response body: %w", xerrors.New("unexpected EOF")),
			wantIsError: true,
			wantContent: []string{"outcome unknown", "answer could not be read", "may have been applied", "unexpected EOF", "Check the file"},
		},
		{
			// An earlier attempt of the tool call may have applied the
			// change.
			name:        "ConnError",
			connErr:     xerrors.New("dial failed"),
			wantIsError: true,
			wantContent: []string{"outcome unknown", "could not be reached", "dial failed", "an earlier attempt may have applied", "Check the file"},
		},
		{name: "ConnErrorNoIdentity", noIdentity: true, connErr: xerrors.New("dial failed"), wantIsError: true, wantContent: []string{"dial failed"}, wantAbsent: []string{"outcome unknown"}},
		{name: "ConnErrorNoAgent", connErr: chattool.ErrWorkspaceHasNoAgent, wantIsError: true, wantContent: []string{"workspace has no running agent"}, wantAbsent: []string{"outcome unknown"}},
		{name: "ConnErrorWorkspaceDeleted", connErr: chattool.ErrWorkspaceDeleted, wantIsError: true, wantContent: []string{"workspace was deleted"}, wantAbsent: []string{"outcome unknown"}},
		{name: "TransportErrorNoIdentity", noIdentity: true, err: transportErr, wantIsError: true, wantContent: []string{"connection reset by peer"}, wantAbsent: []string{"outcome unknown"}},
		{
			// A canceled context means the result will not be committed.
			name:        "TransportErrorContextDone",
			canceled:    true,
			err:         transportErr,
			wantIsError: true,
			wantContent: []string{"connection reset by peer"},
			wantAbsent:  []string{"outcome unknown"},
		},
		{
			name:        "AgentStartedAfterToolCall",
			err:         toolCallError(workspacesdk.ToolCallErrorAgentStartedAfterToolCall),
			wantIsError: true,
			wantContent: []string{"outcome unknown", "workspace agent restarted after this tool call", "may have been applied", "Check the file"},
		},
		{
			name:        "InputMismatch",
			err:         toolCallError(workspacesdk.ToolCallErrorInputMismatch),
			wantIsError: true,
			wantContent: []string{"this request changed nothing", "already exists with a different input"},
		},
		{name: "StaleToolCall", err: toolCallError(workspacesdk.ToolCallErrorStale), wantIsError: true, wantContent: []string{string(workspacesdk.ToolCallErrorStale)}},
		{name: "ToolCallCanceled", err: toolCallError(workspacesdk.ToolCallErrorCanceled), wantIsError: true, wantContent: []string{string(workspacesdk.ToolCallErrorCanceled)}},
	}
}

// workspaceConnDelay is how long obtaining the workspace connection takes
// on the tool call age clock in fileToolCallContext.
const workspaceConnDelay = 5 * time.Second

// fileToolCallContext returns a context carrying a tool call identity
// unless tc.noIdentity, the tool call the agent request must carry, and
// a GetWorkspaceConn that returns conn after workspaceConnDelay. The
// expected age includes the delay, which proves the age is measured just
// before the request.
func fileToolCallContext(t *testing.T, tc fileToolCallCase, conn workspacesdk.AgentConn) (context.Context, workspacesdk.ToolCall, func(context.Context) (workspacesdk.AgentConn, error)) {
	t.Helper()

	const toolCallAge = 30 * time.Second
	clock := quartz.NewMock(t)
	getConn := func(context.Context) (workspacesdk.AgentConn, error) {
		clock.Advance(workspaceConnDelay).MustWait(testutil.Context(t, testutil.WaitShort))
		if tc.connErr != nil {
			return nil, tc.connErr
		}
		return conn, nil
	}
	ctx := context.Background()
	if tc.canceled {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		ctx = canceledCtx
	}
	dbNow := time.Now()
	identity := chattool.ToolCallIdentity{
		ChatID:     uuid.New(),
		MessageID:  42,
		ToolCallID: "call_" + uuid.NewString(),
		Age:        chattool.NewToolCallAge(clock, dbNow, dbNow.Add(-toolCallAge)),
	}
	if tc.noIdentity {
		return ctx, workspacesdk.ToolCall{}, getConn
	}
	want := workspacesdk.ToolCall{MessageID: 42, ID: identity.ToolCallID, Age: toolCallAge + workspaceConnDelay}
	return chattool.WithToolCallIdentity(ctx, identity), want, getConn
}

func assertToolCallInContext(ctx context.Context, t *testing.T, tc fileToolCallCase, want workspacesdk.ToolCall) {
	t.Helper()

	got, ok := workspacesdk.ToolCallFromContext(ctx)
	if tc.noIdentity {
		assert.False(t, ok, "request must not carry a tool call")
		return
	}
	assert.True(t, ok, "request must carry the tool call")
	assert.Equal(t, want, got)
}

func assertFileToolCallResult(t *testing.T, tc fileToolCallCase, resp fantasy.ToolResponse) {
	t.Helper()

	assert.Equal(t, tc.wantIsError, resp.IsError, resp.Content)
	for _, want := range tc.wantContent {
		assert.Contains(t, resp.Content, want)
	}
	for _, absent := range tc.wantAbsent {
		assert.NotContains(t, resp.Content, absent)
	}
}

// TestEditFilesToolCall covers edit_files when the dispatch identifies
// the tool call: the request carries the tool call and agent refusals
// map to results.
func TestEditFilesToolCall(t *testing.T) {
	t.Parallel()

	for _, tc := range fileToolCallCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			ctx, wantToolCall, getConn := fileToolCallContext(t, tc, mockConn)
			if tc.connErr == nil {
				mockConn.EXPECT().
					EditFiles(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, _ workspacesdk.FileEditRequest) (workspacesdk.FileEditResponse, error) {
						assertToolCallInContext(ctx, t, tc, wantToolCall)
						return workspacesdk.FileEditResponse{}, tc.err
					})
			}
			tool := chattool.EditFiles(chattool.EditFilesOptions{
				GetWorkspaceConn: getConn,
			})

			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "edit_files",
				Input: `{"files":[{"path":"/a.txt","edits":[{"old_text":"old","new_text":"new"}]}]}`,
			})
			require.NoError(t, err)
			assertFileToolCallResult(t, tc, resp)
		})
	}
}

// TestWriteFileToolCall covers write_file when the dispatch identifies
// the tool call: the request carries the tool call and agent refusals
// map to results.
func TestWriteFileToolCall(t *testing.T) {
	t.Parallel()

	for _, tc := range fileToolCallCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			ctx, wantToolCall, getConn := fileToolCallContext(t, tc, mockConn)
			if tc.connErr == nil {
				mockConn.EXPECT().
					WriteFile(gomock.Any(), "/a.txt", gomock.Any()).
					DoAndReturn(func(ctx context.Context, _ string, _ io.Reader) error {
						assertToolCallInContext(ctx, t, tc, wantToolCall)
						return tc.err
					})
			}
			tool := chattool.WriteFile(chattool.WriteFileOptions{
				GetWorkspaceConn: getConn,
			})

			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "write_file",
				Input: `{"path":"/a.txt","content":"content"}`,
			})
			require.NoError(t, err)
			assertFileToolCallResult(t, tc, resp)
		})
	}
}

// TestInterruptFileToolCall covers the result an interrupted edit_files
// or write_file call gets from the workspace agent's answer to cancel.
func TestInterruptFileToolCall(t *testing.T) {
	t.Parallel()

	transportErr := &url.Error{Op: http.MethodPost, URL: "http://agent/api/v0/edit-files/x/cancel", Err: xerrors.New("connection reset by peer")}
	tests := []struct {
		name string
		resp workspacesdk.CancelFileToolCallResponse
		err  error
		// generic is set when the call keeps today's interrupted result.
		generic     bool
		wantIsError bool
		// wantContent is the whole result per tool, and wantContains lists
		// substrings of it.
		wantContent  map[string]string
		wantContains []string
	}{
		{
			name: "Applied",
			resp: workspacesdk.CancelFileToolCallResponse{
				Started:    true,
				StatusCode: http.StatusOK,
				Body:       json.RawMessage(`{"files":[{"path":"/a.txt","diff":"@@ -1 +1 @@"}]}`),
			},
			wantContent: map[string]string{
				chattool.EditFilesToolName: `{"files":[{"path":"/a.txt","diff":"@@ -1 +1 @@"}],"ok":true}`,
				chattool.WriteFileToolName: `{"ok":true}`,
			},
		},
		{
			name: "RecordedError",
			resp: workspacesdk.CancelFileToolCallResponse{
				Started:    true,
				StatusCode: http.StatusNotFound,
				Body:       json.RawMessage(`{"message":"open /a.txt: file does not exist"}`),
			},
			wantIsError: true,
			wantContent: map[string]string{
				chattool.EditFilesToolName: "open /a.txt: file does not exist",
				chattool.WriteFileToolName: "unexpected status code 404: open /a.txt: file does not exist",
			},
		},
		{
			name:         "NotStarted",
			wantIsError:  true,
			wantContains: []string{"not applied", "canceled before the workspace agent received it"},
		},
		{
			name:         "AgentStartedAfterToolCall",
			err:          &workspacesdk.ToolCallError{Response: codersdk.Response{Message: "refused"}, Code: workspacesdk.ToolCallErrorAgentStartedAfterToolCall},
			wantIsError:  true,
			wantContains: []string{"outcome unknown", "workspace agent restarted after this tool call", "may have been applied", "Check the file"},
		},
		{
			name:    "StaleToolCall",
			err:     &workspacesdk.ToolCallError{Response: codersdk.Response{Message: "stale"}, Code: workspacesdk.ToolCallErrorStale},
			generic: true,
		},
		{
			name:    "OldAgentWithoutCancelRoute",
			err:     codersdk.NewTestError(http.StatusNotFound, http.MethodPost, "/api/v0/edit-files/x/cancel"),
			generic: true,
		},
		{
			name:         "TransportError",
			err:          transportErr,
			wantIsError:  true,
			wantContains: []string{"outcome unknown", "could not be reached", "may have been applied", "connection reset by peer", "Check the file"},
		},
		{
			name:         "UnreadableResponse",
			err:          xerrors.New("unexpected EOF"),
			wantIsError:  true,
			wantContains: []string{"outcome unknown", "answer could not be read", "may have been applied", "unexpected EOF"},
		},
	}
	for _, toolName := range []string{chattool.EditFilesToolName, chattool.WriteFileToolName} {
		t.Run(toolName, func(t *testing.T) {
			t.Parallel()

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					dbNow := time.Now()
					id := chattool.ToolCallIdentity{
						ChatID:     uuid.New(),
						MessageID:  42,
						ToolCallID: "call_" + uuid.NewString(),
						Age:        chattool.NewToolCallAge(quartz.NewMock(t), dbNow, dbNow.Add(-time.Minute)),
					}
					cancel := func(ctx context.Context, gotID string) (workspacesdk.CancelFileToolCallResponse, error) {
						assert.Equal(t, id.UUID(), gotID)
						got, ok := workspacesdk.ToolCallFromContext(ctx)
						assert.True(t, ok, "cancel request must carry the tool call")
						assert.Equal(t, workspacesdk.ToolCall{MessageID: 42, ID: id.ToolCallID, Age: time.Minute}, got)
						return tc.resp, tc.err
					}
					mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
					if toolName == chattool.EditFilesToolName {
						mockConn.EXPECT().CancelEditFiles(gomock.Any(), gomock.Any()).DoAndReturn(cancel)
					} else {
						mockConn.EXPECT().CancelWriteFile(gomock.Any(), gomock.Any()).DoAndReturn(cancel)
					}

					resp, ok := chattool.InterruptFileToolCall(context.Background(), mockConn, toolName, id)
					if tc.generic {
						assert.False(t, ok)
						return
					}
					require.True(t, ok)
					assert.Equal(t, tc.wantIsError, resp.IsError, resp.Content)
					if want, ok := tc.wantContent[toolName]; ok {
						if tc.wantIsError {
							assert.Equal(t, want, resp.Content)
						} else {
							assert.JSONEq(t, want, resp.Content)
						}
					}
					for _, want := range tc.wantContains {
						assert.Contains(t, resp.Content, want)
					}
				})
			}
		})
	}
}

func TestAgentUnreachableFileToolCallResult(t *testing.T) {
	t.Parallel()

	for _, toolName := range []string{chattool.EditFilesToolName, chattool.WriteFileToolName} {
		resp := chattool.AgentUnreachableFileToolCallResult(toolName, xerrors.New("dial failed"))
		assert.True(t, resp.IsError)
		for _, want := range []string{"outcome unknown", "could not be reached", "may have been applied", "dial failed", "Check the file"} {
			assert.Contains(t, resp.Content, want)
		}
	}
}
