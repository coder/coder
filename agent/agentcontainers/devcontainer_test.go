package agentcontainers_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/codersdk"
)

func TestExtractAndInitializeDevcontainerScripts(t *testing.T) {
	t.Parallel()

	scriptIDs := []uuid.UUID{uuid.New(), uuid.New()}
	devcontainerIDs := []uuid.UUID{uuid.New(), uuid.New()}

	type args struct {
		expandPath    func(string) (string, error)
		devcontainers []codersdk.WorkspaceAgentDevcontainer
		scripts       []codersdk.WorkspaceAgentScript
	}
	tests := []struct {
		name                    string
		args                    args
		wantFilteredScripts     []codersdk.WorkspaceAgentScript
		wantDevcontainerScripts []codersdk.WorkspaceAgentScript

		skipOnWindowsDueToPathSeparator bool
	}{
		{
			name: "no scripts",
			args: args{
				expandPath:    nil,
				devcontainers: nil,
				scripts:       nil,
			},
			wantFilteredScripts:     nil,
			wantDevcontainerScripts: nil,
		},
		{
			name: "no devcontainers",
			args: args{
				expandPath:    nil,
				devcontainers: nil,
				scripts: []codersdk.WorkspaceAgentScript{
					{ID: scriptIDs[0]},
					{ID: scriptIDs[1]},
				},
			},
			wantFilteredScripts: []codersdk.WorkspaceAgentScript{
				{ID: scriptIDs[0]},
				{ID: scriptIDs[1]},
			},
			wantDevcontainerScripts: nil,
		},
		{
			name: "no scripts match devcontainers",
			args: args{
				expandPath: nil,
				devcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{ID: devcontainerIDs[0]},
					{ID: devcontainerIDs[1]},
				},
				scripts: []codersdk.WorkspaceAgentScript{
					{ID: scriptIDs[0]},
					{ID: scriptIDs[1]},
				},
			},
			wantFilteredScripts: []codersdk.WorkspaceAgentScript{
				{ID: scriptIDs[0]},
				{ID: scriptIDs[1]},
			},
			wantDevcontainerScripts: nil,
		},
		{
			name: "scripts match devcontainers and sets RunOnStart=false",
			args: args{
				expandPath: nil,
				devcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{ID: devcontainerIDs[0], WorkspaceFolder: "workspace1"},
					{ID: devcontainerIDs[1], WorkspaceFolder: "workspace2"},
				},
				scripts: []codersdk.WorkspaceAgentScript{
					{ID: scriptIDs[0], RunOnStart: true},
					{ID: scriptIDs[1], RunOnStart: true},
					{ID: devcontainerIDs[0], RunOnStart: true},
					{ID: devcontainerIDs[1], RunOnStart: true},
				},
			},
			wantFilteredScripts: []codersdk.WorkspaceAgentScript{
				{ID: scriptIDs[0], RunOnStart: true},
				{ID: scriptIDs[1], RunOnStart: true},
			},
			wantDevcontainerScripts: []codersdk.WorkspaceAgentScript{
				{
					ID:         devcontainerIDs[0],
					Script:     "devcontainer up --log-format json --workspace-folder \"workspace1\"",
					RunOnStart: false,
				},
				{
					ID:         devcontainerIDs[1],
					Script:     "devcontainer up --log-format json --workspace-folder \"workspace2\"",
					RunOnStart: false,
				},
			},
		},
		{
			name: "scripts match devcontainers with config path",
			args: args{
				expandPath: nil,
				devcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{
						ID:              devcontainerIDs[0],
						WorkspaceFolder: "workspace1",
						ConfigPath:      "config1",
					},
					{
						ID:              devcontainerIDs[1],
						WorkspaceFolder: "workspace2",
						ConfigPath:      "config2",
					},
				},
				scripts: []codersdk.WorkspaceAgentScript{
					{ID: devcontainerIDs[0]},
					{ID: devcontainerIDs[1]},
				},
			},
			wantFilteredScripts: []codersdk.WorkspaceAgentScript{},
			wantDevcontainerScripts: []codersdk.WorkspaceAgentScript{
				{
					ID:         devcontainerIDs[0],
					Script:     "devcontainer up --log-format json --workspace-folder \"workspace1\" --config \"workspace1/config1\"",
					RunOnStart: false,
				},
				{
					ID:         devcontainerIDs[1],
					Script:     "devcontainer up --log-format json --workspace-folder \"workspace2\" --config \"workspace2/config2\"",
					RunOnStart: false,
				},
			},
			skipOnWindowsDueToPathSeparator: true,
		},
		{
			name: "scripts match devcontainers with expand path",
			args: args{
				expandPath: func(s string) (string, error) {
					return "/home/" + s, nil
				},
				devcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{
						ID:              devcontainerIDs[0],
						WorkspaceFolder: "workspace1",
						ConfigPath:      "config1",
					},
					{
						ID:              devcontainerIDs[1],
						WorkspaceFolder: "workspace2",
						ConfigPath:      "config2",
					},
				},
				scripts: []codersdk.WorkspaceAgentScript{
					{ID: devcontainerIDs[0], RunOnStart: true},
					{ID: devcontainerIDs[1], RunOnStart: true},
				},
			},
			wantFilteredScripts: []codersdk.WorkspaceAgentScript{},
			wantDevcontainerScripts: []codersdk.WorkspaceAgentScript{
				{
					ID:         devcontainerIDs[0],
					Script:     "devcontainer up --log-format json --workspace-folder \"/home/workspace1\" --config \"/home/workspace1/config1\"",
					RunOnStart: false,
				},
				{
					ID:         devcontainerIDs[1],
					Script:     "devcontainer up --log-format json --workspace-folder \"/home/workspace2\" --config \"/home/workspace2/config2\"",
					RunOnStart: false,
				},
			},
			skipOnWindowsDueToPathSeparator: true,
		},
		{
			name: "expand config path when ~",
			args: args{
				expandPath: func(s string) (string, error) {
					s = strings.Replace(s, "~/", "", 1)
					if filepath.IsAbs(s) {
						return s, nil
					}
					return "/home/" + s, nil
				},
				devcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{
						ID:              devcontainerIDs[0],
						WorkspaceFolder: "workspace1",
						ConfigPath:      "~/config1",
					},
					{
						ID:              devcontainerIDs[1],
						WorkspaceFolder: "workspace2",
						ConfigPath:      "/config2",
					},
				},
				scripts: []codersdk.WorkspaceAgentScript{
					{ID: devcontainerIDs[0], RunOnStart: true},
					{ID: devcontainerIDs[1], RunOnStart: true},
				},
			},
			wantFilteredScripts: []codersdk.WorkspaceAgentScript{},
			wantDevcontainerScripts: []codersdk.WorkspaceAgentScript{
				{
					ID:         devcontainerIDs[0],
					Script:     "devcontainer up --log-format json --workspace-folder \"/home/workspace1\" --config \"/home/config1\"",
					RunOnStart: false,
				},
				{
					ID:         devcontainerIDs[1],
					Script:     "devcontainer up --log-format json --workspace-folder \"/home/workspace2\" --config \"/config2\"",
					RunOnStart: false,
				},
			},
			skipOnWindowsDueToPathSeparator: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.skipOnWindowsDueToPathSeparator && filepath.Separator == '\\' {
				t.Skip("Skipping test on Windows due to path separator difference.")
			}

			logger := slogtest.Make(t, nil)
			if tt.args.expandPath == nil {
				tt.args.expandPath = func(s string) (string, error) {
					return s, nil
				}
			}
			gotFilteredScripts, gotDevcontainerScripts := agentcontainers.ExtractAndInitializeDevcontainerScripts(
				logger,
				tt.args.expandPath,
				tt.args.devcontainers,
				tt.args.scripts,
			)

			if diff := cmp.Diff(tt.wantFilteredScripts, gotFilteredScripts, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("ExtractAndInitializeDevcontainerScripts() gotFilteredScripts mismatch (-want +got):\n%s", diff)
			}

			// Preprocess the devcontainer scripts to remove scripting part.
			for i := range gotDevcontainerScripts {
				gotDevcontainerScripts[i].Script = textGrep("devcontainer up", gotDevcontainerScripts[i].Script)
				require.NotEmpty(t, gotDevcontainerScripts[i].Script, "devcontainer up --log-format json script not found")
			}
			if diff := cmp.Diff(tt.wantDevcontainerScripts, gotDevcontainerScripts); diff != "" {
				t.Errorf("ExtractAndInitializeDevcontainerScripts() gotDevcontainerScripts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// textGrep returns matching lines from multiline string.
func textGrep(want, got string) (filtered string) {
	var lines []string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, want) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
