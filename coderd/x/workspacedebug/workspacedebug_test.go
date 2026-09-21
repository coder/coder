package workspacedebug_test

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/workspacedebug"
)

func tarWith(t *testing.T, files [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, file := range files {
		name, content := file[0], file[1]
		require.NoError(t, w.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(content)),
		}))
		_, err := w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestReadTerraformFiles(t *testing.T) {
	t.Parallel()

	data := tarWith(t, [][2]string{
		{"main.tf", "resource \"a\" \"b\" {}"},
		{"vars.tfvars", "x = 1"},
		{"README.md", "ignored"},
		{".terraform/providers/x.tf", "hidden"},
		{"modules/net/main.tf", "module"},
	})

	files, truncated, err := workspacedebug.ReadTerraformFiles(data, 1<<20)
	require.NoError(t, err)
	require.False(t, truncated)
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	require.Equal(t, []string{"main.tf", "modules/net/main.tf", "vars.tfvars"}, paths)

	// A tight byte budget cuts the first file and drops the rest.
	files, truncated, err = workspacedebug.ReadTerraformFiles(data, 5)
	require.NoError(t, err)
	require.True(t, truncated)
	require.Len(t, files, 1)
	require.Len(t, files[0].Content, 5)
	require.True(t, files[0].Truncated)
}

func TestBundlePrompt(t *testing.T) {
	t.Parallel()

	buildID := uuid.New()
	versionID := uuid.New()
	b := workspacedebug.Bundle{
		Workspace:     database.Workspace{Name: "ws", ID: uuid.New()},
		OwnerUsername: "alice",
		Build: database.WorkspaceBuild{
			ID:                buildID,
			BuildNumber:       3,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: versionID,
		},
		Job: database.ProvisionerJob{
			JobStatus: database.ProvisionerJobStatusFailed,
			Error:     sql.NullString{String: "terraform apply: exit status 1\nsecond line", Valid: true},
		},
		Template:             database.Template{Name: "tmpl", ActiveVersionID: uuid.New()},
		TemplateVersion:      database.TemplateVersion{ID: versionID, Name: "v3"},
		ActiveVersionDiffers: true,
		Parameters: []database.WorkspaceBuildParameter{
			{Name: "image", Value: "ubuntu"},
			{Name: "registry_token", Value: "hunter2"},
		},
		BuildLogTail: []workspacedebug.LogLine{
			{Stage: "apply", Level: "error", Output: "Error: boom"},
		},
		BuildLogTotal:      400,
		SourceAccessDenied: true,
		RequesterRoles:     []string{"member"},
	}

	prompt := b.Prompt()
	require.Contains(t, prompt, "- image = ubuntu")
	require.Contains(t, prompt, "- registry_token = [redacted]")
	require.NotContains(t, prompt, "hunter2")
	require.Contains(t, prompt, "[apply] ERROR: Error: boom")
	require.Contains(t, prompt, "differs from the version this build used")
	require.Contains(t, prompt, "not permitted to read this template's source")
	require.Contains(t, prompt, "can_edit_template: false")
	require.Contains(t, prompt, "get_workspace_build_logs with build_id "+buildID.String())
	require.Contains(t, prompt, "No agents exist for this build")

	require.Equal(t, "terraform apply: exit status 1", b.FailureSummary())
}

func TestFailureSummaryAgent(t *testing.T) {
	t.Parallel()

	b := workspacedebug.Bundle{
		Job: database.ProvisionerJob{JobStatus: database.ProvisionerJobStatusSucceeded},
		Agents: []workspacedebug.AgentInfo{{
			Name:           "main",
			LifecycleState: database.WorkspaceAgentLifecycleStateStartTimeout,
		}},
	}
	require.Equal(t, `agent "main" startup script timed out`, b.FailureSummary())
}

func TestPromptIsBounded(t *testing.T) {
	t.Parallel()

	b := workspacedebug.Bundle{
		Job: database.ProvisionerJob{JobStatus: database.ProvisionerJobStatusFailed},
	}
	for i := 0; i < 2000; i++ {
		b.BuildLogTail = append(b.BuildLogTail, workspacedebug.LogLine{Output: strings.Repeat("x", 200)})
	}
	prompt := b.Prompt()
	require.LessOrEqual(t, len(prompt), workspacedebug.MaxPromptBytes+64)
	require.Contains(t, prompt, "(context truncated)")
}

func TestIsDebugChat(t *testing.T) {
	t.Parallel()

	require.True(t, workspacedebug.IsDebugChat(map[string]string{workspacedebug.LabelKind: "true"}))
	require.False(t, workspacedebug.IsDebugChat(map[string]string{workspacedebug.LabelKind: "yes"}))
	require.False(t, workspacedebug.IsDebugChat(nil))
}
