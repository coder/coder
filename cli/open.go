package cli

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) open() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "open",
		Short: "Open a workspace",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.openVSCode(),
			r.openCursor(),
			r.openWindsurf(),
			r.openFleet(),
			r.openZed(),
		},
	}
	return cmd
}

const (
	vscodeDesktopName   = "VS Code Desktop"
	cursorDesktopName   = "Cursor Desktop"
	windsurfDesktopName = "Windsurf Desktop"
	fleetDesktopName    = "JetBrains Fleet"
	zedDesktopName      = "Zed"
)

func (r *RootCmd) openVSCode() *serpent.Command {
	return r.createGenericVSCodeIDECommand(
		"vscode",
		fmt.Sprintf("Open a workspace in %s", vscodeDesktopName),
		vscodeDesktopName,
		"vscode",
	)
}

// openCursor implements the "coder open cursor" command which opens a workspace in Cursor IDE
func (r *RootCmd) openCursor() *serpent.Command {
	return r.createGenericVSCodeIDECommand(
		"cursor",
		fmt.Sprintf("Open a workspace in %s", cursorDesktopName),
		cursorDesktopName,
		"cursor",
	)
}

// openWindsurf implements the "coder open windsurf" command which opens a workspace in Windsurf IDE
func (r *RootCmd) openWindsurf() *serpent.Command {
	return r.createGenericVSCodeIDECommand(
		"windsurf",
		fmt.Sprintf("Open a workspace in %s", windsurfDesktopName),
		windsurfDesktopName,
		"windsurf",
	)
}

// openFleet implements the "coder open fleet" command which opens a workspace in JetBrains Fleet
func (r *RootCmd) openFleet() *serpent.Command {
	var (
		testOpenError    bool
		appearanceConfig codersdk.AppearanceConfig
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "fleet <workspace> [<directory in workspace>]",
		Short:       fmt.Sprintf("Open a workspace in %s", fleetDesktopName),
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			insideAWorkspace := inv.Environ.Get("CODER") == "true"
			inWorkspaceName := inv.Environ.Get("CODER_WORKSPACE_NAME") + "." + inv.Environ.Get("CODER_WORKSPACE_AGENT_NAME")

			workspaceQuery := inv.Args[0]
			autostart := true
			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, workspaceQuery)
			if err != nil {
				return xerrors.Errorf("get workspace and agent: %w", err)
			}

			workspaceName := workspace.Name + "." + workspaceAgent.Name
			insideThisWorkspace := insideAWorkspace && inWorkspaceName == workspaceName

			if !insideThisWorkspace {
				err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
					Fetch:     client.WorkspaceAgent,
					FetchLogs: nil,
					Wait:      false,
					DocsURL:   appearanceConfig.DocsURL,
				})
				if err != nil {
					if xerrors.Is(err, context.Canceled) {
						return cliui.Canceled
					}
					return xerrors.Errorf("agent: %w", err)
				}

				if workspaceAgent.Directory != "" {
					workspace, workspaceAgent, err = waitForAgentCond(ctx, client, workspace, workspaceAgent, func(a codersdk.WorkspaceAgent) bool {
						return workspaceAgent.LifecycleState != codersdk.WorkspaceAgentLifecycleCreated
					})
					if err != nil {
						return xerrors.Errorf("wait for agent: %w", err)
					}
				}
			}

			var directory string
			if len(inv.Args) > 1 {
				directory = inv.Args[1]
			}
			directory, err = resolveAgentAbsPath(workspaceAgent.ExpandedDirectory, directory, workspaceAgent.OperatingSystem, insideThisWorkspace)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}

			// Fleet uses a different URI scheme: fleet://fleet.ssh/coder.<workspace>?pwd=<path>
			u := &url.URL{
				Scheme: "fleet",
				Host:   "fleet.ssh",
				Path:   fmt.Sprintf("/coder.%s", workspaceName),
			}

			qp := url.Values{}
			if directory != "" {
				qp.Add("pwd", directory)
			}
			u.RawQuery = qp.Encode()

			openingPath := workspaceName
			if directory != "" {
				openingPath += ":" + directory
			}

			if insideAWorkspace {
				_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s is not supported inside a workspace, please open the following URI on your local machine instead:\n\n", openingPath, fleetDesktopName)
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s\n", openingPath, fleetDesktopName)

			if !testOpenError {
				err = open.Run(u.String())
			} else {
				err = xerrors.New("test.open-error")
			}
			if err != nil {
				_, _ = fmt.Fprintf(inv.Stderr, "Could not automatically open %s in %s: %s\n", openingPath, fleetDesktopName, err)
				_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI instead:\n\n")
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenError),
			Hidden:      true, // This is for testing!
		},
	}

	return cmd
}

// openZed implements the "coder open zed" command which opens a workspace in Zed
func (r *RootCmd) openZed() *serpent.Command {
	var (
		testOpenError    bool
		appearanceConfig codersdk.AppearanceConfig
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "zed <workspace> [<directory in workspace>]",
		Short:       fmt.Sprintf("Open a workspace in %s", zedDesktopName),
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			insideAWorkspace := inv.Environ.Get("CODER") == "true"
			inWorkspaceName := inv.Environ.Get("CODER_WORKSPACE_NAME") + "." + inv.Environ.Get("CODER_WORKSPACE_AGENT_NAME")

			workspaceQuery := inv.Args[0]
			autostart := true
			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, workspaceQuery)
			if err != nil {
				return xerrors.Errorf("get workspace and agent: %w", err)
			}

			workspaceName := workspace.Name + "." + workspaceAgent.Name
			insideThisWorkspace := insideAWorkspace && inWorkspaceName == workspaceName

			if !insideThisWorkspace {
				err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
					Fetch:     client.WorkspaceAgent,
					FetchLogs: nil,
					Wait:      false,
					DocsURL:   appearanceConfig.DocsURL,
				})
				if err != nil {
					if xerrors.Is(err, context.Canceled) {
						return cliui.Canceled
					}
					return xerrors.Errorf("agent: %w", err)
				}

				if workspaceAgent.Directory != "" {
					workspace, workspaceAgent, err = waitForAgentCond(ctx, client, workspace, workspaceAgent, func(a codersdk.WorkspaceAgent) bool {
						return workspaceAgent.LifecycleState != codersdk.WorkspaceAgentLifecycleCreated
					})
					if err != nil {
						return xerrors.Errorf("wait for agent: %w", err)
					}
				}
			}

			var directory string
			if len(inv.Args) > 1 {
				directory = inv.Args[1]
			}
			directory, err = resolveAgentAbsPath(workspaceAgent.ExpandedDirectory, directory, workspaceAgent.OperatingSystem, insideThisWorkspace)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}

			// Zed uses URI scheme: zed://ssh/coder.<workspace>/<path>
			u := &url.URL{
				Scheme: "zed",
				Host:   "ssh",
				Path:   fmt.Sprintf("/coder.%s", workspaceName),
			}

			if directory != "" {
				u.Path = fmt.Sprintf("%s/%s", u.Path, directory)
			}

			openingPath := workspaceName
			if directory != "" {
				openingPath += ":" + directory
			}

			if insideAWorkspace {
				_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s is not supported inside a workspace, please open the following URI on your local machine instead:\n\n", openingPath, zedDesktopName)
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s\n", openingPath, zedDesktopName)

			if !testOpenError {
				err = open.Run(u.String())
			} else {
				err = xerrors.New("test.open-error")
			}
			if err != nil {
				_, _ = fmt.Fprintf(inv.Stderr, "Could not automatically open %s in %s: %s\n", openingPath, zedDesktopName, err)
				_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI instead:\n\n")
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenError),
			Hidden:      true, // This is for testing!
		},
	}

	return cmd
}

// createGenericVSCodeIDECommand creates a command for opening a workspace in an IDE that uses VSCode-like URL scheme
// This works for VS Code, Cursor, Windsurf and other editors that follow the same URI pattern
func (r *RootCmd) createGenericVSCodeIDECommand(use, short, ideName, scheme string) *serpent.Command {
	var (
		generateToken    bool
		testOpenError    bool
		appearanceConfig codersdk.AppearanceConfig
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         use + " <workspace> [<directory in workspace>]",
		Short:       short,
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			// Check if we're inside a workspace, and especially inside _this_
			// workspace so we can perform path resolution/expansion
			insideAWorkspace := inv.Environ.Get("CODER") == "true"
			inWorkspaceName := inv.Environ.Get("CODER_WORKSPACE_NAME") + "." + inv.Environ.Get("CODER_WORKSPACE_AGENT_NAME")

			workspaceQuery := inv.Args[0]
			autostart := true
			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, workspaceQuery)
			if err != nil {
				return xerrors.Errorf("get workspace and agent: %w", err)
			}

			workspaceName := workspace.Name + "." + workspaceAgent.Name
			insideThisWorkspace := insideAWorkspace && inWorkspaceName == workspaceName

			if !insideThisWorkspace {
				// Wait for the agent to connect, we don't care about readiness
				// otherwise (e.g. wait).
				err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
					Fetch:     client.WorkspaceAgent,
					FetchLogs: nil,
					Wait:      false,
					DocsURL:   appearanceConfig.DocsURL,
				})
				if err != nil {
					if xerrors.Is(err, context.Canceled) {
						return cliui.Canceled
					}
					return xerrors.Errorf("agent: %w", err)
				}

				// The agent will report it's expanded directory before leaving
				// the created state, so we need to wait for that to happen.
				// However, if no directory is set, the expanded directory will
				// not be set either.
				if workspaceAgent.Directory != "" {
					workspace, workspaceAgent, err = waitForAgentCond(ctx, client, workspace, workspaceAgent, func(a codersdk.WorkspaceAgent) bool {
						return workspaceAgent.LifecycleState != codersdk.WorkspaceAgentLifecycleCreated
					})
					if err != nil {
						return xerrors.Errorf("wait for agent: %w", err)
					}
				}
			}

			var directory string
			if len(inv.Args) > 1 {
				directory = inv.Args[1]
			}
			directory, err = resolveAgentAbsPath(workspaceAgent.ExpandedDirectory, directory, workspaceAgent.OperatingSystem, insideThisWorkspace)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}

			u := &url.URL{
				Scheme: scheme,
				Host:   "coder.coder-remote",
				Path:   "/open",
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
				// Prepare an API key for automagical configuration of the IDE
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
				_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s is not supported inside a workspace, please open the following URI on your local machine instead:\n\n", openingPath, ideName)
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s\n", openingPath, ideName)

			if !testOpenError {
				err = open.Run(u.String())
			} else {
				err = xerrors.New("test.open-error")
			}
			if err != nil {
				if !generateToken {
					// Clean up the token if we can't open the IDE
					token := qp.Get("token")
					wait := doAsync(func() {
						// Best effort, we don't care if this fails
						apiKeyID := strings.SplitN(token, "-", 2)[0]
						_ = client.DeleteAPIKey(ctx, codersdk.Me, apiKeyID)
					})
					defer wait()

					qp.Del("token")
					u.RawQuery = qp.Encode()
				}

				_, _ = fmt.Fprintf(inv.Stderr, "Could not automatically open %s in %s: %s\n", openingPath, ideName, err)
				_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI instead:\n\n")
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", u.String())
				return nil
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag: "generate-token",
			Env:  "CODER_OPEN_" + strings.ToUpper(use) + "_GENERATE_TOKEN",
			Description: fmt.Sprintf(
				"Generate an auth token and include it in the %s:// URI. This is for automagical configuration of %s and not needed if already configured. "+
					"This flag does not need to be specified when running this command on a local machine unless automatic open fails.",
				scheme, ideName,
			),
			Value: serpent.BoolOf(&generateToken),
		},
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenError),
			Hidden:      true, // This is for testing!
		},
	}

	return cmd
}

// waitForAgentCond uses the watch workspace API to update the agent information
// until the condition is met.
func waitForAgentCond(ctx context.Context, client *codersdk.Client, workspace codersdk.Workspace, workspaceAgent codersdk.WorkspaceAgent, cond func(codersdk.WorkspaceAgent) bool) (codersdk.Workspace, codersdk.WorkspaceAgent, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if cond(workspaceAgent) {
		return workspace, workspaceAgent, nil
	}

	wc, err := client.WatchWorkspace(ctx, workspace.ID)
	if err != nil {
		return workspace, workspaceAgent, xerrors.Errorf("watch workspace: %w", err)
	}

	for workspace = range wc {
		workspaceAgent, err = getWorkspaceAgent(workspace, workspaceAgent.Name)
		if err != nil {
			return workspace, workspaceAgent, xerrors.Errorf("get workspace agent: %w", err)
		}
		if cond(workspaceAgent) {
			return workspace, workspaceAgent, nil
		}
	}

	return workspace, workspaceAgent, xerrors.New("watch workspace: unexpected closed channel")
}

// isWindowsAbsPath does a simplistic check for if the path is an absolute path
// on Windows. Drive letter or preceding `\` is interpreted as absolute.
func isWindowsAbsPath(p string) bool {
	// Remove the drive letter, if present.
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}

	switch {
	case len(p) == 0:
		return false
	case p[0] == '\\':
		return true
	default:
		return false
	}
}

// windowsJoinPath joins the elements into a path, using Windows path separator
// and converting forward slashes to backslashes.
func windowsJoinPath(elem ...string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(elem...)
	}

	var s string
	for _, e := range elem {
		e = unixToWindowsPath(e)
		if e == "" {
			continue
		}
		if s == "" {
			s = e
			continue
		}
		s += "\\" + strings.TrimSuffix(e, "\\")
	}
	return s
}

func unixToWindowsPath(p string) string {
	return strings.ReplaceAll(p, "/", "\\")
}

// resolveAgentAbsPath resolves the absolute path to a file or directory in the
// workspace. If the path is relative, it will be resolved relative to the
// workspace's expanded directory. If the path is absolute, it will be returned
// as-is. If the path is relative and the workspace directory is not expanded,
// an error will be returned.
//
// If the path is being resolved within the workspace, the path will be resolved
// relative to the current working directory.
func resolveAgentAbsPath(workingDirectory, relOrAbsPath, agentOS string, local bool) (string, error) {
	switch {
	case relOrAbsPath == "":
		return workingDirectory, nil

	case relOrAbsPath == "~" || strings.HasPrefix(relOrAbsPath, "~/"):
		return "", xerrors.Errorf("path %q requires expansion and is not supported, use an absolute path instead", relOrAbsPath)

	case local:
		p, err := filepath.Abs(relOrAbsPath)
		if err != nil {
			return "", xerrors.Errorf("expand path: %w", err)
		}
		return p, nil

	case agentOS == "windows":
		relOrAbsPath = unixToWindowsPath(relOrAbsPath)
		switch {
		case workingDirectory != "" && !isWindowsAbsPath(relOrAbsPath):
			return windowsJoinPath(workingDirectory, relOrAbsPath), nil
		case isWindowsAbsPath(relOrAbsPath):
			return relOrAbsPath, nil
		default:
			return "", xerrors.Errorf("path %q not supported, use an absolute path instead", relOrAbsPath)
		}

	// Note that we use `path` instead of `filepath` since we want Unix behavior.
	case workingDirectory != "" && !path.IsAbs(relOrAbsPath):
		return path.Join(workingDirectory, relOrAbsPath), nil
	case path.IsAbs(relOrAbsPath):
		return relOrAbsPath, nil
	default:
		return "", xerrors.Errorf("path %q not supported, use an absolute path instead", relOrAbsPath)
	}
}

func doAsync(f func()) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	return func() {
		<-done
	}
}
