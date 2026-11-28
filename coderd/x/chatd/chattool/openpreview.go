package chattool

import (
	"context"
	"errors"
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
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// OpenPreviewToolName is the name of the open_preview tool. The dashboard
// renders results with this name as preview cards, and chatd would treat a
// built-in call as client-run if a dynamic tool shared the name, so no other
// tool may use it.
const OpenPreviewToolName = "open_preview"

const (
	maxPreviewTitleLength  = 80
	maxHostnameLabelLength = 63
)

// previewKind says whether an open_preview call opens a port or a path.
type previewKind int

const (
	previewKindPort previewKind = iota
	previewKindPath
)

// OpenPreviewResult is the open_preview tool result. Chat messages store it,
// so its JSON keys must stay stable.
type OpenPreviewResult struct {
	Title       string    `json:"title"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentName   string    `json:"agent_name"`
	Port        int       `json:"port,omitempty"`
	PreviewSlug string    `json:"preview_slug,omitempty"`
	Path        string    `json:"path,omitempty"`
	// URLPath is the path and query the preview opens at, such as
	// "/?folder=/home/coder". It is empty for the root.
	URLPath string `json:"url_path,omitempty"`
}

// PreviewWorkspace is the workspace agent a preview opens on.
type PreviewWorkspace struct {
	Conn          workspacesdk.AgentConn
	WorkspaceID   uuid.UUID
	WorkspaceName string
	OwnerID       uuid.UUID
	OwnerUsername string
	Agent         database.WorkspaceAgent
}

// OpenPreviewOptions configures the open_preview tool.
type OpenPreviewOptions struct {
	// PreviewsUnavailable is why the primary deployment serves no previews,
	// or nil. The tool checks only the primary, because the user picks the
	// region when opening the preview.
	PreviewsUnavailable error
	// ChatOwnerID is the chat's owner. Only the workspace owner can open a
	// preview, so the tool refuses another user's workspace.
	ChatOwnerID         uuid.UUID
	GetPreviewWorkspace func(context.Context) (PreviewWorkspace, error)
}

// OpenPreviewArgs are the open_preview tool arguments.
type OpenPreviewArgs struct {
	Title   string `json:"title" description:"Short name for the preview, such as Docs site or Signup form. At most 80 characters."`
	Port    *int   `json:"port,omitempty" description:"Port of a plain HTTP (not HTTPS) server already running in the workspace, between 9 and 65535. Do not combine with path."`
	Path    string `json:"path,omitempty" description:"Absolute workspace path to an .html file or a directory containing index.html. Do not combine with port."`
	URLPath string `json:"url_path,omitempty" description:"Path and query to open the preview at, such as /?folder=/home/coder/project or /docs/index.html. Defaults to /."`
}

type previewRequest struct {
	title   string
	kind    previewKind
	port    int
	path    string
	urlPath string
}

func parseOpenPreviewArgs(args OpenPreviewArgs) (previewRequest, error) {
	req := previewRequest{
		title:   strings.TrimSpace(args.Title),
		path:    strings.TrimSpace(args.Path),
		urlPath: strings.TrimSpace(args.URLPath),
	}
	if req.title == "" {
		return previewRequest{}, xerrors.New("title is required")
	}
	if utf8.RuneCountInString(req.title) > maxPreviewTitleLength {
		return previewRequest{}, xerrors.Errorf("title must be at most %d characters", maxPreviewTitleLength)
	}
	// Invisible and direction-changing characters could make the title
	// read as something else where the user decides to open it.
	if strings.ContainsFunc(req.title, func(r rune) bool {
		return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
	}) {
		return previewRequest{}, xerrors.New("title must not contain control, format, or line and paragraph separator characters")
	}

	switch {
	case args.Port != nil && req.path == "":
		req.kind, req.port = previewKindPort, *args.Port
	case args.Port == nil && req.path != "":
		req.kind = previewKindPath
	default:
		return previewRequest{}, xerrors.New("provide exactly one of port or path")
	}
	if req.kind == previewKindPort && (req.port < workspacesdk.AgentMinimumListeningPort || req.port > 65535) {
		return previewRequest{}, xerrors.Errorf("port must be between %d and 65535", workspacesdk.AgentMinimumListeningPort)
	}

	if req.urlPath == "/" {
		req.urlPath = ""
	}
	if err := previewhost.CheckURLPath(req.urlPath); err != nil {
		return previewRequest{}, err
	}
	return req, nil
}

// OpenPreview returns a tool that opens a workspace preview in the user's
// right panel.
func OpenPreview(options OpenPreviewOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		OpenPreviewToolName,
		"Open a preview for the user in the chat's right panel. Pass a short title and exactly one of port or path. "+
			"Use port for an HTTP server already running in the workspace; this does not start the server or wait for it to be ready. "+
			"Use path for an .html file or a directory containing index.html; the workspace agent serves the files through Coder. "+
			"A file preview serves only that file, so its url_path can add only a query. To include CSS, JavaScript, images, or other pages, open their directory instead. "+
			"Everything in the directory is served, so use a dedicated output directory without secrets. Directory listing is disabled. "+
			"Add url_path to open at a path and query other than /, for example /?folder=/home/coder/project. "+
			"File previews serve the files' current contents and last until the workspace agent restarts; call this again with the same path to restore one. "+
			"Port and path previews may load only from their own address, so a page that loads scripts, styles, or fonts from a CDN or another server breaks; bundle them instead. "+
			"A dev server that checks the Host header, such as Vite, must allow the preview's address (for Vite, server.allowedHosts), or you can build the site and preview the output directory with path instead. "+
			"The user opens the preview from the chat with a click.",
		func(ctx context.Context, args OpenPreviewArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.PreviewsUnavailable != nil {
				return fantasy.NewTextErrorResponse(previewsUnavailableMessage(options.PreviewsUnavailable)), nil
			}
			req, err := parseOpenPreviewArgs(args)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			workspace, err := options.GetPreviewWorkspace(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if workspace.OwnerID != options.ChatOwnerID {
				return fantasy.NewTextErrorResponse("Previews open only for the workspace owner, and this chat's workspace belongs to another user."), nil
			}
			if req.kind == previewKindPort && !slices.Contains(workspace.Agent.DisplayApps, database.DisplayAppPortForwardingHelper) {
				return fantasy.NewTextErrorResponse("this agent's template hides the port forwarding app, so open_preview does not open ports here; " +
					"if the page can be built as static files, preview their folder with path instead, " +
					"or tell the user a template administrator can show port forwarding for this agent"), nil
			}
			// The agent assigns a file's ID when it publishes, so a
			// placeholder of the same length stands in for it here.
			slug := appurl.PreviewID("00000000").Slug()
			if req.kind == previewKindPort {
				slug = appurl.PreviewSlugPrefix + strconv.Itoa(req.port)
			}
			// Sign-in adds the region's wildcard prefix and suffix to this
			// label and checks the full length.
			app := appurl.ApplicationURL{
				AppSlugOrPort: slug,
				AgentName:     workspace.Agent.Name,
				WorkspaceName: workspace.WorkspaceName,
				Username:      workspace.OwnerUsername,
			}
			if len(app.String()) > maxHostnameLabelLength {
				return fantasy.NewTextErrorResponse("The preview address would be longer than DNS allows. Use a shorter workspace or agent name."), nil
			}

			result := OpenPreviewResult{
				Title:       req.title,
				WorkspaceID: workspace.WorkspaceID,
				AgentName:   workspace.Agent.Name,
				URLPath:     req.urlPath,
			}
			switch req.kind {
			case previewKindPort:
				result.Port = req.port
				result.PreviewSlug = slug
			case previewKindPath:
				preview, err := workspace.Conn.OpenPreview(ctx, workspacesdk.OpenPreviewRequest{Path: req.path})
				if errors.Is(err, workspacesdk.ErrPreviewsUnsupported) {
					return fantasy.NewTextErrorResponse("the workspace agent is outdated; restart the workspace to preview files"), nil
				}
				if err != nil {
					return fantasy.NewTextErrorResponse(workspacetools.AgentAPIErrorMessage(err)), nil
				}
				if pathname, _, _ := strings.Cut(req.urlPath, "?"); preview.File && pathname != "" && pathname != "/" {
					return fantasy.NewTextErrorResponse("a file preview serves only its file, so url_path can add only a query, such as /?page=2; to open other pages, preview their directory instead"), nil
				}
				result.PreviewSlug = preview.ID.Slug()
				result.Path = preview.Path
			}
			return marshalToolResponse(result), nil
		},
	)
}

// previewsUnavailableMessage tells the model why the primary deployment
// serves no previews, and what the user can do about it.
func previewsUnavailableMessage(err error) string {
	if errors.Is(err, previewhost.ErrHostPrefixCookieOff) {
		return "previews require --host-prefix-cookie; tell the user an administrator can enable it"
	}
	return "previews need a wildcard app hostname on the primary deployment; tell the user an administrator can set one"
}
