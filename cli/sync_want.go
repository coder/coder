package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncWant(socketPath *string) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "want <unit> <depends-on> [depends-on...]",
		Short: "Declare that a unit depends on other units completing before it can start",
		Long:  "Declare that a unit depends on one or more other units completing before it can start. The unit specified first will not start until all subsequent units have signaled that they have completed.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) < 2 {
				return xerrors.New("at least two arguments are required: unit and one or more depends-on")
			}
			dependentUnit := unit.ID(i.Args[0])

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			for _, dep := range i.Args[1:] {
				if err := client.SyncWant(ctx, dependentUnit, unit.ID(dep)); err != nil {
					return xerrors.Errorf("declare dependency failed: %w", err)
				}
			}

			cliui.Info(i.Stdout, "Success")

			return nil
		},
	}

	return cmd
}
