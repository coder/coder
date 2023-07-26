package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) netcheck() *clibase.Cmd {
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:   "netcheck",
		Short: "Print network debug information for DERP and STUN",
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithTimeout(inv.Context(), 30*time.Second)
			defer cancel()

			connInfo, err := client.WorkspaceAgentConnectionInfoGeneric(ctx)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprint(inv.Stderr, "Gathering a network report. This may take a few seconds...\n\n")

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

			_, _ = inv.Stdout.Write([]byte("\n"))
			return nil
		},
	}

	cmd.Options = clibase.OptionSet{}
	return cmd
}
