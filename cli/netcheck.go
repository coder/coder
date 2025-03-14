package cli

import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) netcheck() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{

		Use:   "netcheck",
		Short: "Print network debug information for DERP and STUN",
		Middleware: serpent.Chain(

			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithTimeout(inv.Context(), 30*time.Second)
			defer cancel()
			connInfo, err := workspacesdk.New(client).AgentConnectionInfoGeneric(ctx)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(inv.Stderr, "Gathering a network report. This may take a few seconds...\n\n")

			var derpReport derphealth.Report
			derpReport.Run(ctx, &derphealth.ReportOptions{
				DERPMap: connInfo.DERPMap,
			})
			ifReport, err := healthsdk.RunInterfacesReport()

			if err != nil {
				return fmt.Errorf("failed to run interfaces report: %w", err)

			}
			report := healthsdk.ClientNetcheckReport{
				DERP:       healthsdk.DERPHealthReport(derpReport),
				Interfaces: ifReport,
			}

			raw, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			n, err := inv.Stdout.Write(raw)

			if err != nil {
				return err
			}
			if n != len(raw) {
				return fmt.Errorf("failed to write all bytes to stdout; wrote %d, len %d", n, len(raw))

			}
			_, _ = inv.Stdout.Write([]byte("\n"))
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{}
	return cmd
}
