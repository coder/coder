package docgenenv_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

//nolint:paralleltest // Prepare mutates the process environment.
func TestPrepare(t *testing.T) {
	t.Setenv("CODER_ACCESS_URL", "https://example.com")
	t.Setenv("CLIDOCGEN_CACHE_DIRECTORY", "")
	t.Setenv("CLIDOCGEN_CONFIG_DIRECTORY", "")
	t.Setenv("TMPDIR", "")

	docgenenv.Prepare()

	_, ok := os.LookupEnv("CODER_ACCESS_URL")
	require.False(t, ok, "CODER_ prefixed variables should be cleared")
	require.Equal(t, "~/.cache", os.Getenv("CLIDOCGEN_CACHE_DIRECTORY"))
	require.Equal(t, "~/.config/coderv2", os.Getenv("CLIDOCGEN_CONFIG_DIRECTORY"))
	require.Equal(t, "/tmp", os.Getenv("TMPDIR"))
}
