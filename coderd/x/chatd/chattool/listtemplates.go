//go:build !slim

package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

const listTemplatesPageSize = 10

// ListTemplatesOptions configures the list_templates tool.
type ListTemplatesOptions struct {
	DB                 database.Store
	OwnerID            uuid.UUID
	OrganizationID     uuid.UUID
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

type listTemplatesArgs struct {
	Query string `json:"query,omitempty" description:"Optional text to filter templates by name or description."`
	Page  int    `json:"page,omitempty" description:"Page number for pagination (starts at 1). Each page returns up to 10 templates."`
}

// ListTemplates returns a tool that lists available workspace templates.
// The agent uses this to discover templates before creating a workspace.
// Results are ordered by number of active developers (most popular first)
// and paginated at 10 per page.
func ListTemplates(options ListTemplatesOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"list_templates",
		"List available workspace templates. Optionally filter by a "+
			"search query matching template name or description. "+
			"Use this to find a template before creating a workspace. "+
			"Results are ordered by number of active developers (most popular first). "+
			"Returns 10 per page. Use the page parameter to paginate through results.",
		func(ctx context.Context, args listTemplatesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.DB == nil {
				return fantasy.NewTextErrorResponse("database is not configured"), nil
			}

			var allowlist map[uuid.UUID]bool
			if options.AllowedTemplateIDs != nil {
				allowlist = options.AllowedTemplateIDs()
			}

			ctx, err := AsOwner(ctx, options.DB, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result, err := ListTemplatesHelper(
				ctx,
				options.DB,
				options.OrganizationID,
				allowlist,
				args.Query,
				args.Page,
			)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			items := make([]map[string]any, 0, len(result.Templates))
			for _, template := range result.Templates {
				item := map[string]any{
					"id":              template.ID.String(),
					"name":            template.Name,
					"organization_id": template.OrganizationID.String(),
				}
				if display := strings.TrimSpace(template.DisplayName); display != "" {
					item["display_name"] = display
				}
				if desc := strings.TrimSpace(template.Description); desc != "" {
					item["description"] = truncateRunes(desc, 200)
				}
				if template.ActiveDevelopers > 0 {
					item["active_developers"] = template.ActiveDevelopers
				}
				items = append(items, item)
			}

			return toolResponse(map[string]any{
				"templates":   items,
				"count":       len(items),
				"page":        result.Page,
				"total_pages": result.TotalPages,
				"total_count": result.TotalCount,
			}), nil
		},
	)
}

// asOwner sets up a dbauthz context for the given owner so that
// subsequent database calls are scoped to what that user can access.
//
//nolint:revive // Legacy wrapper name maintained for existing callers.
func asOwner(ctx context.Context, db database.Store, ownerID uuid.UUID) (context.Context, error) {
	return AsOwner(ctx, db, ownerID)
}
