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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return nil, uuid.Nil, xerrors.New("workspace offline")
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
var testAgentID = uuid.New()

func newExecuteTool(t *testing.T, mockConn *agentconnmock.MockAgentConn) fantasy.AgentTool {
	t.Helper()
	return chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
			return mockConn, testAgentID, nil
		},
	})
}

func ptr[T any](v T) *T {
	return &v
}

// fakeRecorder is an in-memory chattool.ExecutionRecorder for
// exercising the ledger paths without a database. It mirrors the
// production claim semantics: only reserved or missing rows are
// claimable, and terminal observations follow the same source
// guards.
type fakeRecorder struct {
	mu             sync.Mutex
	records        map[string]chattool.ExecutionRecord
	inputHashes    map[string]string
	commands       map[string]string
	claimCalls     int
	getCalls       int
	recordStartErr error
	claimErr       error
	// recordStartCtxErr captures ctx.Err() as observed by
	// RecordStart, so tests can assert the write runs on an
	// uncanceled context.
	recordStartCtxErr error
	recordStartCalled bool
	// onGet runs on every Get call with the call count, letting
	// tests mutate records mid-poll.
	onGet func(calls int)
	// onMarkStaleClaim runs at the start of MarkStaleClaimUnknown,
	// letting tests land a concurrent RecordStart in the race
	// window between the staleness verdict and the write.
	onMarkStaleClaim func()
}

func newFakeRecorder() *fakeRecorder {
	return &fakeRecorder{
		records:     map[string]chattool.ExecutionRecord{},
		inputHashes: map[string]string{},
		commands:    map[string]string{},
	}
}

func (f *fakeRecorder) Claim(_ context.Context, toolCallID string, inputSHA256 string, command string, background bool, timeout time.Duration, agentID uuid.UUID, staleBefore time.Time) (chattool.ExecutionRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	if f.claimErr != nil {
		return chattool.ExecutionRecord{}, false, f.claimErr
	}
	if storedHash, ok := f.inputHashes[toolCallID]; ok && storedHash != inputSHA256 {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("tool call %s: %w", toolCallID, chattool.ErrExecutionInputMismatch)
	}
	rec, ok := f.records[toolCallID]
	staleTakeover := rec.Status == chattool.ExecutionStatusStarting && !staleBefore.IsZero() && rec.ClaimedAt.Before(staleBefore)
	if !ok || rec.Status == chattool.ExecutionStatusReserved || staleTakeover {
		rec.ID = "exec-" + toolCallID
		rec.Status = chattool.ExecutionStatusStarting
		f.commands[toolCallID] = command
		rec.Background = background
		rec.Timeout = timeout
		rec.WorkspaceAgentID = agentID
		rec.ClaimEpoch++
		rec.ClaimedAt = time.Now()
		f.records[toolCallID] = rec
		return rec, true, nil
	}
	return rec, false, nil
}

func (f *fakeRecorder) Get(_ context.Context, toolCallID string) (chattool.ExecutionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	if f.onGet != nil {
		f.onGet(f.getCalls)
	}
	rec, ok := f.records[toolCallID]
	if !ok {
		return chattool.ExecutionRecord{}, xerrors.Errorf("tool call %s: %w", toolCallID, chattool.ErrExecutionRecordNotFound)
	}
	return rec, nil
}

func (f *fakeRecorder) RecordStart(ctx context.Context, toolCallID string, claimEpoch int64, processID string, agentID uuid.UUID, startedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recordStartCalled = true
	f.recordStartCtxErr = ctx.Err()
	if f.recordStartErr != nil {
		return f.recordStartErr
	}
	rec := f.records[toolCallID]
	if rec.Status != chattool.ExecutionStatusStarting || rec.ClaimEpoch != claimEpoch {
		return xerrors.Errorf("claim epoch %d superseded", claimEpoch)
	}
	rec.Status = chattool.ExecutionStatusRunning
	rec.ProcessID = processID
	rec.StartedAt = startedAt
	f.records[toolCallID] = rec
	return nil
}

// fakeTerminalSources mirrors the production recorder's per-status
// source guards.
var fakeTerminalSources = map[chattool.ExecutionStatus][]chattool.ExecutionStatus{
	chattool.ExecutionStatusExited:   {chattool.ExecutionStatusStarting, chattool.ExecutionStatusRunning, chattool.ExecutionStatusDetached},
	chattool.ExecutionStatusDetached: {chattool.ExecutionStatusStarting, chattool.ExecutionStatusRunning},
	chattool.ExecutionStatusUnknown:  {chattool.ExecutionStatusReserved, chattool.ExecutionStatusStarting, chattool.ExecutionStatusRunning, chattool.ExecutionStatusExited, chattool.ExecutionStatusDetached},
	chattool.ExecutionStatusNoEffect: {chattool.ExecutionStatusReserved, chattool.ExecutionStatusStarting},
}

func (f *fakeRecorder) MarkTerminal(_ context.Context, toolCallID string, status chattool.ExecutionStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := fakeTerminalSources[status]; !ok {
		// Mirror the production recorder, which rejects statuses
		// that are not tool-observable.
		return xerrors.Errorf("status %q is not a tool-observable terminal status", status)
	}
	rec, ok := f.records[toolCallID]
	if !ok {
		return nil
	}
	for _, source := range fakeTerminalSources[status] {
		if rec.Status == source {
			rec.Status = status
			f.records[toolCallID] = rec
			return nil
		}
	}
	return nil
}

func (f *fakeRecorder) MarkStaleClaimUnknown(_ context.Context, toolCallID string) (chattool.ExecutionRecord, bool, error) {
	if f.onMarkStaleClaim != nil {
		f.onMarkStaleClaim()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[toolCallID]
	if !ok {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("tool call %s: %w", toolCallID, chattool.ErrExecutionRecordNotFound)
	}
	if rec.Status != chattool.ExecutionStatusStarting {
		return rec, false, nil
	}
	rec.Status = chattool.ExecutionStatusUnknown
	f.records[toolCallID] = rec
	return rec, true, nil
}

func (f *fakeRecorder) seed(toolCallID string, rec chattool.ExecutionRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[toolCallID] = rec
}

func (f *fakeRecorder) seedInputHash(toolCallID string, hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputHashes[toolCallID] = hash
}

func (f *fakeRecorder) record(toolCallID string) chattool.ExecutionRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.records[toolCallID]
}

func (f *fakeRecorder) command(toolCallID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commands[toolCallID]
}

func newRecordedExecuteTool(t *testing.T, mockConn *agentconnmock.MockAgentConn, recorder chattool.ExecutionRecorder) fantasy.AgentTool {
	t.Helper()
	return chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
			return mockConn, testAgentID, nil
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
		assert.Equal(t, "echo hi", recorder.command("call-1"))
		assert.False(t, rec.StartedAt.IsZero())
		assert.Equal(t, chattool.ExecutionStatusExited, rec.Status)
	})

	t.Run("ReattachRunningProcess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
			Timeout:   time.Minute,
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
		assert.Equal(t, chattool.ExecutionStatusExited, recorder.record("call-1").Status)
	})

	t.Run("ReattachExitedPastDeadline", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
			Timeout:   time.Second,
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
		assert.Equal(t, chattool.ExecutionStatusExited, recorder.record("call-1").Status)
	})

	t.Run("ReattachRunningPastDeadline", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
			Timeout:   time.Second,
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
		assert.Equal(t, chattool.ExecutionStatusDetached, recorder.record("call-1").Status)
	})

	t.Run("ReattachNotFound", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
			Timeout:   time.Minute,
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
		assert.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	t.Run("ReattachTransportError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
			Timeout:   time.Minute,
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
		// A transport error leaves the process potentially
		// retrievable, so the lifecycle stays running.
		assert.Equal(t, chattool.ExecutionStatusRunning, recorder.record("call-1").Status)
	})

	t.Run("ReattachBackground", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:     chattool.ExecutionStatusDetached,
			ProcessID:  "proc-1",
			Background: true,
			Timeout:    time.Minute,
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

	t.Run("WaitSnapshot404MarksUnknown", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1", Started: true}, nil)
		gomock.InOrder(
			mockConn.EXPECT().
				ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{}, xerrors.New("transport failure")),
			// The recovery snapshot reaches the agent, which
			// definitively does not know the process anymore.
			mockConn.EXPECT().
				ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{}, notFoundError(t)),
		)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "no longer known")
		assert.NotContains(t, resp.Content, "background_process_id")
		assert.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	// seedStaleStarting seeds a starting claim whose owner died
	// claimedAgo ago without recording a process handle. The claim
	// targeted the same agent the current attempt is connected to.
	seedStaleStarting := func(recorder *fakeRecorder, claimedAgo time.Duration) {
		recorder.seed("call-1", chattool.ExecutionRecord{
			ID:               "exec-call-1",
			Status:           chattool.ExecutionStatusStarting,
			Timeout:          time.Minute,
			ClaimEpoch:       1,
			ClaimedAt:        time.Now().Add(-claimedAgo),
			WorkspaceAgentID: testAgentID,
		})
	}

	t.Run("StaleClaimAgentRebindUnknown", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		// The stale claim dispatched to a different agent, so the
		// current connection's token index cannot observe that
		// dispatch: no probe, no re-dispatch, outcome unknown.
		recorder.seed("call-1", chattool.ExecutionRecord{
			ID:               "exec-call-1",
			Status:           chattool.ExecutionStatusStarting,
			Timeout:          time.Minute,
			ClaimEpoch:       1,
			ClaimedAt:        time.Now().Add(-2 * time.Minute),
			WorkspaceAgentID: uuid.New(),
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
		assert.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	t.Run("StaleClaimOldAgentUnknown", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		seedStaleStarting(recorder, 2*time.Minute)

		// An agent that predates the token probe endpoint
		// answers 404 for the route itself.
		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{}, notFoundError(t))

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
		assert.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	t.Run("StaleClaimProbeFoundAdopts", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		seedStaleStarting(recorder, 2*time.Minute)

		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{Found: true, ProcessID: "proc-1"}, nil)
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
		assert.Equal(t, chattool.ExecutionStatusExited, rec.Status)
		// Adoption anchors started_at at the claim time, the lower
		// bound of the true start, so a long-running recovered
		// process cannot win a fresh full timeout.
		assert.WithinDuration(t, time.Now().Add(-2*time.Minute), rec.StartedAt, 30*time.Second)
	})

	t.Run("StaleClaimAbsentTokenYoungIndexUnknown", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		seedStaleStarting(recorder, 2*time.Minute)

		// The agent restarted after the claim: its token index is
		// younger than the claim, so absence proves nothing about
		// what the previous agent process may have started.
		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{Found: false, TokenIndexAgeMS: (10 * time.Second).Milliseconds()}, nil)

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
		assert.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	t.Run("StaleClaimProbeNotFoundRedispatches", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		seedStaleStarting(recorder, 2*time.Minute)

		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{Found: false, TokenIndexAgeMS: time.Hour.Milliseconds()}, nil)
		// The re-dispatch reuses the same execution token so the
		// agent dedups a race with the original dispatch.
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Equal(t, "exec-call-1", req.ClientToken)
				return workspacesdk.StartProcessResponse{ID: "proc-2", Started: true, ClientToken: req.ClientToken}, nil
			}).
			Times(1)
		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-2", gomock.Any()).
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
		assert.Equal(t, "proc-2", rec.ProcessID)
		assert.EqualValues(t, 2, rec.ClaimEpoch)
		assert.Equal(t, chattool.ExecutionStatusExited, rec.Status)
	})

	t.Run("StaleClaimProbePendingRedispatches", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		// Even past the trust window, a pending reservation means
		// a start owning this token is in flight on the agent, so
		// re-dispatching the same token attaches to it instead of
		// resolving to unknown.
		seedStaleStarting(recorder, chattool.TokenTrustWindow+time.Minute)

		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{Pending: true}, nil)
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Equal(t, "exec-call-1", req.ClientToken)
				return workspacesdk.StartProcessResponse{ID: "proc-3", ClientToken: req.ClientToken, Attached: true}, nil
			}).
			Times(1)
		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-3", gomock.Any()).
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
	})

	t.Run("StaleClaimBeyondTrustWindowUnknown", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		// Older than TokenTrustWindow: the agent may have reaped
		// the token with its exited process.
		seedStaleStarting(recorder, chattool.TokenTrustWindow+time.Minute)

		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{Found: false}, nil)

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
		assert.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	t.Run("StaleClaimProbeTransportErrorKeepsRow", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		seedStaleStarting(recorder, 2*time.Minute)

		mockConn.EXPECT().
			ProcessByToken(gomock.Any(), "exec-call-1").
			Return(workspacesdk.ProcessByTokenResponse{}, xerrors.New("dial tcp: connection refused"))

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "probe execution token")
		// The row is untouched so a later attempt can probe again.
		assert.Equal(t, chattool.ExecutionStatusStarting, recorder.record("call-1").Status)
	})

	t.Run("FreshStartingClaimRecovers", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:     chattool.ExecutionStatusStarting,
			Timeout:    time.Minute,
			ClaimEpoch: 1,
			ClaimedAt:  time.Now(),
		})
		// The owning claim records the handle while this attempt
		// polls the fresh claim.
		recorder.onGet = func(calls int) {
			if calls >= 2 {
				rec := recorder.records["call-1"]
				rec.Status = chattool.ExecutionStatusRunning
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

	t.Run("RecordStartFailureKeepsWaiting", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.recordStartErr = xerrors.New("database gone")

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

		// The failed ledger write costs diagnostics, never the
		// result: the attached wait still returns the real exit.
		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Success)
		assert.Equal(t, "done", result.Output)
		assert.True(t, recorder.recordStartCalled)
		// Without the recorded handle, the wait outcome must not
		// terminalize the row: an exited row with no process would
		// strand a retry, while a starting row resolves through
		// claim recovery.
		assert.Equal(t, chattool.ExecutionStatusStarting, recorder.record("call-1").Status)
	})

	t.Run("ClaimInputMismatch", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusReserved,
		})
		recorder.seedInputHash("call-1", chattool.HashToolInput(`{"command":"something else"}`))

		// No StartProcess expectation: a mismatched claim must
		// never dispatch.
		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "stale lineage")
	})

	t.Run("ClaimInfrastructureFailureAborts", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.claimErr = xerrors.New("database connection lost")

		// No StartProcess expectation: a failed claim must never
		// dispatch, and the failure aborts the batch instead of
		// committing an error result that would end the call.
		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.Error(t, err)
		var abortErr *chattool.AbortToolExecutionError
		require.ErrorAs(t, err, &abortErr)
	})

	t.Run("ConnFailureResumableRowAborts", func(t *testing.T) {
		t.Parallel()
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
			Timeout:   time.Minute,
			StartedAt: time.Now(),
		})

		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return nil, uuid.Nil, xerrors.New("workspace agent unreachable")
			},
			Recorder: recorder,
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.Error(t, err)
		var abortErr *chattool.AbortToolExecutionError
		require.ErrorAs(t, err, &abortErr)
		// The lifecycle stays running so a later attempt with a
		// working connection re-attaches.
		require.Equal(t, chattool.ExecutionStatusRunning, recorder.record("call-1").Status)
	})

	t.Run("ConnFailureStaleStartingResolvesUnknown", func(t *testing.T) {
		t.Parallel()
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusStarting,
			Timeout:   time.Minute,
			ClaimedAt: time.Now().Add(-2 * time.Minute),
		})

		// Stale-claim resolution is a ledger write, so an
		// unreachable agent must not wedge the chat by aborting.
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return nil, uuid.Nil, xerrors.New("workspace agent unreachable")
			},
			Recorder: recorder,
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "process state is unknown")
		require.Equal(t, chattool.ExecutionStatusUnknown, recorder.record("call-1").Status)
	})

	t.Run("ConnFailureStaleStartingLateProcessAborts", func(t *testing.T) {
		t.Parallel()
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusStarting,
			Timeout:   time.Minute,
			ClaimedAt: time.Now().Add(-2 * time.Minute),
		})
		// The dead owner's uncanceled RecordStart lands in the
		// race window: the guarded unknown write must lose, and
		// resuming the foreground process needs the agent, so the
		// attempt aborts instead of committing a result.
		recorder.onMarkStaleClaim = func() {
			rec := recorder.record("call-1")
			rec.Status = chattool.ExecutionStatusRunning
			rec.ProcessID = "proc-late"
			rec.StartedAt = time.Now()
			recorder.seed("call-1", rec)
		}

		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return nil, uuid.Nil, xerrors.New("workspace agent unreachable")
			},
			Recorder: recorder,
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.Error(t, err)
		var abortErr *chattool.AbortToolExecutionError
		require.ErrorAs(t, err, &abortErr)
		require.Equal(t, chattool.ExecutionStatusRunning, recorder.record("call-1").Status)
	})

	t.Run("ConnFailureUndispatchedRowErrors", func(t *testing.T) {
		t.Parallel()
		recorder := newFakeRecorder()
		recorder.seed("call-reserved", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusReserved,
		})

		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return nil, uuid.Nil, xerrors.New("workspace agent unreachable")
			},
			Recorder: recorder,
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		// A reserved row and a row that predates the ledger both
		// prove nothing was dispatched: the dial error commits as
		// a normal tool result.
		for _, callID := range []string{"call-reserved", "call-missing"} {
			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    callID,
				Name:  "execute",
				Input: `{"command":"echo hi"}`,
			})
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, "unreachable")
		}
	})

	t.Run("Reattach404WrongAgentAborts", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:           chattool.ExecutionStatusRunning,
			ProcessID:        "proc-1",
			Timeout:          time.Minute,
			StartedAt:        time.Now(),
			WorkspaceAgentID: uuid.New(),
		})

		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, notFoundError(t))

		// The connection targets testAgentID, not the recorded
		// owner, so its 404 proves nothing about the process.
		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.Error(t, err)
		var abortErr *chattool.AbortToolExecutionError
		require.ErrorAs(t, err, &abortErr)
		require.Equal(t, chattool.ExecutionStatusRunning, recorder.record("call-1").Status)
	})

	t.Run("ReattachDialsRecordedAgent", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		// No expectations on the turn's connection: the probe
		// must go to the recorded owner agent instead.
		turnConn := agentconnmock.NewMockAgentConn(ctrl)
		ownerConn := agentconnmock.NewMockAgentConn(ctrl)
		ownerAgentID := uuid.New()
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:           chattool.ExecutionStatusRunning,
			ProcessID:        "proc-1",
			Timeout:          time.Minute,
			StartedAt:        time.Now(),
			WorkspaceAgentID: ownerAgentID,
		})

		exitCode := 0
		ownerConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "done",
			}, nil)

		released := false
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return turnConn, testAgentID, nil
			},
			Recorder: recorder,
			DialAgent: func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				assert.Equal(t, ownerAgentID, agentID)
				return ownerConn, func() { released = true }, nil
			},
		})
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
		assert.True(t, released, "the dialed owner connection must be released")
		assert.Equal(t, chattool.ExecutionStatusExited, recorder.record("call-1").Status)
	})

	t.Run("ReattachRecordedAgentDialFailureAborts", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		turnConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status:           chattool.ExecutionStatusRunning,
			ProcessID:        "proc-1",
			Timeout:          time.Minute,
			StartedAt:        time.Now(),
			WorkspaceAgentID: uuid.New(),
		})

		// A failed dial to the owner proves nothing about the
		// process; committing a result would end re-attachment.
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return turnConn, testAgentID, nil
			},
			Recorder: recorder,
			DialAgent: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, nil, xerrors.New("agent is unreachable")
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.Error(t, err)
		var abortErr *chattool.AbortToolExecutionError
		require.ErrorAs(t, err, &abortErr)
		require.Equal(t, chattool.ExecutionStatusRunning, recorder.record("call-1").Status)
	})

	t.Run("EmptyToolCallIDRefused", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		// No StartProcess expectation: an ID-less call cannot be
		// tracked in the ledger and must not dispatch.
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "tool call has no ID")
		recorder.mu.Lock()
		claims := recorder.claimCalls
		recorder.mu.Unlock()
		assert.Zero(t, claims, "an ID-less call must be refused before claiming")
	})

	t.Run("ResolvedRowNeverRestarts", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusCanceled,
		})

		// No StartProcess expectation: a resolved row must never
		// be re-dispatched.
		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "already resolved")
		assert.Contains(t, resp.Content, "canceled")
	})

	t.Run("UnknownRowStableResult", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusUnknown,
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

	t.Run("TerminalRowResolvesWithoutConn", func(t *testing.T) {
		t.Parallel()
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusUnknown,
		})
		recorder.seed("call-2", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusCanceled,
		})
		recorder.seed("call-3", chattool.ExecutionRecord{
			Status:    chattool.ExecutionStatusRunning,
			ProcessID: "proc-1",
		})
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return nil, uuid.Nil, xerrors.New("workspace agent unreachable")
			},
			Recorder: recorder,
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Rows the ledger already resolved return their stable
		// results even though the workspace is unreachable.
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "execute", Input: `{"command":"echo hi"}`})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "unknown")

		resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "call-2", Name: "execute", Input: `{"command":"echo hi"}`})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "already resolved")

		// A row that needs the agent to make progress aborts
		// instead of committing an error result that would end
		// re-attachment.
		_, err = tool.Run(ctx, fantasy.ToolCall{ID: "call-3", Name: "execute", Input: `{"command":"echo hi"}`})
		require.Error(t, err)
		var abortErr *chattool.AbortToolExecutionError
		require.ErrorAs(t, err, &abortErr)
	})

	t.Run("ValidationFailureMarksNoEffect", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()
		recorder.seed("call-1", chattool.ExecutionRecord{
			Status: chattool.ExecutionStatusReserved,
		})

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi","timeout":"potato"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "invalid timeout")
		assert.Equal(t, chattool.ExecutionStatusNoEffect, recorder.record("call-1").Status)
		assert.Zero(t, recorder.claimCalls)
	})

	t.Run("StartErrorKeepsRowRecoverable", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{}, xerrors.New("dial tcp: connection refused"))

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
		assert.Contains(t, result.Error, "start process")
		// The request may have reached the agent, and the token is
		// durable. The row stays starting so a retry resolves the
		// dispatch through the token probe instead of dead-ending
		// on a terminal unknown.
		assert.Equal(t, chattool.ExecutionStatusStarting, recorder.record("call-1").Status)
	})

	t.Run("BackgroundStartMarksDetached", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"npm run dev","run_in_background":true}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		rec := recorder.record("call-1")
		assert.Equal(t, "proc-1", rec.ProcessID)
		assert.Equal(t, chattool.ExecutionStatusDetached, rec.Status)
	})

	t.Run("RecordStartSurvivesInterruptCancel", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		recorder := newFakeRecorder()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitMedium))
		defer cancel()
		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				// Simulate an interrupt canceling the generation
				// context while the start request is in flight.
				cancel()
				return workspacesdk.StartProcessResponse{ID: "proc-1"}, nil
			})
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{}, context.Canceled).
			AnyTimes()

		tool := newRecordedExecuteTool(t, mockConn, recorder)
		_, _ = tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "execute",
			Input: `{"command":"echo hi"}`,
		})

		// The handle must be recorded even though the tool context
		// was canceled, so the interrupt path can kill the process.
		assert.True(t, recorder.recordStartCalled)
		assert.NoError(t, recorder.recordStartCtxErr)
		assert.Equal(t, "proc-1", recorder.record("call-1").ProcessID)
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
			assert.Zero(t, recorder.claimCalls)
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

		t.Run("BackgroundIgnoresTimeout", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			recorder := newFakeRecorder()

			mockConn.EXPECT().
				StartProcess(gomock.Any(), gomock.Any()).
				Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)

			tool := newRecordedExecuteTool(t, mockConn, recorder)
			ctx := testutil.Context(t, testutil.WaitMedium)
			// The timeout argument only applies to foreground
			// commands, so a nonpositive value must not fail a
			// backgrounded call.
			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "execute",
				Input: `{"command":"npm run dev","run_in_background":true,"timeout":"0s"}`,
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError)

			var result chattool.ExecuteResult
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			assert.True(t, result.Success)
			assert.Equal(t, "proc-1", result.BackgroundProcessID)
			assert.Zero(t, recorder.record("call-1").Timeout,
				"background rows must not record a foreground deadline")
		})
	})
}

func TestProcessOutputToolWaitTimeoutClamp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	var deadline time.Time
	var hasDeadline bool
	exitCode := 0
	mockConn.EXPECT().
		ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
			deadline, hasDeadline = ctx.Deadline()
			return workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
			}, nil
		})

	tool := chattool.ProcessOutput(chattool.ProcessToolOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
	before := time.Now()
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "process_output",
		Input: `{"process_id":"proc-1","wait_timeout":"25h"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	require.True(t, hasDeadline, "the blocking wait must carry a deadline")
	remaining := deadline.Sub(before)
	assert.Less(t, remaining, 4*time.Hour+time.Minute, "wait_timeout must be clamped to 4h")
	assert.Greater(t, remaining, 3*time.Hour+55*time.Minute, "clamp must not shorten the wait below the maximum")
}

// TestExecuteWaitRetriesEarlyServerReturn covers the blocking-wait
// retry: the agent's server-side wait bound can elapse before the
// process exits, and the client must re-issue the wait (with a
// pause) instead of treating the running snapshot as final.
func TestExecuteWaitRetriesEarlyServerReturn(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		StartProcess(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)

	exitCode := 0
	calls := 0
	mockConn.EXPECT().
		ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
		Times(3).
		DoAndReturn(func(_ context.Context, _ string, opts *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
			calls++
			require.NotNil(t, opts)
			require.True(t, opts.Wait)
			if calls < 3 {
				return workspacesdk.ProcessOutputResponse{Running: true, Output: "partial"}, nil
			}
			return workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode, Output: "done"}, nil
		})

	tool := newExecuteTool(t, mockConn)
	ctx := testutil.Context(t, testutil.WaitMedium)
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "execute",
		Input: `{"command":"echo hi","timeout":"30s"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	assert.True(t, result.Success)
	assert.Equal(t, "done", result.Output)
	assert.Equal(t, 3, calls)
}

func TestProcessOutputToolRejectsNegativeWaitTimeout(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	// No ProcessOutput expectation: a negative wait must be
	// rejected before any agent call.
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	tool := chattool.ProcessOutput(chattool.ProcessToolOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "process_output",
		Input: `{"process_id":"proc-1","wait_timeout":"-5s"}`,
	})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "must not be negative")
}

func TestExecuteToolClientToken(t *testing.T) {
	t.Parallel()

	// runExecute runs "echo hi" against a mock whose StartProcess
	// response is produced by respond, returning the parsed result
	// and the captured log entries. Every variant must produce the
	// same tool result; only the logging differs.
	runExecute := func(t *testing.T, respond func(req workspacesdk.StartProcessRequest) workspacesdk.StartProcessResponse) (chattool.ExecuteResult, *testutil.FakeSink) {
		t.Helper()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		sink := testutil.NewFakeSink(t)

		mockConn.EXPECT().
			StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Equal(t, "exec-call-1", req.ClientToken)
				return respond(req), nil
			})
		exitCode := 0
		mockConn.EXPECT().
			ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{
				Running:  false,
				ExitCode: &exitCode,
				Output:   "done",
			}, nil)

		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, uuid.UUID, error) {
				return mockConn, testAgentID, nil
			},
			Logger:   sink.Logger(),
			Recorder: newFakeRecorder(),
		})
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
		return result, sink
	}

	missingSupport := func(entry slog.SinkEntry) bool {
		return entry.Message == "workspace agent does not support idempotent process starts"
	}
	deduped := func(entry slog.SinkEntry) bool {
		return entry.Message == "execute_agent_deduped"
	}

	t.Run("EchoWithoutAttach", func(t *testing.T) {
		t.Parallel()

		result, sink := runExecute(t, func(req workspacesdk.StartProcessRequest) workspacesdk.StartProcessResponse {
			return workspacesdk.StartProcessResponse{
				ID:          "proc-1",
				Started:     true,
				ClientToken: req.ClientToken,
			}
		})
		assert.True(t, result.Success)
		assert.Equal(t, "done", result.Output)
		assert.Empty(t, sink.Entries(missingSupport))
		assert.Empty(t, sink.Entries(deduped))
	})

	t.Run("EchoWithAttachLogsDeduped", func(t *testing.T) {
		t.Parallel()

		result, sink := runExecute(t, func(req workspacesdk.StartProcessRequest) workspacesdk.StartProcessResponse {
			return workspacesdk.StartProcessResponse{
				ID:          "proc-1",
				ClientToken: req.ClientToken,
				Attached:    true,
			}
		})
		assert.True(t, result.Success)
		assert.Equal(t, "done", result.Output)
		assert.Empty(t, sink.Entries(missingSupport))
		assert.Len(t, sink.Entries(deduped), 1)
	})

	t.Run("MissingEchoLogsAndKeepsBehavior", func(t *testing.T) {
		t.Parallel()

		result, sink := runExecute(t, func(workspacesdk.StartProcessRequest) workspacesdk.StartProcessResponse {
			// An agent that predates idempotent starts drops the
			// unknown request field and echoes nothing back.
			return workspacesdk.StartProcessResponse{
				ID:      "proc-1",
				Started: true,
			}
		})
		assert.True(t, result.Success)
		assert.Equal(t, "done", result.Output)
		assert.Len(t, sink.Entries(missingSupport), 1)
		assert.Empty(t, sink.Entries(deduped))
	})
}
