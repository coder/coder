//go:build linux || darwin

package pulumi

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

// nolint: paralleltest
// This test execs the shared fake shell binary.
func TestVersionFromBinaryPath(t *testing.T) {
	binaryPath := fakeBinaryPath(t, "fake_pulumi.sh")

	got, err := versionFromBinaryPath(context.Background(), binaryPath)
	require.NoError(t, err)
	require.Equal(t, "3.100.0", got.String())
}

// nolint: paralleltest
// The test file uses Setenv in sibling tests, so keep this file serial.
func TestCheckMinVersion(t *testing.T) {
	tests := []struct {
		name    string
		version *version.Version
		wantErr string
	}{
		{
			name:    "TooOld",
			version: version.Must(version.NewVersion("2.99.0")),
			wantErr: `pulumi version "2.99.0" is too old. required >= "3.0.0"`,
		},
		{
			name:    "Exact",
			version: version.Must(version.NewVersion("3.0.0")),
		},
		{
			name:    "Newer",
			version: version.Must(version.NewVersion("3.100.0")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkMinVersion(tt.version)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

// nolint: paralleltest
// This test mutates process environment variables with Setenv.
func TestBasicEnv(t *testing.T) {
	t.Setenv("SAFE_ENV_KEEP", "keep")
	t.Setenv("CODER_SECRET_VALUE", "secret")

	workDir := t.TempDir()
	cachePath := t.TempDir()
	e := &executor{
		cachePath: cachePath,
		files:     tfpath.Layout(workDir),
	}

	env := envSliceToMap(t, e.basicEnv())
	backendURL := (&url.URL{Scheme: "file", Path: filepath.Join(workDir, ".pulumi-backend")}).String()

	require.Equal(t, "keep", env["SAFE_ENV_KEEP"])
	require.Equal(t, backendURL, env["PULUMI_BACKEND_URL"])
	require.Equal(t, cachePath, env["PULUMI_HOME"])
	require.Equal(t, "true", env["PULUMI_SKIP_UPDATE_CHECK"])
	require.Equal(t, "", env["PULUMI_CONFIG_PASSPHRASE"])
	require.Equal(t, backendURL, env["PULUMI_DIY_BACKEND_URL"])
	require.NotContains(t, env, "CODER_SECRET_VALUE")
	require.NotContains(t, env, unsafeEnvCanary)
}

// nolint: paralleltest
// This test mutates process environment variables with Setenv.
func TestSafeEnviron(t *testing.T) {
	t.Setenv("SAFE_ENV_KEEP", "keep")
	t.Setenv("CODER_SECRET_VALUE", "secret")

	env := envSliceToMap(t, safeEnviron())

	require.Equal(t, "keep", env["SAFE_ENV_KEEP"])
	require.NotContains(t, env, "CODER_SECRET_VALUE")
	require.NotContains(t, env, unsafeEnvCanary)
	for key := range env {
		require.False(t, strings.HasPrefix(key, "CODER_"))
	}
}

// nolint: paralleltest
// This test mutates process environment variables with Setenv.
func TestProvisionEnv(t *testing.T) {
	t.Setenv("CODER_SECRET_VALUE", "secret")
	t.Setenv("SAFE_ENV_KEEP", "keep")

	metadata := &proto.Metadata{
		CoderUrl:                      "https://coder.example.com",
		WorkspaceTransition:           proto.WorkspaceTransition_START,
		WorkspaceName:                 "workspace",
		WorkspaceOwner:                "owner",
		WorkspaceId:                   "workspace-id",
		WorkspaceOwnerId:              "owner-id",
		WorkspaceOwnerEmail:           "owner@example.com",
		TemplateName:                  "template",
		TemplateVersion:               "v1",
		WorkspaceOwnerOidcAccessToken: "oidc-token",
		WorkspaceOwnerSessionToken:    "session-token",
		TemplateId:                    "template-id",
		WorkspaceOwnerName:            "Owner Name",
		WorkspaceOwnerGroups:          []string{"group-a", "group-b"},
		WorkspaceOwnerSshPublicKey:    "ssh-public",
		WorkspaceOwnerSshPrivateKey:   "ssh-private",
		WorkspaceBuildId:              "build-id",
		WorkspaceOwnerLoginType:       "password",
		WorkspaceOwnerRbacRoles: []*proto.Role{
			{Name: "role-a", OrgId: "org-a"},
		},
		PrebuiltWorkspaceBuildStage: proto.PrebuiltWorkspaceBuildStage_CLAIM,
		RunningAgentAuthTokens: []*proto.RunningAgentAuthToken{
			{AgentId: "agent-a", Token: "token-a"},
			{AgentId: "agent-b", Token: "token-b"},
		},
		TaskId:     "task-id",
		TaskPrompt: "task prompt",
	}
	previousParams := []*proto.RichParameterValue{{Name: "before", Value: "old"}}
	richParams := []*proto.RichParameterValue{{Name: "after", Value: "new"}}
	externalAuth := []*proto.ExternalAuthProvider{{Id: "github", AccessToken: "gh-token"}}

	env, err := provisionEnv(&proto.Config{}, metadata, previousParams, richParams, externalAuth)
	require.NoError(t, err)

	envMap := envSliceToMap(t, env)
	require.Equal(t, "keep", envMap["SAFE_ENV_KEEP"])
	require.NotContains(t, envMap, "CODER_SECRET_VALUE")
	require.Equal(t, "https://coder.example.com", envMap["CODER_AGENT_URL"])
	require.Equal(t, "start", envMap["CODER_WORKSPACE_TRANSITION"])
	require.Equal(t, "workspace", envMap["CODER_WORKSPACE_NAME"])
	require.Equal(t, "owner", envMap["CODER_WORKSPACE_OWNER"])
	require.Equal(t, "owner@example.com", envMap["CODER_WORKSPACE_OWNER_EMAIL"])
	require.Equal(t, "Owner Name", envMap["CODER_WORKSPACE_OWNER_NAME"])
	require.Equal(t, "oidc-token", envMap["CODER_WORKSPACE_OWNER_OIDC_ACCESS_TOKEN"])
	require.Equal(t, `["group-a","group-b"]`, envMap["CODER_WORKSPACE_OWNER_GROUPS"])
	require.Equal(t, "ssh-public", envMap["CODER_WORKSPACE_OWNER_SSH_PUBLIC_KEY"])
	require.Equal(t, "ssh-private", envMap["CODER_WORKSPACE_OWNER_SSH_PRIVATE_KEY"])
	require.Equal(t, "password", envMap["CODER_WORKSPACE_OWNER_LOGIN_TYPE"])
	require.Equal(t, `[{"name":"role-a","org_id":"org-a"}]`, envMap["CODER_WORKSPACE_OWNER_RBAC_ROLES"])
	require.Equal(t, "workspace-id", envMap["CODER_WORKSPACE_ID"])
	require.Equal(t, "owner-id", envMap["CODER_WORKSPACE_OWNER_ID"])
	require.Equal(t, "session-token", envMap["CODER_WORKSPACE_OWNER_SESSION_TOKEN"])
	require.Equal(t, "template-id", envMap["CODER_WORKSPACE_TEMPLATE_ID"])
	require.Equal(t, "template", envMap["CODER_WORKSPACE_TEMPLATE_NAME"])
	require.Equal(t, "v1", envMap["CODER_WORKSPACE_TEMPLATE_VERSION"])
	require.Equal(t, "build-id", envMap["CODER_WORKSPACE_BUILD_ID"])
	require.Equal(t, "task-id", envMap["CODER_TASK_ID"])
	require.Equal(t, "task prompt", envMap["CODER_TASK_PROMPT"])
	require.NotContains(t, envMap, "CODER_WORKSPACE_IS_PREBUILD")
	require.Equal(t, "true", envMap["CODER_WORKSPACE_IS_PREBUILD_CLAIM"])
	require.Equal(t, "token-a", envMap["CODER_RUNNING_WORKSPACE_AGENT_TOKEN_agent-a"])
	require.Equal(t, "token-b", envMap["CODER_RUNNING_WORKSPACE_AGENT_TOKEN_agent-b"])
	require.Equal(t, "old", envMap["CODER_PARAMETER_PREVIOUS_before"])
	require.Equal(t, "new", envMap["CODER_PARAMETER_after"])
	require.Equal(t, "gh-token", envMap["CODER_GIT_AUTH_ACCESS_TOKEN_github"])
	require.Equal(t, "gh-token", envMap["CODER_EXTERNAL_AUTH_ACCESS_TOKEN_github"])

	scripts := provisionersdk.AgentScriptEnv()
	require.NotEmpty(t, scripts)
	for key, value := range scripts {
		require.Equal(t, value, envMap[key])
		break
	}
}

func fakeBinaryPath(t *testing.T, name string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, "testdata", name)
}

func envSliceToMap(t *testing.T, env []string) map[string]string {
	t.Helper()
	result := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, found := strings.Cut(entry, "=")
		require.True(t, found, "expected key=value env entry, got %q", entry)
		result[name] = value
	}
	return result
}
