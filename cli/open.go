package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
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
			r.openApp(),
		},
	}
	return cmd
}

const vscodeDesktopName = "VS Code Desktop"

const externalSessionTokenPlaceholder = "$SESSION_TOKEN"

// bareOriginPattern matches `scheme://host[:port]` and an optional `/`.
var bareOriginPattern = regexp.MustCompile(`(?i)^[a-z][a-z\d+.-]*://[^/\\?#@\s]+/?$`)

// isSessionTokenDestinationTrusted mirrors the dashboard check of the same name
// in site/src/modules/apps/apps.ts, but trusts only the CLI's deployment.
func isSessionTokenDestinationTrusted(appURL string, deployment *url.URL) bool {
	// Openers strip tab, LF and CR, so `ur\tl` would become a second `url`.
	if strings.ContainsFunc(appURL, unicode.IsControl) ||
		strings.Count(appURL, externalSessionTokenPlaceholder) != 1 {
		return false
	}
	params, ok := sessionTokenHandlerParams(appURL)
	if !ok || strings.Contains(params, "#") {
		return false
	}
	// The raw placeholder is replaced, so it must be the raw `token` entry.
	// JetBrains Gateway doesn't decode names, so they must be literal.
	entries := strings.Split(params, "&")
	if !slices.Contains(entries, "token="+externalSessionTokenPlaceholder) {
		return false
	}
	for _, entry := range entries {
		if name, _, _ := strings.Cut(entry, "="); strings.Contains(name, "%") {
			return false
		}
	}
	// IDEs decode values twice, so the decoded view must pass too.
	decoded, err := url.PathUnescape(params)
	return err == nil && utf8.ValidString(decoded) && !strings.Contains(decoded, "%") &&
		hasTrustedSessionTokenParams(params, deployment) &&
		hasTrustedSessionTokenParams(decoded, deployment)
}

// sessionTokenHandlerPrefixes are the link prefixes that may receive the
// session token.
var sessionTokenHandlerPrefixes = func() []string {
	var prefixes []string
	for _, scheme := range codersdk.VSCodeSessionTokenSchemes {
		for _, route := range codersdk.VSCodeSessionTokenPaths {
			prefixes = append(prefixes, string(scheme)+"://"+codersdk.VSCodeCoderExtensionID+string(route))
		}
	}
	for _, h := range codersdk.JetBrainsSessionTokenHandlers {
		prefixes = append(prefixes, string(h))
	}
	return prefixes
}()

// sessionTokenHandlerParams returns what follows the first `?` or `#` of the
// handler prefix that appURL starts with. The scheme and authority match
// ASCII case-insensitively.
func sessionTokenHandlerParams(appURL string) (string, bool) {
	for _, prefix := range sessionTokenHandlerPrefixes {
		authority := strings.Index(prefix, "://") + len("://")
		base := authority + strings.IndexAny(prefix[authority:], "/?#")
		if len(appURL) >= len(prefix) && asciiLower(appURL[:base]) == prefix[:base] &&
			strings.HasPrefix(appURL[base:], prefix[base:]) {
			return appURL[strings.IndexAny(prefix, "?#")+1:], true
		}
	}
	return "", false
}

// hasTrustedSessionTokenParams reports whether every name appears once,
// `token` is the placeholder and `url` is the deployment's bare origin.
func hasTrustedSessionTokenParams(query string, deployment *url.URL) bool {
	values, err := url.ParseQuery(query)
	if err != nil {
		return false
	}
	for _, v := range values {
		if len(v) != 1 {
			return false
		}
	}
	dest := values.Get("url")
	u, err := url.Parse(dest)
	return err == nil && bareOriginPattern.MatchString(dest) &&
		values.Get("token") == externalSessionTokenPlaceholder &&
		strings.EqualFold(u.Scheme, deployment.Scheme) && strings.EqualFold(u.Host, deployment.Host)
}

// asciiLower lowercases ASCII letters only, so a Kelvin sign isn't `k`.
func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
}

func (r *RootCmd) openVSCode() *serpent.Command {
	var (
		generateToken bool
		testOpenError bool
	)

	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "vscode <workspace> [<directory in workspace>]",
		Short:       fmt.Sprintf("Open a workspace in %s", vscodeDesktopName),
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			appearanceConfig := initAppearance(ctx, client)

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
			workspace, workspaceAgent, otherWorkspaceAgents, err := GetWorkspaceAndAgent(ctx, inv, client, autostart, workspaceQuery)
			if err != nil {
				return xerrors.Errorf("get workspace and agent: %w", err)
			}

			workspaceName := workspace.Name + "." + workspaceAgent.Name
			insideThisWorkspace := insideAWorkspace && inWorkspaceName == workspaceName

			// To properly work with devcontainers, VS Code has to connect to
			// parent workspace agent. It will then proceed to enter the
			// container given the correct parameters. There is inherently no
			// dependency on the devcontainer agent in this scenario, but
			// relying on it simplifies the logic and ensures the devcontainer
			// is ready. To eliminate the dependency we would need to know that
			// a sub-agent that hasn't been created yet may be a devcontainer,
			// and thus will be created at a later time as well as expose the
			// container folder on the API response.
			var parentWorkspaceAgent codersdk.WorkspaceAgent
			var devcontainer codersdk.WorkspaceAgentDevcontainer
			if workspaceAgent.ParentID.Valid {
				// This is likely a devcontainer agent, so we need to find the
				// parent workspace agent as well as the devcontainer.
				for _, otherAgent := range otherWorkspaceAgents {
					if otherAgent.ID == workspaceAgent.ParentID.UUID {
						parentWorkspaceAgent = otherAgent
						break
					}
				}
				if parentWorkspaceAgent.ID == uuid.Nil {
					return xerrors.Errorf("parent workspace agent %s not found", workspaceAgent.ParentID.UUID)
				}

				printedWaiting := false
				for {
					resp, err := client.WorkspaceAgentListContainers(ctx, parentWorkspaceAgent.ID, nil)
					if err != nil {
						return xerrors.Errorf("list parent workspace agent containers: %w", err)
					}

					for _, dc := range resp.Devcontainers {
						if dc.Agent.ID == workspaceAgent.ID {
							devcontainer = dc
							break
						}
					}
					if devcontainer.ID == uuid.Nil {
						cliui.Warnf(inv.Stderr, "Devcontainer %q not found, opening as a regular workspace...", workspaceAgent.Name)
						parentWorkspaceAgent = codersdk.WorkspaceAgent{} // Reset to empty, so we don't use it later.
						break
					}

					// Precondition, the devcontainer must be running to enter
					// it. Once running, devcontainer.Container will be set.
					if devcontainer.Status == codersdk.WorkspaceAgentDevcontainerStatusRunning {
						break
					}
					if devcontainer.Status != codersdk.WorkspaceAgentDevcontainerStatusStarting {
						return xerrors.Errorf("devcontainer %q is in unexpected status %q, expected %q or %q",
							devcontainer.Name, devcontainer.Status,
							codersdk.WorkspaceAgentDevcontainerStatusRunning,
							codersdk.WorkspaceAgentDevcontainerStatusStarting,
						)
					}

					if !printedWaiting {
						_, _ = fmt.Fprintf(inv.Stderr, "Waiting for devcontainer %q status to change from %q to %q...\n", devcontainer.Name, devcontainer.Status, codersdk.WorkspaceAgentDevcontainerStatusRunning)
						printedWaiting = true
					}
					time.Sleep(5 * time.Second) // Wait a bit before retrying.
				}
			}

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
						return cliui.ErrCanceled
					}
					return xerrors.Errorf("agent: %w", err)
				}

				// The agent will report it's expanded directory before leaving
				// the created state, so we need to wait for that to happen.
				// However, if no directory is set, the expanded directory will
				// not be set either.
				//
				// Note that this is irrelevant for devcontainer sub agents, as
				// they always have a directory set.
				if workspaceAgent.Directory != "" {
					workspace, workspaceAgent, err = waitForAgentCond(ctx, client, workspace, workspaceAgent, func(wa codersdk.WorkspaceAgent) bool {
						return wa.LifecycleState != codersdk.WorkspaceAgentLifecycleCreated
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

			// If we're opening into a dev container, we should use the directory of the dev container.
			workingDirectory := workspaceAgent.ExpandedDirectory
			if workingDirectory == "" && devcontainer.Agent != nil {
				workingDirectory = devcontainer.Agent.Directory
			}

			directory, err = resolveAgentAbsPath(workingDirectory, directory, workspaceAgent.OperatingSystem, insideThisWorkspace)
			if err != nil {
				return xerrors.Errorf("resolve agent path: %w", err)
			}

			var token string
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
				token = apiKey.Key
			}

			var (
				u  *url.URL
				qp url.Values
			)
			if devcontainer.ID != uuid.Nil {
				u, qp = buildVSCodeWorkspaceDevContainerLink(
					token,
					client.URL.String(),
					workspace,
					parentWorkspaceAgent,
					devcontainer.Container.FriendlyName,
					directory,
					devcontainer.WorkspaceFolder,
					devcontainer.ConfigPath,
				)
			} else {
				u, qp = buildVSCodeWorkspaceLink(
					token,
					client.URL.String(),
					workspace,
					workspaceAgent,
					directory,
				)
			}

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

func (r *RootCmd) openApp() *serpent.Command {
	var (
		regionArg     string
		testOpenError bool
	)

	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "app <workspace> <app slug>",
		Short:       "Open a workspace application.",
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			if len(inv.Args) == 0 || len(inv.Args) > 2 {
				return inv.Command.HelpHandler(inv)
			}

			workspaceName := inv.Args[0]
			ws, agt, _, err := GetWorkspaceAndAgent(ctx, inv, client, false, workspaceName)
			if err != nil {
				var sdkErr *codersdk.Error
				if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
					cliui.Errorf(inv.Stderr, "Workspace %q not found!", workspaceName)
					return sdkErr
				}
				cliui.Errorf(inv.Stderr, "Failed to get workspace and agent: %s", err)
				return err
			}

			allAppSlugs := make([]string, len(agt.Apps))
			for i, app := range agt.Apps {
				allAppSlugs[i] = app.Slug
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
			// and path app URL for the region.
			regions, err := client.Regions(ctx)
			if err != nil {
				return xerrors.Errorf("failed to fetch regions: %w", err)
			}
			var region codersdk.Region
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

			baseURL, err := url.Parse(region.PathAppURL)
			if err != nil {
				return xerrors.Errorf("failed to parse proxy URL: %w", err)
			}
			baseURL.Path = ""
			pathAppURL := strings.TrimPrefix(region.PathAppURL, baseURL.String())
			appURL := buildAppLinkURL(baseURL, ws, agt, foundApp, region.WildcardHostname, pathAppURL)

			// Sub-agent apps are registered from inside the workspace and never
			// get the token.
			if foundApp.External && !agt.ParentID.Valid && isSessionTokenDestinationTrusted(appURL, client.URL) {
				appURL = strings.ReplaceAll(appURL, externalSessionTokenPlaceholder, client.SessionToken())
			}

			// Check if we're inside a workspace.  Generally, we know
			// that if we're inside a workspace, `open` can't be used.
			insideAWorkspace := inv.Environ.Get("CODER") == "true"
			if insideAWorkspace {
				_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI on your local machine:\n\n")
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", appURL)
				return nil
			}

			if foundApp.External && strings.Contains(appURL, externalSessionTokenPlaceholder) {
				reason := "This app's URL isn't trusted to receive your session token."
				if agt.ParentID.Valid {
					reason = "This app was registered from inside the workspace rather than from the workspace template."
				}
				cliui.Warnf(inv.Stderr,
					"%s Inspect the URL below carefully and, if you trust the source, substitute the $SESSION_TOKEN placeholder "+
						"with your session token and manually open it:", reason)
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", appURL)
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Opening %s\n", appURL)

			if !testOpenError {
				err = open.Run(appURL)
			} else {
				err = xerrors.New("test.open-error: " + appURL)
			}
			return err
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag: "region",
			Env:  "CODER_OPEN_APP_REGION",
			Description: fmt.Sprintf("Region to use when opening the app." +
				" By default, the app will be opened using the main Coder deployment (a.k.a. \"primary\")."),
			Value:   serpent.StringOf(&regionArg),
			Default: "primary",
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

func buildVSCodeWorkspaceLink(
	token string,
	clientURL string,
	workspace codersdk.Workspace,
	workspaceAgent codersdk.WorkspaceAgent,
	directory string,
) (*url.URL, url.Values) {
	qp := url.Values{}
	qp.Add("url", clientURL)
	qp.Add("owner", workspace.OwnerName)
	qp.Add("workspace", workspace.Name)
	qp.Add("agent", workspaceAgent.Name)

	if directory != "" {
		qp.Add("folder", directory)
	}

	if token != "" {
		qp.Add("token", token)
	}

	return &url.URL{
		Scheme:   "vscode",
		Host:     codersdk.VSCodeCoderExtensionID,
		Path:     "/open",
		RawQuery: qp.Encode(),
	}, qp
}

func buildVSCodeWorkspaceDevContainerLink(
	token string,
	clientURL string,
	workspace codersdk.Workspace,
	workspaceAgent codersdk.WorkspaceAgent,
	containerName string,
	containerFolder string,
	localWorkspaceFolder string,
	localConfigFile string,
) (*url.URL, url.Values) {
	containerFolder = filepath.ToSlash(containerFolder)
	localWorkspaceFolder = filepath.ToSlash(localWorkspaceFolder)
	if localConfigFile != "" {
		localConfigFile = filepath.ToSlash(localConfigFile)
	}

	qp := url.Values{}
	qp.Add("url", clientURL)
	qp.Add("owner", workspace.OwnerName)
	qp.Add("workspace", workspace.Name)
	qp.Add("agent", workspaceAgent.Name)
	qp.Add("devContainerName", containerName)
	qp.Add("devContainerFolder", containerFolder)
	qp.Add("localWorkspaceFolder", localWorkspaceFolder)
	qp.Add("localConfigFile", localConfigFile)

	if token != "" {
		qp.Add("token", token)
	}

	return &url.URL{
		Scheme:   "vscode",
		Host:     codersdk.VSCodeCoderExtensionID,
		Path:     "/openDevContainer",
		RawQuery: qp.Encode(),
	}, qp
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
		workspaceAgent, _, err = getWorkspaceAgent(workspace, workspaceAgent.Name)
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
	if app.Command != "" {
		u.Path = fmt.Sprintf(
			"%s/@%s/%s.%s/terminal",
			preferredPathBase,
			workspace.OwnerName,
			workspace.Name,
			agent.Name,
		)
		q := u.Query()
		q.Set("app", app.Slug)
		u.RawQuery = q.Encode()
	}

	if appsHost != "" && app.Subdomain && app.SubdomainName != "" {
		u.Host = strings.Replace(appsHost, "*", app.SubdomainName, 1)
		u.Path = "/"
	}
	return u.String()
}
