package cli

import (
	"context"
	"encoding/json"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) netcheck() *clibase.Cmd {
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:    "netcheck",
		Short:  "Print network debug information",
		Hidden: true,
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithTimeout(inv.Context(), 10*time.Second)
			defer cancel()

			connInfo, err := client.WorkspaceAgentConnectionInfo(ctx)
			if err != nil {
				return err
			}

			var report healthcheck.DERPReport
			report.Run(ctx, &healthcheck.DERPReportOptions{
				DERPMap: connInfo.DERPMap,
			})

			raw, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}

			n, err := inv.Stdout.Write(raw)
			if err != nil {
				return err
			}
			if n != len(raw) {
				return xerrors.Errorf("failed to write all bytes to stdout; wrote %d, len %d", n, len(raw))
			}

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{}
	return cmd
}
