package chattool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
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

func TestIdempotencyKeyFromContext(t *testing.T) {
	t.Parallel()

	key := func(assistantMessageID int64, toolCallID string) string {
		ctx := WithDispatchIdentity(context.Background(), uuid.New(), assistantMessageID)
		return idempotencyKeyFromContext(WithToolCallID(ctx, toolCallID))
	}

	t.Run("Format", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "42-toolu_abc", key(42, "toolu_abc"))
	})

	t.Run("ChatIsNotPartOfTheKey", func(t *testing.T) {
		t.Parallel()

		keyForChat := func(chatID uuid.UUID) string {
			ctx := WithDispatchIdentity(context.Background(), chatID, 42)
			return idempotencyKeyFromContext(WithToolCallID(ctx, "toolu_abc"))
		}
		require.Equal(t, keyForChat(uuid.New()), keyForChat(uuid.New()))
	})

	t.Run("Unidentified", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, idempotencyKeyFromContext(context.Background()))
		require.Empty(t, key(0, "toolu_abc"))
		require.Empty(t, key(42, ""))
	})

	// The message ID is decimal digits, so the first "-" always splits
	// the pair back apart even when the tool call ID contains one.
	t.Run("DistinctPairsCannotCollide", func(t *testing.T) {
		t.Parallel()

		seen := make(map[string]struct{})
		for _, tc := range []struct {
			id         int64
			toolCallID string
		}{
			{4, "2-x"},
			{42, "-x"},
			{42, "x"},
			{4, "2x"},
			{1, "1-1"},
			{11, "1"},
			{11, "-1"},
		} {
			got := key(tc.id, tc.toolCallID)
			require.NotContains(t, seen, got)
			seen[got] = struct{}{}
		}
	})

	// Tool call IDs are provider-supplied, so the key must survive
	// characters that need escaping in a URL or a shell.
	t.Run("UnusualToolCallIDs", func(t *testing.T) {
		t.Parallel()

		for _, toolCallID := range []string{"a/b", "a b", "a%b", "a?b#c", "..", "ünïcøde"} {
			require.Equal(t, "7-"+toolCallID, key(7, toolCallID))
		}
	})
}
