package chattool

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

const listTemplatesPageSize = 10

// ListTemplatesOptions configures the list_templates tool.
type ListTemplatesOptions struct {
	DB      database.Store
	OwnerID uuid.UUID
}

type listTemplatesArgs struct {
	Query string `json:"query,omitempty"`
	Page  int    `json:"page,omitempty"`
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

			// Look up active developer counts so we can sort by popularity.
			templateIDs := make([]uuid.UUID, len(templates))
			for i, t := range templates {
				templateIDs[i] = t.ID
			}
			ownerCounts := make(map[uuid.UUID]int64)
			if len(templateIDs) > 0 {
				rows, countErr := options.DB.GetWorkspaceUniqueOwnerCountByTemplateIDs(ctx, templateIDs)
				if countErr == nil {
					for _, row := range rows {
						ownerCounts[row.TemplateID] = row.UniqueOwnersSum
					}
				}
			}

			// Sort by active developer count descending.
			sort.SliceStable(templates, func(i, j int) bool {
				return ownerCounts[templates[i].ID] > ownerCounts[templates[j].ID]
			})

			// Paginate.
			page := args.Page
			if page < 1 {
				page = 1
			}
			totalCount := len(templates)
			totalPages := (totalCount + listTemplatesPageSize - 1) / listTemplatesPageSize
			if totalPages == 0 {
				totalPages = 1
			}
			start := (page - 1) * listTemplatesPageSize
			end := start + listTemplatesPageSize
			if start > totalCount {
				start = totalCount
			}
			if end > totalCount {
				end = totalCount
			}
			pageTemplates := templates[start:end]

			items := make([]map[string]any, 0, len(pageTemplates))
			for _, t := range pageTemplates {
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
				if count, ok := ownerCounts[t.ID]; ok && count > 0 {
					item["active_developers"] = count
				}
				items = append(items, item)
			}

			return toolResponse(map[string]any{
				"templates":   items,
				"count":       len(items),
				"page":        page,
				"total_pages": totalPages,
				"total_count": totalCount,
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
