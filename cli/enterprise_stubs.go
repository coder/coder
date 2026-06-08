package cli

import (
	"fmt"
	"io"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

// slimEnterpriseUnsupportedMsg writes a helpful, actionable error to w
// explaining that the requested command requires the full Coder build.
// It is used by stub commands registered in slim AGPL builds (see
// enterprise_stubs_slim.go) so that users running e.g. the homebrew-core
// formula get a discoverable upgrade path instead of "unknown command".
//
// The helper is kept out of any //go:build slim file so it can be unit
// tested without rebuilding the world with -tags slim.
func slimEnterpriseUnsupportedMsg(w io.Writer, cmd string) {
	_, _ = fmt.Fprintf(w, "%s is not available in this binary.\n", pretty.Sprint(cliui.DefaultStyles.Code, cmd))
	_, _ = fmt.Fprintln(w, "Your current Coder binary is the slim AGPL build, e.g. from homebrew-core,")
	_, _ = fmt.Fprintln(w, "which does not include enterprise CLI commands.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "To install the full Coder build:")
	_, _ = fmt.Fprintln(w, "  brew install coder/coder/coder")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Or download a release from:")
	_, _ = fmt.Fprintln(w, "  https://github.com/coder/coder/releases")
}
