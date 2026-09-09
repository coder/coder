package chattool_test

import (
	"context"
	"encoding/json"
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
)

func TestExecuteTool(t *testing.T) {
	t.Parallel()

	t.Run("SchemaIncludesOptionalModelIntent", func(t *testing.T) {
		t.Parallel()

		tool := chattool.Execute(chattool.ExecuteOptions{})
		info := tool.Info()
		modelIntentParam, ok := info.Parameters["model_intent"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "string", modelIntentParam["type"])
		assert.Contains(t, modelIntentParam["description"], "alongside the command")
		assert.Contains(t, modelIntentParam["description"], "do not include the word")
		assert.Contains(t, info.Required, "command")
		assert.NotContains(t, info.Required, "model_intent")
	})

	t.Run("SchemaDisclosesShell", func(t *testing.T) {
		t.Parallel()

		tool := chattool.Execute(chattool.ExecuteOptions{})
		info := tool.Info()
		assert.Contains(t, info.Description, `Runs under "sh -c" (POSIX)`)

		commandParam, ok := info.Parameters["command"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "string", commandParam["type"])
		assert.Contains(t, commandParam["description"], `Runs under "sh -c" (POSIX)`)
	})

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
		assert.NotContains(t, capturedReq.Env, "AGENT_BROWSER_SESSION")
	})

	t.Run("AgentBrowserSessionEnv", func(t *testing.T) {
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
			}, nil)

		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			AgentBrowserSession: "chat-123",
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hello"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "chat-123", capturedReq.Env["AGENT_BROWSER_SESSION"])
	})

	t.Run("KeepaliveKickedOnAgentRoundTrips", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		exitCode := 0
		gomock.InOrder(
			// The server-side wait returns while the process is
			// still running, then a second poll sees it exit.
			mockConn.EXPECT().
				ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{Running: true}, nil),
			mockConn.EXPECT().
				ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{
					Running:  false,
					ExitCode: &exitCode,
					Output:   "done",
				}, nil),
		)

		kicks := 0
		ctx := chattool.WithAttemptKeepalive(
			testutil.Context(t, testutil.WaitMedium),
			func() { kicks++ },
		)
		tool := newExecuteTool(t, mockConn)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"sleep 1"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		// One kick for the successful start, one per successful
		// poll round, including the round where the process was
		// still running.
		assert.Equal(t, 3, kicks)
	})

	t.Run("ProcessOutputToolKicksKeepalive", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "done",
			}, nil)

		kicks := 0
		ctx := chattool.WithAttemptKeepalive(
			testutil.Context(t, testutil.WaitMedium),
			func() { kicks++ },
		)
		tool := chattool.ProcessOutput(chattool.ProcessToolOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "process_output",
			Input: `{"process_id":"proc-1"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, 1, kicks)
	})

	t.Run("ModelIntentIgnoredByExecution", func(t *testing.T) {
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
			Input: `{"command":"echo hello","model_intent":"Running a smoke test"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "echo hello", capturedReq.Command)
		assert.False(t, capturedReq.Background)

		var parsedArgs chattool.ExecuteArgs
		require.NoError(t, json.Unmarshal([]byte(`{"command":"echo hello","model_intent":"Running a smoke test"}`), &parsedArgs))
		require.NotNil(t, parsedArgs.ModelIntent)
		assert.Equal(t, "Running a smoke test", *parsedArgs.ModelIntent)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, "hello world", result.Output)

		var resultMap map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &resultMap))
		assert.NotContains(t, resultMap, "model_intent")
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
		// Unrelated errors must not trigger the missing-shell
		// guidance.
		assert.NotContains(t, result.Error, "Git Bash")
		assert.NotContains(t, result.Error, "coder.com/docs")
	})

	t.Run("MissingShellError", func(t *testing.T) {
		t.Parallel()

		// OS rendering differs (%PATH% vs $PATH); the fragment
		// omits the suffix to match both.
		tests := []struct {
			name       string
			input      string
			agentErr   string
			wantPrefix string
		}{
			{
				name:       "ForegroundWindows",
				input:      `{"command":"echo hi"}`,
				agentErr:   "unexpected status code 500: Failed to start process.\n\tError: start process: exec: \"sh\": executable file not found in %PATH%",
				wantPrefix: "start process:",
			},
			{
				name:       "BackgroundWindows",
				input:      `{"command":"echo hi","run_in_background":true}`,
				agentErr:   "unexpected status code 500: Failed to start process.\n\tError: start process: exec: \"sh\": executable file not found in %PATH%",
				wantPrefix: "start background process:",
			},
			{
				name:       "ForegroundPOSIX",
				input:      `{"command":"echo hi"}`,
				agentErr:   "unexpected status code 500: Failed to start process.\n\tError: start process: exec: \"sh\": executable file not found in $PATH",
				wantPrefix: "start process:",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)

				mockConn.EXPECT().
					StartProcess(gomock.Any(), gomock.Any()).
					Return(workspacesdk.StartProcessResponse{}, xerrors.New(tt.agentErr))

				tool := newExecuteTool(t, mockConn)
				ctx := testutil.Context(t, testutil.WaitMedium)
				resp, err := tool.Run(ctx, fantasy.ToolCall{
					ID:    "call-1",
					Name:  "execute",
					Input: tt.input,
				})
				require.NoError(t, err)
				assert.False(t, resp.IsError)

				var result chattool.ExecuteResult
				require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
				assert.False(t, result.Success)
				// The result keeps the original error for debugging.
				assert.Contains(t, result.Error, tt.wantPrefix)
				assert.Contains(t, result.Error, `exec: "sh": executable file not found`)
				assert.Contains(t, result.Error, "Git Bash")
				assert.Contains(t, result.Error, "https://coder.com/docs/ai-coder/agents/architecture#windows-workspace-shell-requirement")
			})
		}
	})

	t.Run("BackgroundedFlagOnlyOnIntentionalLaunch", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-bg"}, nil)

		tool := newExecuteTool(t, mockConn)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"npm start","run_in_background":true}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Backgrounded)
		assert.Equal(t, "proc-bg", result.BackgroundProcessID)
	})

	t.Run("ProcessOutputStillRunningSetsRunningFlag", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running: true,
				Output:  "starting...",
				Command: "npm start",
			}, nil)

		tool := chattool.ProcessOutput(chattool.ProcessToolOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "process_output",
			Input: `{"process_id":"proc-1","wait_timeout":"0s"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.True(t, result.Running)
		assert.Equal(t, "process is still running", result.Note)
		assert.Equal(t, "npm start", result.Command)
	})

	t.Run("ProcessOutputCommandPropagated", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		exitCode := 1
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "server exited: EADDRINUSE",
				Command:  "npm start",
			}, nil)

		tool := chattool.ProcessOutput(chattool.ProcessToolOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "process_output",
			Input: `{"process_id":"proc-1","wait_timeout":"0s"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Equal(t, 1, result.ExitCode)
		assert.Equal(t, "npm start", result.Command)
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

func TestExecuteToolIdempotencyKey(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	identityCtx := func(t *testing.T) context.Context {
		return chattool.WithDispatchIdentity(testutil.Context(t, testutil.WaitMedium), chatID, 42)
	}
	exitedOutput := func(code int, output string) workspacesdk.ProcessOutputResponse {
		return workspacesdk.ProcessOutputResponse{
			Running:  false,
			ExitCode: &code,
			Output:   output,
		}
	}
	runExecute := func(t *testing.T, ctx context.Context, tool fantasy.AgentTool, callID string, input string) chattool.ExecuteResult {
		t.Helper()
		resp, err := tool.Run(chattool.WithToolCallID(ctx, callID), fantasy.ToolCall{
			ID:    callID,
			Name:  "execute",
			Input: input,
		})
		require.NoError(t, err)
		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		return result
	}

	t.Run("KeyDerivedFromIdentity", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		var keys []string
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				keys = append(keys, req.IdempotencyKey)
				return workspacesdk.StartProcessResponse{ID: "proc-1", Started: true, IdempotencyKey: req.IdempotencyKey}, nil
			}).
			Times(3)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(exitedOutput(0, "ok"), nil).
			AnyTimes()

		tool := newExecuteTool(t, mockConn)
		runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi"}`)
		runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi"}`)
		runExecute(t, identityCtx(t), tool, "call-2", `{"command":"echo hi"}`)

		require.Len(t, keys, 3)
		assert.Equal(t, "42-call-1", keys[0])
		assert.Equal(t, keys[0], keys[1], "same identity and call ID must derive the same key")
		assert.Equal(t, "42-call-2", keys[2])
	})

	t.Run("NoIdentityMeansNoKey", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Empty(t, req.IdempotencyKey)
				return workspacesdk.StartProcessResponse{ID: "proc-1", Started: true}, nil
			})
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(exitedOutput(0, "ok"), nil).
			AnyTimes()

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, testutil.Context(t, testutil.WaitMedium), tool, "call-1", `{"command":"echo hi"}`)
		assert.True(t, result.Success)
	})

	t.Run("NoCallIDMeansNoKey", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Empty(t, req.IdempotencyKey)
				return workspacesdk.StartProcessResponse{ID: "proc-1", Started: true}, nil
			})
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(exitedOutput(0, "ok"), nil).
			AnyTimes()

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "", `{"command":"echo hi"}`)
		assert.True(t, result.Success)
	})

	t.Run("ConflictNeverStartsASecondProcess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{}, &workspacesdk.ProcessConflictError{
				Code: workspacesdk.ProcessConflictInputMismatch,
				Response: codersdk.Response{
					Message: "Client token was already used to start a process with different parameters.",
				},
			})

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi"}`)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "no new process was started")
	})

	t.Run("TokenWaitAbortedRetriesAndAttaches", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		startPending := &workspacesdk.ProcessConflictError{
			Code: workspacesdk.ProcessConflictStartPending,
			Response: codersdk.Response{
				Message: "Timed out waiting for the concurrent start that holds this idempotency key.",
			},
		}
		gomock.InOrder(
			mockConn.EXPECT().
				StartProcess(gomock.Any(), gomock.Any()).
				Return(workspacesdk.StartProcessResponse{}, startPending),
			mockConn.EXPECT().
				StartProcess(gomock.Any(), gomock.Any()).
				Return(workspacesdk.StartProcessResponse{
					ID:        "proc-1",
					Attached:  true,
					StartedAt: time.Now().Unix(),
				}, nil),
		)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(exitedOutput(0, "resolved by retry"), nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi","timeout":"30s"}`)
		assert.True(t, result.Success)
		assert.Equal(t, "resolved by retry", result.Output)
	})

	t.Run("TokenWaitAbortedBudgetExpiresUnresolved", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{}, &workspacesdk.ProcessConflictError{
				Code: workspacesdk.ProcessConflictStartPending,
				Response: codersdk.Response{
					Message: "Timed out waiting for the concurrent start that holds this idempotency key.",
					Detail:  "manager is closed",
				},
			})

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi","timeout":"100ms"}`)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "outcome is unresolved")
		assert.NotContains(t, result.Error, "no new process was started")
	})

	t.Run("AttachedWithFutureStartClampsWallDuration", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		// An agent clock ahead of coderd reports a start time in
		// the future; the wall duration must not go negative.
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{
				ID:        "proc-1",
				Attached:  true,
				StartedAt: time.Now().Add(time.Hour).Unix(),
			}, nil)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(exitedOutput(0, "ok"), nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi","timeout":"30s"}`)
		assert.True(t, result.Success)
		assert.GreaterOrEqual(t, result.WallDurationMs, int64(0))
	})

	t.Run("AttachedWithinBudgetWaitsForResult", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{
				ID:        "proc-1",
				Attached:  true,
				StartedAt: time.Now().Unix(),
			}, nil)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(exitedOutput(0, "replayed result"), nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi","timeout":"30s"}`)
		assert.True(t, result.Success)
		assert.Equal(t, "replayed result", result.Output)
		assert.Empty(t, result.BackgroundProcessID)
	})

	t.Run("AttachedWithAgentClockAheadBoundsWait", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		// The agent-reported start time is an hour ahead of the
		// local clock; the replay wait must still be bounded by
		// the requested timeout.
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{
				ID:        "proc-1",
				Attached:  true,
				StartedAt: time.Now().Add(time.Hour).Unix(),
			}, nil)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Not(gomock.Nil())).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				<-ctx.Done()
				return workspacesdk.ProcessOutputResponse{}, ctx.Err()
			})
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", nil).
			Return(workspacesdk.ProcessOutputResponse{Running: true, Output: "partial"}, nil)

		tool := newExecuteTool(t, mockConn)
		start := time.Now()
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi","timeout":"250ms"}`)
		require.Less(t, time.Since(start), testutil.WaitMedium)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "timed out")
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
	})

	t.Run("AttachedAfterSlowStartDoesNotGetFreshBudget", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		// The start request consumes the whole command budget
		// before attaching (slow token wait), and the agent clock
		// runs ahead so the reported remaining budget is large.
		// The attach wait must inherit the exhausted command
		// deadline instead of a fresh window.
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				<-ctx.Done()
				return workspacesdk.StartProcessResponse{
					ID:        "proc-1",
					Attached:  true,
					StartedAt: time.Now().Add(time.Hour).Unix(),
				}, nil
			})
		waitCtxErr := make(chan error, 1)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Not(gomock.Nil())).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				select {
				case waitCtxErr <- ctx.Err():
				default:
				}
				<-ctx.Done()
				return workspacesdk.ProcessOutputResponse{}, ctx.Err()
			})
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", nil).
			Return(workspacesdk.ProcessOutputResponse{Running: true, Output: "partial"}, nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi","timeout":"250ms"}`)
		assert.Error(t, <-waitCtxErr, "attach wait was granted a fresh budget after the command deadline expired")
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "timed out")
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
	})

	t.Run("AttachedPastDeadlineExitedCommitsResult", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{
				ID:        "proc-1",
				Attached:  true,
				StartedAt: time.Now().Add(-time.Hour).Unix(),
			}, nil)
		// Past the deadline the tool resolves from a non-blocking
		// snapshot instead of waiting.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", nil).
			Return(exitedOutput(3, "late result"), nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi"}`)
		assert.False(t, result.Success)
		assert.Equal(t, 3, result.ExitCode)
		assert.Equal(t, "late result", result.Output)
		assert.Empty(t, result.BackgroundProcessID)
	})

	t.Run("AttachedPastDeadlineRunningTimesOut", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{
				ID:        "proc-1",
				Attached:  true,
				StartedAt: time.Now().Add(-time.Hour).Unix(),
			}, nil)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", nil).
			Return(workspacesdk.ProcessOutputResponse{Running: true, Output: "partial"}, nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi"}`)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "timed out")
		assert.Equal(t, "partial", result.Output)
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
	})

	t.Run("AttachedWithoutStartTimeResolvesFromSnapshot", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{
				ID:       "proc-1",
				Attached: true,
			}, nil)
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", nil).
			Return(exitedOutput(0, "ok"), nil)

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"echo hi"}`)
		assert.True(t, result.Success)
		assert.Equal(t, "ok", result.Output)
	})

	t.Run("BackgroundAttachReturnsSameHandle", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.True(t, req.Background)
				assert.NotEmpty(t, req.IdempotencyKey)
				return workspacesdk.StartProcessResponse{ID: "proc-bg", Attached: true, IdempotencyKey: req.IdempotencyKey}, nil
			})

		tool := newExecuteTool(t, mockConn)
		result := runExecute(t, identityCtx(t), tool, "call-1", `{"command":"sleep 100","run_in_background":true}`)
		assert.True(t, result.Success)
		assert.Equal(t, "proc-bg", result.BackgroundProcessID)
	})
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
