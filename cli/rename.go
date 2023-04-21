package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) rename() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "rename <workspace> <new name>",
		Short:       "Rename a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "%s\n\n",
				cliui.Styles.Wrap.Render("WARNING: A rename can result in data loss if a resource references the workspace name in the template (e.g volumes). Please backup any data before proceeding."),
			)
			_, _ = fmt.Fprintf(inv.Stdout, "See: %s\n\n", "https://coder.com/docs/coder-oss/latest/templates/resource-persistence#%EF%B8%8F-persistence-pitfalls")
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text: fmt.Sprintf("Type %q to confirm rename:", workspace.Name),
				Validate: func(s string) error {
					if s == workspace.Name {
						return nil
					}
					return xerrors.Errorf("Input %q does not match %q", s, workspace.Name)
				},
			})
			if err != nil {
				return err
			}

			err = client.UpdateWorkspace(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceRequest{
				Name: inv.Args[1],
			})
			if err != nil {
				return xerrors.Errorf("rename workspace: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Workspace %q renamed to %q\n", workspace.Name, inv.Args[1])
			return nil
		},
	}

	cmd.Options = append(cmd.Options, cliui.SkipPromptOption())

	return cmd
}
