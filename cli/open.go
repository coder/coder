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

const (
	vscodeDesktopName   = "VS Code Desktop"
	cursorDesktopName   = "Cursor Desktop"
	windsurfDesktopName = "Windsurf Desktop"
	fleetDesktopName    = "JetBrains Fleet"
	zedDesktopName      = "Zed"
)

// prepareWorkspace encapsulates the common workflow for retrieving the workspace and agent,
// checking if we’re already inside the workspace, and waiting for the agent to be ready.
func prepareWorkspace(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, appearanceConfig codersdk.AppearanceConfig) (ws codersdk.Workspace, agent codersdk.WorkspaceAgent, inside bool, err error) {
	insideEnv := inv.Environ.Get("CODER") == "true"
	currentWSName := inv.Environ.Get("CODER_WORKSPACE_NAME") + "." + inv.Environ.Get("CODER_WORKSPACE_AGENT_NAME")
	workspaceQuery := inv.Args[0]
	autostart := true

	ws, agent, err = getWorkspaceAndAgent(ctx, inv, client, autostart, workspaceQuery)
	if err != nil {
		return
	}
	wsName := ws.Name + "." + agent.Name
	inside = insideEnv && (currentWSName == wsName)

	if !inside {
		// Wait for the agent to connect (we don't care about full readiness)
		err = cliui.Agent(ctx, inv.Stderr, agent.ID, cliui.AgentOptions{
			Fetch:     client.WorkspaceAgent,
			FetchLogs: nil,
			Wait:      false,
			DocsURL:   appearanceConfig.DocsURL,
		})
		if err != nil {
			if xerrors.Is(err, context.Canceled) {
				err = cliui.Canceled
			}
			return
		}
		if agent.Directory != "" {
			ws, agent, err = waitForAgentCond(ctx, client, ws, agent, func(a codersdk.WorkspaceAgent) bool {
				return agent.LifecycleState != codersdk.WorkspaceAgentLifecycleCreated
			})
			if err != nil {
				return
			}
		}
	}
	return
}

// resolveDirectory handles directory resolution using the agent's expanded directory.
func resolveDirectory(inv *serpent.Invocation, agent codersdk.WorkspaceAgent, inside bool) (string, error) {
	var dir string
	if len(inv.Args) > 1 {
		dir = inv.Args[1]
	}
	return resolveAgentAbsPath(agent.ExpandedDirectory, dir, agent.OperatingSystem, inside)
}

// executeOpenURL consolidates the logic for opening the URL (or printing it if inside a workspace).
// The cleanup function (if non-nil) is used for any necessary token cleanup on error.
func executeOpenURL(inv *serpent.Invocation, ideName, openingPath string, u *url.URL, testOpenError bool, cleanup func()) error {
	if inv.Environ.Get("CODER") == "true" {
		fmt.Fprintf(inv.Stderr, "Opening %s in %s is not supported inside a workspace. Please open the following URI on your local machine instead:\n\n", openingPath, ideName)
		fmt.Fprintf(inv.Stdout, "%s\n", u.String())
		return nil
	}

	fmt.Fprintf(inv.Stderr, "Opening %s in %s\n", openingPath, ideName)
	var err error
	if !testOpenError {
		err = open.Run(u.String())
	} else {
		err = xerrors.New("test.open-error")
	}
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		fmt.Fprintf(inv.Stderr, "Could not automatically open %s in %s: %s\n", openingPath, ideName, err)
		fmt.Fprintf(inv.Stderr, "Please open the following URI instead:\n\n")
		fmt.Fprintf(inv.Stdout, "%s\n", u.String())
		return nil
	}
	return nil
}

// Command registration

func (r *RootCmd) open() *serpent.Command {
	return &serpent.Command{
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
}

// createGenericVSCodeIDECommand creates a command for IDEs using a VSCode-like URI scheme.
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

			ws, agent, inside, err := prepareWorkspace(ctx, inv, client, appearanceConfig)
			if err != nil {
				return xerrors.Errorf("prepare workspace: %w", err)
			}
			resolvedDir, err := resolveDirectory(inv, agent, inside)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}
			wsName := ws.Name + "." + agent.Name
			openingPath := wsName
			if resolvedDir != "" {
				openingPath += ":" + resolvedDir
			}

			u := &url.URL{
				Scheme: scheme,
				Host:   "coder.coder-remote",
				Path:   "/open",
			}
			qp := url.Values{}
			qp.Add("url", client.URL.String())
			qp.Add("owner", ws.OwnerName)
			qp.Add("workspace", ws.Name)
			qp.Add("agent", agent.Name)
			if resolvedDir != "" {
				qp.Add("folder", resolvedDir)
			}
			// Include token if we're not inside or if explicitly requested.
			if !inside || generateToken {
				apiKey, err := client.CreateAPIKey(ctx, codersdk.Me)
				if err != nil {
					return xerrors.Errorf("create API key: %w", err)
				}
				qp.Add("token", apiKey.Key)
			}
			u.RawQuery = qp.Encode()

			// Define cleanup to remove the token if open fails.
			cleanup := func() {
				if !inside || generateToken {
					token := qp.Get("token")
					wait := doAsync(func() {
						apiKeyID := strings.SplitN(token, "-", 2)[0]
						_ = client.DeleteAPIKey(ctx, codersdk.Me, apiKeyID)
					})
					wait()
				}
			}
			return executeOpenURL(inv, ideName, openingPath, u, testOpenError, cleanup)
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag: "generate-token",
			Env:  "CODER_OPEN_" + strings.ToUpper(use) + "_GENERATE_TOKEN",
			Description: fmt.Sprintf(
				"Generate an auth token and include it in the %s:// URI. This is for automagical configuration of %s and not needed if already configured. "+
					"This flag does not need to be specified on a local machine unless automatic open fails.",
				scheme, ideName,
			),
			Value: serpent.BoolOf(&generateToken),
		},
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenError),
			Hidden:      true,
		},
	}
	return cmd
}

func (r *RootCmd) openVSCode() *serpent.Command {
	return r.createGenericVSCodeIDECommand("vscode", fmt.Sprintf("Open a workspace in %s", vscodeDesktopName), vscodeDesktopName, "vscode")
}

func (r *RootCmd) openCursor() *serpent.Command {
	return r.createGenericVSCodeIDECommand("cursor", fmt.Sprintf("Open a workspace in %s", cursorDesktopName), cursorDesktopName, "cursor")
}

func (r *RootCmd) openWindsurf() *serpent.Command {
	return r.createGenericVSCodeIDECommand("windsurf", fmt.Sprintf("Open a workspace in %s", windsurfDesktopName), windsurfDesktopName, "windsurf")
}

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

			ws, agent, inside, err := prepareWorkspace(ctx, inv, client, appearanceConfig)
			if err != nil {
				return xerrors.Errorf("prepare workspace: %w", err)
			}
			resolvedDir, err := resolveDirectory(inv, agent, inside)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}
			wsName := ws.Name + "." + agent.Name
			openingPath := wsName
			if resolvedDir != "" {
				openingPath += ":" + resolvedDir
			}
			// Fleet uses a different URI scheme: fleet://fleet.ssh/coder.<workspace>?pwd=<path>
			u := &url.URL{
				Scheme: "fleet",
				Host:   "fleet.ssh",
				Path:   fmt.Sprintf("/coder.%s", wsName),
			}
			qp := url.Values{}
			if resolvedDir != "" {
				qp.Add("pwd", resolvedDir)
			}
			u.RawQuery = qp.Encode()

			return executeOpenURL(inv, fleetDesktopName, openingPath, u, testOpenError, nil)
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenError),
			Hidden:      true,
		},
	}
	return cmd
}

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

			ws, agent, inside, err := prepareWorkspace(ctx, inv, client, appearanceConfig)
			if err != nil {
				return xerrors.Errorf("prepare workspace: %w", err)
			}
			resolvedDir, err := resolveDirectory(inv, agent, inside)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}
			wsName := ws.Name + "." + agent.Name
			openingPath := wsName
			if resolvedDir != "" {
				openingPath += ":" + resolvedDir
			}
			// Zed uses URI scheme: zed://ssh/coder.<workspace>/<path>
			u := &url.URL{
				Scheme: "zed",
				Host:   "ssh",
				Path:   fmt.Sprintf("/coder.%s", wsName),
			}
			if resolvedDir != "" {
				u.Path = fmt.Sprintf("%s/%s", u.Path, resolvedDir)
			}
			return executeOpenURL(inv, zedDesktopName, openingPath, u, testOpenError, nil)
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
