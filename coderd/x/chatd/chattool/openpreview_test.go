package chattool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenPreview(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	agent := database.WorkspaceAgent{
		Name:        "main",
		DisplayApps: []database.DisplayApp{database.DisplayAppPortForwardingHelper},
	}

	for _, tt := range []struct {
		name      string
		input     string
		agent     *database.WorkspaceAgent
		path      string
		agentPath string
		agentFile bool
		openErr   error
		want      chattool.OpenPreviewResult
		wantError string
	}{
		{
			name:  "Port",
			input: `{"title":"  Signup form  ","port":3000,"url_path":" /?folder=/home/coder "}`,
			want:  chattool.OpenPreviewResult{Kind: chattool.OpenPreviewKindPort, Title: "Signup form", Port: 3000, PreviewSlug: "preview--3000", URLPath: "/?folder=/home/coder"},
		},
		{
			name:  "RootURLPath",
			input: `{"title":"Editor","port":9,"url_path":"/"}`,
			want:  chattool.OpenPreviewResult{Kind: chattool.OpenPreviewKindPort, Title: "Editor", Port: 9, PreviewSlug: "preview--9"},
		},
		{
			name:      "Path",
			input:     `{"title":"Docs site","path":" /home/coder/output/ ","url_path":"/guide/"}`,
			path:      "/home/coder/output/",
			agentPath: "/home/coder/output",
			want: chattool.OpenPreviewResult{
				Kind:        chattool.OpenPreviewKindFile,
				Title:       "Docs site",
				PreviewSlug: "preview--0123abcd",
				Path:        "/home/coder/output",
				URLPath:     "/guide/",
			},
		},
		{
			name:      "FileWithQuery",
			input:     `{"title":"Report","path":"/home/coder/report.html","url_path":"/?page=2"}`,
			path:      "/home/coder/report.html",
			agentPath: "/home/coder/report.html",
			agentFile: true,
			want: chattool.OpenPreviewResult{
				Kind:        chattool.OpenPreviewKindFile,
				Title:       "Report",
				PreviewSlug: "preview--0123abcd",
				Path:        "/home/coder/report.html",
				URLPath:     "/?page=2",
			},
		},
		{
			name:      "FileWithPath",
			input:     `{"title":"Report","path":"/home/coder/report.html","url_path":"/other.html"}`,
			path:      "/home/coder/report.html",
			agentPath: "/home/coder/report.html",
			agentFile: true,
			wantError: "a file preview serves only its file",
		},
		{
			name:  "AgentError",
			input: `{"title":"Docs site","path":"/home/coder/missing"}`,
			path:  "/home/coder/missing",
			openErr: func() error {
				err := codersdk.NewTestError(http.StatusBadRequest, "POST", "http://[fd7a::1]:4/api/v0/previews")
				err.Message = "Could not open preview."
				err.Detail = "stat /home/coder/missing: no such file or directory"
				return err
			}(),
			wantError: "Could not open preview.: stat /home/coder/missing: no such file or directory",
		},
		{
			name:      "OutdatedAgent",
			input:     `{"title":"Docs site","path":"/home/coder/output"}`,
			path:      "/home/coder/output",
			openErr:   workspacesdk.ErrPreviewsUnsupported,
			wantError: "the workspace agent is outdated",
		},
		{
			name:      "BlankTitle",
			input:     `{"title":" \t ","port":3000}`,
			wantError: "title is required",
		},
		{
			name:      "LongTitle",
			input:     `{"title":"` + strings.Repeat("é", 81) + `","port":3000}`,
			wantError: "at most 80 characters",
		},
		{
			// Invisible and direction-changing characters could disguise
			// the title where the user decides to open the preview.
			name:      "DisguisedTitle",
			input:     `{"title":"Docs\u202esite","port":3000}`,
			wantError: "must not contain control, format",
		},
		{
			name:      "ZeroWidthTitle",
			input:     `{"title":"Docs\u200bsite","port":3000}`,
			wantError: "must not contain control, format",
		},
		{
			name:      "TwoTargets",
			input:     `{"title":"App","port":3000,"path":"/home/coder/output"}`,
			wantError: "exactly one",
		},
		{
			name:      "NoTarget",
			input:     `{"title":"App"}`,
			wantError: "exactly one",
		},
		{
			name:      "ReservedPort",
			input:     `{"title":"App","port":8}`,
			wantError: "between 9 and 65535",
		},
		{
			name:      "PortForwardingDisabled",
			input:     `{"title":"App","port":3000}`,
			agent:     &database.WorkspaceAgent{Name: agent.Name},
			wantError: "does not allow port forwarding",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			if tt.path != "" {
				conn.EXPECT().OpenPreview(gomock.Any(), workspacesdk.OpenPreviewRequest{Path: tt.path}).
					Return(workspacesdk.OpenPreviewResponse{ID: "0123abcd", Path: tt.agentPath, File: tt.agentFile}, tt.openErr)
			}
			workspaceAgent := agent
			if tt.agent != nil {
				workspaceAgent = *tt.agent
			}
			tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
				PreviewsAvailable: true,
				GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
					return chattool.PreviewWorkspace{
						Conn:        conn,
						WorkspaceID: workspaceID,
						Agent:       workspaceAgent,
					}, nil
				},
			})

			response := runOpenPreview(t, tool, tt.input)
			if tt.wantError != "" {
				require.True(t, response.IsError)
				require.Contains(t, response.Content, tt.wantError)
				require.NotContains(t, response.Content, "http://")
				return
			}
			require.False(t, response.IsError, response.Content)

			want := tt.want
			want.WorkspaceID = workspaceID
			want.AgentName = agent.Name
			var result chattool.OpenPreviewResult
			require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
			require.Equal(t, want, result)
		})
	}
}

func TestOpenPreviewURLPath(t *testing.T) {
	t.Parallel()

	tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
		PreviewsAvailable: true,
		GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
			return chattool.PreviewWorkspace{
				WorkspaceID: uuid.New(),
				Agent: database.WorkspaceAgent{
					Name:        "main",
					DisplayApps: []database.DisplayApp{database.DisplayAppPortForwardingHelper},
				},
			}, nil
		},
	})

	for urlPath, valid := range map[string]bool{
		"/docs/index.html":              true,
		"/?folder=/home/coder/coder":    true,
		"/" + strings.Repeat("a", 2047): true,
		"/" + strings.Repeat("a", 2048): false,
		"docs":                          false,
		// Browsers read a leading // or /\ as another host.
		"//evil.example":  false,
		"/\\evil.example": false,
		"/a#b":            false,
		"/a\u0007b":       false,
	} {
		input, err := json.Marshal(chattool.OpenPreviewArgs{Title: "App", Port: new(3000), URLPath: urlPath})
		require.NoError(t, err)
		response := runOpenPreview(t, tool, string(input))
		if valid {
			require.False(t, response.IsError, urlPath)
			continue
		}
		require.True(t, response.IsError, urlPath)
		require.Contains(t, response.Content, "url_path must start with a single /", urlPath)
	}
}

func TestOpenPreviewWithoutWildcard(t *testing.T) {
	t.Parallel()

	tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
		GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
			return chattool.PreviewWorkspace{}, xerrors.New("must not look up the workspace")
		},
	})
	response := runOpenPreview(t, tool, `{"title":"App","port":3000}`)
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "previews need a wildcard app hostname")
}

func runOpenPreview(t *testing.T, tool fantasy.AgentTool, input string) fantasy.ToolResponse {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	response, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    uuid.NewString(),
		Name:  "open_preview",
		Input: input,
	})
	require.NoError(t, err)
	return response
}
