package workspacesecrets_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspacesecrets"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	secret := func(env, file string, enabled bool) workspacesecrets.Secret {
		return workspacesecrets.Secret{ID: uuid.New(), EnvName: env, FilePath: file, Enabled: enabled}
	}

	var (
		userBoth     = secret("TOKEN", "/etc/token", true)
		userFileOnly = secret("", "~/a/cert", true)
		userDisabled = secret("DISABLED", "", false)
		userPlain    = secret("PLAIN", "", true)
		buildEnv     = secret("TOKEN", "", true)
		// Written differently from userFileOnly's path, but the same file.
		buildFile = secret("", "~/a//cert", true)
		// Enabled is ignored for build secrets.
		buildOnDis = secret("DISABLED", "", false)
	)
	user := []workspacesecrets.Secret{userBoth, userFileOnly, userDisabled, userPlain}
	build := []workspacesecrets.Secret{buildEnv, buildFile, buildOnDis}

	t.Run("FilePathAllowed", func(t *testing.T) {
		t.Parallel()
		gotUser, gotBuild := workspacesecrets.Resolve(user, build, workspacesecrets.FilePathAllowed)
		require.Len(t, gotUser, len(user))
		require.Len(t, gotBuild, len(build))

		// Partial override: the env var is taken, the file is still delivered.
		require.Empty(t, gotUser[0].DeliveredEnvName)
		require.Equal(t, uuid.NullUUID{UUID: buildEnv.ID, Valid: true}, gotUser[0].EnvReplacedBy)
		require.Equal(t, "/etc/token", gotUser[0].DeliveredFilePath)
		require.False(t, gotUser[0].FileReplacedBy.Valid)
		require.True(t, gotUser[0].Delivered())

		// Full override, matched on the cleaned path.
		require.False(t, gotUser[1].Delivered())
		require.Equal(t, uuid.NullUUID{UUID: buildFile.ID, Valid: true}, gotUser[1].FileReplacedBy)

		// Disabled user secrets are neither delivered nor replaced.
		require.False(t, gotUser[2].Delivered())
		require.False(t, gotUser[2].EnvReplacedBy.Valid)

		require.Equal(t, "PLAIN", gotUser[3].DeliveredEnvName)
		require.Equal(t, "TOKEN", gotBuild[0].DeliveredEnvName)
		require.Equal(t, "~/a//cert", gotBuild[1].DeliveredFilePath, "the path is delivered as written")
		require.Equal(t, "DISABLED", gotBuild[2].DeliveredEnvName)
	})

	t.Run("FilePathBlocked", func(t *testing.T) {
		t.Parallel()
		gotUser, gotBuild := workspacesecrets.Resolve(user, build, workspacesecrets.FilePathBlocked)

		// No file is delivered, so no file target is reported as replaced.
		require.False(t, gotUser[0].Delivered())
		require.False(t, gotUser[1].Delivered())
		require.False(t, gotUser[1].FileReplacedBy.Valid)
		require.False(t, gotBuild[1].Delivered())
	})
}
