//go:build !slim

package cli

import (
	"os/signal"

	"golang.org/x/xerrors"

	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	"github.com/coder/serpent"
)

// AGPLExperimental shadows the embedded RootCmd.AGPLExperimental to inject the
// enterprise-only agentfake scaletest subcommand into the scaletest subtree.
func (r *RootCmd) AGPLExperimental() []*serpent.Command {
	cmds := r.RootCmd.AGPLExperimental()
	for _, cmd := range cmds {
		if cmd.Use == "scaletest" {
			cmd.Children = append(cmd.Children, r.scaletestAgentFake())
		}
	}
	return cmds
}

func (r *RootCmd) scaletestAgentFake() *serpent.Command {
	var (
		template string
		owner    string
	)

	cmd := &serpent.Command{
		Use:   "agentfake",
		Short: "Run fake external agents against workspaces of the given template.",
		Long: agplcli.FormatExamples(
			agplcli.Example{
				Description: "Connect a fake agent for every external-agent workspace built from the template named " +
					"\"agentfake-runner\".",
				Command: "coder exp scaletest agentfake --template agentfake-runner",
			},
		) + "\n\n" +
			"Enumerates external-agent workspaces matching --template (optionally filtered by --owner), " +
			"fetches each workspace agent's external-agent credentials, and supervises one in-process fake " +
			"agent per token until the command is interrupted.\n\n" +
			"Requires a session token whose user is template-admin (or higher) on a deployment licensed " +
			"for the workspace external-agent feature; both the workspace builds and the credentials " +
			"endpoint are gated server-side. Pair with `coder exp scaletest create-workspaces " +
			"--no-wait-for-agents` to seed the workspaces this command will pick up. Workspaces created " +
			"after this command starts are NOT picked up; rerun the command after seeding more.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, agplcli.StopSignals...)
			defer stop()
			ctx = notifyCtx

			if _, err := agplcli.RequireAdmin(ctx, client); err != nil {
				return err
			}

			if template == "" {
				return xerrors.New("--template is required")
			}

			logger := inv.Logger
			mgr := agentfake.NewManager(client, logger, agentfake.ManagerOptions{
				Template: template,
				Owner:    owner,
			})
			defer mgr.Close()

			if err := mgr.Run(ctx); err != nil {
				return xerrors.Errorf("run agentfake manager: %w", err)
			}
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "template",
			Env:         "CODER_SCALETEST_AGENTFAKE_TEMPLATE",
			Description: "Name of the template whose external-agent workspaces should be supervised. Required.",
			Value:       serpent.StringOf(&template),
		},
		{
			Flag:        "owner",
			Env:         "CODER_SCALETEST_AGENTFAKE_OWNER",
			Description: "Optional workspace-owner filter (username). When empty, all owners' workspaces of the template are included.",
			Value:       serpent.StringOf(&owner),
		},
	}

	return cmd
}
