package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
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
		assert.Contains(t, modelIntentParam["description"], "do not repeat the command")
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

// fakeRecorder is an in-memory chattool.ExecutionRecorder for
// exercising the idempotent-start paths without a database.
type fakeRecorder struct {
	mu             sync.Mutex
	records        map[string]chattool.ExecutionRecord
	reserveCalls   int
	recordStartErr error
	// onReserve runs on every Reserve call with the call count,
	// letting tests mutate records mid-grace.
	onReserve func(calls int)
}

func newFakeRecorder() *fakeRecorder {
	return &fakeRecorder{records: map[string]chattool.ExecutionRecord{}}
}

func (f *fakeRecorder) Reserve(_ context.Context, toolCallID string, command string, background bool, timeout time.Duration) (chattool.ExecutionRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserveCalls++
	if f.onReserve != nil {
		f.onReserve(f.reserveCalls)
	}
	if rec, ok := f.records[toolCallID]; ok {
		return rec, false, nil
	}
	rec := chattool.ExecutionRecord{
		Command:    command,
		Background: background,
		Timeout:    timeout,
		CreatedAt:  time.Now(),
	}
	f.records[toolCallID] = rec
	return rec, true, nil
}

func (f *fakeRecorder) RecordStart(_ context.Context, toolCallID string, processID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recordStartErr != nil {
		return f.recordStartErr
	}
	rec := f.records[toolCallID]
	rec.ProcessID = processID
	rec.StartedAt = time.Now()
	f.records[toolCallID] = rec
	return nil
}

func (f *fakeRecorder) seed(toolCallID string, rec chattool.ExecutionRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[toolCallID] = rec
}

func (f *fakeRecorder) record(toolCallID string) chattool.ExecutionRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.records[toolCallID]
}

func newRecordedExecuteTool(t *testing.T, mockConn *agentconnmock.MockAgentConn, recorder chattool.ExecutionRecorder) fantasy.AgentTool {
	t.Helper()
	return chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		Recorder: recorder,
	})
}

func notFoundError(t *testing.T) error {
	t.Helper()
	res := &http.Response{
		StatusCode: http.StatusNotFound,
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Path: "/api/v0/processes/proc-1"},
		},
		Body: io.NopCloser(strings.NewReader(`{"message":"process not found"}`)),
	}
	err := codersdk.ReadBodyAsError(res)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	return err
}

func TestExecuteToolRecorder(t *testing.T) {
	t.Parallel()

	t.Run("FreshStartRecordsProcess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "done",
			}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
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
		assert.True(t, result.Success)
		assert.Equal(t, "done", result.Output)

		rec := recorder.record("call-1")
		assert.Equal(t, "proc-1", rec.ProcessID)
		assert.Equal(t, "echo hi", rec.Command)
		assert.False(t, rec.StartedAt.IsZero())
	})

	t.Run("ReattachRunningProcess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			ProcessID: "proc-1",
			Command:   "sleep 5",
			Timeout:   time.Minute,
			CreatedAt: time.Now(),
			StartedAt: time.Now(),
		})

		// No StartProcess expectation: a second start would fail
		// the mock controller. The snapshot shows the process
		// running, then the blocking wait returns the result.
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: true}, nil)
		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "finished",
			}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"sleep 5","timeout":"1m"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, "finished", result.Output)
	})

	t.Run("ReattachExitedPastDeadline", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			ProcessID: "proc-1",
			Command:   "make build",
			Timeout:   time.Second,
			CreatedAt: time.Now().Add(-time.Hour),
			StartedAt: time.Now().Add(-time.Hour),
		})

		exitCode := 2
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "build failed",
			}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"make build"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Equal(t, 2, result.ExitCode)
		assert.Equal(t, "build failed", result.Output)
		assert.Empty(t, result.BackgroundProcessID)
	})

	t.Run("ReattachRunningPastDeadline", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			ProcessID: "proc-1",
			Command:   "sleep 600",
			Timeout:   time.Second,
			CreatedAt: time.Now().Add(-time.Hour),
			StartedAt: time.Now().Add(-time.Hour),
		})

		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running: true,
				Output:  "partial",
			}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"sleep 600"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
		assert.Contains(t, result.Error, "timed out")
		assert.Equal(t, "partial", result.Output)
	})

	t.Run("ReattachNotFound", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			ProcessID: "proc-1",
			Command:   "rm -rf ./build",
			Timeout:   time.Minute,
			CreatedAt: time.Now(),
			StartedAt: time.Now(),
		})

		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, notFoundError(t))

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"rm -rf ./build"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "proc-1")
		assert.Contains(t, resp.Content, "unknown")
	})

	t.Run("ReattachTransportError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			ProcessID: "proc-1",
			Command:   "echo hi",
			Timeout:   time.Minute,
			CreatedAt: time.Now(),
			StartedAt: time.Now(),
		})

		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, xerrors.New("dial tcp: connection refused"))

		tool := newRecordedExecuteTool(t, mockConn, recorder)
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
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
		assert.Contains(t, result.Error, "re-attach")
	})

	t.Run("ReattachBackground", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			ProcessID:  "proc-1",
			Command:    "npm run dev",
			Background: true,
			Timeout:    time.Minute,
			CreatedAt:  time.Now(),
			StartedAt:  time.Now(),
		})

		// No agent calls at all: the background handle is returned
		// straight from the record.
		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"npm run dev","run_in_background":true}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
	})

	t.Run("NullHandlePastGraceUnknown", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Command:   "echo hi",
			Timeout:   time.Minute,
			CreatedAt: time.Now().Add(-2 * time.Minute),
		})

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "unknown")
		assert.Contains(t, resp.Content, "safe")
	})

	t.Run("NullHandleGraceRecovers", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Command:   "echo hi",
			Timeout:   time.Minute,
			CreatedAt: time.Now(),
		})
		// The owning attempt records the handle while this attempt
		// waits out the grace window.
		recorder.onReserve = func(calls int) {
			if calls >= 2 {
				rec := recorder.records["call-1"]
				rec.ProcessID = "proc-1"
				rec.StartedAt = time.Now()
				recorder.records["call-1"] = rec
			}
		}

		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "hi",
			}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, "hi", result.Output)
	})

	t.Run("RecordStartFailureKeepsProcess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.recordStartErr = xerrors.New("database gone")

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
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
		assert.Equal(t, "proc-1", result.BackgroundProcessID)
		assert.Contains(t, result.Error, "record process start")
	})

	t.Run("TimeoutClamp", func(t *testing.T) {
		t.Parallel()

		t.Run("RejectsZero", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			recorder := newFakeRecorder()

			tool := newRecordedExecuteTool(t, mockConn, recorder)
			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-1",
				Name:  "execute",
				Input: `{"command":"echo hi","timeout":"0s"}`,
			})
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, "timeout must be positive")
			assert.Zero(t, recorder.reserveCalls)
		})

		t.Run("ClampsExcessive", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			recorder := newFakeRecorder()

			mockConn.EXPECT().
				StartProcess(gomock.Any(), gomock.Any()).
				Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
			exitCode := 0
			mockConn.EXPECT().
				ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{
					Running:  false,
					ExitCode: &exitCode,
				}, nil)

			tool := newRecordedExecuteTool(t, mockConn, recorder)
			ctx := testutil.Context(t, testutil.WaitMedium)
			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "execute",
				Input: `{"command":"echo hi","timeout":"25h"}`,
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError)

			rec := recorder.record("call-1")
			assert.Equal(t, 4*time.Hour, rec.Timeout)
		})
	})
}
