package chattool

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
// trusts its results, so no other tool source may use the name.
const OpenPreviewToolName = "open_preview"

const maxPreviewTitleLength = 80

// OpenPreviewKind names the kind of target an open_preview result opens.
type OpenPreviewKind string

const (
	OpenPreviewKindPort OpenPreviewKind = "port"
	OpenPreviewKindFile OpenPreviewKind = "file"
	OpenPreviewKindApp  OpenPreviewKind = "app"
)

// OpenPreviewResult is the open_preview tool result. Chat messages store it,
// so its JSON keys must stay stable.
type OpenPreviewResult struct {
	// Kind says which of Port, PreviewSlug, or AppSlug identifies the target
	// on the agent.
	Kind        OpenPreviewKind `json:"kind"`
	Title       string          `json:"title"`
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	AgentName   string          `json:"agent_name"`
	Port        int             `json:"port,omitempty"`
	PreviewSlug string          `json:"preview_slug,omitempty"`
	AppSlug     string          `json:"app_slug,omitempty"`
	Path        string          `json:"path,omitempty"`
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
	// Hosting is how the primary serves previews. Nil means the deployment
	// has no wildcard app hostname, so nothing can be previewed.
	Hosting             *previewhost.Hosting
	GetPreviewWorkspace func(context.Context) (PreviewWorkspace, error)
	DB                  database.Store
	OwnerID             uuid.UUID
}

// OpenPreviewArgs are the open_preview tool arguments.
type OpenPreviewArgs struct {
	Title   string `json:"title" description:"Short name for the preview, such as Docs site or Signup form. At most 80 characters."`
	Port    *int   `json:"port,omitempty" description:"Port of a plain HTTP (not HTTPS) server already running in the workspace, between 9 and 65535. Do not combine with path or app."`
	Path    string `json:"path,omitempty" description:"Absolute workspace path to an .html file or a directory containing index.html. Do not combine with port or app."`
	App     string `json:"app,omitempty" description:"Slug of a subdomain coder_app on the workspace agent. Do not combine with port or path."`
	URLPath string `json:"url_path,omitempty" description:"Path and query to open the preview at, such as /?folder=/home/coder/project or /docs/index.html. Defaults to /."`
}

type previewRequest struct {
	title   string
	kind    OpenPreviewKind
	port    int
	path    string
	app     string
	urlPath string
}

func parseOpenPreviewArgs(args OpenPreviewArgs) (previewRequest, error) {
	req := previewRequest{
		title:   strings.TrimSpace(args.Title),
		path:    strings.TrimSpace(args.Path),
		app:     strings.TrimSpace(args.App),
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
	case args.Port != nil && req.path == "" && req.app == "":
		req.kind, req.port = OpenPreviewKindPort, *args.Port
	case args.Port == nil && req.path != "" && req.app == "":
		req.kind = OpenPreviewKindFile
	case args.Port == nil && req.path == "" && req.app != "":
		req.kind = OpenPreviewKindApp
	default:
		return previewRequest{}, xerrors.New("provide exactly one of port, path or app")
	}
	if req.kind == OpenPreviewKindPort && (req.port < workspacesdk.AgentMinimumListeningPort || req.port > 65535) {
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
		"Open a preview for the user in the chat's right panel. Pass a short title and exactly one of port, path or app. "+
			"Use port for an HTTP server already running in the workspace; this does not start the server or wait for it to be ready. "+
			"Use path for an .html file or a directory containing index.html; the workspace agent serves the files through Coder. "+
			"A file preview serves only that file, so its url_path can add only a query. To include CSS, JavaScript, images, or other pages, open their directory instead. "+
			"Everything in the directory is served, so use a dedicated output directory without secrets. Directory listing is disabled. "+
			"Use app to open a subdomain coder_app defined on the workspace agent, such as an IDE. "+
			"Add url_path to open at a path and query other than /, for example /?folder=/home/coder/project. "+
			"File previews serve the files' current contents and last until the workspace agent restarts; call this again with the same path to restore one. "+
			"Port and path previews may load only from their own address, so a page that loads scripts, styles, or fonts from a CDN or another server breaks; bundle them instead. "+
			"The user opens the preview from the chat with a click.",
		func(ctx context.Context, args OpenPreviewArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.Hosting == nil {
				return fantasy.NewTextErrorResponse("previews need a wildcard app hostname, which this deployment does not have; tell the user an administrator can set one"), nil
			}
			req, err := parseOpenPreviewArgs(args)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			workspace, err := options.GetPreviewWorkspace(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result := OpenPreviewResult{
				Kind:        req.kind,
				Title:       req.title,
				WorkspaceID: workspace.WorkspaceID,
				AgentName:   workspace.Agent.Name,
				URLPath:     req.urlPath,
			}
			var slug string
			switch req.kind {
			case OpenPreviewKindPort:
				if !slices.Contains(workspace.Agent.DisplayApps, database.DisplayAppPortForwardingHelper) {
					return fantasy.NewTextErrorResponse("the workspace agent does not allow port forwarding, so ports cannot be previewed"), nil
				}
				result.Port = req.port
				//nolint:gosec // parseOpenPreviewArgs bounds the port to 65535.
				result.PreviewSlug = appurl.PreviewTarget{Port: uint16(req.port)}.Slug()
				slug = result.PreviewSlug
			case OpenPreviewKindApp:
				if err := checkOpenableApp(ctx, options, workspace.Agent.ID, req.app); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				result.AppSlug = req.app
				slug = req.app
			case OpenPreviewKindFile:
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
				slug = result.PreviewSlug
			}

			_, err = options.Hosting.URL(appurl.ApplicationURL{
				AppSlugOrPort: slug,
				AgentName:     workspace.Agent.Name,
				WorkspaceName: workspace.WorkspaceName,
				Username:      workspace.OwnerName,
			}, "")
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("%s, so the browser cannot load it. Tell the user the agent, workspace, and owner names are too long; a shorter workspace name fixes it", err)), nil
			}
			return marshalToolResponse(result), nil
		},
	)
}

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
