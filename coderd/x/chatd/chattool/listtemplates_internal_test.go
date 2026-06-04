package chattool

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

func TestComputeAffinityScore(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

	// No signals at all scores zero.
	require.Zero(t, computeAffinityScore(templateRankingSignals{}, now))

	// With no personal usage the score collapses to the log-scaled org term.
	orgOnly := computeAffinityScore(templateRankingSignals{OrgDevs: 3}, now)
	require.InDelta(t, listTemplatesOrgWeight*math.Log1p(3), orgOnly, 1e-9)

	// Org popularity is monotonic in the developer count.
	require.Greater(t,
		computeAffinityScore(templateRankingSignals{OrgDevs: 3}, now),
		computeAffinityScore(templateRankingSignals{OrgDevs: 1}, now),
	)

	// Recency decay: the same usage counts more when it is more recent.
	recent := computeAffinityScore(templateRankingSignals{ActiveCount: 2, LastUsedAt: now.Add(-1 * 24 * time.Hour)}, now)
	stale := computeAffinityScore(templateRankingSignals{ActiveCount: 2, LastUsedAt: now.Add(-30 * 24 * time.Hour)}, now)
	require.Greater(t, recent, stale)

	// Deleted workspaces contribute at reduced weight, so the same number of
	// active workspaces outscores deleted ones.
	last := now.Add(-1 * time.Hour)
	activeOnly := computeAffinityScore(templateRankingSignals{ActiveCount: 2, LastUsedAt: last}, now)
	deletedOnly := computeAffinityScore(templateRankingSignals{DeletedRecentCount: 2, LastUsedAt: last}, now)
	require.Greater(t, activeOnly, deletedOnly)
	require.Greater(t, deletedOnly, 0.0)

	// A future last_used_at clamps the age to zero rather than amplifying.
	future := computeAffinityScore(templateRankingSignals{ActiveCount: 1, LastUsedAt: now.Add(time.Hour)}, now)
	atNow := computeAffinityScore(templateRankingSignals{ActiveCount: 1, LastUsedAt: now}, now)
	require.InDelta(t, atNow, future, 1e-9)
}

func TestSelectTemplateRecommendation(t *testing.T) {
	t.Parallel()

	loadErr := xerrors.New("signals failed to load")

	t.Run("NoMatches", func(t *testing.T) {
		t.Parallel()
		hint, id, reason := selectTemplateRecommendation(nil, 0, nil)
		require.Equal(t, listTemplatesHintNoConfidence, hint)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, "no_matching_templates", reason)
	})

	t.Run("OnlyAvailable", func(t *testing.T) {
		t.Parallel()
		only := uuid.New()
		hint, id, reason := selectTemplateRecommendation(
			[]rankedTemplate{{Template: database.Template{ID: only}}}, 1, loadErr,
		)
		require.Equal(t, listTemplatesHintOnlyAvailable, hint)
		require.Equal(t, only, id)
		require.Equal(t, "only_available_template", reason)
	})

	t.Run("DecisiveQueryRecommendsEvenWithLoadError", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		for _, err := range []error{nil, loadErr} {
			hint, id, reason := selectTemplateRecommendation(
				[]rankedTemplate{
					{Template: database.Template{ID: top}, QueryScore: queryScoreExactName},
					{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreDescriptionMatch},
				}, 2, err,
			)
			require.Equal(t, listTemplatesHintHighConfidence, hint)
			require.Equal(t, top, id)
			require.Equal(t, "matches_query", reason)
		}
	})

	t.Run("QueryTieBrokenByAffinityGap", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		hint, id, reason := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: top}, QueryScore: queryScoreNamePrefix, AffinityScore: 10, Signals: templateRankingSignals{ActiveCount: 1}},
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0},
			}, 2, nil,
		)
		require.Equal(t, listTemplatesHintHighConfidence, hint)
		require.Equal(t, top, id)
		require.Equal(t, "matches_query_and_used_by_you", reason)
	})

	t.Run("QueryTieWithSmallGapIsAmbiguous", func(t *testing.T) {
		t.Parallel()
		hint, id, _ := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0.1},
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0},
			}, 2, nil,
		)
		require.Equal(t, listTemplatesHintAmbiguous, hint)
		require.Equal(t, uuid.Nil, id)
	})

	t.Run("QueryTieWithLoadErrorIsUnavailable", func(t *testing.T) {
		t.Parallel()
		hint, id, reason := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
			}, 2, loadErr,
		)
		require.Equal(t, listTemplatesHintNoConfidence, hint)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, "ranking_signals_unavailable", reason)
	})

	t.Run("NoQueryNoSignal", func(t *testing.T) {
		t.Parallel()
		hint, _, reason := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}},
				{Template: database.Template{ID: uuid.New()}},
			}, 2, nil,
		)
		require.Equal(t, listTemplatesHintNoConfidence, hint)
		require.Equal(t, "no_ranking_signal", reason)
	})

	t.Run("NoQueryWeakSignalBelowFloor", func(t *testing.T) {
		t.Parallel()
		// One active developer scores ln(2), below the ln(3) floor.
		hint, _, reason := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, AffinityScore: math.Log1p(1), Signals: templateRankingSignals{OrgDevs: 1}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 0},
			}, 2, nil,
		)
		require.Equal(t, listTemplatesHintNoConfidence, hint)
		require.Equal(t, "weak_ranking_signal", reason)
	})

	t.Run("NoQueryConfidentWhenLeadsRunnerUp", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		hint, id, reason := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: top}, AffinityScore: math.Log1p(3), Signals: templateRankingSignals{OrgDevs: 3}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: math.Log1p(1), Signals: templateRankingSignals{OrgDevs: 1}},
			}, 2, nil,
		)
		require.Equal(t, listTemplatesHintHighConfidence, hint)
		require.Equal(t, top, id)
		require.Equal(t, "popular_in_org", reason)
	})

	t.Run("NoQueryAmbiguousWhenBothClearFloorAndClose", func(t *testing.T) {
		t.Parallel()
		hint, id, _ := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.20, Signals: templateRankingSignals{OrgDevs: 2}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.15, Signals: templateRankingSignals{OrgDevs: 2}},
			}, 2, nil,
		)
		require.Equal(t, listTemplatesHintAmbiguous, hint)
		require.Equal(t, uuid.Nil, id)
	})

	t.Run("NoQueryLoadErrorIsUnavailable", func(t *testing.T) {
		t.Parallel()
		hint, id, reason := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, AffinityScore: math.Log1p(3), Signals: templateRankingSignals{OrgDevs: 3}},
				{Template: database.Template{ID: uuid.New()}},
			}, 2, loadErr,
		)
		require.Equal(t, listTemplatesHintNoConfidence, hint)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, "ranking_signals_unavailable", reason)
	})
}
