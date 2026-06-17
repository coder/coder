package chattool

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type gapObservation struct {
	recommended bool
	gap         float64
}

type fakeListTemplatesMetrics struct {
	outcomes        []string
	signalsFailures int
	gaps            []gapObservation
}

func (m *fakeListTemplatesMetrics) RecordListTemplatesOutcome(outcome string) {
	m.outcomes = append(m.outcomes, outcome)
}

func (m *fakeListTemplatesMetrics) RecordListTemplatesSignalsFailure() {
	m.signalsFailures++
}

func (m *fakeListTemplatesMetrics) RecordListTemplatesAffinityGap(recommended bool, gap float64) {
	m.gaps = append(m.gaps, gapObservation{recommended: recommended, gap: gap})
}

func rankedWith(queryScore int, affinity float64) rankedTemplate {
	return rankedTemplate{
		Template:      database.Template{ID: uuid.New()},
		QueryScore:    queryScore,
		AffinityScore: affinity,
	}
}

func TestRecordListTemplatesTelemetry_Metrics(t *testing.T) {
	t.Parallel()

	t.Run("OutcomeAndSignalsFailure", func(t *testing.T) {
		t.Parallel()
		m := &fakeListTemplatesMetrics{}
		recordListTemplatesTelemetry(context.Background(), ListTemplatesOptions{Metrics: m}, "tc", uuid.New(), listTemplatesTelemetry{
			ranked:     []rankedTemplate{rankedWith(0, 0)},
			nextStep:   NextStepAskUser,
			reason:     recommendationReasonAffinityLow,
			signalsErr: xerrors.New("boom"),
		})
		require.Equal(t, []string{listTemplatesOutcomeAskUser}, m.outcomes)
		require.Equal(t, 1, m.signalsFailures)
	})

	t.Run("GapRecordedWhenNoQuery", func(t *testing.T) {
		t.Parallel()
		m := &fakeListTemplatesMetrics{}
		recordListTemplatesTelemetry(context.Background(), ListTemplatesOptions{Metrics: m}, "tc", uuid.New(), listTemplatesTelemetry{
			ranked:        []rankedTemplate{rankedWith(0, 5), rankedWith(0, 2)},
			recommendedID: uuid.New(),
			nextStep:      NextStepUseRecommended,
		})
		require.Len(t, m.gaps, 1)
		require.True(t, m.gaps[0].recommended)
		require.InDelta(t, 3.0, m.gaps[0].gap, 1e-9)
		require.Zero(t, m.signalsFailures)
	})

	t.Run("GapRecordedWhenEqualQueryTier", func(t *testing.T) {
		t.Parallel()
		m := &fakeListTemplatesMetrics{}
		recordListTemplatesTelemetry(context.Background(), ListTemplatesOptions{Metrics: m}, "tc", uuid.New(), listTemplatesTelemetry{
			query:    "py",
			ranked:   []rankedTemplate{rankedWith(queryScoreNamePrefix, 5), rankedWith(queryScoreNamePrefix, 1)},
			nextStep: NextStepAskUser,
		})
		require.Len(t, m.gaps, 1)
		require.False(t, m.gaps[0].recommended)
		require.InDelta(t, 4.0, m.gaps[0].gap, 1e-9)
	})

	t.Run("GapSkippedWhenQueryTierDecides", func(t *testing.T) {
		t.Parallel()
		m := &fakeListTemplatesMetrics{}
		// Different query tiers: the order is decided by relevance, not
		// affinity, so the affinity gap would be misleading.
		recordListTemplatesTelemetry(context.Background(), ListTemplatesOptions{Metrics: m}, "tc", uuid.New(), listTemplatesTelemetry{
			query:         "py",
			ranked:        []rankedTemplate{rankedWith(queryScoreExactName, 0), rankedWith(queryScoreDescriptionMatch, 9)},
			recommendedID: uuid.New(),
			nextStep:      NextStepUseRecommended,
		})
		require.Empty(t, m.gaps)
	})

	t.Run("GapSkippedWhenSingleCandidate", func(t *testing.T) {
		t.Parallel()
		m := &fakeListTemplatesMetrics{}
		recordListTemplatesTelemetry(context.Background(), ListTemplatesOptions{Metrics: m}, "tc", uuid.New(), listTemplatesTelemetry{
			ranked:        []rankedTemplate{rankedWith(0, 5)},
			recommendedID: uuid.New(),
			nextStep:      NextStepUseRecommended,
		})
		require.Empty(t, m.gaps)
	})

	t.Run("NilMetricsDoesNotPanic", func(t *testing.T) {
		t.Parallel()
		recordListTemplatesTelemetry(context.Background(), ListTemplatesOptions{}, "tc", uuid.New(), listTemplatesTelemetry{
			ranked:   []rankedTemplate{rankedWith(0, 0)},
			nextStep: NextStepAskUser,
		})
	})
}

func TestSelectTemplateRecommendation_Reasons(t *testing.T) {
	t.Parallel()

	loadErr := xerrors.New("signals failed to load")

	t.Run("Outcomes", func(t *testing.T) {
		t.Parallel()
		_, _, reason := selectTemplateRecommendation(nil, 0, nil)
		require.Equal(t, recommendationReasonNoTemplates, reason)

		_, _, reason = selectTemplateRecommendation(nil, 2, nil)
		require.Equal(t, recommendationReasonNoMatches, reason)

		_, _, reason = selectTemplateRecommendation(
			[]rankedTemplate{{Template: database.Template{ID: uuid.New()}}}, 1, loadErr,
		)
		require.Equal(t, recommendationReasonOnlyAvailable, reason)
	})

	t.Run("Query", func(t *testing.T) {
		t.Parallel()
		_, _, reason := selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreExactName},
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreDescriptionMatch},
		}, 2, nil)
		require.Equal(t, recommendationReasonDecisiveQuery, reason)

		// A failed signal load past a decisive query falls back to asking.
		_, _, reason = selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
		}, 2, loadErr)
		require.Equal(t, recommendationReasonSignalsUnavailable, reason)

		_, _, reason = selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 10, Signals: templateRankingSignals{ActiveCount: 1}},
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0},
		}, 2, nil)
		require.Equal(t, recommendationReasonQueryTieConfident, reason)

		_, _, reason = selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0.1},
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix, AffinityScore: 0},
		}, 2, nil)
		require.Equal(t, recommendationReasonQueryTieAmbiguous, reason)
	})

	t.Run("Affinity", func(t *testing.T) {
		t.Parallel()
		_, _, reason := selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, AffinityScore: 0.1},
			{Template: database.Template{ID: uuid.New()}, AffinityScore: 0},
		}, 2, nil)
		require.Equal(t, recommendationReasonAffinityLow, reason)

		_, _, reason = selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.20, Signals: templateRankingSignals{OrgDevs: 2}},
			{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.15, Signals: templateRankingSignals{OrgDevs: 2}},
		}, 2, nil)
		require.Equal(t, recommendationReasonAffinityAmbiguous, reason)

		_, _, reason = selectTemplateRecommendation([]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, AffinityScore: 2.0, Signals: templateRankingSignals{OrgDevs: 6}},
			{Template: database.Template{ID: uuid.New()}, AffinityScore: 1.2, Signals: templateRankingSignals{OrgDevs: 2}},
		}, 2, nil)
		require.Equal(t, recommendationReasonAffinityConfident, reason)
	})
}
