package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
)

func (*RootCmd) syncWant(socketPath *string) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "want <unit> <depends-on>",
		Short: "Declare that a unit depends on another unit completing before it can start",
		Long:  "Declare that a unit depends on another unit completing before it can start. The unit specified first will not start until the second has signaled that it has completed.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 2 {
				return xerrors.New("exactly two arguments are required: unit and depends-on")
			}
			dependentUnit := unit.ID(i.Args[0])
			dependsOn := unit.ID(i.Args[1])

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			if err := client.SyncWant(ctx, dependentUnit, dependsOn); err != nil {
				return xerrors.Errorf("declare dependency failed: %w", err)
			}

			cliui.Info(i.Stdout, "Success")

			return nil
		},
	}

	return cmd
}
