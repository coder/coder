package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) state() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "state",
		Short: "Manually manage Terraform state to fix broken workspaces",
		Handler: func(inv *clibase.Invokation) error {
			return inv.Command.HelpHandler(inv)
		},
	}
	cmd.AddCommand(statePull(), statePush())
	return cmd
}

func (r *RootCmd) statePull() *clibase.Cmd {
	var buildNumber int
	var client *codersdk.Client
	cmd := &clibase.Cmd{
		Use:   "pull <workspace> [file]",
		Short: "Pull a Terraform state file from a workspace.",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(1, -1),
			r.useClient(client),
		),
		Handler: func(inv *clibase.Invokation) error {
			var build codersdk.WorkspaceBuild
			if buildNumber == 0 {
				workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
				if err != nil {
					return err
				}
				build = workspace.LatestBuild
			} else {
				build, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(inv.Context(), codersdk.Me, inv.Args[0], strconv.Itoa(buildNumber))
				if err != nil {
					return err
				}
			}

			state, err := client.WorkspaceBuildState(inv.Context(), build.ID)
			if err != nil {
				return err
			}

			if len(inv.Args) < 2 {
				_, _ = fmt.Fprintln(inv.Stdout, string(state))
				return nil
			}

			return os.WriteFile(inv.Args[1], state, 0o600)
		},
	}
	cmd.Flags().IntVarP(&buildNumber, "build", "b", 0, "Specify a workspace build to target by name.")
	return cmd
}

func (r *RootCmd) statePush() *clibase.Cmd {
	var buildNumber int
	cmd := &clibase.Cmd{
		Use:        "push <workspace> <file>",
		Middleware: clibase.RequireNArgs(2),
		Short:      "Push a Terraform state file to a workspace.",
		Middleware: clibase.Chain(r.useClient(client)),
		Handler: func(inv *clibase.Invokation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			if buildNumber == 0 {
				build = workspace.LatestBuild
			} else {
				build, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(inv.Context(), codersdk.Me, inv.Args[0], strconv.Itoa(buildNumber))
				if err != nil {
					return err
				}
			}

			var state []byte
			if inv.Args[1] == "-" {
				state, err = io.ReadAll(inv.Stdin)
			} else {
				state, err = os.ReadFile(inv.Args[1])
			}
			if err != nil {
				return err
			}

			build, err = client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: build.TemplateVersionID,
				Transition:        build.Transition,
				ProvisionerState:  state,
			})
			if err != nil {
				return err
			}
			return cliui.WorkspaceBuild(inv.Context(), inv.Stderr, client, build.ID)
		},
	}
	cmd.Flags().IntVarP(&buildNumber, "build", "b", 0, "Specify a workspace build to target by name.")
	return cmd
}
