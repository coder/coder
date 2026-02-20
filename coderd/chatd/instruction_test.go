package chatd

import (
	"context"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestSanitizeInstructionMarkdown(t *testing.T) {
	t.Parallel()

	input := "line 1\r\n<!-- hidden -->\r\nline 2\r\n"
	require.Equal(t, "line 1\n\nline 2", sanitizeInstructionMarkdown(input))
}

func TestReadHomeInstructionFileNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).DoAndReturn(
		func(context.Context, string, workspacesdk.LSRequest) (workspacesdk.LSResponse, error) {
			return workspacesdk.LSResponse{}, codersdk.NewTestError(404, "POST", "/api/v0/list-directory")
		},
	)

	content, sourcePath, truncated, err := readHomeInstructionFile(context.Background(), conn)
	require.NoError(t, err)
	require.Empty(t, content)
	require.Empty(t, sourcePath)
	require.False(t, truncated)
}

func TestReadHomeInstructionFileSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)

	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).DoAndReturn(
		func(context.Context, string, workspacesdk.LSRequest) (workspacesdk.LSResponse, error) {
			return workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{{
					Name:               "AGENTS.md",
					AbsolutePathString: "/home/coder/.coder/AGENTS.md",
				}},
			}, nil
		},
	)
	conn.EXPECT().ReadFile(
		gomock.Any(),
		"/home/coder/.coder/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		io.NopCloser(strings.NewReader("base\n<!-- hidden -->\nlocal")),
		"text/markdown",
		nil,
	)

	content, sourcePath, truncated, err := readHomeInstructionFile(context.Background(), conn)
	require.NoError(t, err)
	require.Equal(t, "base\n\nlocal", content)
	require.Equal(t, "/home/coder/.coder/AGENTS.md", sourcePath)
	require.False(t, truncated)
}

func TestReadHomeInstructionFileTruncates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	content := strings.Repeat("a", maxInstructionFileBytes+8)

	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
		workspacesdk.LSResponse{
			Contents: []workspacesdk.LSFile{{
				Name:               "AGENTS.md",
				AbsolutePathString: "/home/coder/.coder/AGENTS.md",
			}},
		},
		nil,
	)
	conn.EXPECT().ReadFile(
		gomock.Any(),
		"/home/coder/.coder/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(io.NopCloser(strings.NewReader(content)), "text/markdown", nil)

	got, _, truncated, err := readHomeInstructionFile(context.Background(), conn)
	require.NoError(t, err)
	require.True(t, truncated)
	require.Len(t, got, maxInstructionFileBytes)
}

func TestInsertSystemInstructionAfterSystemMessages(t *testing.T) {
	t.Parallel()

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "base"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "hello"},
			},
		},
	}

	got := insertSystemInstruction(prompt, "project rules")
	require.Len(t, got, 3)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleSystem, got[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[2].Role)

	part, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "project rules", part.Text)
}
