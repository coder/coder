package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSlimEnterpriseUnsupportedMsg locks in the helpful error text used by
// the slim AGPL stubs. Mirrors Kayla's complaint that "the version of
// coder in homebrew-core doesn't support issuing provisioner keys?? that
// was kind of annoying to discover": the message must name the command,
// say it is a slim AGPL build (so users know it isn't a bug), and point
// at the upgrade path (the coder/coder/coder tap and the GitHub
// releases page).
func TestSlimEnterpriseUnsupportedMsg(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	slimEnterpriseUnsupportedMsg(&buf, "coder provisioner keys")

	got := buf.String()
	for _, want := range []string{
		"coder provisioner keys",
		"slim AGPL build",
		"homebrew-core",
		"brew install coder/coder/coder",
		"github.com/coder/coder/releases",
	} {
		require.True(t, strings.Contains(got, want),
			"message missing %q\nfull message:\n%s", want, got)
	}
}
