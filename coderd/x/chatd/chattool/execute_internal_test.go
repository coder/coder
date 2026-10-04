package chattool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestExecuteProcessWait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		defaultTimeout time.Duration
		background     bool
		wantWait       time.Duration
		wantCalls      int
	}{
		{
			name:      "Requested",
			input:     `{"command":"echo test","timeout":"31m"}`,
			wantWait:  31 * time.Minute,
			wantCalls: 1,
		},
		{
			name:      "Default",
			input:     `{"command":"echo test"}`,
			wantWait:  defaultTimeout,
			wantCalls: 1,
		},
		{
			name:           "ConfiguredDefault",
			input:          `{"command":"echo test"}`,
			defaultTimeout: 2 * time.Minute,
			wantWait:       2 * time.Minute,
			wantCalls:      1,
		},
		{
			name:       "Background",
			input:      `{"command":"echo test","timeout":"31m","run_in_background":true}`,
			background: true,
		},
		{
			name:  "Invalid",
			input: `{"command":"echo test","timeout":"invalid"}`,
		},
		{
			name:  "Zero",
			input: `{"command":"echo test","timeout":"0s"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			if tt.name != "Invalid" {
				mockConn.EXPECT().
					StartProcess(gomock.Any(), gomock.Any()).
					Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
			}
			if !tt.background && tt.name != "Invalid" {
				exitCode := 0
				mockConn.EXPECT().
					ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
					Return(workspacesdk.ProcessOutputResponse{ExitCode: &exitCode}, nil)
			}

			var waits []time.Duration
			ctx := WithProcessWait(
				testutil.Context(t, testutil.WaitMedium),
				func(wait time.Duration) { waits = append(waits, wait) },
			)
			tool := Execute(ExecuteOptions{
				GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
					return mockConn, nil
				},
				DefaultTimeout: tt.defaultTimeout,
			})
			_, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  ExecuteToolName,
				Input: tt.input,
			})
			require.NoError(t, err)
			require.Len(t, waits, tt.wantCalls)
			if tt.wantCalls > 0 {
				require.Equal(t, tt.wantWait, waits[0])
			}
		})
	}
}

func TestExecuteProcessWaitPollingReportsOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		StartProcess(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StartProcessResponse{ID: "proc-1"}, nil)
	mockConn.EXPECT().
		ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
		Return(workspacesdk.ProcessOutputResponse{Running: true}, nil)
	exitCode := 0
	mockConn.EXPECT().
		ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
		Return(workspacesdk.ProcessOutputResponse{ExitCode: &exitCode}, nil)

	var waits []time.Duration
	ctx := WithProcessWait(
		testutil.Context(t, testutil.WaitMedium),
		func(wait time.Duration) { waits = append(waits, wait) },
	)
	tool := Execute(ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
	_, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  ExecuteToolName,
		Input: `{"command":"echo test","timeout":"31m"}`,
	})
	require.NoError(t, err)
	require.Equal(t, []time.Duration{31 * time.Minute}, waits)
}

func TestProcessOutputProcessWait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		responses []workspacesdk.ProcessOutputResponse
		wantWait  time.Duration
		wantCalls int
	}{
		{
			name:      "Requested",
			input:     `{"process_id":"proc-1","wait_timeout":"31m"}`,
			responses: []workspacesdk.ProcessOutputResponse{{}},
			wantWait:  31 * time.Minute,
			wantCalls: 1,
		},
		{
			name:      "Default",
			input:     `{"process_id":"proc-1"}`,
			responses: []workspacesdk.ProcessOutputResponse{{}},
			wantWait:  defaultProcessOutputTimeout,
			wantCalls: 1,
		},
		{
			name:      "Zero",
			input:     `{"process_id":"proc-1","wait_timeout":"0s"}`,
			responses: []workspacesdk.ProcessOutputResponse{{Running: true}},
		},
		{
			name:  "Invalid",
			input: `{"process_id":"proc-1","wait_timeout":"invalid"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			for _, response := range tt.responses {
				mockConn.EXPECT().
					ProcessOutput(gomock.Any(), "proc-1", gomock.Any()).
					Return(response, nil)
			}

			var waits []time.Duration
			ctx := WithProcessWait(
				testutil.Context(t, testutil.WaitMedium),
				func(wait time.Duration) { waits = append(waits, wait) },
			)
			tool := ProcessOutput(ProcessToolOptions{
				GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
					return mockConn, nil
				},
			})
			_, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "process_output",
				Input: tt.input,
			})
			require.NoError(t, err)
			require.Len(t, waits, tt.wantCalls)
			if tt.wantCalls > 0 {
				require.Equal(t, tt.wantWait, waits[0])
			}
		})
	}
}

func TestTruncateOutput(t *testing.T) {
	t.Parallel()

	t.Run("EmptyOutput", func(t *testing.T) {
		t.Parallel()
		result := runForegroundWithOutput(t, "")
		assert.Empty(t, result.Output)
	})

	t.Run("ShortOutput", func(t *testing.T) {
		t.Parallel()
		result := runForegroundWithOutput(t, "short")
		assert.Equal(t, "short", result.Output)
	})

	t.Run("ExactlyAtLimit", func(t *testing.T) {
		t.Parallel()
		output := strings.Repeat("a", maxOutputToModel)
		result := runForegroundWithOutput(t, output)
		assert.Equal(t, maxOutputToModel, len(result.Output))
		assert.Equal(t, output, result.Output)
	})

	t.Run("OverLimit", func(t *testing.T) {
		t.Parallel()
		output := strings.Repeat("b", maxOutputToModel+1024)
		result := runForegroundWithOutput(t, output)
		assert.Equal(t, maxOutputToModel, len(result.Output))
	})

	t.Run("MultiByteCutMidCharacter", func(t *testing.T) {
		t.Parallel()
		// Build output that places a 3-byte UTF-8 character
		// (U+2603, snowman ☃) right at the truncation boundary
		// so the cut falls mid-character.
		padding := strings.Repeat("x", maxOutputToModel-1)
		output := padding + "☃" // ☃ is 3 bytes, only 1 byte fits
		result := runForegroundWithOutput(t, output)
		assert.LessOrEqual(t, len(result.Output), maxOutputToModel)
		assert.True(t, utf8.ValidString(result.Output),
			"truncated output must be valid UTF-8")
	})
}

// runForegroundWithOutput runs a foreground command through the
// Execute tool with a mock that returns the given output, and
// returns the parsed result.
func runForegroundWithOutput(t *testing.T, output string) ExecuteResult {
	t.Helper()
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
			Output:   output,
		}, nil)

	tool := Execute(ExecuteOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
	ctx := testutil.Context(t, testutil.WaitMedium)
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "execute",
		Input: `{"command":"echo test"}`,
	})
	require.NoError(t, err)

	var result ExecuteResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}
