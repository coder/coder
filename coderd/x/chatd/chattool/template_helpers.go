//go:build !slim

package chattool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// ErrTemplateNotFound reports that a template is either not allowed or not
// visible to the current caller.
var ErrTemplateNotFound = xerrors.New("template not found")

// TemplateListItem describes a template returned by ListTemplatesHelper.
type TemplateListItem struct {
	ID               uuid.UUID
	Name             string
	DisplayName      string
	Description      string
	Icon             string
	OrganizationID   uuid.UUID
	ActiveDevelopers int64
}

// TemplateListResult contains one page of template list results.
type TemplateListResult struct {
	Templates  []TemplateListItem
	TotalCount int
	Page       int
	TotalPages int
}

// TemplateDetail describes a template returned by ReadTemplateHelper.
type TemplateDetail struct {
	ID              uuid.UUID
	Name            string
	DisplayName     string
	Description     string
	Icon            string
	OrganizationID  uuid.UUID
	ActiveVersionID uuid.UUID
}

// TemplateParameterDetail describes a template rich parameter.
type TemplateParameterDetail struct {
	Name            string
	DisplayName     string
	Description     string
	Type            string
	DefaultValue    string
	Required        bool
	Mutable         bool
	Ephemeral       bool
	FormType        string
	Options         json.RawMessage
	ValidationRegex string
	ValidationMin   sql.NullInt32
	ValidationMax   sql.NullInt32
}

// TemplateReadResult contains a template and its configurable parameters.
type TemplateReadResult struct {
	Template   TemplateDetail
	Parameters []TemplateParameterDetail
}

// AsOwner scopes database access to the RBAC actor for the given owner.
//
//nolint:revive // Exported for HTTP handlers while legacy package callers keep asOwner.
func AsOwner(ctx context.Context, db database.Store, ownerID uuid.UUID) (context.Context, error) {
	actor, _, err := httpmw.UserRBACSubject(ctx, db, ownerID, rbac.ScopeAll)
	if err != nil {
		return ctx, xerrors.Errorf("load user authorization: %w", err)
	}
	return dbauthz.As(ctx, actor), nil
}

// IsTemplateAllowed reports whether a template ID is allowed by the resolved
// allowlist. A nil or empty allowlist means all templates are allowed.
//
//nolint:revive // Exported for HTTP handlers while legacy package callers keep isTemplateAllowed.
func IsTemplateAllowed(allowlist map[uuid.UUID]bool, id uuid.UUID) bool {
	if len(allowlist) == 0 {
		return true
	}
	return allowlist[id]
}

// ListTemplatesHelper returns one page of templates filtered for the caller.
func ListTemplatesHelper(
	ctx context.Context,
	db database.Store,
	orgID uuid.UUID,
	allowlist map[uuid.UUID]bool,
	query string,
	page int,
) (TemplateListResult, error) {
	page = normalizeTemplatePage(page)

	filterParams := database.GetTemplatesWithFilterParams{
		Deleted:        false,
		OrganizationID: orgID,
		Deprecated: sql.NullBool{
			Bool:  false,
			Valid: true,
		},
	}
	if query = strings.TrimSpace(query); query != "" {
		filterParams.FuzzyName = query
	}

	if len(allowlist) > 0 {
		filterParams.IDs = allowedTemplateIDs(allowlist)
		if len(filterParams.IDs) == 0 {
			return emptyTemplateListResult(page), nil
		}
	}

	templates, err := db.GetTemplatesWithFilter(ctx, filterParams)
	if err != nil {
		return TemplateListResult{}, err
	}

	items := make([]TemplateListItem, len(templates))
	templateIDs := make([]uuid.UUID, len(templates))
	for i, template := range templates {
		items[i] = TemplateListItem{
			ID:             template.ID,
			Name:           template.Name,
			DisplayName:    template.DisplayName,
			Description:    template.Description,
			Icon:           template.Icon,
			OrganizationID: template.OrganizationID,
		}
		templateIDs[i] = template.ID
	}

	if len(templateIDs) > 0 {
		rows, countErr := db.GetWorkspaceUniqueOwnerCountByTemplateIDs(ctx, templateIDs)
		if countErr == nil {
			ownerCounts := make(map[uuid.UUID]int64, len(rows))
			for _, row := range rows {
				ownerCounts[row.TemplateID] = row.UniqueOwnersSum
			}
			for i := range items {
				items[i].ActiveDevelopers = ownerCounts[items[i].ID]
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ActiveDevelopers > items[j].ActiveDevelopers
	})

	totalCount := len(items)
	totalPages := (totalCount + listTemplatesPageSize - 1) / listTemplatesPageSize
	if totalPages == 0 {
		totalPages = 1
	}

	start := (page - 1) * listTemplatesPageSize
	if start > totalCount {
		start = totalCount
	}
	end := start + listTemplatesPageSize
	if end > totalCount {
		end = totalCount
	}

	pageItems := items[start:end]
	if pageItems == nil {
		pageItems = []TemplateListItem{}
	}

	return TemplateListResult{
		Templates:  pageItems,
		TotalCount: totalCount,
		Page:       page,
		TotalPages: totalPages,
	}, nil
}

// ReadTemplateHelper returns template details and its rich parameters.
func ReadTemplateHelper(
	ctx context.Context,
	db database.Store,
	allowlist map[uuid.UUID]bool,
	templateID uuid.UUID,
) (TemplateReadResult, error) {
	if !IsTemplateAllowed(allowlist, templateID) {
		return TemplateReadResult{}, ErrTemplateNotFound
	}

	template, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TemplateReadResult{}, ErrTemplateNotFound
		}
		return TemplateReadResult{}, err
	}

	params, err := db.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
	if err != nil {
		return TemplateReadResult{}, xerrors.Errorf("failed to get template parameters: %w", err)
	}

	result := TemplateReadResult{
		Template: TemplateDetail{
			ID:              template.ID,
			Name:            template.Name,
			DisplayName:     template.DisplayName,
			Description:     template.Description,
			Icon:            template.Icon,
			OrganizationID:  template.OrganizationID,
			ActiveVersionID: template.ActiveVersionID,
		},
		Parameters: make([]TemplateParameterDetail, 0, len(params)),
	}
	for _, param := range params {
		result.Parameters = append(result.Parameters, TemplateParameterDetail{
			Name:            param.Name,
			DisplayName:     param.DisplayName,
			Description:     param.Description,
			Type:            param.Type,
			DefaultValue:    param.DefaultValue,
			Required:        param.Required,
			Mutable:         param.Mutable,
			Ephemeral:       param.Ephemeral,
			FormType:        string(param.FormType),
			Options:         param.Options,
			ValidationRegex: param.ValidationRegex,
			ValidationMin:   param.ValidationMin,
			ValidationMax:   param.ValidationMax,
		})
	}

	return result, nil
}

func allowedTemplateIDs(allowlist map[uuid.UUID]bool) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(allowlist))
	for id, allowed := range allowlist {
		if allowed {
			ids = append(ids, id)
		}
	}
	return ids
}

func emptyTemplateListResult(page int) TemplateListResult {
	return TemplateListResult{
		Templates:  []TemplateListItem{},
		TotalCount: 0,
		Page:       page,
		TotalPages: 1,
	}
}

func normalizeTemplatePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}
