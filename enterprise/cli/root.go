package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
)

type RootCmd struct {
	cli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*clibase.Cmd {
	return []*clibase.Cmd{
		r.Server(nil),
		r.workspaceProxy(),
		r.features(),
		r.licenses(),
		r.groups(),
		r.provisionerDaemons(),
	}
}

func (r *RootCmd) EnterpriseSubcommands() []*clibase.Cmd {
	all := append(r.Core(), r.enterpriseOnly()...)
	return all
}

//nolint:unused
func slimUnsupported(w io.Writer, cmd string) {
	_, _ = fmt.Fprintf(w, "You are using a 'slim' build of Coder, which does not support the %s subcommand.\n", cliui.DefaultStyles.Code.Render(cmd))
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Please use a build of Coder from GitHub releases:")
	_, _ = fmt.Fprintln(w, "  https://github.com/coder/coder/releases")

	//nolint:revive
	os.Exit(1)
}
