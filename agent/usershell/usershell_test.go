package usershell_test

import (
	"os/user"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/usershell"
)

//nolint:paralleltest,tparallel // This test sets an environment variable.
func TestGet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}

	t.Run("Fallback", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/sh")

		t.Run("NonExistentUser", func(t *testing.T) {
			shell, err := usershell.Get("notauser")
			require.NoError(t, err)
			require.Equal(t, "/bin/sh", shell)
		})
	})

	t.Run("NoFallback", func(t *testing.T) {
		// Disable env fallback for these tests.
		t.Setenv("SHELL", "")

		t.Run("NotFound", func(t *testing.T) {
			_, err := usershell.Get("notauser")
			require.Error(t, err)
		})

		t.Run("User", func(t *testing.T) {
			u, err := user.Current()
			require.NoError(t, err)
			shell, err := usershell.Get(u.Username)
			require.NoError(t, err)
			require.NotEmpty(t, shell)
		})
	})
}
