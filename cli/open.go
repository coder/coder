package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) open() *serpent.Command {
	var (
		client           = new(codersdk.Client)
		regionArg        string
		testOpenErrorArg bool
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "open <workspace> [<app slug>]",
		Short:       "Open a workspace in an IDE or workspace app. If no app slug is provided, lists available apps in <workspace>.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			openFn := open.Run
			if testOpenErrorArg {
				openFn = testOpenErrorFn
			}

			// This behavior will eventually be the default, but we need to
			// maintain backwards compatibility for coder open vscode <workspace>.
			if len(inv.Args) > 0 && inv.Args[0] != "vscode" {
				return handleOpenCmd(inv, client, regionArg, openFn)
			}

			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.openVSCode(), // Deprecated and will be removed in a future version.
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag: "region",
			Env:  "CODER_OPEN_REGION",
			Description: fmt.Sprintf("Region to use when opening the app." +
				" By default, the app will be opened using the main Coder deployment (a.k.a. \"primary\"). This has no effect on external application URLs."),
			Value:   serpent.StringOf(&regionArg),
			Default: "primary",
		},
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenErrorArg),
			Hidden:      true, // This is for testing!
		},
	}
	return cmd
}

const vscodeDesktopName = "VS Code Desktop"

func (r *RootCmd) openVSCode() *serpent.Command {
	var (
		generateToken bool
		testOpenError bool
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "vscode <workspace> [<directory in workspace>]",
		Short:       fmt.Sprintf("Open a workspace in %s", vscodeDesktopName),
		Deprecated:  "Use 'coder open <workspace> vscode' instead.",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			openFn := open.Run
			if testOpenError {
				openFn = testOpenErrorFn
			}
			return handleOpenVSCode(inv, client, openFn, generateToken)
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

func handleOpenCmd(inv *serpent.Invocation, client *codersdk.Client, regionArg string, openFn func(string) error) error {
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()

	if len(inv.Args) == 0 || len(inv.Args) > 2 {
		return inv.Command.HelpHandler(inv)
	}

	workspaceName := inv.Args[0]
	ws, agt, err := getWorkspaceAndAgent(ctx, inv, client, false, workspaceName)
	if err != nil {
		var sdkErr *codersdk.Error
		if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
			cliui.Errorf(inv.Stderr, "Workspace %q not found!", workspaceName)
			return sdkErr
		}
		cliui.Errorf(inv.Stderr, "Failed to get workspace and agent: %s", err)
		return err
	}

	allAppSlugs := make([]string, 0, len(agt.Apps))

	for _, app := range agt.Apps {
		allAppSlugs = append(allAppSlugs, app.Slug)
	}
	// We also want to offer vscode as an option for all workspaces.
	if !slices.Contains(allAppSlugs, "vscode") {
		allAppSlugs = append(allAppSlugs, "vscode")
		agt.Apps = append(agt.Apps, codersdk.WorkspaceApp{
			Slug:     "vscode",
			External: true,
		})
	}
	slices.Sort(allAppSlugs)

	// If a user doesn't specify an app slug, we'll just list the available
	// apps and exit.
	if len(inv.Args) == 1 {
		cliui.Infof(inv.Stderr, "Available apps in %q: %v", workspaceName, allAppSlugs)
		return nil
	}

	appSlug := inv.Args[1]
	var foundApp codersdk.WorkspaceApp
	appIdx := slices.IndexFunc(agt.Apps, func(a codersdk.WorkspaceApp) bool {
		return a.Slug == appSlug
	})
	if appIdx == -1 {
		cliui.Errorf(inv.Stderr, "App %q not found in workspace %q!\nAvailable apps: %v", appSlug, workspaceName, allAppSlugs)
		return xerrors.Errorf("app not found")
	}
	foundApp = agt.Apps[appIdx]

	// To build the app URL, we need to know the wildcard hostname
	// and path app URL for the region. This isn't needed for external
	// apps.
	var region codersdk.Region
	if !foundApp.External {
		regions, err := client.Regions(ctx)
		if err != nil {
			return xerrors.Errorf("failed to fetch regions: %w", err)
		}
		preferredIdx := slices.IndexFunc(regions, func(r codersdk.Region) bool {
			return r.Name == regionArg
		})
		if preferredIdx == -1 {
			allRegions := make([]string, len(regions))
			for i, r := range regions {
				allRegions[i] = r.Name
			}
			cliui.Errorf(inv.Stderr, "Preferred region %q not found!\nAvailable regions: %v", regionArg, allRegions)
			return xerrors.Errorf("region not found")
		}
		region = regions[preferredIdx]
	}

	baseURL, err := url.Parse(region.PathAppURL)
	if err != nil {
		return xerrors.Errorf("failed to parse proxy URL: %w", err)
	}
	baseURL.Path = ""
	pathAppURL := strings.TrimPrefix(region.PathAppURL, baseURL.String())

	// We need to special-case vscode to open the remote URL.
	var appURL string
	if foundApp.Slug == "vscode" {
		// TODO: support generating a token and specifying a directory.
		// Omitting the folder query parameter will simply cause VSCode
		// to prompt the user.
		appURL, _, err = buildVSCodeRemoteURL(ctx, client, ws, agt, "", false)
		if err != nil {
			return xerrors.Errorf("failed to build VS Code remote URL: %w", err)
		}
	} else {
		// Note that external apps don't have a region, and that
		// region.WildcardHostname and region.PathAppURL will be empty.
		appURL = buildAppLinkURL(baseURL, ws, agt, foundApp, region.WildcardHostname, pathAppURL)
	}

	dontPrintAppURL := appURL
	if foundApp.External {
		dontPrintAppURL = replacePlaceholderExternalSessionTokenString(client, appURL)
	}

	// Check if we're inside a workspace.  Generally, we know
	// that if we're inside a workspace, `open` can't be used.
	insideAWorkspace := inv.Environ.Get("CODER") == "true"
	if insideAWorkspace {
		_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI on your local machine:\n\n")
		_, _ = fmt.Fprintf(inv.Stdout, "%s\n", appURL)
		return nil
	}
	_, _ = fmt.Fprintf(inv.Stderr, "Opening %s\n", appURL)

	return openFn(dontPrintAppURL)
}

//nolint:revive // Correct, generateToken *is* a control flag and that's *fine*.
func handleOpenVSCode(inv *serpent.Invocation, client *codersdk.Client, openFn func(string) error, generateToken bool) error {
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
		return xerrors.Errorf("get workspace and agent: %w", err)
	}

	workspaceName := workspace.Name + "." + workspaceAgent.Name
	insideThisWorkspace := insideAWorkspace && inWorkspaceName == workspaceName

	if !insideThisWorkspace {
		// Wait for the agent to connect, we don't care about readiness
		// otherwise (e.g. wait).
		docsURL := codersdk.DefaultDocsURL()
		if appearanceConfig, err := client.Appearance(ctx); err == nil {
			docsURL = appearanceConfig.DocsURL
		} else {
			// Don't fail for this, we can still open the app.
			cliui.Warnf(inv.Stderr, "Failed to get appearance config: %s", err)
		}
		err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
			Fetch:     client.WorkspaceAgent,
			FetchLogs: nil,
			Wait:      false,
			DocsURL:   docsURL,
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
				return a.LifecycleState != codersdk.WorkspaceAgentLifecycleCreated
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
	resolvedDirectory, err := resolveAgentAbsPath(workspaceAgent.ExpandedDirectory, directory, workspaceAgent.OperatingSystem, insideThisWorkspace)
	if err != nil {
		return xerrors.Errorf("resolve agent path: %w", err)
	}

	// We always set the token if we believe we can open without
	// printing the URI, otherwise the token must be explicitly
	// requested as it will be printed in plain text.
	urlWithMaybeToken, deleteTokenFn, err := buildVSCodeRemoteURL(ctx, client, workspace, workspaceAgent, resolvedDirectory, !insideAWorkspace || generateToken)
	if err != nil {
		return xerrors.Errorf("build VS Code remote URL: %w", err)
	}
	// We don't want to print the URL with the token, so we'll use this
	// variable to store the printable URL that we may print to stdout.
	printableURL := removeTokenParameter(urlWithMaybeToken)

	openingPath := workspaceName
	if directory != "" {
		openingPath += ":" + directory
	}

	// If we are inside a workspace, the user becomes our open command.
	// In this case, we need to print the URL that may contain a token.
	if insideAWorkspace {
		_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s is not supported inside a workspace.\nPlease open the following URI on your local machine instead:\n\n", openingPath, vscodeDesktopName)
		_, _ = fmt.Fprintf(inv.Stdout, "%s\n", urlWithMaybeToken)
		return nil
	}
	_, _ = fmt.Fprintf(inv.Stderr, "Opening %s in %s\n", openingPath, vscodeDesktopName)

	if err := openFn(urlWithMaybeToken); err != nil {
		// We need to delete the token if the open fails.
		wait := doAsync(func() {
			_ = deleteTokenFn()
		})
		defer wait()

		_, _ = fmt.Fprintf(inv.Stderr, "Could not automatically open %s in %s: %s\n", openingPath, vscodeDesktopName, err)
		_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI instead:\n\n")
		// If we were explicitly asked for a token, we'll print the URL with the token.
		if generateToken {
			_, _ = fmt.Fprintf(inv.Stdout, "%s\n", urlWithMaybeToken)
		} else {
			_, _ = fmt.Fprintf(inv.Stdout, "%s\n", printableURL)
		}
		return nil
	}

	return nil
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

// buildAppLinkURL returns the URL to open the app in the browser.
// It follows similar logic to the TypeScript implementation in site/src/utils/app.ts
// except that all URLs returned are absolute and based on the provided base URL.
func buildAppLinkURL(baseURL *url.URL, workspace codersdk.Workspace, agent codersdk.WorkspaceAgent, app codersdk.WorkspaceApp, appsHost, preferredPathBase string) string {
	// If app is external, return the URL directly
	if app.External {
		return app.URL
	}

	var u url.URL
	u.Scheme = baseURL.Scheme
	u.Host = baseURL.Host
	// We redirect if we don't include a trailing slash, so we always include one to avoid extra roundtrips.
	u.Path = fmt.Sprintf(
		"%s/@%s/%s.%s/apps/%s/",
		preferredPathBase,
		workspace.OwnerName,
		workspace.Name,
		agent.Name,
		url.PathEscape(app.Slug),
	)
	// The frontend leaves the returns a relative URL for the terminal, but we don't have that luxury.
	if app.Command != "" {
		u.Path = fmt.Sprintf(
			"%s/@%s/%s.%s/terminal",
			preferredPathBase,
			workspace.OwnerName,
			workspace.Name,
			agent.Name,
		)
		q := u.Query()
		q.Set("command", app.Command)
		u.RawQuery = q.Encode()
		// encodeURIComponent replaces spaces with %20 but url.QueryEscape replaces them with +.
		// We replace them with %20 to match the TypeScript implementation.
		u.RawQuery = strings.ReplaceAll(u.RawQuery, "+", "%20")
	}

	if appsHost != "" && app.Subdomain && app.SubdomainName != "" {
		u.Host = strings.Replace(appsHost, "*", app.SubdomainName, 1)
		u.Path = "/"
	}
	return u.String()
}

// buildVSCodeRemoteURL builds a VS Code remote URL.
// If generateToken is true, it will add a token to the URL.
// The returned function can be used to delete the token at the discretion of
// the caller.
// If no token was generated, the returned function is a no-op.
//
//nolint:revive // Yep, control flag.
func buildVSCodeRemoteURL(ctx context.Context, client *codersdk.Client, workspace codersdk.Workspace, agent codersdk.WorkspaceAgent, resolvedDir string, generateToken bool) (string, func() error, error) {
	u := &url.URL{
		Scheme: "vscode",
		Host:   "coder.coder-remote",
		Path:   "/open",
	}
	deleteTokenFn := func() error { return nil }
	qp := u.Query()

	qp.Add("url", client.URL.String())
	qp.Add("owner", workspace.OwnerName)
	qp.Add("workspace", workspace.Name)
	qp.Add("agent", agent.Name)

	if resolvedDir != "" {
		qp.Add("folder", resolvedDir)
	}

	if generateToken {
		apiKey, err := client.CreateAPIKey(ctx, codersdk.Me)
		if err != nil {
			return "", deleteTokenFn, xerrors.Errorf("create API key: %w", err)
		}
		qp.Add("token", apiKey.Key)
		u.RawQuery = qp.Encode()
		deleteTokenFn = func() error {
			apiKeyID := strings.SplitN(apiKey.Key, "-", 2)[0]
			return client.DeleteAPIKey(ctx, codersdk.Me, apiKeyID)
		}
	}
	u.RawQuery = qp.Encode()
	return u.String(), deleteTokenFn, nil
}

func removeTokenParameter(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}

	qp := u.Query()
	qp.Del("token")
	u.RawQuery = qp.Encode()
	return u.String()
}

// replacePlaceholderExternalSessionTokenString replaces any $SESSION_TOKEN
// strings in the URL with the actual session token.
// This is consistent behavior with the frontend. See: site/src/modules/resources/AppLink/AppLink.tsx
func replacePlaceholderExternalSessionTokenString(client *codersdk.Client, appURL string) string {
	if !strings.Contains(appURL, "$SESSION_TOKEN") {
		return appURL
	}

	// We will just re-use the existing session token we're already using.
	return strings.ReplaceAll(appURL, "$SESSION_TOKEN", client.SessionToken())
}

// Only used for testing!
func testOpenErrorFn(appURL string) error {
	return xerrors.New("test.open-error: " + appURL)
}
