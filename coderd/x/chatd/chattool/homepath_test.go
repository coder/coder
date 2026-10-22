package chattool_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

// The agent's file endpoints accept only absolute paths, while chat
// prompts name workspace attachments home-relative.
func TestWorkspaceFileToolsResolveHomePaths(t *testing.T) {
	t.Parallel()

	const home = "/home/coder"
	expectHome := func(mockConn *agentconnmock.MockAgentConn) {
		mockConn.EXPECT().
			LS(gomock.Any(), "", workspacesdk.LSRequest{Path: []string{}, Relativity: workspacesdk.LSRelativityHome}).
			Return(workspacesdk.LSResponse{AbsolutePathString: home}, nil)
	}
	run := func(t *testing.T, tool fantasy.AgentTool, input string) fantasy.ToolResponse {
		t.Helper()
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{ID: "call-1", Name: tool.Info().Name, Input: input})
		require.NoError(t, err)
		require.False(t, resp.IsError, resp.Content)
		return resp
	}

	t.Run("ReadFile", func(t *testing.T) {
		t.Parallel()
		mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectHome(mockConn)
		mockConn.EXPECT().
			ReadFileLines(gomock.Any(), home+"/.coder/chats/c/files/f/report.zip", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, Content: "1\tPK"}, nil)
		tool := chattool.ReadFile(chattool.ReadFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
		})
		run(t, tool, `{"path":"~/.coder/chats/c/files/f/report.zip"}`)
	})

	t.Run("ReadFileHomeItself", func(t *testing.T) {
		t.Parallel()
		mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectHome(mockConn)
		mockConn.EXPECT().
			ReadFileLines(gomock.Any(), home, int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true}, nil)
		tool := chattool.ReadFile(chattool.ReadFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
		})
		run(t, tool, `{"path":"~"}`)
	})

	t.Run("ReadFileLeavesOtherPathsAlone", func(t *testing.T) {
		t.Parallel()
		// No LS expectation: paths that do not start with ~/ never ask the
		// agent for its home directory.
		mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		for _, p := range []string{"/tmp/~/a.txt", "~other/a.txt"} {
			mockConn.EXPECT().
				ReadFileLines(gomock.Any(), p, int64(1), int64(0), gomock.Any()).
				Return(workspacesdk.ReadFileLinesResponse{Success: true}, nil)
		}
		tool := chattool.ReadFile(chattool.ReadFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
		})
		run(t, tool, `{"path":"/tmp/~/a.txt"}`)
		run(t, tool, `{"path":"~other/a.txt"}`)
	})

	t.Run("WriteFile", func(t *testing.T) {
		t.Parallel()
		mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectHome(mockConn)
		mockConn.EXPECT().
			WriteFile(gomock.Any(), home+"/notes.txt", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, r io.Reader) error {
				data, err := io.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, "hello", string(data))
				return nil
			})
		tool := chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
		})
		run(t, tool, `{"path":"~/notes.txt","content":"hello"}`)
	})

	t.Run("EditFiles", func(t *testing.T) {
		t.Parallel()
		mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		// One home lookup serves every home-relative path in the batch.
		expectHome(mockConn)
		mockConn.EXPECT().
			EditFiles(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req workspacesdk.FileEditRequest) (workspacesdk.FileEditResponse, error) {
				paths := make([]string, 0, len(req.Files))
				for _, f := range req.Files {
					paths = append(paths, f.Path)
				}
				require.Equal(t, []string{home + "/a.go", "/srv/b.go", home + "/c.go"}, paths)
				return workspacesdk.FileEditResponse{}, nil
			})
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
		})
		edit := `"edits":[{"old_text":"a","new_text":"b"}]`
		run(t, tool, `{"files":[{"path":"~/a.go",`+edit+`},{"path":"/srv/b.go",`+edit+`},{"path":"~/c.go",`+edit+`}]}`)
	})

	t.Run("AttachFile", func(t *testing.T) {
		t.Parallel()
		mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectHome(mockConn)
		mockConn.EXPECT().
			ReadFile(gomock.Any(), home+"/build.log", int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader("ok\n")), "text/plain", nil)
		tool := chattool.AttachFile(chattool.AttachFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
			StoreFile: func(_ context.Context, name string, detectName string, _ []byte) (chattool.AttachmentMetadata, error) {
				require.Equal(t, "build.log", name)
				require.Equal(t, home+"/build.log", detectName)
				return chattool.AttachmentMetadata{FileID: uuid.New(), MediaType: "text/plain", Name: name}, nil
			},
		})
		run(t, tool, `{"path":"~/build.log"}`)
	})
}
