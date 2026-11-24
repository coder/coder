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
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenPreview(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("6d1f1a4e-8c55-4a43-9df3-2b0f4c1e7a10")
	chatOwnerID := uuid.New()
	agent := database.WorkspaceAgent{
		Name:        "main",
		DisplayApps: []database.DisplayApp{database.DisplayAppPortForwardingHelper},
	}
	// With agent "main" and owner "testuser", a port 65000 preview of this
	// workspace has a 64-character hostname label.
	longWorkspaceName := "coder-agents-workspace-long-name"

	for _, tt := range []struct {
		name          string
		input         string
		agent         *database.WorkspaceAgent
		workspaceName string
		otherOwner    bool
		// path is the path the agent is asked to publish. Without it, any
		// publish fails the test.
		path      string
		agentPath string
		agentFile bool
		openErr   error
		// workspaceErr fails the workspace lookup.
		workspaceErr error
		want         chattool.OpenPreviewResult
		// wantJSON is the stored result, which the dashboard reads by key.
		wantJSON  string
		wantError string
	}{
		{
			name:     "Port",
			input:    `{"title":"  Signup form  ","port":3000,"url_path":" /?folder=/home/coder "}`,
			want:     chattool.OpenPreviewResult{Title: "Signup form", Port: 3000, PreviewSlug: "preview--3000", URLPath: "/?folder=/home/coder"},
			wantJSON: `{"title":"Signup form","workspace_id":"6d1f1a4e-8c55-4a43-9df3-2b0f4c1e7a10","agent_name":"main","port":3000,"preview_slug":"preview--3000","url_path":"/?folder=/home/coder"}`,
		},
		{
			name:  "RootURLPath",
			input: `{"title":"Editor","port":9,"url_path":"/"}`,
			want:  chattool.OpenPreviewResult{Title: "Editor", Port: 9, PreviewSlug: "preview--9"},
		},
		{
			name:      "Path",
			input:     `{"title":"Docs site","path":" /home/coder/output/ ","url_path":"/guide/"}`,
			path:      "/home/coder/output/",
			agentPath: "/home/coder/output",
			want: chattool.OpenPreviewResult{
				Title:       "Docs site",
				PreviewSlug: "preview--0123abcd",
				Path:        "/home/coder/output",
				URLPath:     "/guide/",
			},
			wantJSON: `{"title":"Docs site","workspace_id":"6d1f1a4e-8c55-4a43-9df3-2b0f4c1e7a10","agent_name":"main","preview_slug":"preview--0123abcd","path":"/home/coder/output","url_path":"/guide/"}`,
		},
		{
			name:      "FileWithQuery",
			input:     `{"title":"Report","path":"/home/coder/report.html","url_path":"/?page=2"}`,
			path:      "/home/coder/report.html",
			agentPath: "/home/coder/report.html",
			agentFile: true,
			want: chattool.OpenPreviewResult{
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
			// The url_path is checked before the agent publishes anything.
			name:      "FileBadURLPath",
			input:     `{"title":"Docs site","path":"/home/coder/output","url_path":"/a%2"}`,
			wantError: "url_path must use valid percent-encoding",
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
			name:         "WorkspaceUnavailable",
			input:        `{"title":"Docs site","path":"/home/coder/output"}`,
			workspaceErr: xerrors.New("workspace is stopped"),
			wantError:    "workspace is stopped",
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
			name:      "TitleWithNewline",
			input:     `{"title":"Docs\nsite","port":3000}`,
			wantError: "must not contain control, format",
		},
		{
			name:      "TitleWithLineSeparator",
			input:     `{"title":"Docs\u2028site","port":3000}`,
			wantError: "must not contain control, format",
		},
		{
			name:      "TitleWithParagraphSeparator",
			input:     `{"title":"Docs\u2029site","port":3000}`,
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
			name:      "PortOverRange",
			input:     `{"title":"App","port":65536}`,
			wantError: "between 9 and 65535",
		},
		{
			name:  "HighestPort",
			input: `{"title":"App","port":65535}`,
			want:  chattool.OpenPreviewResult{Title: "App", Port: 65535, PreviewSlug: "preview--65535"},
		},
		{
			name:          "PortForwardingDisabled",
			input:         `{"title":"App","port":65000}`,
			agent:         &database.WorkspaceAgent{Name: agent.Name},
			workspaceName: longWorkspaceName,
			wantError:     "hides the port forwarding app",
		},
		{
			name:       "OtherOwnerPort",
			input:      `{"title":"App","port":3000}`,
			otherOwner: true,
			wantError:  "Previews open only for the workspace owner, and this chat's workspace belongs to another user.",
		},
		{
			name:       "OtherOwnerFile",
			input:      `{"title":"Docs site","path":"/home/coder/output"}`,
			otherOwner: true,
			wantError:  "Previews open only for the workspace owner, and this chat's workspace belongs to another user.",
		},
		{
			name:          "LongPortLabel",
			input:         `{"title":"App","port":65000}`,
			workspaceName: longWorkspaceName,
			wantError:     "The preview address would be longer than DNS allows. Use a shorter workspace or agent name.",
		},
		{
			name:          "PortLabelAtLimit",
			input:         `{"title":"App","port":6500}`,
			workspaceName: longWorkspaceName,
			want:          chattool.OpenPreviewResult{Title: "App", Port: 6500, PreviewSlug: "preview--6500"},
		},
		{
			// A file slug is three characters longer than "preview--65000".
			name:          "LongFileLabel",
			input:         `{"title":"Docs site","path":"/home/coder/output"}`,
			workspaceName: longWorkspaceName[3:],
			wantError:     "The preview address would be longer than DNS allows.",
		},
		{
			name:          "FileLabelAtLimit",
			input:         `{"title":"Docs site","path":"/home/coder/output"}`,
			workspaceName: longWorkspaceName[4:],
			path:          "/home/coder/output",
			agentPath:     "/home/coder/output",
			want: chattool.OpenPreviewResult{
				Title:       "Docs site",
				PreviewSlug: "preview--0123abcd",
				Path:        "/home/coder/output",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			if tt.path != "" {
				conn.EXPECT().OpenPreview(gomock.Any(), workspacesdk.OpenPreviewRequest{Path: tt.path}).
					Return(workspacesdk.OpenPreviewResponse{ID: "0123abcd", Path: tt.agentPath, File: tt.agentFile}, tt.openErr)
			}
			workspace := chattool.PreviewWorkspace{
				Conn:          conn,
				WorkspaceID:   workspaceID,
				WorkspaceName: "dev",
				OwnerID:       chatOwnerID,
				OwnerUsername: "testuser",
				Agent:         agent,
			}
			if tt.agent != nil {
				workspace.Agent = *tt.agent
			}
			if tt.workspaceName != "" {
				workspace.WorkspaceName = tt.workspaceName
			}
			if tt.otherOwner {
				workspace.OwnerID = uuid.New()
			}
			tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
				ChatOwnerID: chatOwnerID,
				GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
					return workspace, tt.workspaceErr
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
			if tt.wantJSON != "" {
				require.JSONEq(t, tt.wantJSON, response.Content)
			}
		})
	}
}

func TestOpenPreviewURLPath(t *testing.T) {
	t.Parallel()

	tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
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

	const shape = "url_path must start with a single /"
	for urlPath, wantErr := range map[string]string{
		"/docs/index.html":              "",
		"/?folder=/home/coder/coder":    "",
		"/" + strings.Repeat("a", 2047): "",
		"/" + strings.Repeat("a", 2048): shape,
		"docs":                          shape,
		// Browsers read a leading // or /\ as another host.
		"//evil.example":  shape,
		"/\\evil.example": shape,
		"/a#b":            shape,
		"/a\u0007b":       shape,
		// Sign-in parses the path, and the preview host reads these
		// parameters itself.
		"/%zz":                      "url_path must use valid percent-encoding",
		"/?coder_preview_sign_in=x": "coder_preview_sign_in",
		"/?page=2&coder_application_connect_api_key_35e783=x": "coder_application_connect_api_key_35e783",
	} {
		input, err := json.Marshal(chattool.OpenPreviewArgs{Title: "App", Port: new(3000), URLPath: urlPath})
		require.NoError(t, err)
		response := runOpenPreview(t, tool, string(input))
		if wantErr == "" {
			require.False(t, response.IsError, urlPath)
			continue
		}
		require.True(t, response.IsError, urlPath)
		require.Contains(t, response.Content, wantErr, urlPath)
	}
}

func TestOpenPreviewUnavailable(t *testing.T) {
	t.Parallel()

	for reason, want := range map[error]string{
		xerrors.New("the deployment has no wildcard app hostname"): "previews need a wildcard app hostname on the primary deployment; tell the user an administrator can set one",
		previewhost.ErrHostPrefixCookieOff:                         "previews require --host-prefix-cookie; tell the user an administrator can enable it",
	} {
		tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
			PreviewsUnavailable: reason,
			GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
				return chattool.PreviewWorkspace{}, xerrors.New("must not look up the workspace")
			},
		})
		response := runOpenPreview(t, tool, `{"title":"App","port":3000}`)
		require.True(t, response.IsError, reason)
		require.Equal(t, want, response.Content)
	}
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
