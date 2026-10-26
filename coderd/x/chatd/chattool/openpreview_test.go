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

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenPreview(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()

	for _, tt := range []struct {
		name        string
		input       string
		path        string
		agentPath   string
		wantPort    int
		wantTitle   string
		wantPath    string
		wantURLPath string
		openErr     error
		wantError   string
	}{
		{
			name:      "ExistingServer",
			input:     `{"title":"  Signup form  ","port":3000}`,
			wantPort:  3000,
			wantTitle: "Signup form",
		},
		{
			name:        "ExistingServerAtPath",
			input:       `{"title":"Editor","port":6006,"url_path":" /?folder=/home/coder/coder "}`,
			wantPort:    6006,
			wantTitle:   "Editor",
			wantURLPath: "/?folder=/home/coder/coder",
		},
		{
			name:      "RootURLPath",
			input:     `{"title":"Editor","port":6006,"url_path":"/"}`,
			wantPort:  6006,
			wantTitle: "Editor",
		},
		{
			name:      "URLPathWithoutPort",
			input:     `{"title":"Docs","path":"/home/coder/output","url_path":"/docs"}`,
			wantError: "url_path can only be used with port",
		},
		{
			// Browsers treat a leading // or /\ as another host.
			name:      "URLPathOtherHost",
			input:     `{"title":"Editor","port":6006,"url_path":"/\\evil.example"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathDoubleSlash",
			input:     `{"title":"Editor","port":6006,"url_path":"//evil.example"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathRelative",
			input:     `{"title":"Editor","port":6006,"url_path":"docs"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathFragment",
			input:     `{"title":"Editor","port":6006,"url_path":"/a#b"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathControlCharacter",
			input:     `{"title":"Editor","port":6006,"url_path":"/a\u0007b"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathTooLong",
			input:     `{"title":"Editor","port":6006,"url_path":"/` + strings.Repeat("a", 2048) + `"}`,
			wantError: "at most 2048 characters",
		},
		{
			name:      "PublishDirectory",
			input:     `{"title":"Docs site","path":" /home/coder/output/ "}`,
			path:      "/home/coder/output/",
			agentPath: "/srv/output",
			wantTitle: "Docs site",
			// The previews endpoint lists the path the agent resolved, not the
			// input.
			wantPath: "/srv/output",
		},
		{
			name:  "AgentError",
			input: `{"title":"Docs site","path":"/home/coder/missing"}`,
			path:  "/home/coder/missing",
			openErr: func() error {
				err := codersdk.NewTestError(http.StatusBadRequest, "POST", "http://[fd7a::1]:4/api/v0/previews")
				err.Message = "Could not open preview."
				err.Detail = "preview path does not exist or cannot be read"
				return err
			}(),
			wantError: "Could not open preview.: preview path does not exist or cannot be read",
		},
		{
			name:      "OutdatedAgent",
			input:     `{"title":"Docs site","path":"/home/coder/output"}`,
			path:      "/home/coder/output",
			openErr:   codersdk.NewTestError(http.StatusNotFound, "POST", "http://[fd7a::1]:4/api/v0/previews"),
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
			name:      "BothTargets",
			input:     `{"title":"App","port":3000,"path":"/home/coder/output"}`,
			wantError: "exactly one",
		},
		{
			name:      "ShortPortHostname",
			input:     `{"title":"App","port":999}`,
			wantError: "between 1000 and 65535",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			if tt.path != "" {
				conn.EXPECT().OpenPreview(gomock.Any(), workspacesdk.OpenPreviewRequest{Path: tt.path}).
					Return(workspacesdk.OpenPreviewResponse{ID: "00000001", Path: tt.agentPath}, tt.openErr)
			}
			tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
				GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
					return chattool.PreviewWorkspace{
						Conn:        conn,
						WorkspaceID: workspaceID,
						AgentName:   "main",
					}, nil
				},
			})

			ctx := testutil.Context(t, testutil.WaitShort)
			response, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    uuid.NewString(),
				Name:  "open_preview",
				Input: tt.input,
			})
			require.NoError(t, err)
			if tt.wantError != "" {
				require.True(t, response.IsError)
				require.Contains(t, response.Content, tt.wantError)
				require.NotContains(t, response.Content, "http://")
				return
			}
			require.False(t, response.IsError)

			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
			want := map[string]any{
				"ok":           true,
				"title":        tt.wantTitle,
				"workspace_id": workspaceID.String(),
				"agent_name":   "main",
			}
			if tt.wantPath != "" {
				want["path"] = tt.wantPath
				want["preview_id"] = "00000001"
			} else {
				want["port"] = float64(tt.wantPort)
			}
			if tt.wantURLPath != "" {
				want["url_path"] = tt.wantURLPath
			}
			require.Equal(t, want, result)
		})
	}
}
