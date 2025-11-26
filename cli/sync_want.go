package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
)

func (*RootCmd) syncWant() *serpent.Command {
	return &serpent.Command{
		Use:   "want <unit> <depends-on>",
		Short: "Declare that a unit depends on another unit completing before it can start",
		Long:  "Declare that a unit depends on another unit completing before it can start. The unit specified first will not start until the second has signaled that it has completed.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 2 {
				return xerrors.New("exactly two arguments are required: unit and depends-on")
			}
			unit := i.Args[0]
			dependsOn := i.Args[1]

			client, err := agentsocket.NewClient(ctx)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			if err := client.SyncWant(ctx, unit, dependsOn); err != nil {
				return xerrors.Errorf("declare dependency failed: %w", err)
			}

			cliui.Info(i.Stdout, "Success")

			return nil
		},
	}
}
