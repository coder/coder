package cli
import (
	"errors"
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"github.com/skratchdot/open-golang/open"
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
		},
	}
	return cmd
}
const vscodeDesktopName = "VS Code Desktop"
func (r *RootCmd) openVSCode() *serpent.Command {
	var (
		generateToken    bool
		testOpenError    bool
		appearanceConfig codersdk.AppearanceConfig
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "vscode <workspace> [<directory in workspace>]",
		Short:       fmt.Sprintf("Open a workspace in %s", vscodeDesktopName),
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) error {
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
			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, workspaceQuery)
			if err != nil {
				return fmt.Errorf("get workspace and agent: %w", err)
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
					if errors.Is(err, context.Canceled) {
						return cliui.Canceled
					}
					return fmt.Errorf("agent: %w", err)
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
						return fmt.Errorf("wait for agent: %w", err)
					}
				}
			}
			var directory string
			if len(inv.Args) > 1 {
				directory = inv.Args[1]
			}
			directory, err = resolveAgentAbsPath(workspaceAgent.ExpandedDirectory, directory, workspaceAgent.OperatingSystem, insideThisWorkspace)
			if err != nil {
				return fmt.Errorf("resolve agent path: %w", err)
			}
			u := &url.URL{
				Scheme: "vscode",
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
				// Prepare an API key. This is for automagical configuration of
				// VS Code, however, if running on a local machine we could try
				// to probe VS Code settings to see if the current configuration
				// is valid. Future improvement idea.
				apiKey, err := client.CreateAPIKey(ctx, codersdk.Me)
				if err != nil {
					return fmt.Errorf("create API key: %w", err)
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
				err = errors.New("test.open-error")
			}
			if err != nil {
				if !generateToken {
					// This is not an important step, so we don't want
					// to block the user here.
					token := qp.Get("token")
					wait := doAsync(func() {
						// Best effort, we don't care if this fails.
						apiKeyID := strings.SplitN(token, "-", 2)[0]
						_ = client.DeleteAPIKey(ctx, codersdk.Me, apiKeyID)
					})
					defer wait()
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
	cmd.Options = serpent.OptionSet{
		{
			Flag: "generate-token",
			Env:  "CODER_OPEN_VSCODE_GENERATE_TOKEN",
			Description: fmt.Sprintf(
				"Generate an auth token and include it in the vscode:// URI. This is for automagical configuration of %s and not needed if already configured. "+
					"This flag does not need to be specified when running this command on a local machine unless automatic open fails.",
				vscodeDesktopName,
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
		return workspace, workspaceAgent, fmt.Errorf("watch workspace: %w", err)
	}
	for workspace = range wc {
		workspaceAgent, err = getWorkspaceAgent(workspace, workspaceAgent.Name)
		if err != nil {
			return workspace, workspaceAgent, fmt.Errorf("get workspace agent: %w", err)
		}
		if cond(workspaceAgent) {
			return workspace, workspaceAgent, nil
		}
	}
	return workspace, workspaceAgent, errors.New("watch workspace: unexpected closed channel")
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
		return "", fmt.Errorf("path %q requires expansion and is not supported, use an absolute path instead", relOrAbsPath)
	case local:
		p, err := filepath.Abs(relOrAbsPath)
		if err != nil {
			return "", fmt.Errorf("expand path: %w", err)
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
			return "", fmt.Errorf("path %q not supported, use an absolute path instead", relOrAbsPath)
		}
	// Note that we use `path` instead of `filepath` since we want Unix behavior.
	case workingDirectory != "" && !path.IsAbs(relOrAbsPath):
		return path.Join(workingDirectory, relOrAbsPath), nil
	case path.IsAbs(relOrAbsPath):
		return relOrAbsPath, nil
	default:
		return "", fmt.Errorf("path %q not supported, use an absolute path instead", relOrAbsPath)
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
