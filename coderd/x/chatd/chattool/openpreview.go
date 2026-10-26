package chattool

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	maxPreviewTitleLength   = 80
	maxPreviewURLPathLength = 2048
)

// OpenPreviewResult is the open_preview tool result. Chat messages store it,
// so its JSON keys must stay stable.
type OpenPreviewResult struct {
	OK          bool      `json:"ok"`
	Title       string    `json:"title"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentName   string    `json:"agent_name"`
	// Exactly one of Port or PreviewID identifies the preview on the agent.
	Port      int32  `json:"port,omitempty"`
	PreviewID string `json:"preview_id,omitempty"`
	// Path is the file or directory a file preview serves, with symlinks resolved.
	Path string `json:"path,omitempty"`
	// URLPath is the path and query a port preview opens at, such as
	// "/?folder=/home/coder". It is empty for the server's root.
	URLPath string `json:"url_path,omitempty"`
}

// PreviewWorkspace is the workspace agent a preview opens on.
type PreviewWorkspace struct {
	Conn          workspacesdk.AgentConn
	WorkspaceID   uuid.UUID
	WorkspaceName string
	OwnerName     string
	AgentName     string
}

// OpenPreviewOptions configures the open_preview tool.
type OpenPreviewOptions struct {
	// AppHostname is the wildcard app hostname previews are served on, such
	// as "*.apps.example.com".
	AppHostname         string
	GetPreviewWorkspace func(context.Context) (PreviewWorkspace, error)
}

// OpenPreviewArgs are the open_preview tool arguments.
type OpenPreviewArgs struct {
	Title   string `json:"title" description:"Short name for the preview shown in the user's preview selector, such as Docs site or Signup form. At most 80 characters."`
	Port    *int   `json:"port,omitempty" description:"Port of an existing plain HTTP (not HTTPS) server, between 1000 and 65535. Do not combine with path."`
	Path    string `json:"path,omitempty" description:"Absolute workspace path to an .html file or a directory containing index.html. Do not combine with port."`
	URLPath string `json:"url_path,omitempty" description:"Path and query to open on the port's server, such as /?folder=/home/coder/project. Only with port. Defaults to /."`
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

// OpenPreview returns a tool that offers a workspace preview to the user.
func OpenPreview(options OpenPreviewOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"open_preview",
		"Offer an embedded preview to the user, who opens it from the chat. Pass a short title and exactly one of port or path. "+
			"Use port for an existing HTTP web app; this does not start the app or wait for it to be ready. "+
			"Add url_path to open the app at a path and query other than /, for example /?folder=/home/coder/project. "+
			"Use path for an .html file or a directory containing index.html; the workspace agent publishes the files through Coder. "+
			"A file publishes only that file. To include CSS, JavaScript, images, or other pages, publish their directory instead. "+
			"Directory contents are published, so use a dedicated output directory without secrets. Directory listing is disabled. "+
			"Call this again to refresh after changing static files, or to restore a preview after its server or the workspace agent restarts. "+
			"File previews last until the workspace agent stops, and at most 64 paths can be published until it restarts.",
		func(ctx context.Context, args OpenPreviewArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			args.Title = strings.TrimSpace(args.Title)
			if args.Title == "" {
				return fantasy.NewTextErrorResponse("title is required"), nil
			}
			if utf8.RuneCountInString(args.Title) > maxPreviewTitleLength {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("title must be at most %d characters", maxPreviewTitleLength)), nil
			}

			args.Path = strings.TrimSpace(args.Path)
			hasPort, hasPath := args.Port != nil, args.Path != ""
			if hasPort == hasPath {
				return fantasy.NewTextErrorResponse("provide exactly one of port or path"), nil
			}

			var port int32
			if hasPort {
				if *args.Port < appurl.PreviewMinimumPort || *args.Port > 65535 {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("port must be between %d and 65535", appurl.PreviewMinimumPort)), nil
				}
				port = int32(*args.Port)
			}

			args.URLPath = strings.TrimSpace(args.URLPath)
			if args.URLPath == "/" {
				// Store "/" as empty so it is listed as the same preview as no
				// URL path.
				args.URLPath = ""
			}
			if args.URLPath != "" && !hasPort {
				return fantasy.NewTextErrorResponse("url_path can only be used with port"), nil
			}
			if args.URLPath != "" && !IsPreviewURLPath(args.URLPath) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("url_path must start with a single /, contain only a path and query, have no backslashes, fragments or control characters, and be at most %d characters", maxPreviewURLPathLength)), nil
			}

			if options.GetPreviewWorkspace == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			workspace, err := options.GetPreviewWorkspace(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result := OpenPreviewResult{
				OK:          true,
				Title:       args.Title,
				WorkspaceID: workspace.WorkspaceID,
				AgentName:   workspace.AgentName,
			}
			if hasPort {
				if err := checkHostnameLength(options.AppHostname, workspace, fmt.Sprintf("preview--%d", port)); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				result.Port = port
				result.URLPath = args.URLPath
				return marshalToolResponse(result), nil
			}

			preview, err := workspace.Conn.OpenPreview(ctx, workspacesdk.OpenPreviewRequest{Path: args.Path})
			if err != nil {
				// The publish handler never returns 404, so a 404 means the agent
				// predates the previews route.
				if sdkErr, ok := codersdk.AsError(err); ok && sdkErr.StatusCode() == http.StatusNotFound {
					return fantasy.NewTextErrorResponse("the workspace agent is outdated; restart the workspace to open file previews"), nil
				}
				return fantasy.NewTextErrorResponse(workspacetools.AgentAPIErrorMessage(err)), nil
			}

			if err := checkHostnameLength(options.AppHostname, workspace, "preview--"+preview.ID); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result.PreviewID = preview.ID
			result.Path = preview.Path
			return marshalToolResponse(result), nil
		},
	)
}

// maxHostnameLabelLength is the DNS limit on one hostname label.
const maxHostnameLabelLength = 63

// checkHostnameLength returns an error when the hostname for slug on the
// workspace agent has a first label longer than DNS allows, because browsers
// cannot load it.
func checkHostnameLength(appHostname string, workspace PreviewWorkspace, slug string) error {
	pattern, _, _ := strings.Cut(appHostname, ".")
	label := strings.Replace(pattern, "*", appurl.ApplicationURL{
		AppSlugOrPort: slug,
		AgentName:     workspace.AgentName,
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
