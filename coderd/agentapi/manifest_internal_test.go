package agentapi

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

func Test_dbUserSecretsToProto(t *testing.T) {
	t.Parallel()

	// One row per injection shape, reused by every case so the
	// expectations below differ only by policy.
	var (
		envOnly    = database.UserSecret{Name: "env-only", EnvName: "ENV_ONLY", Value: "env-val", Enabled: true}
		fileOnly   = database.UserSecret{Name: "file-only", FilePath: "~/.ssh/id_rsa", Value: "file-val", Enabled: true}
		dualTarget = database.UserSecret{Name: "dual", EnvName: "DUAL_ENV", FilePath: "/etc/dual", Value: "dual-val", Enabled: true}
		// A disabled row is never injected regardless of policy. Its
		// file target proves the policy is not what filters it out.
		disabled = database.UserSecret{Name: "disabled", EnvName: "DISABLED_ENV", FilePath: "/etc/disabled", Value: "disabled-val"}
	)

	secrets := []database.UserSecret{envOnly, fileOnly, dualTarget, disabled}

	cases := []struct {
		name                      string
		disableUserSecretFilePath bool
		expected                  []*agentproto.WorkspaceSecret
	}{
		{
			name:                      "PolicyOff",
			disableUserSecretFilePath: false,
			expected: []*agentproto.WorkspaceSecret{
				{EnvName: "ENV_ONLY", FilePath: "", Value: []byte("env-val")},
				{EnvName: "", FilePath: "~/.ssh/id_rsa", Value: []byte("file-val")},
				{EnvName: "DUAL_ENV", FilePath: "/etc/dual", Value: []byte("dual-val")},
			},
		},
		{
			name:                      "PolicyOn",
			disableUserSecretFilePath: true,
			expected: []*agentproto.WorkspaceSecret{
				// Env-only is untouched by the policy.
				{EnvName: "ENV_ONLY", FilePath: "", Value: []byte("env-val")},
				// The dual-target secret keeps its env injection and
				// loses only the file target.
				{EnvName: "DUAL_ENV", FilePath: "", Value: []byte("dual-val")},
				// The file-only secret is dropped entirely so its
				// plaintext value is never transmitted.
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			policy := userSecretFilePathAllowed
			if c.disableUserSecretFilePath {
				policy = userSecretFilePathBlocked
			}
			got := dbUserSecretsToProto(secrets, policy)
			require.Len(t, got, len(c.expected))
			for i, want := range c.expected {
				require.Equal(t, want.EnvName, got[i].EnvName, "secret %d env_name", i)
				require.Equal(t, want.FilePath, got[i].FilePath, "secret %d file_path", i)
				require.Equal(t, want.Value, got[i].Value, "secret %d value", i)
			}
		})
	}

	t.Run("PolicyOnDoesNotTransmitFileOnlyValues", func(t *testing.T) {
		t.Parallel()

		// Guards the plaintext contract directly: no part of a file-only
		// secret may reach the manifest under the policy.
		for _, secret := range dbUserSecretsToProto(secrets, userSecretFilePathBlocked) {
			require.NotEqual(t, []byte("file-val"), secret.Value)
			require.Empty(t, secret.FilePath)
		}
	})

	t.Run("PolicyOffRestoresStoredTargets", func(t *testing.T) {
		t.Parallel()

		// The policy only filters the outbound manifest, so turning it
		// back off yields the stored targets again on the next fetch
		// without any change to the rows.
		before := dbUserSecretsToProto(secrets, userSecretFilePathAllowed)
		_ = dbUserSecretsToProto(secrets, userSecretFilePathBlocked)
		after := dbUserSecretsToProto(secrets, userSecretFilePathAllowed)

		require.Equal(t, before, after)
		require.Len(t, after, 3)
		require.Equal(t, "~/.ssh/id_rsa", after[1].FilePath)
		require.Equal(t, "/etc/dual", after[2].FilePath)
	})
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
