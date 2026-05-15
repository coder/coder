package chattool

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/quartz"
)

const (
	listTemplatesPageSize = 10

	listTemplatesMinPersonalWorkspacesForRecommendation = 2
	listTemplatesMinActiveDevelopersForRecommendation   = 2
	listTemplatesRecentUsageWindow                      = 90 * 24 * time.Hour
)

const (
	listTemplatesHintOnlyAvailable  = "only_available_template"
	listTemplatesHintHighConfidence = "high_confidence_recommendation"
	listTemplatesHintAmbiguous      = "ambiguous_top_matches"
	listTemplatesHintNoConfidence   = "no_confident_match"
)

const (
	queryScoreExactName        = 4
	queryScoreNamePrefix       = 3
	queryScoreNameContains     = 2
	queryScoreDescriptionMatch = 1
)

// ListTemplatesOptions configures the list_templates tool.
type ListTemplatesOptions struct {
	OwnerID            uuid.UUID
	Logger             slog.Logger
	Clock              quartz.Clock
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

type listTemplatesArgs struct {
	Query string `json:"query,omitempty" description:"Optional text to filter templates by name, display name, or description."`
	Page  int    `json:"page,omitempty" description:"Page number for pagination (starts at 1). Each page returns up to 10 ranked templates."`
}

type rankedTemplate struct {
	Template         database.Template
	QueryScore       int
	ActiveDevelopers int64
	Usage            templateUsage
	Rank             int
}

type templateUsage struct {
	WorkspaceCount int64
	LastUsedAt     time.Time
}

type templateRankSignals struct {
	QueryScore         int
	WorkspaceCount     int64
	LastUsedAtUnixNano int64
	ActiveDevelopers   int64
}

// ListTemplates returns a tool that lists available workspace templates.
// The agent uses this to discover templates before creating a workspace.
// Results are ranked before pagination using query relevance, current-user
// usage, and organization-wide popularity.
// db must not be nil.
func ListTemplates(db database.Store, organizationID uuid.UUID, options ListTemplatesOptions) fantasy.AgentTool {
	clock := options.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}

	return fantasy.NewAgentTool(
		"list_templates",
		"List available workspace templates as a ranked shortlist. "+
			"Optionally provide a search query matching template name, "+
			"display name, or description. Use recommended_template_id "+
			"or rank 1 as the default choice when selection_hint is "+
			"only_available_template or high_confidence_recommendation. "+
			"Do not paginate unless the returned templates do not fit the "+
			"request, selection_hint reports ambiguity or no confident match, "+
			"or the user asked to browse templates. Returns 10 per page.",
		func(ctx context.Context, args listTemplatesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			ctx, err := asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			filterParams := database.GetTemplatesWithFilterParams{
				Deleted:        false,
				OrganizationID: organizationID,
				Deprecated: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			}

			var allowlist map[uuid.UUID]bool
			if options.AllowedTemplateIDs != nil {
				allowlist = options.AllowedTemplateIDs()
			}
			if len(allowlist) > 0 {
				filterParams.IDs = slices.Collect(maps.Keys(allowlist))
			}
			templates, err := db.GetTemplatesWithFilter(ctx, filterParams)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			query := strings.TrimSpace(args.Query)
			visibleTemplateCount := len(templates)
			ranked := scoreTemplateCandidates(templates, query)

			templateIDs := make([]uuid.UUID, len(ranked))
			for i, t := range ranked {
				templateIDs[i] = t.Template.ID
			}
			ownerCounts, ownerCountsErr := loadTemplateActiveDeveloperCounts(ctx, db, templateIDs)
			if ownerCountsErr != nil {
				options.Logger.Warn(ctx, "failed to load template active developer counts",
					slog.F("template_count", len(templateIDs)),
					slog.Error(ownerCountsErr),
				)
			}
			usageByTemplate, usageErr := loadTemplateUsage(
				ctx, db, options.OwnerID, organizationID, templateIDs,
			)
			if usageErr != nil {
				options.Logger.Warn(ctx, "failed to load template usage",
					slog.F("owner_id", options.OwnerID),
					slog.F("organization_id", organizationID),
					slog.F("template_count", len(templateIDs)),
					slog.Error(usageErr),
				)
			}

			for i := range ranked {
				ranked[i].ActiveDevelopers = ownerCounts[ranked[i].Template.ID]
				ranked[i].Usage = usageByTemplate[ranked[i].Template.ID]
			}

			rankTemplates(ranked, query)
			selectionHint, recommendedID, recommendationReason := selectTemplateRecommendation(
				ranked,
				visibleTemplateCount,
				errors.Join(ownerCountsErr, usageErr),
				clock.Now(),
			)

			// Paginate.
			page := args.Page
			if page < 1 {
				page = 1
			}
			totalCount := len(ranked)
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
			pageTemplates := ranked[start:end]

			items := make([]map[string]any, 0, len(pageTemplates))
			for _, t := range pageTemplates {
				items = append(items, templateItem(t, recommendedID))
			}

			result := map[string]any{
				"templates":                items,
				"count":                    len(items),
				"page":                     page,
				"total_pages":              totalPages,
				"total_count":              totalCount,
				"available_template_count": visibleTemplateCount,
				"selection_hint":           selectionHint,
				"recommendation_reason":    recommendationReason,
			}
			if recommendedID != uuid.Nil {
				result["recommended_template_id"] = recommendedID.String()
			}
			return toolResponse(result), nil
		},
	)
}

func scoreTemplateCandidates(templates []database.Template, query string) []rankedTemplate {
	candidates := make([]rankedTemplate, 0, len(templates))
	for _, t := range templates {
		queryScore := templateQueryScore(t, query)
		if query != "" && queryScore == 0 {
			continue
		}
		candidates = append(candidates, rankedTemplate{
			Template:   t,
			QueryScore: queryScore,
		})
	}
	return candidates
}

func loadTemplateActiveDeveloperCounts(
	ctx context.Context,
	db database.Store,
	templateIDs []uuid.UUID,
) (map[uuid.UUID]int64, error) {
	ownerCounts := make(map[uuid.UUID]int64)
	if len(templateIDs) == 0 {
		return ownerCounts, nil
	}

	// Templates are already filtered with the owner's permissions. The
	// aggregate count query requires system read because it spans workspace
	// owners, but it only receives IDs the owner can already see.
	rows, err := db.GetWorkspaceUniqueOwnerCountByTemplateIDs(dbauthz.AsSystemRestricted(ctx), templateIDs) //nolint:gocritic // see above
	if err != nil {
		return ownerCounts, err
	}
	for _, row := range rows {
		ownerCounts[row.TemplateID] = row.UniqueOwnersSum
	}
	return ownerCounts, nil
}

func loadTemplateUsage(
	ctx context.Context,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	templateIDs []uuid.UUID,
) (map[uuid.UUID]templateUsage, error) {
	usageByTemplate := make(map[uuid.UUID]templateUsage)
	if ownerID == uuid.Nil || len(templateIDs) == 0 {
		return usageByTemplate, nil
	}

	rows, err := db.GetWorkspaceUsageGroupedByTemplateIDByOwnerID(ctx, database.GetWorkspaceUsageGroupedByTemplateIDByOwnerIDParams{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		TemplateIDs:    templateIDs,
	})
	if err != nil {
		return usageByTemplate, err
	}
	for _, row := range rows {
		usageByTemplate[row.TemplateID] = templateUsage{
			WorkspaceCount: row.WorkspaceCount,
			LastUsedAt:     row.LastUsedAt,
		}
	}
	return usageByTemplate, nil
}

func rankTemplates(ranked []rankedTemplate, query string) {
	slices.SortStableFunc(ranked, func(a, b rankedTemplate) int {
		if c := compareTemplateRankSignals(
			templateRankSignalsFor(a),
			templateRankSignalsFor(b),
			query,
		); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Template.Name, b.Template.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.Template.ID.String(), b.Template.ID.String())
	})

	for i := range ranked {
		ranked[i].Rank = i + 1
	}
}

func templateRankSignalsFor(t rankedTemplate) templateRankSignals {
	return templateRankSignals{
		QueryScore:         t.QueryScore,
		WorkspaceCount:     t.Usage.WorkspaceCount,
		LastUsedAtUnixNano: templateRankTime(t.Usage.LastUsedAt),
		ActiveDevelopers:   t.ActiveDevelopers,
	}
}

func templateRankTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func compareTemplateRankSignals(a, b templateRankSignals, query string) int {
	if query != "" {
		if c := cmp.Compare(b.QueryScore, a.QueryScore); c != 0 {
			return c
		}
	}
	if c := cmp.Compare(b.WorkspaceCount, a.WorkspaceCount); c != 0 {
		return c
	}
	if c := cmp.Compare(b.LastUsedAtUnixNano, a.LastUsedAtUnixNano); c != 0 {
		return c
	}
	return cmp.Compare(b.ActiveDevelopers, a.ActiveDevelopers)
}

func selectTemplateRecommendation(
	ranked []rankedTemplate,
	visibleTemplateCount int,
	rankingSignalsErr error,
	now time.Time,
) (string, uuid.UUID, string) {
	if len(ranked) == 0 {
		return listTemplatesHintNoConfidence, uuid.Nil, "no_matching_templates"
	}

	top := ranked[0]
	if visibleTemplateCount == 1 && len(ranked) == 1 {
		return listTemplatesHintOnlyAvailable, top.Template.ID, "only_available_template"
	}
	if rankingSignalsErr != nil {
		if templateHasDecisiveQuerySignal(ranked) {
			return listTemplatesHintHighConfidence, top.Template.ID, relevanceSignals(top)
		}
		return listTemplatesHintNoConfidence, uuid.Nil, "ranking_signals_unavailable"
	}
	if !templateHasRankingSignal(top) {
		return listTemplatesHintNoConfidence, uuid.Nil, "no_ranking_signal"
	}
	if len(ranked) > 1 && templatesAreAmbiguous(top, ranked[1]) {
		return listTemplatesHintAmbiguous, uuid.Nil, "top_templates_are_ambiguous"
	}
	if !templateHasConfidentRankingSignal(top, now) {
		return listTemplatesHintNoConfidence, uuid.Nil, "weak_ranking_signal"
	}
	return listTemplatesHintHighConfidence, top.Template.ID, relevanceSignals(top)
}

func templatesAreAmbiguous(a, b rankedTemplate) bool {
	return templateRankSignalsFor(a) == templateRankSignalsFor(b)
}

func templateHasRankingSignal(t rankedTemplate) bool {
	signals := templateRankSignalsFor(t)
	return signals.QueryScore > 0 || signals.WorkspaceCount > 0 || signals.ActiveDevelopers > 0
}

func templateHasDecisiveQuerySignal(ranked []rankedTemplate) bool {
	if len(ranked) == 0 || ranked[0].QueryScore == 0 {
		return false
	}
	return len(ranked) == 1 || ranked[0].QueryScore > ranked[1].QueryScore
}

func templateHasConfidentRankingSignal(t rankedTemplate, now time.Time) bool {
	signals := templateRankSignalsFor(t)
	if signals.QueryScore > 0 {
		return true
	}
	if signals.WorkspaceCount >= listTemplatesMinPersonalWorkspacesForRecommendation {
		return true
	}
	if signals.WorkspaceCount > 0 &&
		!t.Usage.LastUsedAt.IsZero() &&
		now.Sub(t.Usage.LastUsedAt) <= listTemplatesRecentUsageWindow {
		return true
	}
	return signals.ActiveDevelopers >= listTemplatesMinActiveDevelopersForRecommendation
}

func templateItem(t rankedTemplate, recommendedID uuid.UUID) map[string]any {
	item := map[string]any{
		"id":                t.Template.ID.String(),
		"name":              t.Template.Name,
		"organization_id":   t.Template.OrganizationID.String(),
		"rank":              t.Rank,
		"relevance_signals": relevanceSignals(t),
	}
	if display := strings.TrimSpace(t.Template.DisplayName); display != "" {
		item["display_name"] = display
	}
	if desc := strings.TrimSpace(t.Template.Description); desc != "" {
		item["description"] = truncateRunes(desc, 200)
	}
	if t.ActiveDevelopers > 0 {
		item["active_developers"] = t.ActiveDevelopers
	}
	if t.Usage.WorkspaceCount > 0 {
		item["your_workspace_count"] = t.Usage.WorkspaceCount
		item["last_used_by_you"] = t.Usage.LastUsedAt.Format(time.RFC3339Nano)
	}
	if t.Template.ID == recommendedID {
		item["recommended"] = true
	}
	return item
}

func relevanceSignals(t rankedTemplate) string {
	signals := templateRankSignalsFor(t)
	switch {
	case signals.QueryScore > 0 && signals.WorkspaceCount > 0:
		return "matches_query_and_used_by_you"
	case signals.QueryScore > 0:
		return "matches_query"
	case signals.WorkspaceCount > 0:
		return "used_by_you"
	case signals.ActiveDevelopers > 0:
		return "popular_in_org"
	default:
		return "ordered_by_name"
	}
}

func templateQueryScore(t database.Template, query string) int {
	query = normalizeTemplateSearch(query)
	if query == "" {
		return 0
	}

	queryCompact := compactTemplateSearch(query)
	for _, field := range []string{t.Name, t.DisplayName} {
		field = normalizeTemplateSearch(field)
		if field == "" {
			continue
		}
		if field == query || compactTemplateSearch(field) == queryCompact {
			return queryScoreExactName
		}
	}
	for _, field := range []string{t.Name, t.DisplayName} {
		field = normalizeTemplateSearch(field)
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, query) || strings.HasPrefix(compactTemplateSearch(field), queryCompact) {
			return queryScoreNamePrefix
		}
	}
	for _, field := range []string{t.Name, t.DisplayName} {
		field = normalizeTemplateSearch(field)
		if field == "" {
			continue
		}
		if strings.Contains(field, query) || strings.Contains(compactTemplateSearch(field), queryCompact) {
			return queryScoreNameContains
		}
	}
	desc := normalizeTemplateSearch(t.Description)
	if strings.Contains(desc, query) || strings.Contains(compactTemplateSearch(desc), queryCompact) {
		return queryScoreDescriptionMatch
	}
	return 0
}

func normalizeTemplateSearch(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

var templateSearchCompactReplacer = strings.NewReplacer(" ", "", "-", "", "_", "")

func compactTemplateSearch(value string) string {
	return templateSearchCompactReplacer.Replace(value)
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
