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
	// ListTemplatesReadmeExcerptMaxRunes bounds the README excerpt surfaced
	// per template by list_templates.
	ListTemplatesReadmeExcerptMaxRunes = 1000

	// Minimum active developers before organization popularity alone is a
	// confident recommendation.
	listTemplatesMinActiveDevelopersForRecommendation = 2

	// Affinity ("frecency") parameters: recency-decayed personal usage plus
	// log-scaled organization popularity. The score is computed in Go so the
	// ranking policy and its confidence thresholds live in one place.
	listTemplatesLookbackDays   = 60
	listTemplatesHalfLife       = 14 * 24 * time.Hour
	listTemplatesPersonalWeight = 10.0
	listTemplatesOrgWeight      = 1.0
	listTemplatesDeletedWeight  = 0.5
)

var (
	// Confidence floor: organization popularity alone is confident at the
	// active-developer minimum.
	minConfidentAffinityScore = listTemplatesOrgWeight * math.Log1p(listTemplatesMinActiveDevelopersForRecommendation)

	// Required rank-1 lead over rank 2, derived so "2 developers versus 1"
	// recommends while "16 versus 15" does not.
	minConfidentGap = listTemplatesOrgWeight * (math.Log1p(listTemplatesMinActiveDevelopersForRecommendation) - math.Log1p(listTemplatesMinActiveDevelopersForRecommendation-1))
)

// affinityScoreEpsilon absorbs float rounding at threshold boundaries.
const affinityScoreEpsilon = 1e-9

func affinityScoreAtLeast(score, threshold float64) bool {
	return score >= threshold-affinityScoreEpsilon
}

// NextStepField is the list_templates result field carrying the instruction
// the model should follow next. Tool descriptions and prompts reference it
// by name.
const NextStepField = "next_step"

// Next-step instructions returned with every list_templates result.
const (
	NextStepUseRecommended = "Use recommended_template_id with create_workspace. Call read_template first only if you need parameter or preset details."
	NextStepAskUser        = "Do not call create_workspace yet. Ask the user to choose a template, unless they already named one."
	NextStepNoMatches      = "No templates matched the query. Retry without a query or ask the user."
	NextStepNoTemplates    = "No templates are available to this chat. Inform the user."
)

const (
	queryScoreExactName        = 4
	queryScoreNamePrefix       = 3
	queryScoreNameContains     = 2
	queryScoreDescriptionMatch = 1
)

// ListTemplatesOptions configures the list_templates tool. OwnerID is
// required; Clock defaults to a real clock when nil. AllowedTemplateIDs
// optionally restricts which templates can be returned.
type ListTemplatesOptions struct {
	OwnerID            uuid.UUID
	Logger             slog.Logger
	Clock              quartz.Clock
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

type listTemplatesArgs struct {
	Query string `json:"query,omitempty" description:"Optional text to filter templates by name, display name, or description."`
	Page  int    `json:"page,omitempty" description:"Page number (starts at 1)."`
}

type rankedTemplate struct {
	Template      database.Template
	QueryScore    int
	Signals       templateRankingSignals
	AffinityScore float64
}

// templateRankingSignals holds the per-template ranking inputs returned by
// GetTemplateRankingSignalsByOwnerID.
type templateRankingSignals struct {
	ActiveCount        int64
	DeletedRecentCount int64
	LastUsedAt         time.Time
	OrgDevs            int64
}

// ListTemplates returns a tool that lists workspace templates as a ranked
// shortlist, ordered by query relevance, the user's recent usage, and
// organization popularity. db must not be nil.
func ListTemplates(db database.Store, organizationID uuid.UUID, options ListTemplatesOptions) fantasy.AgentTool {
	clock := options.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}

	return fantasy.NewAgentTool(
		"list_templates",
		"List workspace templates as a ranked shortlist, optionally filtered "+
			"by a query matching template name, display name, or description. "+
			"Each result includes a short README excerpt for routing context; "+
			"call read_template for the full README and parameters. "+
			"Follow the "+NextStepField+" field in the result. Returns 10 per "+
			"page; fetch next_page only when no listed template fits the request.",
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
			recommendedID, nextStep := selectTemplateRecommendation(
				ranked,
				visibleTemplateCount,
				signalsErr,
			)

			page := args.Page
			if page < 1 {
				page = 1
			}
			totalCount := len(ranked)
			start := min((page-1)*listTemplatesPageSize, totalCount)
			end := min(start+listTemplatesPageSize, totalCount)

			items := make([]map[string]any, 0, end-start)
			for _, t := range ranked[start:end] {
				item := templateItem(t)
				// Per-template README fetch: the batched query needs ResourceSystem
				// and would drop excerpts for non-owners, so accept an N+1 bounded
				// by the page size.
				if version, vErr := db.GetTemplateVersionByID(ctx, t.Template.ActiveVersionID); vErr == nil {
					if excerpt := readmeText(version.Readme, ListTemplatesReadmeExcerptMaxRunes); excerpt != "" {
						item["readme_excerpt"] = excerpt
					}
				}
				items = append(items, item)
			}

			result := map[string]any{
				"templates":   items,
				"page":        page,
				NextStepField: nextStep,
			}
			if end < totalCount {
				result["next_page"] = page + 1
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

	// Runs with the owner's permissions; no system escalation. See the
	// dbauthz GetTemplateRankingSignalsByOwnerID authorization notes.
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
// recency-decayed personal usage plus log-scaled organization popularity.
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

// rankTemplates orders by query relevance (when a query is present), then
// affinity score, then name and ID for determinism.
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
}

// selectTemplateRecommendation returns the recommended template (uuid.Nil for
// none) and the next-step instruction. A decisive query match recommends on
// its own; otherwise the affinity score must clear a floor and lead the
// runner-up by a margin.
func selectTemplateRecommendation(
	ranked []rankedTemplate,
	visibleTemplateCount int,
	rankingSignalsErr error,
) (uuid.UUID, string) {
	if len(ranked) == 0 {
		if visibleTemplateCount == 0 {
			return uuid.Nil, NextStepNoTemplates
		}
		return uuid.Nil, NextStepNoMatches
	}

	top := ranked[0]
	if visibleTemplateCount == 1 && len(ranked) == 1 {
		return top.Template.ID, NextStepUseRecommended
	}

	// A decisive query match recommends even when signals failed to load.
	if top.QueryScore > 0 && (len(ranked) == 1 || top.QueryScore > ranked[1].QueryScore) {
		return top.Template.ID, NextStepUseRecommended
	}

	// Beyond a decisive query match, confidence comes from the affinity
	// score, so a failed signal load means asking the user.
	if rankingSignalsErr != nil {
		return uuid.Nil, NextStepAskUser
	}

	// Query tie: break it with a clear affinity gap.
	if top.QueryScore > 0 {
		if len(ranked) > 1 && affinityScoreAtLeast(top.AffinityScore-ranked[1].AffinityScore, minConfidentGap) {
			return top.Template.ID, NextStepUseRecommended
		}
		return uuid.Nil, NextStepAskUser
	}

	// No query: the affinity score alone decides.
	if !affinityScoreAtLeast(top.AffinityScore, minConfidentAffinityScore) {
		return uuid.Nil, NextStepAskUser
	}
	if len(ranked) > 1 &&
		affinityScoreAtLeast(ranked[1].AffinityScore, minConfidentAffinityScore) &&
		!affinityScoreAtLeast(top.AffinityScore-ranked[1].AffinityScore, minConfidentGap) {
		return uuid.Nil, NextStepAskUser
	}
	return top.Template.ID, NextStepUseRecommended
}

func templateItem(t rankedTemplate) map[string]any {
	item := map[string]any{
		"id":   t.Template.ID.String(),
		"name": t.Template.Name,
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
	if !t.Signals.LastUsedAt.IsZero() {
		item["last_used_by_you"] = t.Signals.LastUsedAt.Format(time.RFC3339Nano)
	}
	return item
}

func templateQueryScore(t database.Template, query string) int {
	query = normalizeTemplateSearch(query)
	queryCompact := compactTemplateSearch(query)
	if query == "" || queryCompact == "" {
		return 0
	}

	best := 0
	for _, field := range []string{t.Name, t.DisplayName} {
		best = max(best, nameQueryScore(field, query, queryCompact))
	}
	if best > 0 {
		return best
	}
	desc := normalizeTemplateSearch(t.Description)
	if strings.Contains(desc, query) || strings.Contains(compactTemplateSearch(desc), queryCompact) {
		return queryScoreDescriptionMatch
	}
	return 0
}

// nameQueryScore returns the relevance tier of a single name-like field:
// exact match outranks prefix match, which outranks substring match.
func nameQueryScore(field, query, queryCompact string) int {
	field = normalizeTemplateSearch(field)
	if field == "" {
		return 0
	}
	fieldCompact := compactTemplateSearch(field)
	switch {
	case field == query || fieldCompact == queryCompact:
		return queryScoreExactName
	case strings.HasPrefix(field, query) || strings.HasPrefix(fieldCompact, queryCompact):
		return queryScoreNamePrefix
	case strings.Contains(field, query) || strings.Contains(fieldCompact, queryCompact):
		return queryScoreNameContains
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

// asOwner sets up a dbauthz context scoped to what the owner can access.
func asOwner(ctx context.Context, db database.Store, ownerID uuid.UUID) (context.Context, error) {
	actor, _, err := httpmw.UserRBACSubject(ctx, db, ownerID, rbac.ScopeAll)
	if err != nil {
		return ctx, xerrors.Errorf("load user authorization: %w", err)
	}
	return dbauthz.As(ctx, actor), nil
}
