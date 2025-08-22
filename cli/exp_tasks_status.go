package cli

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) expTasksStatus() *serpent.Command {
	var (
		client = new(codersdk.Client)
		ec     = codersdk.NewExperimentalClient(client)
	)

	cmd := &serpent.Command{
		Aliases: []string{"get", "show"},
		Handler: func(inv *serpent.Invocation) error {
			// TODO: when tasks become their own domain object, replace with
			// GetTaskByIDOrName or its equivalent.
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			if ws.LatestBuild.HasAITask == nil || !*ws.LatestBuild.HasAITask {
				cliui.Errorf(inv.Stderr, "Workspace %q does not have an AI task", cliui.Code(ws.Name))
				return nil
			}
			wt, err := ec.TaskByID(inv.Context(), ws.ID)
			if err != nil {
				return err
			}

			var sb strings.Builder
			_, _ = sb.WriteString("[")
			_, _ = sb.WriteString(string(wt.Status))
			_, _ = sb.WriteString("[")
			_, _ = fmt.Fprint(inv.Stdout, sb.String())
			return nil
		},
		Long: "Show task status",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: []serpent.Option{},
		Use:     "status",
	}
	return cmd
}
