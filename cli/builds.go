package cli

import (
	"fmt"
	"slices"
	"time"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) builds() *serpent.Command {
	var (
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat([]codersdk.WorkspaceBuild{}, []string{"build number", "template version name", "transition", "created at", "job completed at", "reason", "initiated by"}),
			cliui.JSONFormat(),
		)
		since time.Duration
	)
	cmd := &serpent.Command{
		Use:   "builds <workspace>",
		Short: "View builds for a workspace",
		Long:  "View builds for a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			if since == 0 {
				since = time.Hour * 24 * 7
			}
			builds, err := client.WorkspaceBuilds(inv.Context(), codersdk.WorkspaceBuildsRequest{
				WorkspaceID: ws.ID,
				Since:       time.Now().Add(-since),
			})
			if err != nil {
				return err
			}
			slices.SortStableFunc(builds, func(a, b codersdk.WorkspaceBuild) int {
				return a.CreatedAt.Compare(b.CreatedAt)
			})
			out, err := formatter.Format(inv.Context(), builds)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
		Options: serpent.OptionSet{
			{
				Name:        "since",
				Flag:        "since",
				Description: "Only show builds created this duration ago.",
				Value:       serpent.DurationOf(&since),
			},
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
