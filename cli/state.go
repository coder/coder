package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) state() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "state",
		Short: "Manually manage Terraform state to fix broken workspaces",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.statePull(),
			r.statePush(),
		},
	}
	return cmd
}

func (r *RootCmd) statePull() *serpent.Command {
	var buildNumber int64
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "pull <workspace> [file]",
		Short: "Pull a Terraform state file from a workspace.",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			var err error
			var build codersdk.WorkspaceBuild
			if buildNumber == 0 {
				workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
				if err != nil {
					return err
				}
				build = workspace.LatestBuild
			} else {
				owner, workspace, err := splitNamedWorkspace(inv.Args[0])
				if err != nil {
					return err
				}
				build, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(inv.Context(), owner, workspace, strconv.FormatInt(buildNumber, 10))
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
	cmd.Options = serpent.OptionSet{
		buildNumberOption(&buildNumber),
	}
	return cmd
}

func buildNumberOption(n *int64) serpent.Option {
	return serpent.Option{
		Flag:          "build",
		FlagShorthand: "b",
		Description:   "Specify a workspace build to target by name. Defaults to latest.",
		Value:         serpent.Int64Of(n),
	}
}

func (r *RootCmd) statePush() *serpent.Command {
	var buildNumber int64
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "push <workspace> <file>",
		Short: "Push a Terraform state file to a workspace.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			if buildNumber == 0 {
				build = workspace.LatestBuild
			} else {
				owner, workspace, err := splitNamedWorkspace(inv.Args[0])
				if err != nil {
					return err
				}
				build, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(inv.Context(), owner, workspace, strconv.FormatInt((buildNumber), 10))
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
	cmd.Options = serpent.OptionSet{
		buildNumberOption(&buildNumber),
	}
	return cmd
}
