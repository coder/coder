package codermcp

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ResourceHandler associates a resource or resource template with its handler creation function
type ResourceHandler struct {
	ResourceTemplate mcp.ResourceTemplate
	MakeHandler      func(ToolDeps) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)
}

// RegisterResources registers all resources with the given server.
func RegisterResources(srv *server.MCPServer, deps ToolDeps) {
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			`coder://templates`,
			"Coder Templates",
			mcp.WithTemplateDescription("List of all Coder templates available to the current user"),
			mcp.WithTemplateMIMEType("application/json"),
		), handleCoderTemplates(deps))
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			`coder://user/{id}`,
			"Coder User By ID",
			mcp.WithTemplateDescription("Information about a given Coder user. The string {id} is replaced with the user ID. Alternatively, you can use the pre-defined alias 'me' to get information about the current user."),
			mcp.WithTemplateMIMEType("application/json"),
		), handleCoderUser(deps))
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			`coder://workspaces{?limit,offset,owner}`,
			"Coder Workspaces",
			mcp.WithTemplateDescription("List of Coder workspaces with optional filtering parameters owner, limit, and offset. The owner parameter can be 'me' or a username. The limit and offset parameters are used for pagination. The default values are limit=10, offset=0, and owner=me. Note that only a subset of fields is returned. To get detailed information about a specific workspace, use the Coder Workspace by ID resource."),
			mcp.WithTemplateMIMEType("application/json"),
		), handleCoderWorkspaces(deps))
	srv.AddResourceTemplate(mcp.NewResourceTemplate(
		`coder://workspace/{id}`,
		"Coder Workspace by ID",
		mcp.WithTemplateDescription("Detailed information about a specific Coder workspace by ID. The {id} parameter is replaced with the workspace UUID."),
		mcp.WithTemplateMIMEType("application/json"),
	), handleCoderWorkspaceByID(deps))
}

type handleCoderUserParams struct {
	IDs []string `json:"id"`
}

// handleCoderUser handles requests for the user resource.
func handleCoderUser(deps ToolDeps) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		args, err := unmarshalArgs[handleCoderUserParams](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		if len(args.IDs) < 1 {
			return nil, xerrors.Errorf("missing required {id} argument")
		}
		fetchedUser, err := deps.Client.User(ctx, args.IDs[0])
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch the current user: %w", err)
		}

		data, err := json.Marshal(fetchedUser)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode the current user: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

// handleCoderTemplates handles requests for the templates resource.
func handleCoderTemplates(deps ToolDeps) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		templates, err := deps.Client.Templates(ctx, codersdk.TemplateFilter{})
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch templates: %w", err)
		}

		data, err := json.Marshal(templates)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode templates: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

// handleCoderWorkspaces handles requests for the workspaces resource.
func handleCoderWorkspaces(deps ToolDeps) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		// Unfortunately there appears to be an issue with the uritemplate library
		// when parsing multiple query parameters (e.g. {?foo,bar})
		// See here: https://github.com/yosida95/uritemplate/issues/12
		// Parsing the arguments manually for now.
		u, err := url.Parse(request.Params.URI)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse request URI: %w", err)
		}
		queryParams := u.Query()
		wf := codersdk.WorkspaceFilter{ // Set some sensible defaults
			Owner:  codersdk.Me,
			Offset: 0,
			Limit:  10,
		}
		if limitArg, ok := queryParams["limit"]; ok && len(limitArg) > 0 {
			limit, err := strconv.Atoi(limitArg[0])
			if err != nil {
				return nil, xerrors.Errorf("failed to parse limit: %w", err)
			}
			wf.Limit = limit
		}
		if offsetArg, ok := queryParams["offset"]; ok && len(offsetArg) > 0 {
			offset, err := strconv.Atoi(offsetArg[0])
			if err != nil {
				return nil, xerrors.Errorf("failed to parse offset: %w", err)
			}
			wf.Offset = offset
		}
		if ownerArg, err := queryParams["owner"]; err && len(ownerArg) > 0 {
			wf.Owner = ownerArg[0]
		}

		workspaces, err := deps.Client.Workspaces(ctx, wf)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspaces: %w", err)
		}

		// To reduce the amount of data returned, we only return a subset of the
		// fields of the workspace response.
		reduced := make([]codersdk.ReducedWorkspace, 0, len(workspaces.Workspaces))
		for _, ws := range workspaces.Workspaces {
			reduced = append(reduced, ws.Reduced())
		}

		data, err := json.Marshal(reduced)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode workspaces: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

type handleCoderWorkspaceByIDParams struct {
	IDs []string `json:"id"`
}

// handleCoderWorkspaceByID handles requests for the workspace by ID resource.
func handleCoderWorkspaceByID(deps ToolDeps) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		args, err := unmarshalArgs[handleCoderWorkspaceByIDParams](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		var wsID uuid.UUID
		if len(args.IDs) > 0 {
			wsID, err = uuid.Parse(args.IDs[0])
			if err != nil {
				return nil, xerrors.Errorf("failed to parse workspace ID: %w", err)
			}
		}

		// Fetch the workspace
		workspace, err := deps.Client.Workspace(ctx, wsID)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspace by ID: %w", err)
		}

		data, err := json.Marshal(workspace)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode workspace: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}
