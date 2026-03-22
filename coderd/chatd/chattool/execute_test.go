package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestExecuteTool(t *testing.T) {
	t.Parallel()

	t.Run("EmptyCommand", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		tool := newExecuteTool(t, mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":""}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "command is required")
	})

	t.Run("AmpersandDetection", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name               string
			command            string
			runInBackground    *bool
			wantCommand        string
			wantBackground     bool
			wantBackgroundResp bool // true if the response should contain a background_process_id
			comment            string
		}{
			{
				name:               "SimpleBackground",
				command:            "cmd &",
				wantCommand:        "cmd",
				wantBackground:     true,
				wantBackgroundResp: true,
				comment:            "Trailing & is correctly detected and stripped.",
			},
			{
				name:               "TrailingDoubleAmpersand",
				command:            "cmd &&",
				wantCommand:        "cmd &&",
				wantBackground:     false,
				wantBackgroundResp: false,
				comment:            "Ends with &&, excluded by the && suffix check.",
			},
			{
				name:               "NoAmpersand",
				command:            "cmd",
				wantCommand:        "cmd",
				wantBackground:     false,
				wantBackgroundResp: false,
			},
			{
				name:               "ChainThenBackground",
				command:            "cmd1 && cmd2 &",
				wantCommand:        "cmd1 && cmd2",
				wantBackground:     true,
				wantBackgroundResp: true,
				comment: "Ends with & but not &&, so it gets promoted " +
					"to background and the trailing & is stripped. " +
					"The remaining command runs in background mode.",
			},
			{
				// "|&" is bash's pipe-stderr operator, not
				// backgrounding. It must not be detected as a
				// trailing "&".
				name:               "BashPipeStderr",
				command:            "cmd |&",
				wantCommand:        "cmd |&",
				wantBackground:     false,
				wantBackgroundResp: false,
			},
			{
				name:               "AlreadyBackgroundWithTrailingAmpersand",
				command:            "cmd &",
				runInBackground:    ptr(true),
				wantCommand:        "cmd &",
				wantBackground:     true,
				wantBackgroundResp: true,
				comment: "When run_in_background is already true, " +
					"the stripping logic is skipped, preserving " +
					"the original command.",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)

				var capturedReq workspacesdk.StartProcessRequest
				mockConn.EXPECT().
					StartProcess(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
						capturedReq = req
						return workspacesdk.StartProcessResponse{ID: "proc-1"}, nil
					})

				// For foreground cases, ProcessOutput is polled.
				exitCode := 0
				mockConn.EXPECT().
					ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
					Return(workspacesdk.ProcessOutputResponse{
						Running:  false,
						ExitCode: &exitCode,
					}, nil).
					AnyTimes()

				tool := newExecuteTool(t, mockConn)

				input := map[string]any{"command": tc.command}
				if tc.runInBackground != nil {
					input["run_in_background"] = *tc.runInBackground
				}
				inputJSON, err := json.Marshal(input)
				require.NoError(t, err)

				ctx := testutil.Context(t, testutil.WaitMedium)
				resp, err := tool.Run(ctx, fantasy.ToolCall{
					ID:    "call-1",
					Name:  "execute",
					Input: string(inputJSON),
				})
				require.NoError(t, err)
				assert.False(t, resp.IsError, "response should not be an error")
				assert.Equal(t, tc.wantCommand, capturedReq.Command,
					"command passed to StartProcess")
				assert.Equal(t, tc.wantBackground, capturedReq.Background,
					"background flag passed to StartProcess")

				var result chattool.ExecuteResult
				require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
				if tc.wantBackgroundResp {
					assert.NotEmpty(t, result.BackgroundProcessID,
						"expected background_process_id in response")
				} else {
					assert.Empty(t, result.BackgroundProcessID,
						"expected no background_process_id")
				}
			})
		}
	})

	t.Run("ForegroundSuccess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		var capturedReq workspacesdk.StartProcessRequest
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				capturedReq = req
				return workspacesdk.StartProcessResponse{ID: "proc-1"}, nil
			})
		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "hello world",
			}, nil)

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hello"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.ExitCode)
		assert.Equal(t, "hello world", result.Output)
		assert.Empty(t, result.BackgroundProcessID)
		assert.Equal(t, "true", capturedReq.Env["CODER_CHAT_AGENT"])
	})

	t.Run("ForegroundNonZeroExit", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		exitCode := 42
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "something failed",
			}, nil)

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"exit 42"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Equal(t, 42, result.ExitCode)
		assert.Equal(t, "something failed", result.Output)
	})

	t.Run("BackgroundExecution", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.True(t, req.Background)
				return workspacesdk.StartProcessResponse{ID: "bg-42"}, nil
			})

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"sleep 999","run_in_background":true}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, "bg-42", result.BackgroundProcessID)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)

		// First call (blocking wait) returns context error
		// because the 50ms timeout expires.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				<-ctx.Done()
				return workspacesdk.ProcessOutputResponse{}, ctx.Err()
			})
		// Second call (snapshot fallback) returns partial output.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running: true,
				Output:  "partial output",
			}, nil)
		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:   "call-1",
			Name: "execute",
			// 50ms timeout expires during the blocking wait.
			Input: `{"command":"sleep 999","timeout":"50ms"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Equal(t, -1, result.ExitCode)
		assert.Contains(t, result.Error, "timed out")
		assert.Equal(t, "partial output", result.Output)
	})

	t.Run("StartProcessError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{}, xerrors.New("connection lost"))

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		// Errors from StartProcess are returned as a JSON body
		// with success=false, not as a ToolResponse error.
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "connection lost")
	})

	t.Run("ProcessOutputError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		// First call: blocking wait fails.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, xerrors.New("agent disconnected"))
		// Second call: snapshot fallback also fails.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, xerrors.New("agent disconnected"))

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "agent disconnected")
		// Snapshot fallback should provide the process ID
		// so the agent can retry manually.
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
	})

	t.Run("TransportErrorRecoveryProcessDone", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		exitCode := 0
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		// Blocking wait fails with transport error.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, xerrors.New("EOF"))
		// Snapshot fallback finds the process completed.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Output:   "hello\n",
				Running:  false,
				ExitCode: &exitCode,
			}, nil)

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hello"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		// Transparent recovery: success with real output.
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.ExitCode)
		assert.Equal(t, "hello\n", result.Output)
		assert.Empty(t, result.BackgroundProcessID)
	})

	t.Run("TransportErrorProcessStillRunning", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		// Blocking wait fails with transport error.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, xerrors.New("EOF"))
		// Snapshot fallback: process still running.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Output:  "partial output",
				Running: true,
			}, nil)

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"sleep 60"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "process still running")
		assert.Contains(t, result.Error, "process_output")
		assert.Equal(t, "partial output", result.Output)
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
	})

	t.Run("GetWorkspaceConnNil", func(t *testing.T) {
		t.Parallel()
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: nil,
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "not configured")
	})

	t.Run("GetWorkspaceConnError", func(t *testing.T) {
		t.Parallel()
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
				return nil, xerrors.New("workspace offline")
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "workspace offline")
	})
}

func TestDetectFileDump(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		wantHit bool
	}{
		{
			name:    "CatFile",
			command: "cat foo.txt",
			wantHit: true,
		},
		{
			name:    "NotCatPrefix",
			command: "concatenate foo",
			wantHit: false,
		},
		{
			name:    "GrepIncludeAll",
			command: "grep --include-all pattern",
			wantHit: true,
		},
		{
			name:    "RgListFiles",
			command: "rg -l pattern",
			wantHit: true,
		},
		{
			name:    "GrepRecursive",
			command: "grep -r pattern",
			wantHit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)

			mockConn.EXPECT().
				StartProcess(gomock.Any(), gomock.Any()).
				Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
			exitCode := 0
			mockConn.EXPECT().
				ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{
					Running:  false,
					ExitCode: &exitCode,
					Output:   "output",
				}, nil)

			tool := newExecuteTool(t, mockConn)
			ctx := testutil.Context(t, testutil.WaitMedium)
			input, err := json.Marshal(map[string]any{
				"command": tc.command,
			})
			require.NoError(t, err)

			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "execute",
				Input: string(input),
			})
			require.NoError(t, err)

			var result chattool.ExecuteResult
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			if tc.wantHit {
				assert.Contains(t, result.Note, "read_file",
					"expected advisory note for %q", tc.command)
			} else {
				assert.Empty(t, result.Note,
					"expected no note for %q", tc.command)
			}
		})
	}
}

// newExecuteTool creates an Execute tool wired to the given mock.
func newExecuteTool(t *testing.T, mockConn *agentconnmock.MockAgentConn) fantasy.AgentTool {
	t.Helper()
	return chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
}

func ptr[T any](v T) *T {
	return &v
}
