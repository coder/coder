package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestReadFile(t *testing.T) {
	t.Parallel()

	t.Run("ReadsFileLines", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/main.go",
				int64(1),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success:    true,
				Content:    "1\tpackage main\n",
				FileSize:   13,
				TotalLines: 1,
				LinesRead:  1,
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/main.go"}`,
		})

		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result struct {
			Content    string `json:"content"`
			FileSize   int64  `json:"file_size"`
			TotalLines int    `json:"total_lines"`
			LinesRead  int    `json:"lines_read"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.Equal(t, "1\tpackage main\n", result.Content)
		assert.Equal(t, int64(13), result.FileSize)
		assert.Equal(t, 1, result.TotalLines)
		assert.Equal(t, 1, result.LinesRead)
	})

	t.Run("ListsDirectory", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/project",
				int64(1),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "not a file: /home/coder/project",
			}, nil)
		mockConn.EXPECT().
			LS(
				gomock.Any(),
				"/home/coder/project",
				workspacesdk.LSRequest{Relativity: workspacesdk.LSRelativityRoot},
			).
			Return(workspacesdk.LSResponse{
				AbsolutePath:       []string{"home", "coder", "project"},
				AbsolutePathString: "/home/coder/project",
				Contents: []workspacesdk.LSFile{
					{
						Name:               "src",
						AbsolutePathString: "/home/coder/project/src",
						IsDir:              true,
					},
					{
						Name:               "README.md",
						AbsolutePathString: "/home/coder/project/README.md",
					},
				},
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/project"}`,
		})

		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result struct {
			Content            string                `json:"content"`
			IsDirectory        bool                  `json:"is_directory"`
			AbsolutePath       []string              `json:"absolute_path"`
			AbsolutePathString string                `json:"absolute_path_string"`
			Entries            []workspacesdk.LSFile `json:"entries"`
			EntriesRead        int                   `json:"entries_read"`
			TotalEntries       int                   `json:"total_entries"`
			Truncated          bool                  `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.IsDirectory)
		assert.Equal(t, []string{"home", "coder", "project"}, result.AbsolutePath)
		assert.Equal(t, "/home/coder/project", result.AbsolutePathString)
		assert.Equal(t, 2, result.EntriesRead)
		assert.Equal(t, 2, result.TotalEntries)
		assert.False(t, result.Truncated)
		assert.Equal(t, []workspacesdk.LSFile{
			{
				Name:               "src",
				AbsolutePathString: "/home/coder/project/src",
				IsDir:              true,
			},
			{
				Name:               "README.md",
				AbsolutePathString: "/home/coder/project/README.md",
			},
		}, result.Entries)
		assert.Equal(t, "1\tsrc/\n2\tREADME.md\n", result.Content)
	})

	t.Run("PaginatesDirectoryListing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/project",
				int64(2),
				int64(1),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "not a file: /home/coder/project",
			}, nil)
		mockConn.EXPECT().
			LS(
				gomock.Any(),
				"/home/coder/project",
				workspacesdk.LSRequest{Relativity: workspacesdk.LSRelativityRoot},
			).
			Return(workspacesdk.LSResponse{
				AbsolutePath:       []string{"home", "coder", "project"},
				AbsolutePathString: "/home/coder/project",
				Contents: []workspacesdk.LSFile{
					{Name: "a.txt", AbsolutePathString: "/home/coder/project/a.txt"},
					{Name: "b.txt", AbsolutePathString: "/home/coder/project/b.txt"},
					{Name: "c.txt", AbsolutePathString: "/home/coder/project/c.txt"},
				},
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/project","offset":2,"limit":1}`,
		})

		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result struct {
			Content      string                `json:"content"`
			Entries      []workspacesdk.LSFile `json:"entries"`
			EntriesRead  int                   `json:"entries_read"`
			TotalEntries int                   `json:"total_entries"`
			Truncated    bool                  `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.Equal(t, "2\tb.txt\n", result.Content)
		assert.Equal(t, []workspacesdk.LSFile{{
			Name:               "b.txt",
			AbsolutePathString: "/home/coder/project/b.txt",
		}}, result.Entries)
		assert.Equal(t, 1, result.EntriesRead)
		assert.Equal(t, 3, result.TotalEntries)
		assert.True(t, result.Truncated)
	})

	t.Run("TruncatesLargeDirectoryListing", func(t *testing.T) {
		t.Parallel()

		entries := make([]workspacesdk.LSFile, 2000)
		for i := range entries {
			name := strings.Repeat("a", 128)
			entries[i] = workspacesdk.LSFile{
				Name:               name,
				AbsolutePathString: "/home/coder/project/" + name,
			}
		}

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/project",
				int64(1),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "not a file: /home/coder/project",
			}, nil)
		mockConn.EXPECT().
			LS(
				gomock.Any(),
				"/home/coder/project",
				workspacesdk.LSRequest{Relativity: workspacesdk.LSRelativityRoot},
			).
			Return(workspacesdk.LSResponse{
				AbsolutePath:       []string{"home", "coder", "project"},
				AbsolutePathString: "/home/coder/project",
				Contents:           entries,
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/project"}`,
		})

		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result struct {
			Content      string                `json:"content"`
			Entries      []workspacesdk.LSFile `json:"entries"`
			EntriesRead  int                   `json:"entries_read"`
			TotalEntries int                   `json:"total_entries"`
			Truncated    bool                  `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.True(t, result.Truncated)
		assert.Equal(t, 2000, result.TotalEntries)
		assert.Equal(t, result.EntriesRead, len(result.Entries))
		assert.LessOrEqual(t, len(result.Content), int(workspacesdk.DefaultMaxResponseBytes))
		assert.Less(t, result.EntriesRead, result.TotalEntries)
	})

	t.Run("ListsEmptyDirectory", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/empty",
				int64(1),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "not a file: /home/coder/empty",
			}, nil)
		mockConn.EXPECT().
			LS(
				gomock.Any(),
				"/home/coder/empty",
				workspacesdk.LSRequest{Relativity: workspacesdk.LSRelativityRoot},
			).
			Return(workspacesdk.LSResponse{
				AbsolutePath:       []string{"home", "coder", "empty"},
				AbsolutePathString: "/home/coder/empty",
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/empty"}`,
		})

		require.NoError(t, err)
		require.False(t, resp.IsError)
		var result struct {
			Content      string                `json:"content"`
			Entries      []workspacesdk.LSFile `json:"entries"`
			EntriesRead  int                   `json:"entries_read"`
			TotalEntries int                   `json:"total_entries"`
			Truncated    bool                  `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.Empty(t, result.Content)
		assert.Empty(t, result.Entries)
		assert.Equal(t, 0, result.EntriesRead)
		assert.Equal(t, 0, result.TotalEntries)
		assert.False(t, result.Truncated)
	})

	t.Run("RejectsDirectoryOffsetBeyondEntries", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/project",
				int64(5),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "not a file: /home/coder/project",
			}, nil)
		mockConn.EXPECT().
			LS(
				gomock.Any(),
				"/home/coder/project",
				workspacesdk.LSRequest{Relativity: workspacesdk.LSRelativityRoot},
			).
			Return(workspacesdk.LSResponse{
				AbsolutePath:       []string{"home", "coder", "project"},
				AbsolutePathString: "/home/coder/project",
				Contents: []workspacesdk.LSFile{
					{Name: "a.txt", AbsolutePathString: "/home/coder/project/a.txt"},
				},
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/project","offset":5}`,
		})

		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "offset 5 is beyond the directory length of 1 entries", resp.Content)
	})

	t.Run("DoesNotListOtherReadErrors", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/missing.txt",
				int64(1),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "file does not exist: /home/coder/missing.txt",
			}, nil)

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/missing.txt"}`,
		})

		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "file does not exist: /home/coder/missing.txt", resp.Content)
	})

	t.Run("ReportsDirectoryListingError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFileLines(
				gomock.Any(),
				"/home/coder/private",
				int64(1),
				int64(0),
				workspacesdk.DefaultReadFileLinesLimits(),
			).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: false,
				Error:   "not a file: /home/coder/private",
			}, nil)
		mockConn.EXPECT().
			LS(
				gomock.Any(),
				"/home/coder/private",
				workspacesdk.LSRequest{Relativity: workspacesdk.LSRelativityRoot},
			).
			Return(workspacesdk.LSResponse{}, xerrors.New("permission denied"))

		tool := newReadFileTool(mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: `{"path":"/home/coder/private"}`,
		})

		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "not a file: /home/coder/private; failed to list directory: permission denied", resp.Content)
	})
}

func newReadFileTool(mockConn *agentconnmock.MockAgentConn) fantasy.AgentTool {
	return chattool.ReadFile(chattool.ReadFileOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
}

func TestReadFileRequiresPath(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	tool := newReadFileTool(mockConn)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "read_file",
		Input: `{}`,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Equal(t, "path is required", strings.TrimSpace(resp.Content))
}
