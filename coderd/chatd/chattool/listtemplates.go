package chattool

import (
	"context"
	"database/sql"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// ListTemplatesOptions configures the list_templates tool.
type ListTemplatesOptions struct {
	DB      database.Store
	OwnerID uuid.UUID
}

type listTemplatesArgs struct {
	Query string `json:"query,omitempty"`
}

// ListTemplates returns a tool that lists available workspace templates.
// The agent uses this to discover templates before creating a workspace.
func ListTemplates(options ListTemplatesOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"list_templates",
		"List available workspace templates. Optionally filter by a "+
			"search query matching template name or description. "+
			"Use this to find a template before creating a workspace.",
		func(ctx context.Context, args listTemplatesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.DB == nil {
				return fantasy.NewTextErrorResponse("database is not configured"), nil
			}

			ctx, err := asOwner(ctx, options.DB, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			filterParams := database.GetTemplatesWithFilterParams{
				Deleted: false,
				Deprecated: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			}
			query := strings.TrimSpace(args.Query)
			if query != "" {
				filterParams.FuzzyName = query
			}

			templates, err := options.DB.GetTemplatesWithFilter(ctx, filterParams)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			items := make([]map[string]any, 0, len(templates))
			for _, t := range templates {
				item := map[string]any{
					"id":   t.ID.String(),
					"name": t.Name,
				}
				if display := strings.TrimSpace(t.DisplayName); display != "" {
					item["display_name"] = display
				}
				if desc := strings.TrimSpace(t.Description); desc != "" {
					item["description"] = truncateRunes(desc, 200)
				}
				items = append(items, item)
			}

			return toolResponse(map[string]any{
				"templates": items,
				"count":     len(items),
			}), nil
		},
	)
}

// asOwner sets up a dbauthz context for the given owner so that
// subsequent database calls are scoped to what that user can access.
func asOwner(ctx context.Context, db database.Store, ownerID uuid.UUID) (context.Context, error) {
	actor, _, err := httpmw.UserRBACSubject(ctx, db, ownerID, rbac.ScopeAll)
	if err != nil {
		return ctx, xerrors.Errorf("load user authorization: %w", err)
	}
	return dbauthz.As(ctx, actor), nil
}
