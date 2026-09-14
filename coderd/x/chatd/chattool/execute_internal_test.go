package chattool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

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
