package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenPreview(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		input        string
		path         string
		daemonID     string
		daemonPath   string
		daemonError  error
		resolveError error
		wantPort     int
		wantTitle    string
		wantPath     string
		wantURLPath  string
		wantError    string
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
			name:       "PublishDirectory",
			input:      `{"title":"Docs site","path":" /home/coder/output "}`,
			path:       "/home/coder/output",
			daemonID:   "fedcba9876543210",
			daemonPath: "/srv/output",
			wantTitle:  "Docs site",
			// The catalog shows the path the daemon resolved, not the input.
			wantPath: "/srv/output",
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
			name:      "MultilineTitle",
			input:     `{"title":"Docs\nsite","port":3000}`,
			wantError: "control or invisible",
		},
		{
			name:      "BidiTitle",
			input:     `{"title":"Docs\u202esite","port":3000}`,
			wantError: "control or invisible",
		},
		{
			name:      "NeitherTarget",
			input:     `{"title":"App"}`,
			wantError: "exactly one",
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
		{
			name:      "PortOverflow",
			input:     `{"title":"App","port":65536}`,
			wantError: "between 1000 and 65535",
		},
		{
			name:         "WorkspaceUnavailable",
			input:        `{"title":"App","port":3000}`,
			resolveError: xerrors.New("no workspace is attached"),
			wantError:    "no workspace is attached",
		},
		{
			name:        "PublishError",
			input:       `{"title":"Docs","path":"/home/coder/output"}`,
			path:        "/home/coder/output",
			daemonError: xerrors.New("directory requires index.html"),
			wantError:   "directory requires index.html",
		},
		{
			name:      "InvalidDaemonID",
			input:     `{"title":"Docs","path":"/home/coder/output"}`,
			path:      "/home/coder/output",
			daemonID:  "0123456789ABCDEF",
			wantError: "invalid preview ID",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			workspaceID := uuid.New()
			if tt.path != "" {
				conn.EXPECT().OpenPreview(gomock.Any(), workspacesdk.OpenPreviewRequest{Path: tt.path}).
					Return(workspacesdk.OpenPreviewResponse{ID: tt.daemonID, Path: tt.daemonPath}, tt.daemonError)
			}
			tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
				GetWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
					return chattool.PreviewWorkspace{
						Conn:        conn,
						WorkspaceID: workspaceID,
						AgentName:   "main",
					}, tt.resolveError
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
				want["preview_id"] = tt.daemonID
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
