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

	t.Run("NoTemplatesAvailable", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(nil, 0, nil)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepNoTemplates, next)
	})

	t.Run("QueryFiltersEverything", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(nil, 2, nil)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepNoMatches, next)
	})

	t.Run("OnlyAvailable", func(t *testing.T) {
		t.Parallel()
		only := uuid.New()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{{Template: database.Template{ID: only}}}, 1, loadErr,
		)
		require.Equal(t, only, id)
		require.Equal(t, NextStepUseRecommended, next)
	})

	t.Run("DecisiveQueryRecommendsEvenWithLoadError", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		for _, err := range []error{nil, loadErr} {
			id, next := selectTemplateRecommendation(
				[]rankedTemplate{
					{Template: database.Template{ID: top}, QueryScore: queryScoreExactName},
					{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreDescriptionMatch},
				}, 2, err,
			)
			require.Equal(t, top, id)
			require.Equal(t, NextStepUseRecommended, next)
		}
	})

	t.Run("QueryTieBrokenByAffinityGap", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: top}, QueryScore: queryScoreNamePrefix, AffinityScore: 10, Signals: templateRankingSignals{ActiveCount: 1}},
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0},
			}, 2, nil,
		)
		require.Equal(t, top, id)
		require.Equal(t, NextStepUseRecommended, next)
	})

	t.Run("QueryTieWithSmallGapIsAmbiguous", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0.1},
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0},
			}, 2, nil,
		)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepAskUser, next)
	})

	t.Run("QueryTieWithLoadErrorAsksUser", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
				{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
			}, 2, loadErr,
		)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepAskUser, next)
	})

	t.Run("NoQueryNoSignal", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}},
				{Template: database.Template{ID: uuid.New()}},
			}, 2, nil,
		)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepAskUser, next)
	})

	t.Run("NoQueryWeakSignalBelowFloor", func(t *testing.T) {
		t.Parallel()
		// One active developer scores ln(2), below the ln(3) floor.
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, AffinityScore: math.Log1p(1), Signals: templateRankingSignals{OrgDevs: 1}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 0},
			}, 2, nil,
		)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepAskUser, next)
	})

	t.Run("NoQueryConfidentWhenLeadsRunnerUp", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: top}, AffinityScore: math.Log1p(3), Signals: templateRankingSignals{OrgDevs: 3}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: math.Log1p(1), Signals: templateRankingSignals{OrgDevs: 1}},
			}, 2, nil,
		)
		require.Equal(t, top, id)
		require.Equal(t, NextStepUseRecommended, next)
	})

	t.Run("NoQueryAmbiguousWhenBothClearFloorAndClose", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.20, Signals: templateRankingSignals{OrgDevs: 2}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.15, Signals: templateRankingSignals{OrgDevs: 2}},
			}, 2, nil,
		)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepAskUser, next)
	})

	t.Run("NoQueryConfidentWhenBothClearFloorWithLargeGap", func(t *testing.T) {
		t.Parallel()
		top := uuid.New()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: top}, AffinityScore: 2.0, Signals: templateRankingSignals{OrgDevs: 6}},
				{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.2, Signals: templateRankingSignals{OrgDevs: 2}},
			}, 2, nil,
		)
		require.Equal(t, top, id)
		require.Equal(t, NextStepUseRecommended, next)
	})

	t.Run("NoQueryLoadErrorAsksUser", func(t *testing.T) {
		t.Parallel()
		id, next := selectTemplateRecommendation(
			[]rankedTemplate{
				{Template: database.Template{ID: uuid.New()}, AffinityScore: math.Log1p(3), Signals: templateRankingSignals{OrgDevs: 3}},
				{Template: database.Template{ID: uuid.New()}},
			}, 2, loadErr,
		)
		require.Equal(t, uuid.Nil, id)
		require.Equal(t, NextStepAskUser, next)
	})
}
