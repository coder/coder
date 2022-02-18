//go:build !windows && !darwin
// +build !windows,!darwin

package usershell_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/agent/usershell"
)

func TestGet(t *testing.T) {
	t.Parallel()
	t.Run("Has", func(t *testing.T) {
		t.Parallel()
		shell, err := usershell.Get("root")
		require.NoError(t, err)
		require.NotEmpty(t, shell)
	})
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		_, err := usershell.Get("notauser")
		require.Error(t, err)
	})
}
