package chattool

import (
	"cmp"
	"context"
	"database/sql"
	"maps"
	"math"
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

	// listTemplatesMinActiveDevelopersForRecommendation is the organization
	// popularity floor: a template needs at least this many active developers
	// before organization popularity on its own is a confident recommendation.
	listTemplatesMinActiveDevelopersForRecommendation = 2

	// The following constants parameterize the affinity score, a "frecency"
	// signal (frequency discounted by recency). The personal term is the count
	// of the user's recent workspaces (active plus a fraction of
	// recently-deleted) multiplied by a recency decay; the organization term is
	// a log-scaled active-developer count. Only the ratio of the personal to
	// organization weight matters. They are deliberately explicit so the
	// ranking can be calibrated as ranking-quality signal accrues.
	//
	// The score is computed in Go (computeAffinityScore) rather than SQL
	// because sqlc cannot reliably compile the parameterized decay expression;
	// see GetTemplateRankingSignalsByOwnerID. Keeping the score and the
	// confidence thresholds in the same place also avoids Postgres-versus-Go
	// floating-point differences at confidence boundaries.
	listTemplatesLookbackDays   = 60
	listTemplatesHalfLife       = 14 * 24 * time.Hour
	listTemplatesPersonalWeight = 10.0
	listTemplatesOrgWeight      = 1.0
	listTemplatesDeletedWeight  = 0.5
)

var (
	// minConfidentAffinityScore preserves today's floor: organization
	// popularity alone is confident once a template reaches the active-developer
	// minimum. math.Log1p(n) == ln(1+n) is exactly the organization term of the
	// affinity score, so the threshold and the score stay float-consistent.
	minConfidentAffinityScore = listTemplatesOrgWeight * math.Log1p(listTemplatesMinActiveDevelopersForRecommendation)

	// minConfidentGap requires rank 1 to lead rank 2 by at least the score
	// difference between "min" and "min-1" active developers before
	// recommending when both clear the floor. It is derived, not tuned, so
	// "2 developers versus 1" still recommends while "16 versus 15" does not.
	minConfidentGap = listTemplatesOrgWeight * (math.Log1p(listTemplatesMinActiveDevelopersForRecommendation) - math.Log1p(listTemplatesMinActiveDevelopersForRecommendation-1))
)

// affinityScoreEpsilon absorbs floating-point rounding so a score sitting
// exactly on a threshold boundary counts as meeting it.
const affinityScoreEpsilon = 1e-9

// affinityScoreAtLeast reports whether score meets threshold within the
// comparison epsilon.
func affinityScoreAtLeast(score, threshold float64) bool {
	return score >= threshold-affinityScoreEpsilon
}

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

// ListTemplatesOptions configures the list_templates tool. OwnerID is required.
// Logger may be zero-valued; Clock defaults to a real clock when nil.
// AllowedTemplateIDs optionally restricts which templates can be returned.
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
	Template      database.Template
	QueryScore    int
	Signals       templateRankingSignals
	AffinityScore float64
	Rank          int
}

// templateRankingSignals holds the raw, per-template ranking inputs returned by
// GetTemplateRankingSignalsByOwnerID. ActiveCount and DeletedRecentCount are the
// user's in-window workspace counts; LastUsedAt is the most recent usage within
// the window (zero when there is none); OrgDevs is the count of distinct active
// developers in the organization.
type templateRankingSignals struct {
	ActiveCount        int64
	DeletedRecentCount int64
	LastUsedAt         time.Time
	OrgDevs            int64
}

// hasPersonalUsage reports whether the user used the template within the
// lookback window, counting recently-deleted workspaces so deleted history is
// still treated as personal usage.
func (s templateRankingSignals) hasPersonalUsage() bool {
	return s.ActiveCount+s.DeletedRecentCount > 0
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
			"If user_selection_required is true, or selection_hint is "+
			"no_confident_match or ambiguous_top_matches, do not call "+
			"create_workspace. Ask the user to choose a template unless "+
			"the user already explicitly selected one. "+
			"Do not paginate unless the returned templates do not fit the "+
			"request, selection_hint reports ambiguity or no confident match, "+
			"or the user asked to browse templates. Returns 10 per page.",
		func(ctx context.Context, args listTemplatesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			ctx, err := asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("authorize list_templates owner: %w", err).Error()), nil
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
			now := clock.Now()
			signalsByTemplate, signalsErr := loadTemplateRankingSignals(
				ctx, db, options.OwnerID, organizationID, templateIDs, now,
			)
			if signalsErr != nil {
				options.Logger.Warn(ctx, "failed to load template ranking signals",
					slog.F("owner_id", options.OwnerID),
					slog.F("organization_id", organizationID),
					slog.F("template_count", len(templateIDs)),
					slog.Error(signalsErr),
				)
			}

			for i := range ranked {
				ranked[i].Signals = signalsByTemplate[ranked[i].Template.ID]
				ranked[i].AffinityScore = computeAffinityScore(ranked[i].Signals, now)
			}

			rankTemplates(ranked, query)
			selectionHint, recommendedID, recommendationReason := selectTemplateRecommendation(
				ranked,
				visibleTemplateCount,
				signalsErr,
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
				"user_selection_required":  userSelectionRequired(selectionHint),
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

func loadTemplateRankingSignals(
	ctx context.Context,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	templateIDs []uuid.UUID,
	now time.Time,
) (map[uuid.UUID]templateRankingSignals, error) {
	signals := make(map[uuid.UUID]templateRankingSignals)
	if len(templateIDs) == 0 {
		return signals, nil
	}

	// The templates were already authorized with the owner's permissions by
	// GetTemplatesWithFilter. GetTemplateRankingSignalsByOwnerID authorizes the
	// owner reading their own workspaces plus a template-metadata read for the
	// cross-user popularity count, so no system escalation is needed here.
	rows, err := db.GetTemplateRankingSignalsByOwnerID(ctx, database.GetTemplateRankingSignalsByOwnerIDParams{
		TemplateIDs:     templateIDs,
		OwnerID:         ownerID,
		OrganizationID:  organizationID,
		PrebuildsUserID: database.PrebuildsSystemUserID,
		LookbackCutoff:  now.Add(-listTemplatesLookbackDays * 24 * time.Hour),
	})
	if err != nil {
		return signals, err
	}
	for _, row := range rows {
		s := templateRankingSignals{
			ActiveCount:        row.ActiveCount,
			DeletedRecentCount: row.DeletedRecentCount,
			OrgDevs:            row.OrgDevs,
		}
		if row.LastUsedAt.Valid {
			s.LastUsedAt = row.LastUsedAt.Time
		}
		signals[row.TemplateID] = s
	}
	return signals, nil
}

// computeAffinityScore folds the raw signals into a single "frecency" score:
// the personal workspace count (active plus a fraction of recently-deleted)
// multiplied by a recency decay, plus a log-scaled organization-popularity
// term. When the user has no in-window usage the personal term is zero and the
// score collapses to organization popularity.
func computeAffinityScore(s templateRankingSignals, now time.Time) float64 {
	personal := 0.0
	if !s.LastUsedAt.IsZero() {
		count := float64(s.ActiveCount) + listTemplatesDeletedWeight*float64(s.DeletedRecentCount)
		age := now.Sub(s.LastUsedAt)
		if age < 0 {
			age = 0
		}
		decay := math.Pow(0.5, float64(age)/float64(listTemplatesHalfLife))
		personal = listTemplatesPersonalWeight * count * decay
	}
	org := listTemplatesOrgWeight * math.Log1p(float64(s.OrgDevs))
	return personal + org
}

// rankTemplates orders templates by query relevance first (only when a query is
// present), then by affinity score, with template name and ID as deterministic
// tiebreakers.
func rankTemplates(ranked []rankedTemplate, query string) {
	slices.SortStableFunc(ranked, func(a, b rankedTemplate) int {
		if query != "" {
			if c := cmp.Compare(b.QueryScore, a.QueryScore); c != 0 {
				return c
			}
		}
		if c := cmp.Compare(b.AffinityScore, a.AffinityScore); c != 0 {
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

// selectTemplateRecommendation decides whether to recommend the top-ranked
// template or ask the user to choose. Query relevance is the primary signal: a
// decisive query match recommends on its own. Otherwise confidence comes from
// the affinity score, which must clear a floor and lead the runner-up by a
// margin before recommending.
func selectTemplateRecommendation(
	ranked []rankedTemplate,
	visibleTemplateCount int,
	rankingSignalsErr error,
) (string, uuid.UUID, string) {
	if len(ranked) == 0 {
		return listTemplatesHintNoConfidence, uuid.Nil, "no_matching_templates"
	}

	top := ranked[0]
	if visibleTemplateCount == 1 && len(ranked) == 1 {
		return listTemplatesHintOnlyAvailable, top.Template.ID, "only_available_template"
	}

	// A decisive query match (strictly outscoring the runner-up, or the only
	// match) is a confident recommendation on its own, even when the affinity
	// signals failed to load.
	if top.QueryScore > 0 && (len(ranked) == 1 || top.QueryScore > ranked[1].QueryScore) {
		return listTemplatesHintHighConfidence, top.Template.ID, relevanceSignals(top)
	}

	// Without a decisive query tier the affinity score decides confidence, so an
	// unreliable (failed) signal load means we must ask the user.
	if rankingSignalsErr != nil {
		return listTemplatesHintNoConfidence, uuid.Nil, "ranking_signals_unavailable"
	}

	// Query present but the top two tie on relevance: break the tie with the
	// affinity score when the gap is clear, otherwise ask the user.
	if top.QueryScore > 0 {
		if len(ranked) > 1 && affinityScoreAtLeast(top.AffinityScore-ranked[1].AffinityScore, minConfidentGap) {
			return listTemplatesHintHighConfidence, top.Template.ID, relevanceSignals(top)
		}
		return listTemplatesHintAmbiguous, uuid.Nil, "top_templates_are_ambiguous"
	}

	// No query: recommend purely on the affinity score.
	if !affinityScoreAtLeast(top.AffinityScore, minConfidentAffinityScore) {
		if top.AffinityScore <= 0 {
			return listTemplatesHintNoConfidence, uuid.Nil, "no_ranking_signal"
		}
		return listTemplatesHintNoConfidence, uuid.Nil, "weak_ranking_signal"
	}
	if len(ranked) > 1 &&
		affinityScoreAtLeast(ranked[1].AffinityScore, minConfidentAffinityScore) &&
		!affinityScoreAtLeast(top.AffinityScore-ranked[1].AffinityScore, minConfidentGap) {
		return listTemplatesHintAmbiguous, uuid.Nil, "top_templates_are_ambiguous"
	}
	return listTemplatesHintHighConfidence, top.Template.ID, relevanceSignals(top)
}

func userSelectionRequired(selectionHint string) bool {
	return selectionHint == listTemplatesHintAmbiguous || selectionHint == listTemplatesHintNoConfidence
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
	if t.Signals.OrgDevs > 0 {
		item["active_developers"] = t.Signals.OrgDevs
	}
	if t.Signals.ActiveCount > 0 {
		item["your_workspace_count"] = t.Signals.ActiveCount
	}
	if t.Signals.DeletedRecentCount > 0 {
		item["your_recently_deleted_workspace_count"] = t.Signals.DeletedRecentCount
	}
	if t.Signals.hasPersonalUsage() && !t.Signals.LastUsedAt.IsZero() {
		item["last_used_by_you"] = t.Signals.LastUsedAt.Format(time.RFC3339Nano)
	}
	if t.Template.ID == recommendedID {
		item["recommended"] = true
	}
	return item
}

func relevanceSignals(t rankedTemplate) string {
	hasQuery := t.QueryScore > 0
	hasPersonal := t.Signals.hasPersonalUsage()
	switch {
	case hasQuery && hasPersonal:
		return "matches_query_and_used_by_you"
	case hasQuery:
		return "matches_query"
	case hasPersonal:
		return "used_by_you"
	case t.Signals.OrgDevs > 0:
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
	if queryCompact == "" {
		return 0
	}
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
