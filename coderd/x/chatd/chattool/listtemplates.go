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

// listTemplatesRankingVersion identifies the ranking policy (scoring formula
// and confidence thresholds) for telemetry. Bump it whenever the policy
// changes so recorded decisions can be segmented by version.
const listTemplatesRankingVersion = 1

// list_templates outcomes are the label values for
// list_templates_outcome_total and the "outcome" field of the decision log.
const (
	listTemplatesOutcomeRecommended = "recommended"
	listTemplatesOutcomeAskUser     = "ask_user"
	listTemplatesOutcomeNoMatches   = "no_matches"
	listTemplatesOutcomeNoTemplates = "no_templates"
	// listTemplatesOutcomeUnknown is a defensive bucket for a next-step value
	// that listTemplatesOutcome does not map. It should never appear in
	// practice; an increment signals a new NextStep* constant that was not
	// wired into the outcome mapping.
	listTemplatesOutcomeUnknown = "unknown"
)

// Recommendation reasons explain which branch produced the outcome. They are
// recorded in the decision log (not as a metric label) so the "why" survives
// without reconstructing it from the raw scores.
const (
	recommendationReasonNoTemplates        = "no_templates"
	recommendationReasonNoMatches          = "no_matches"
	recommendationReasonOnlyAvailable      = "only_available_template"
	recommendationReasonDecisiveQuery      = "decisive_query_match"
	recommendationReasonSignalsUnavailable = "signals_unavailable"
	recommendationReasonQueryTieConfident  = "query_tie_broken_by_affinity"
	recommendationReasonQueryTieAmbiguous  = "ambiguous_query_tie"
	recommendationReasonAffinityConfident  = "affinity_confident"
	recommendationReasonAffinityLow        = "affinity_below_floor"
	recommendationReasonAffinityAmbiguous  = "ambiguous_affinity"
)

// ListTemplatesMetrics records list_templates ranking telemetry. It is
// implemented by *chatloop.Metrics and declared here (rather than imported)
// because chatloop imports chattool, so chattool must not import chatloop.
type ListTemplatesMetrics interface {
	RecordListTemplatesOutcome(outcome string)
	RecordListTemplatesSignalsFailure()
	RecordListTemplatesAffinityGap(recommended bool, gap float64)
}

// ListTemplatesOptions configures the list_templates tool. OwnerID is
// required; Clock defaults to a real clock when nil. AllowedTemplateIDs
// optionally restricts which templates can be returned. ChatID, Metrics, and
// Recommendations are optional telemetry hooks: ChatID correlates a result
// with a later create_workspace call, Metrics records aggregate ranking
// outcomes, and Recommendations remembers the result for follow-up
// classification.
type ListTemplatesOptions struct {
	OwnerID            uuid.UUID
	Logger             slog.Logger
	Clock              quartz.Clock
	AllowedTemplateIDs func() map[uuid.UUID]bool
	ChatID             uuid.UUID
	Metrics            ListTemplatesMetrics
	Recommendations    *RecommendationTracker
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
			"Follow the "+NextStepField+" field in the result. Returns 10 per "+
			"page; fetch next_page only when no listed template fits the request.",
		func(ctx context.Context, args listTemplatesArgs, toolCall fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
			recommendedID, nextStep, reason := selectTemplateRecommendation(
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
			pageTemplateIDs := make([]uuid.UUID, 0, end-start)
			for _, t := range ranked[start:end] {
				items = append(items, templateItem(t))
				pageTemplateIDs = append(pageTemplateIDs, t.Template.ID)
			}

			recordListTemplatesTelemetry(ctx, options, toolCall.ID, organizationID, listTemplatesTelemetry{
				query:                query,
				page:                 page,
				visibleTemplateCount: visibleTemplateCount,
				candidateCount:       totalCount,
				returnedCount:        len(items),
				ranked:               ranked,
				recommendedID:        recommendedID,
				nextStep:             nextStep,
				reason:               reason,
				signalsErr:           signalsErr,
			})
			options.Recommendations.Record(options.ChatID, recommendedID, pageTemplateIDs, page)

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
// none), the next-step instruction, and a reason identifying which branch
// decided the outcome (for telemetry). A decisive query match recommends on
// its own; otherwise the affinity score must clear a floor and lead the
// runner-up by a margin.
func selectTemplateRecommendation(
	ranked []rankedTemplate,
	visibleTemplateCount int,
	rankingSignalsErr error,
) (recommendedID uuid.UUID, nextStep string, reason string) {
	if len(ranked) == 0 {
		if visibleTemplateCount == 0 {
			return uuid.Nil, NextStepNoTemplates, recommendationReasonNoTemplates
		}
		return uuid.Nil, NextStepNoMatches, recommendationReasonNoMatches
	}

	top := ranked[0]
	if visibleTemplateCount == 1 && len(ranked) == 1 {
		return top.Template.ID, NextStepUseRecommended, recommendationReasonOnlyAvailable
	}

	// A decisive query match recommends even when signals failed to load.
	if top.QueryScore > 0 && (len(ranked) == 1 || top.QueryScore > ranked[1].QueryScore) {
		return top.Template.ID, NextStepUseRecommended, recommendationReasonDecisiveQuery
	}

	// Beyond a decisive query match, confidence comes from the affinity
	// score, so a failed signal load means asking the user.
	if rankingSignalsErr != nil {
		return uuid.Nil, NextStepAskUser, recommendationReasonSignalsUnavailable
	}

	// Query tie: both candidates matched the query at the same relevance tier,
	// so the query itself is the baseline confidence signal and affinity only
	// breaks the tie. A clear affinity gap is enough here; unlike the no-query
	// branch below, the top score need not clear minConfidentAffinityScore on
	// its own.
	if top.QueryScore > 0 {
		if len(ranked) > 1 && affinityScoreAtLeast(top.AffinityScore-ranked[1].AffinityScore, minConfidentGap) {
			return top.Template.ID, NextStepUseRecommended, recommendationReasonQueryTieConfident
		}
		return uuid.Nil, NextStepAskUser, recommendationReasonQueryTieAmbiguous
	}

	// No query: the affinity score alone decides.
	if !affinityScoreAtLeast(top.AffinityScore, minConfidentAffinityScore) {
		return uuid.Nil, NextStepAskUser, recommendationReasonAffinityLow
	}
	if len(ranked) > 1 &&
		affinityScoreAtLeast(ranked[1].AffinityScore, minConfidentAffinityScore) &&
		!affinityScoreAtLeast(top.AffinityScore-ranked[1].AffinityScore, minConfidentGap) {
		return uuid.Nil, NextStepAskUser, recommendationReasonAffinityAmbiguous
	}
	return top.Template.ID, NextStepUseRecommended, recommendationReasonAffinityConfident
}

// listTemplatesOutcome maps a next-step instruction to its telemetry outcome.
func listTemplatesOutcome(nextStep string) string {
	switch nextStep {
	case NextStepNoTemplates:
		return listTemplatesOutcomeNoTemplates
	case NextStepNoMatches:
		return listTemplatesOutcomeNoMatches
	case NextStepUseRecommended:
		return listTemplatesOutcomeRecommended
	case NextStepAskUser:
		return listTemplatesOutcomeAskUser
	default:
		return listTemplatesOutcomeUnknown
	}
}

// listTemplatesTelemetry carries the data recorded for one list_templates call:
// aggregate ranking metrics plus the inputs for a structured decision log.
type listTemplatesTelemetry struct {
	query                string
	page                 int
	visibleTemplateCount int
	candidateCount       int
	returnedCount        int
	ranked               []rankedTemplate
	recommendedID        uuid.UUID
	nextStep             string
	reason               string
	signalsErr           error
}

// recordListTemplatesTelemetry records the aggregate ranking metrics and emits
// the structured decision log. The raw user query text is never logged: only
// its presence and length are, to avoid leaking task content. The affinity
// score is not included in the tool result shown to the model; the log records
// the inputs (scores, gap, thresholds) so a decision is reconstructable.
func recordListTemplatesTelemetry(
	ctx context.Context,
	options ListTemplatesOptions,
	toolCallID string,
	organizationID uuid.UUID,
	t listTemplatesTelemetry,
) {
	outcome := listTemplatesOutcome(t.nextStep)
	recommended := t.recommendedID != uuid.Nil

	if options.Metrics != nil {
		options.Metrics.RecordListTemplatesOutcome(outcome)
		if t.signalsErr != nil {
			options.Metrics.RecordListTemplatesSignalsFailure()
		}
		// The affinity gap is only meaningful when affinity is the deciding
		// signal: no query, or the top two share the same query tier. In those
		// cases the sort guarantees a non-negative gap. A failed signal load
		// leaves every affinity score at its zero default and forces an
		// ask_user outcome, so recording the gap then would only add
		// meaningless zero samples; the signals-failure counter covers it.
		if t.signalsErr == nil && len(t.ranked) > 1 {
			top, runner := t.ranked[0], t.ranked[1]
			if t.query == "" || top.QueryScore == runner.QueryScore {
				options.Metrics.RecordListTemplatesAffinityGap(recommended, top.AffinityScore-runner.AffinityScore)
			}
		}
	}

	fields := []slog.Field{
		slog.F("tool_call_id", toolCallID),
		slog.F("chat_id", options.ChatID),
		slog.F("owner_id", options.OwnerID),
		slog.F("organization_id", organizationID),
		slog.F("query_present", t.query != ""),
		slog.F("query_len", len(t.query)),
		slog.F("page", t.page),
		slog.F("visible_template_count", t.visibleTemplateCount),
		slog.F("candidate_count", t.candidateCount),
		slog.F("returned_count", t.returnedCount),
		slog.F("outcome", outcome),
		slog.F("recommendation_reason", t.reason),
		slog.F("signals_load_failed", t.signalsErr != nil),
		slog.F("ranking_version", listTemplatesRankingVersion),
		slog.F("min_confident_affinity_score", minConfidentAffinityScore),
		slog.F("min_confident_gap", minConfidentGap),
	}
	if recommended {
		fields = append(fields, slog.F("recommended_template_id", t.recommendedID))
	}
	if len(t.ranked) > 0 {
		top := t.ranked[0]
		fields = append(fields,
			slog.F("top_template_id", top.Template.ID),
			slog.F("top_query_score", top.QueryScore),
			slog.F("top_affinity_score", top.AffinityScore),
		)
	}
	if len(t.ranked) > 1 {
		runner := t.ranked[1]
		fields = append(fields,
			slog.F("runner_up_query_score", runner.QueryScore),
			slog.F("runner_up_affinity_score", runner.AffinityScore),
			slog.F("affinity_gap", t.ranked[0].AffinityScore-runner.AffinityScore),
		)
	}
	options.Logger.Info(ctx, "list_templates decision", fields...)
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
