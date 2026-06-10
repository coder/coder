package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

// TestSlimUnsupported asserts the guidance printed by slim build stubs
// names the unsupported command, identifies the binary as a slim build,
// and points at both install paths: GitHub releases and the Homebrew tap.
// The returned error must exit non-zero without printing a redundant
// error message.
func TestSlimUnsupported(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := SlimUnsupported(&buf, "provisioner keys")
	require.ErrorIs(t, err, ErrSilent)

	got := buf.String()
	for _, want := range []string{
		pretty.Sprint(cliui.DefaultStyles.Code, "provisioner keys"),
		"'slim' build of Coder",
		"https://github.com/coder/coder/releases",
		"brew install coder/coder/coder",
	} {
		require.Contains(t, got, want, "full message:\n%s", got)
	}
}
