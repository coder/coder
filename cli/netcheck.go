package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) netcheck() *serpent.Cmd {
	client := new(codersdk.Client)

	cmd := &serpent.Cmd{
		Use:   "netcheck",
		Short: "Print network debug information for DERP and STUN",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithTimeout(inv.Context(), 30*time.Second)
			defer cancel()

			connInfo, err := client.WorkspaceAgentConnectionInfoGeneric(ctx)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprint(inv.Stderr, "Gathering a network report. This may take a few seconds...\n\n")

			var report derphealth.Report
			report.Run(ctx, &derphealth.ReportOptions{
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

	cmd.Options = serpent.OptionSet{}
	return cmd
}
