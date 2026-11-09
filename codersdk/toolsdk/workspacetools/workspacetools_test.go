package workspacetools_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

var testNames = workspacetools.ToolNames{
	Execute:       "run",
	ProcessOutput: "output",
	ProcessList:   "list",
	ReadFile:      "read",
}

func TestExecute(t *testing.T) {
	t.Parallel()

	t.Run("InvalidRequests", func(t *testing.T) {
		t.Parallel()

		for name, tc := range map[string]struct {
			req     workspacetools.ExecuteRequest
			wantErr string
		}{
			"EmptyCommand": {
				req:     workspacetools.ExecuteRequest{},
				wantErr: "command is required",
			},
			"InvalidTimeout": {
				req:     workspacetools.ExecuteRequest{Command: "true", Timeout: ptr.Ref("soon")},
				wantErr: `invalid timeout "soon"`,
			},
			"TimeoutOverMax": {
				req: workspacetools.ExecuteRequest{
					Command:    "true",
					Timeout:    ptr.Ref("10m"),
					MaxTimeout: testutil.WaitShort,
					Names:      testNames,
				},
				wantErr: "use run_in_background=true and output for longer commands",
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				// No calls are expected: invalid requests must not
				// reach the agent.
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				_, err := workspacetools.Execute(testutil.Context(t, testutil.WaitShort), conn, tc.req)
				require.ErrorContains(t, err, tc.wantErr)
			})
		}
	})

	t.Run("TrailingAmpersandRunsInBackground", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ any, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Equal(t, "npm run dev", req.Command)
				assert.True(t, req.Background)
				assert.Zero(t, req.TimeoutMs)
				return workspacesdk.StartProcessResponse{ID: "proc-1", Started: true}, nil
			})

		res, err := workspacetools.Execute(testutil.Context(t, testutil.WaitShort), conn, workspacetools.ExecuteRequest{
			Command: "npm run dev &",
			// Background commands ignore timeouts, even invalid ones.
			Timeout: ptr.Ref("soon"),
		})
		require.NoError(t, err)
		assert.Equal(t, workspacetools.ExecuteResult{
			Success:             true,
			BackgroundProcessID: "proc-1",
			Backgrounded:        true,
		}, res)
	})

	t.Run("NonInteractiveEnvTakesPrecedence", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ any, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				assert.Equal(t, "cat", req.Env["PAGER"])
				assert.Equal(t, "yes", req.Env["EXTRA"])
				return workspacesdk.StartProcessResponse{ID: "proc-1"}, nil
			})

		_, err := workspacetools.Execute(testutil.Context(t, testutil.WaitShort), conn, workspacetools.ExecuteRequest{
			Command:         "true",
			RunInBackground: true,
			Env:             map[string]string{"PAGER": "less", "EXTRA": "yes"},
		})
		require.NoError(t, err)
	})

	t.Run("TimedOutCommandKeepsRunning", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: true, TimedOut: true, Output: "partial"}, nil)

		res, err := workspacetools.Execute(testutil.Context(t, testutil.WaitShort), conn, workspacetools.ExecuteRequest{
			Command: "sleep 600",
			Timeout: ptr.Ref("1s"),
			Names:   testNames,
		})
		require.NoError(t, err)
		assert.False(t, res.Success)
		assert.Equal(t, -1, res.ExitCode)
		assert.Equal(t, "partial", res.Output)
		assert.Equal(t, "proc-1", res.BackgroundProcessID)
		assert.Equal(t, "command timed out after 1s", res.Error)
	})

	t.Run("FileDumpNoteUsesToolName", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{ExitCode: ptr.Ref(0)}, nil)

		res, err := workspacetools.Execute(testutil.Context(t, testutil.WaitShort), conn, workspacetools.ExecuteRequest{
			Command: "cat main.go",
			Names:   testNames,
		})
		require.NoError(t, err)
		assert.Contains(t, res.Note, "Consider using read instead")
	})

	t.Run("MissingShellGuidance", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{}, xerrors.New(`exec: "sh": executable file not found in $PATH`))

		res, err := workspacetools.Execute(testutil.Context(t, testutil.WaitShort), conn, workspacetools.ExecuteRequest{
			Command: "true",
		})
		require.NoError(t, err)
		assert.False(t, res.Success)
		assert.Contains(t, res.Error, "start process: ")
		assert.Contains(t, res.Error, "Workspace commands run with \"sh -c\"")
	})
}

func TestProcessOutput(t *testing.T) {
	t.Parallel()

	t.Run("InvalidRequests", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := workspacetools.ProcessOutput(ctx, conn, workspacetools.ProcessOutputRequest{})
		require.ErrorContains(t, err, "process_id is required")

		_, err = workspacetools.ProcessOutput(ctx, conn, workspacetools.ProcessOutputRequest{
			ProcessID:   "proc-1",
			WaitTimeout: ptr.Ref("1h"),
			MaxWait:     testutil.WaitShort,
		})
		require.ErrorContains(t, err, "wait_timeout 1h0m0s exceeds the maximum")
	})

	t.Run("SnapshotWithoutWaiting", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-1", gomock.Nil()).
			Return(workspacesdk.ProcessOutputResponse{Running: true, Output: "so far", Command: "make"}, nil)

		res, err := workspacetools.ProcessOutput(testutil.Context(t, testutil.WaitShort), conn, workspacetools.ProcessOutputRequest{
			ProcessID:   "proc-1",
			WaitTimeout: ptr.Ref("0s"),
		})
		require.NoError(t, err)
		assert.Equal(t, workspacetools.ExecuteResult{
			Success: true,
			Running: true,
			Output:  "so far",
			Command: "make",
			Note:    "process is still running",
		}, res)
	})
}

func TestExitedResultTruncatesOutput(t *testing.T) {
	t.Parallel()

	// A 3-byte character straddles the limit, so the cut must drop it
	// rather than leave invalid UTF-8.
	output := strings.Repeat("x", workspacetools.MaxOutputBytes-1) + "☃"
	res := workspacetools.ExitedResult(workspacesdk.ProcessOutputResponse{Output: output, ExitCode: ptr.Ref(3)})
	assert.False(t, res.Success)
	assert.Equal(t, 3, res.ExitCode)
	assert.LessOrEqual(t, len(res.Output), workspacetools.MaxOutputBytes)
	assert.True(t, utf8.ValidString(res.Output))
}

func TestValidateFileEdits(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, workspacetools.ValidateFileEdits(nil), "files is required")
	require.ErrorContains(t, workspacetools.ValidateFileEdits([]workspacesdk.FileEdits{{Path: "  "}}),
		"files[0].path is required")
	require.ErrorContains(t, workspacetools.ValidateFileEdits([]workspacesdk.FileEdits{{Path: "/a"}}),
		"files[0].edits must contain at least one edit")

	files := []workspacesdk.FileEdits{{Path: " /a ", Edits: []workspacesdk.FileEdit{{OldText: "x"}}}}
	require.NoError(t, workspacetools.ValidateFileEdits(files))
	assert.Equal(t, "/a", files[0].Path)
}

func TestValidateSignal(t *testing.T) {
	t.Parallel()

	require.NoError(t, workspacetools.ValidateSignal("terminate"))
	require.NoError(t, workspacetools.ValidateSignal("kill"))
	require.Error(t, workspacetools.ValidateSignal("hangup"))
}

func TestAgentAPIErrorMessage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "plain", workspacetools.AgentAPIErrorMessage(xerrors.New("plain")))

	sdkErr := codersdk.NewTestError(400, "POST", "http://agent/api/v0/edit-files")
	sdkErr.Message = "no match"
	sdkErr.Detail = "old_text not found"
	sdkErr.Validations = []codersdk.ValidationError{{Field: "files[0]", Detail: "bad"}}
	assert.Equal(t, "no match: old_text not found\n- files[0]: bad", workspacetools.AgentAPIErrorMessage(sdkErr))
}
