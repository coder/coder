package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) aibridgeLogRequests() *serpent.Command {
	var (
		enable  bool
		disable bool
	)

	cmd := &serpent.Command{
		Use:   "log-requests",
		Short: "Toggle upstream request/response logging for AI Bridge providers",
		Long: cli.FormatExamples(
			cli.Example{
				Description: "Enable request logging (owner only)",
				Command:     "coder exp aibridge log-requests --enable",
			},
			cli.Example{
				Description: "Disable request logging (owner only)",
				Command:     "coder exp aibridge log-requests --disable",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			if enable == disable {
				return xerrors.New("must specify either --enable or --disable")
			}

			experimental := codersdk.NewExperimentalClient(client)
			err = experimental.AIBridgeSetRequestLogging(ctx, codersdk.AIBridgeSetRequestLoggingRequest{
				Enabled: enable,
			})
			if err != nil {
				return err
			}

			state := "disabled"
			if enable {
				state = "enabled"
			}

			_, err = fmt.Fprintf(inv.Stdout, "Request logging %s successfully.\n", cliui.Bold(state))
			return err
		},
		Options: serpent.OptionSet{
			{
				Flag:        "enable",
				Description: "Enable request logging.",
				Value:       serpent.BoolOf(&enable),
			},
			{
				Flag:        "disable",
				Description: "Disable request logging.",
				Value:       serpent.BoolOf(&disable),
			},
		},
	}

	return cmd
}
