package workspacesecrets_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspacesecrets"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	user := func(name, env, file string, enabled bool) workspacesecrets.Secret {
		return workspacesecrets.Secret{ID: uuid.New(), Source: codersdk.WorkspaceSecretSourceUser, Name: name, EnvName: env, FilePath: file, Enabled: enabled}
	}
	build := func(name, env, file string) workspacesecrets.Secret {
		return workspacesecrets.Secret{ID: uuid.New(), Source: codersdk.WorkspaceSecretSourceBuild, Name: name, EnvName: env, FilePath: file, Enabled: true}
	}

	var (
		userBoth     = user("both", "TOKEN", "/etc/token", true)
		userFileOnly = user("file-only", "", "/etc/cert", true)
		userDisabled = user("disabled", "DISABLED", "", false)
		userPlain    = user("plain", "PLAIN", "", true)
		buildEnv     = build("build-env", "TOKEN", "")
		buildFile    = build("build-file", "", "/etc/cert")
		buildOnDis   = build("build-on-disabled", "DISABLED", "")
	)
	// Build secrets listed last still win, so input order does not matter.
	secrets := []workspacesecrets.Secret{userBoth, userFileOnly, userDisabled, userPlain, buildEnv, buildFile, buildOnDis}

	t.Run("FilePathAllowed", func(t *testing.T) {
		t.Parallel()
		got := workspacesecrets.Resolve(secrets, workspacesecrets.FilePathAllowed)
		require.Len(t, got, len(secrets))

		// Partial override: the env var is taken, the file is still delivered.
		require.Empty(t, got[0].DeliveredEnvName)
		require.Equal(t, uuid.NullUUID{UUID: buildEnv.ID, Valid: true}, got[0].EnvReplacedBy)
		require.Equal(t, "/etc/token", got[0].DeliveredFilePath)
		require.False(t, got[0].FileReplacedBy.Valid)
		require.True(t, got[0].Delivered())

		// Full override.
		require.False(t, got[1].Delivered())
		require.Equal(t, uuid.NullUUID{UUID: buildFile.ID, Valid: true}, got[1].FileReplacedBy)

		// Disabled secrets are neither delivered nor replaced.
		require.False(t, got[2].Delivered())
		require.False(t, got[2].EnvReplacedBy.Valid)

		require.Equal(t, "PLAIN", got[3].DeliveredEnvName)
		require.Equal(t, "TOKEN", got[4].DeliveredEnvName)
		require.Equal(t, "/etc/cert", got[5].DeliveredFilePath)
		require.Equal(t, "DISABLED", got[6].DeliveredEnvName)
	})

	t.Run("FilePathBlocked", func(t *testing.T) {
		t.Parallel()
		got := workspacesecrets.Resolve(secrets, workspacesecrets.FilePathBlocked)

		// No file is delivered, so no file target is reported as replaced.
		require.False(t, got[0].Delivered())
		require.False(t, got[1].Delivered())
		require.False(t, got[1].FileReplacedBy.Valid)
		require.False(t, got[5].Delivered())
	})
}
