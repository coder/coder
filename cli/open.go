package cli

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
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

const vscodeDesktopName = "VS Code Desktop"

func (r *RootCmd) openVSCode() *clibase.Cmd {
	var (
		generateToken bool
		testOpenError bool
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "vscode <workspace> [<directory in workspace>]",
		Short:       "Open a workspace in Visual Studio Code",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(1, 2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			// Check if we're inside a workspace, and especially inside _this_
			// workspace so we can perform path resolution/expansion. Generally,
			// we know that if we're inside a workspace, `open` can't be used.
			insideAWorkspace := inv.Environ.Get("CODER") == "true"
			inWorkspaceName := inv.Environ.Get("CODER_WORKSPACE_NAME") + "." + inv.Environ.Get("CODER_WORKSPACE_AGENT_NAME")

			// We need a started workspace to figure out e.g. expanded directory.
			// Pehraps the vscode-coder extension could handle this by accepting
			// default_directory=true, then probing the agent. Then we wouldn't
			// need to wait for the agent to start.
			workspaceQuery := inv.Args[0]
			autostart := true
			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, codersdk.Me, workspaceQuery)
			if err != nil {
				return xerrors.Errorf("get workspace and agent: %w", err)
			}

			workspaceName := workspace.Name + "." + workspaceAgent.Name
			insideThisWorkspace := insideAWorkspace && inWorkspaceName == workspaceName

			if !insideThisWorkspace {
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
			}

			directory := workspaceAgent.ExpandedDirectory // Empty unless agent directory is set.
			if len(inv.Args) > 1 {
				d := inv.Args[1]

				switch {
				case insideThisWorkspace:
					// TODO(mafredri): Return error if directory doesn't exist?
					directory, err = filepath.Abs(d)
					if err != nil {
						return xerrors.Errorf("expand directory: %w", err)
					}

				case d == "~" || strings.HasPrefix(d, "~/"):
					return xerrors.Errorf("path %q requires expansion and is not supported, use an absolute path instead", d)

				case workspaceAgent.OperatingSystem == "windows":
					// TODO(mafredri): For now we keep this simple instead of discerning out relative paths on Windows.
					directory = d

				// Note that we use `path` instead of `filepath` since we want Unix behavior.
				case directory != "" && !path.IsAbs(d):
					directory = path.Join(directory, d)
				case path.IsAbs(d):
					directory = d
				default:
					return xerrors.Errorf("path %q not supported, use an absolute path instead", d)
				}
			}

			u, err := url.Parse("vscode://coder.coder-remote/open")
			if err != nil {
				return xerrors.Errorf("parse vscode URI: %w", err)
			}

			qp := url.Values{}

			qp.Add("url", client.URL.String())
			qp.Add("owner", workspace.OwnerName)
			qp.Add("workspace", workspace.Name)
			qp.Add("agent", workspaceAgent.Name)
			if directory != "" {
				qp.Add("folder", directory)
			}

			// We always set the token if we believe we can open without
			// printing the URI, otherwise the token must be explicitly
			// requested as it will be printed in plain text.
			if !insideAWorkspace || generateToken {
				// Prepare an API key. This is for automagical configuration of
				// VS Code, however, if running on a local machine we could try
				// to probe VS Code settings to see if the current configuration
				// is valid. Future improvement idea.
				apiKey, err := client.CreateAPIKey(ctx, codersdk.Me)
				if err != nil {
					return xerrors.Errorf("create API key: %w", err)
				}
				qp.Add("token", apiKey.Key)
			}

			u.RawQuery = qp.Encode()

			openingPath := workspaceName
			if directory != "" {
				openingPath += ":" + directory
			}

			if insideAWorkspace {
				_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s is not supported inside a workspace, please open the following URI on your local machine instead:\n\n", openingPath, vscodeDesktopName)
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s\n", openingPath, vscodeDesktopName)

			if !testOpenError {
				err = open.Run(u.String())
			} else {
				err = xerrors.New("test.open-error")
			}
			if err != nil {
				if !generateToken {
					qp.Del("token")
					u.RawQuery = qp.Encode()
				}

				_, _ = fmt.Fprintf(inv.Stderr, "Could not automatically open %s in %s: %s\n", openingPath, vscodeDesktopName, err)
				_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI instead:\n\n")
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag: "generate-token",
			Env:  "CODER_OPEN_VSCODE_GENERATE_TOKEN",
			Description: fmt.Sprintf(
				"Generate an auth token and include it in the vscode:// URI. This is for automagical configuration of %s and not needed if already configured. "+
					"This flag does not need to be specified when running this command on a local machine unless automatic open fails.",
				vscodeDesktopName,
			),
			Value: clibase.BoolOf(&generateToken),
		},
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       clibase.BoolOf(&testOpenError),
			Hidden:      true, // This is for testing!
		},
	}

	return cmd
}
