package chattool

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	maxPreviewTitleLength   = 80
	maxPreviewURLPathLength = 2048
	// maxHostnameLabelLength is the DNS limit on one hostname label.
	maxHostnameLabelLength = 63
)

// OpenPreviewResult is the open_preview tool result. Chat messages store it,
// so its JSON keys must stay stable.
type OpenPreviewResult struct {
	Title       string    `json:"title"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	AgentName   string    `json:"agent_name"`
	// Exactly one of Port, PreviewID, or AppSlug identifies the target on the
	// agent.
	Port      int32  `json:"port,omitempty"`
	PreviewID string `json:"preview_id,omitempty"`
	AppSlug   string `json:"app_slug,omitempty"`
	// Path is the file or directory a file preview serves.
	Path string `json:"path,omitempty"`
	// URLPath is the path and query the preview opens at, such as
	// "/?folder=/home/coder". It is empty for the root.
	URLPath string `json:"url_path,omitempty"`
}

// PreviewWorkspace is the workspace agent a preview opens on.
type PreviewWorkspace struct {
	Conn          workspacesdk.AgentConn
	WorkspaceID   uuid.UUID
	WorkspaceName string
	OwnerName     string
	Agent         database.WorkspaceAgent
}

// OpenPreviewOptions configures the open_preview tool.
type OpenPreviewOptions struct {
	// AppHostname is the wildcard app hostname previews are served on, such
	// as "*.apps.example.com".
	AppHostname         string
	GetPreviewWorkspace func(context.Context) (PreviewWorkspace, error)
	// DB and OwnerID look up the agent's apps as the chat owner.
	DB      database.Store
	OwnerID uuid.UUID
}

// OpenPreviewArgs are the open_preview tool arguments.
type OpenPreviewArgs struct {
	Title   string `json:"title" description:"Short name for the preview, such as Docs site or Signup form. At most 80 characters."`
	Port    *int   `json:"port,omitempty" description:"Port of a plain HTTP (not HTTPS) server already running in the workspace, between 9 and 65535. Do not combine with path or app."`
	Path    string `json:"path,omitempty" description:"Absolute workspace path to an .html file or a directory containing index.html. Do not combine with port or app."`
	App     string `json:"app,omitempty" description:"Slug of a subdomain coder_app on the workspace agent. Do not combine with port or path."`
	URLPath string `json:"url_path,omitempty" description:"Path and query to open the preview at, such as /?folder=/home/coder/project or /docs/index.html. Defaults to /."`
}

// IsPreviewURLPath reports whether s is a path and optional query that cannot
// change the origin when appended to a preview URL.
func IsPreviewURLPath(s string) bool {
	return len(s) <= maxPreviewURLPathLength &&
		strings.HasPrefix(s, "/") &&
		!strings.HasPrefix(s, "//") &&
		!strings.ContainsAny(s, `\#`) &&
		!strings.ContainsFunc(s, unicode.IsControl)
}

// OpenPreview returns a tool that opens a workspace preview in the user's
// right panel.
func OpenPreview(options OpenPreviewOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"open_preview",
		"Open a preview for the user in the chat's right panel. Pass a short title and exactly one of port, path or app. "+
			"Use port for an HTTP server already running in the workspace; this does not start the server or wait for it to be ready. "+
			"Use path for an .html file or a directory containing index.html; the workspace agent serves the files through Coder. "+
			"A file preview serves only that file. To include CSS, JavaScript, images, or other pages, open their directory instead. "+
			"Everything in the directory is served, so use a dedicated output directory without secrets. Directory listing is disabled. "+
			"Use app to open a subdomain coder_app defined on the workspace agent, such as an IDE. "+
			"Add url_path to open at a path and query other than /, for example /?folder=/home/coder/project. "+
			"File previews serve the files' current contents and last until the workspace agent restarts; call this again to restore one.",
		func(ctx context.Context, args OpenPreviewArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			title := strings.TrimSpace(args.Title)
			if title == "" {
				return fantasy.NewTextErrorResponse("title is required"), nil
			}
			if utf8.RuneCountInString(title) > maxPreviewTitleLength {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("title must be at most %d characters", maxPreviewTitleLength)), nil
			}

			path, app := strings.TrimSpace(args.Path), strings.TrimSpace(args.App)
			hasPort, hasPath, hasApp := args.Port != nil, path != "", app != ""
			targets := 0
			for _, set := range []bool{hasPort, hasPath, hasApp} {
				if set {
					targets++
				}
			}
			if targets != 1 {
				return fantasy.NewTextErrorResponse("provide exactly one of port, path or app"), nil
			}
			if hasPort && (*args.Port < workspacesdk.AgentMinimumListeningPort || *args.Port > 65535) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("port must be between %d and 65535", workspacesdk.AgentMinimumListeningPort)), nil
			}

			urlPath := strings.TrimSpace(args.URLPath)
			if urlPath == "/" {
				// Store "/" as empty so it opens the same tab as no URL path.
				urlPath = ""
			}
			if urlPath != "" && !IsPreviewURLPath(urlPath) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("url_path must start with a single /, contain only a path and query, have no backslashes, fragments or control characters, and be at most %d characters", maxPreviewURLPathLength)), nil
			}

			workspace, err := options.GetPreviewWorkspace(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result := OpenPreviewResult{
				Title:       title,
				WorkspaceID: workspace.WorkspaceID,
				AgentID:     workspace.Agent.ID,
				AgentName:   workspace.Agent.Name,
				URLPath:     urlPath,
			}
			var slug string
			switch {
			case hasPort:
				// The right panel offers port tabs only on agents that show
				// the port forwarding helper.
				if !slices.Contains(workspace.Agent.DisplayApps, database.DisplayAppPortForwardingHelper) {
					return fantasy.NewTextErrorResponse("the workspace agent does not allow port forwarding, so ports cannot be previewed"), nil
				}
				result.Port = int32(*args.Port) //nolint:gosec // Checked against 65535 above.
				slug = strconv.Itoa(*args.Port)
			case hasApp:
				if err := checkOpenableApp(ctx, options, workspace.Agent.ID, app); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				result.AppSlug = app
				slug = app
			default:
				preview, err := workspace.Conn.OpenPreview(ctx, workspacesdk.OpenPreviewRequest{Path: path})
				if err != nil {
					// The publish handler never returns 404, so a 404 means the
					// agent predates the previews route.
					if sdkErr, ok := codersdk.AsError(err); ok && sdkErr.StatusCode() == http.StatusNotFound {
						return fantasy.NewTextErrorResponse("the workspace agent is outdated; restart the workspace to preview files"), nil
					}
					return fantasy.NewTextErrorResponse(workspacetools.AgentAPIErrorMessage(err)), nil
				}
				result.PreviewID = preview.ID
				result.Path = preview.Path
				slug = appurl.PreviewSlug(preview.ID)
			}

			if err := checkHostnameLength(options.AppHostname, workspace, slug); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return marshalToolResponse(result), nil
		},
	)
}

// checkOpenableApp returns an error unless the agent has a subdomain app
// with the given slug that is not hidden, external, or a command app. The
// right panel does not show those in tabs. Path-based apps are excluded
// because they run on the dashboard's origin.
func checkOpenableApp(ctx context.Context, options OpenPreviewOptions, agentID uuid.UUID, slug string) error {
	ctx, err := asOwner(ctx, options.DB, options.OwnerID)
	if err != nil {
		return err
	}
	apps, err := options.DB.GetWorkspaceAppsByAgentID(ctx, agentID)
	if err != nil {
		return xerrors.Errorf("look up workspace apps: %w", err)
	}

	var available []string
	for _, app := range apps {
		if app.Hidden || app.External || app.Command.Valid || !app.Subdomain {
			continue
		}
		if app.Slug == slug {
			return nil
		}
		available = append(available, app.Slug)
	}

	const openableRule = "open_preview only opens subdomain apps that are not hidden, external, or command apps"
	if len(available) == 0 {
		return xerrors.Errorf("app %q cannot be opened: %s, and the workspace agent has none", slug, openableRule)
	}
	return xerrors.Errorf("app %q cannot be opened: %s; apps it can open: %s", slug, openableRule, strings.Join(available, ", "))
}

// checkHostnameLength returns an error when the hostname for slug on the
// workspace agent has a first label longer than DNS allows, because browsers
// cannot load it.
func checkHostnameLength(appHostname string, workspace PreviewWorkspace, slug string) error {
	pattern, _, _ := strings.Cut(appHostname, ".")
	label := strings.Replace(pattern, "*", appurl.ApplicationURL{
		AppSlugOrPort: slug,
		AgentName:     workspace.Agent.Name,
		WorkspaceName: workspace.WorkspaceName,
		Username:      workspace.OwnerName,
	}.String(), 1)
	if len(label) <= maxHostnameLabelLength {
		return nil
	}

	return xerrors.Errorf(
		"the hostname label %q is %d characters, over the DNS limit of %d, so the browser cannot load it. "+
			"Tell the user the agent, workspace, and owner names are too long; a shorter workspace name fixes it",
		label, len(label), maxHostnameLabelLength,
	)
}
