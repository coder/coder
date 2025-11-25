package cli

import (
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func (r *RootCmd) syncWant() *serpent.Command {
	return &serpent.Command{
		Use:   "want <unit> <depends-on>",
		Short: "Declare a dependency relationship",
		Long:  "Declare that a service unit depends on another unit completing before it can start. This builds the dependency graph that coordinates service startup order. The dependent unit will not be ready to start until the dependency reaches complete status.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			if len(i.Args) != 2 {
				return xerrors.New("exactly two arguments are required: unit and depends-on")
			}
			unit := i.Args[0]
			dependsOn := i.Args[1]

			fmt.Printf("Declaring dependency: '%s' depends on '%s'...\n", unit, dependsOn)

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			if err := client.SyncWant(ctx, unit, dependsOn); err != nil {
				return xerrors.Errorf("declare dependency failed: %w", err)
			}

			fmt.Printf("Dependency declared: '%s' now depends on '%s'\n", unit, dependsOn)

			return nil
		},
	}
}
