package agentapi

import (
	"database/sql"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspacesecrets"
)

func Test_dbSecretsToProto(t *testing.T) {
	t.Parallel()

	secrets := []database.UserSecret{
		{Name: "env-only", EnvName: "ENV_ONLY", Value: "env-val", Enabled: true},
		{Name: "file-only", FilePath: "~/.ssh/id_rsa", Value: "file-val", Enabled: true},
		{Name: "dual", EnvName: "DUAL_ENV", FilePath: "/etc/dual", Value: "dual-val", Enabled: true},
		{Name: "disabled", EnvName: "DISABLED_ENV", FilePath: "/etc/disabled", Value: "disabled-val"},
		// ws-shared takes this secret's env var; its file is still written.
		{Name: "overridden", EnvName: "SHARED_ENV", FilePath: "/etc/overridden", Value: "user-val", Enabled: true},
	}
	workspaceSecrets := []database.WorkspaceSecret{
		{Name: "ws-env", EnvName: "WS_ENV", Value: sql.NullString{String: "ws-val", Valid: true}},
		{Name: "ws-file", FilePath: "/etc/ws", Value: sql.NullString{String: "ws-file-val", Valid: true}},
		{Name: "ws-cleared", EnvName: "WS_CLEARED"},
		{Name: "ws-shared", EnvName: "SHARED_ENV", Value: sql.NullString{String: "ws-shared-val", Valid: true}},
	}

	cases := []struct {
		name     string
		policy   workspacesecrets.FilePathPolicy
		expected []*agentproto.WorkspaceSecret
	}{
		{
			name:   "FilePathAllowed",
			policy: workspacesecrets.FilePathAllowed,
			expected: []*agentproto.WorkspaceSecret{
				{EnvName: "ENV_ONLY", Value: []byte("env-val")},
				{FilePath: "~/.ssh/id_rsa", Value: []byte("file-val")},
				{EnvName: "DUAL_ENV", FilePath: "/etc/dual", Value: []byte("dual-val")},
				{FilePath: "/etc/overridden", Value: []byte("user-val")},
				{EnvName: "WS_ENV", Value: []byte("ws-val")},
				{FilePath: "/etc/ws", Value: []byte("ws-file-val")},
				{EnvName: "SHARED_ENV", Value: []byte("ws-shared-val")},
			},
		},
		{
			name:   "FilePathBlocked",
			policy: workspacesecrets.FilePathBlocked,
			expected: []*agentproto.WorkspaceSecret{
				{EnvName: "ENV_ONLY", Value: []byte("env-val")},
				{EnvName: "DUAL_ENV", Value: []byte("dual-val")},
				{EnvName: "WS_ENV", Value: []byte("ws-val")},
				{EnvName: "SHARED_ENV", Value: []byte("ws-shared-val")},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := dbSecretsToProto(secrets, workspaceSecrets, c.policy)
			require.Len(t, got, len(c.expected))
			for i, want := range c.expected {
				require.Equal(t, want.EnvName, got[i].EnvName, "secret %d env_name", i)
				require.Equal(t, want.FilePath, got[i].FilePath, "secret %d file_path", i)
				require.Equal(t, want.Value, got[i].Value, "secret %d value", i)
			}
		})
	}
}

func Test_vscodeProxyURI(t *testing.T) {
	t.Parallel()

	coderAccessURL, err := url.Parse("https://coder.com")
	require.NoError(t, err)

	accessURLWithPort, err := url.Parse("https://coder.com:8080")
	require.NoError(t, err)

	basicApp := appurl.ApplicationURL{
		Prefix:        "prefix",
		AppSlugOrPort: "slug",
		AgentName:     "agent",
		WorkspaceName: "workspace",
		Username:      "user",
	}

	cases := []struct {
		Name        string
		App         appurl.ApplicationURL
		AccessURL   *url.URL
		AppHostname string
		Expected    string
	}{
		{
			Name:        "NoHostname",
			AccessURL:   coderAccessURL,
			AppHostname: "",
			App:         basicApp,
			Expected:    "",
		},
		{
			Name:        "NoHostnameAccessURLPort",
			AccessURL:   accessURLWithPort,
			AppHostname: "",
			App:         basicApp,
			Expected:    "",
		},
		{
			Name:        "Hostname",
			AccessURL:   coderAccessURL,
			AppHostname: "*.apps.coder.com",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com", basicApp.String()),
		},
		{
			Name:        "HostnameWithAccessURLPort",
			AccessURL:   accessURLWithPort,
			AppHostname: "*.apps.coder.com",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com:%s", basicApp.String(), accessURLWithPort.Port()),
		},
		{
			Name:        "HostnameWithPort",
			AccessURL:   coderAccessURL,
			AppHostname: "*.apps.coder.com:4444",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com:%s", basicApp.String(), "4444"),
		},
		{
			// Port from hostname takes precedence over access url port.
			Name:        "HostnameWithPortAccessURLWithPort",
			AccessURL:   accessURLWithPort,
			AppHostname: "*.apps.coder.com:4444",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com:%s", basicApp.String(), "4444"),
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			require.NotNilf(t, c.AccessURL, "AccessURL is required")

			output := vscodeProxyURI(c.App, c.AccessURL, c.AppHostname)
			require.Equal(t, c.Expected, output)
		})
	}
}
