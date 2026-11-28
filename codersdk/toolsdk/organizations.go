package toolsdk

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk/tooldef"
)

// ListOrganizations lists the authenticated user's visible memberships.
var ListOrganizations = Tool[NoArgs, []codersdk.Organization]{
	Tool: tooldef.Tool{
		Name:        ToolNameListOrganizations,
		Description: "List organizations the authenticated user belongs to, including IDs, names, and display names. Membership does not grant permission to perform every organization-scoped action.",
		Schema: tooldef.Schema{
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

// organizationArgDescription describes the optional organization_id argument
// that every organization-scoped tool resolves with resolveOrganization.
const organizationArgDescription = "Organization UUID or name. Optional when you belong to exactly one organization; required when you belong to several. Use " + ToolNameListOrganizations + " to discover organizations."

// resolveOrganization returns the organization named by organization, which
// may be a UUID or an organization name. When organization is empty it
// defaults to the caller's only organization and errors, listing the
// candidates, when the caller belongs to several. Explicit UUIDs are not
// checked here; the API authorizes the subsequent organization-scoped call.
func resolveOrganization(ctx context.Context, deps Deps, organization string) (uuid.UUID, error) {
	if organization != "" {
		if id, err := uuid.Parse(organization); err == nil {
			if id == uuid.Nil {
				return uuid.Nil, xerrors.New("organization_id must be a valid nonzero UUID")
			}
			return id, nil
		}
		// Validate before using the value as a URL path segment.
		if err := codersdk.NameValid(organization); err != nil {
			return uuid.Nil, xerrors.Errorf("organization_id must be an organization UUID or name: %w", err)
		}
		org, err := deps.coderClient.OrganizationByName(ctx, organization)
		if err != nil {
			return uuid.Nil, xerrors.Errorf("resolve organization %q: %w", organization, err)
		}
		return org.ID, nil
	}
	organizations, err := deps.coderClient.OrganizationsByUser(ctx, codersdk.Me)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("resolve organization: list your organizations: %w", err)
	}
	switch len(organizations) {
	case 0:
		return uuid.Nil, xerrors.Errorf("organization_id is required because you belong to no organizations; use %s to inspect organization membership details", ToolNameListOrganizations)
	case 1:
		return organizations[0].ID, nil
	default:
		choices := make([]string, 0, len(organizations))
		for _, org := range organizations {
			choices = append(choices, fmt.Sprintf("%s (%s)", org.Name, org.ID))
		}
		slices.Sort(choices)
		return uuid.Nil, xerrors.Errorf("organization_id is required because you belong to multiple organizations: %s; pass the intended organization's name or ID as organization_id, or use %s for organization details", strings.Join(choices, ", "), ToolNameListOrganizations)
	}
}

// resolveOwningOrganization returns owner, the organization of an existing
// resource referenced by argName, after checking that an explicit
// organization matches it. The organization-scoped template endpoints do not
// verify that referenced resources belong to the path organization.
func resolveOwningOrganization(ctx context.Context, deps Deps, organization string, owner uuid.UUID, argName string) (uuid.UUID, error) {
	if organization == "" {
		return owner, nil
	}
	id, err := resolveOrganization(ctx, deps, organization)
	if err != nil {
		return uuid.Nil, err
	}
	if id != owner {
		return uuid.Nil, xerrors.Errorf("%s belongs to organization %s, not organization_id %q; omit organization_id to use the organization of %s", argName, owner, organization, argName)
	}
	return owner, nil
}
