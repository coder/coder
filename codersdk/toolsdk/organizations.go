package toolsdk

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

// ListOrganizations lists the authenticated user's visible memberships.
var ListOrganizations = Tool[NoArgs, []codersdk.Organization]{
	Tool: aisdk.Tool{
		Name:        ToolNameListOrganizations,
		Description: "List organizations the authenticated user belongs to, including IDs, names, and display names. Membership does not grant permission to perform every organization-scoped action.",
		Schema: aisdk.Schema{
			Properties: map[string]any{},
			Required:   []string{},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, _ NoArgs) ([]codersdk.Organization, error) {
		organizations, err := deps.coderClient.OrganizationsByUser(ctx, codersdk.Me)
		if err != nil {
			return nil, xerrors.Errorf("list organizations: %w", err)
		}
		if organizations == nil {
			organizations = []codersdk.Organization{}
		}
		return organizations, nil
	},
}

func resolveOrganization(ctx context.Context, deps Deps, organizationID string) (uuid.UUID, error) {
	if organizationID != "" {
		id, err := uuid.Parse(organizationID)
		if err != nil {
			return uuid.Nil, xerrors.New("organization_id must be a valid UUID")
		}
		if id == uuid.Nil {
			return uuid.Nil, xerrors.New("organization_id must be a valid nonzero UUID")
		}
		return id, nil
	}
	me, err := deps.coderClient.User(ctx, codersdk.Me)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("resolve organization: get authenticated user: %w", err)
	}
	switch len(me.OrganizationIDs) {
	case 0:
		return uuid.Nil, xerrors.Errorf("organization_id is required because you belong to no organizations; use %s to inspect organization membership details", ToolNameListOrganizations)
	case 1:
		return me.OrganizationIDs[0], nil
	default:
		return uuid.Nil, xerrors.Errorf("organization_id is required because you belong to multiple organizations; use %s to obtain organization IDs and details", ToolNameListOrganizations)
	}
}
