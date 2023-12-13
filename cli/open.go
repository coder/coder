package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) open() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "open",
		Short: "Open a workspace",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.openVSCode(),
		},
	}
	return cmd
}

func (r *RootCmd) openVSCode() *clibase.Cmd {
	var testNoOpen bool

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "vscode <workspace> [<directory in workspace>]",
		Short:       "Open a workspace in Visual Studio Code",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(1, -1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			// Prepare an API key. This is for automagical configuration of
			// VS Code, however, we could try to probe VS Code settings to see
			// if the current configuration is valid. Future improvement idea.
			apiKey, err := client.CreateAPIKey(ctx, codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create API key: %w", err)
			}

			// We need a started workspace to figure out e.g. expanded directory.
			// Pehraps the vscode-coder extension could handle this by accepting
			// default_directory=true, then probing the agent. Then we wouldn't
			// need to wait for the agent to start.
			workspaceName := inv.Args[0]
			autostart := true
			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, codersdk.Me, workspaceName)
			if err != nil {
				return xerrors.Errorf("get workspace and agent: %w", err)
			}

			// We could optionally add a flag to skip wait, like with SSH.
			wait := false
			for _, script := range workspaceAgent.Scripts {
				if script.StartBlocksLogin {
					wait = true
					break
				}
			}
			err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
				Fetch:     client.WorkspaceAgent,
				FetchLogs: client.WorkspaceAgentLogsAfter,
				Wait:      wait,
			})
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					return cliui.Canceled
				}
				return xerrors.Errorf("agent: %w", err)
			}

			// If the ExpandedDirectory was initially missing, it could mean
			// that the agent hadn't reported it in yet. Retry once.
			if workspaceAgent.ExpandedDirectory == "" {
				autostart = false // Don't retry autostart.
				workspace, workspaceAgent, err = getWorkspaceAndAgent(ctx, inv, client, autostart, codersdk.Me, workspaceName)
				if err != nil {
					return xerrors.Errorf("get workspace and agent retry: %w", err)
				}
			}

			var folder string
			switch {
			case len(inv.Args) > 1:
				folder = inv.Args[1]
				// Perhaps we could SSH in to expand the directory?
				if strings.HasPrefix(folder, "~") {
					return xerrors.Errorf("folder path %q not supported, use an absolute path instead", folder)
				}
			case workspaceAgent.ExpandedDirectory != "":
				folder = workspaceAgent.ExpandedDirectory
			}

			qp := url.Values{}

			qp.Add("url", client.URL.String())
			qp.Add("token", apiKey.Key)
			qp.Add("owner", workspace.OwnerName)
			qp.Add("workspace", workspace.Name)
			qp.Add("agent", workspaceAgent.Name)
			if folder != "" {
				qp.Add("folder", folder)
			}

			uri := fmt.Sprintf("vscode://coder.coder-remote/open?%s", qp.Encode())
			_, _ = fmt.Fprintf(inv.Stdout, "Opening %s\n", strings.ReplaceAll(uri, apiKey.Key, "<REDACTED>"))

			if testNoOpen {
				return nil
			}

			err = open.Run(uri)
			if err != nil {
				return xerrors.Errorf("open: %w", err)
			}

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "test.no-open",
			Description: "Don't run the open command.",
			Value:       clibase.BoolOf(&testNoOpen),
			Hidden:      true, // This is for testing!
		},
	}

	return cmd
}
