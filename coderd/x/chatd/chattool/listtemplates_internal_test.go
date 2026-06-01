package chattool

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

func TestSelectTemplateRecommendationRankingSignalsUnavailable(t *testing.T) {
	t.Parallel()

	enrichmentErr := xerrors.New("enrichment failed")
	enrichmentErrors := templateRankingSignalErrors{
		ActiveDeveloperCounts: enrichmentErr,
		Usage:                 enrichmentErr,
	}
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

	onlyTemplateID := uuid.New()
	hint, recommendedID, reason := selectTemplateRecommendation(
		[]rankedTemplate{{Template: database.Template{ID: onlyTemplateID}}},
		1,
		enrichmentErrors,
		now,
	)
	require.Equal(t, listTemplatesHintOnlyAvailable, hint)
	require.Equal(t, onlyTemplateID, recommendedID)
	require.Equal(t, "only_available_template", reason)

	topID := uuid.New()
	hint, recommendedID, reason = selectTemplateRecommendation(
		[]rankedTemplate{
			{Template: database.Template{ID: topID}, QueryScore: queryScoreExactName},
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreDescriptionMatch},
		},
		2,
		enrichmentErrors,
		now,
	)
	require.Equal(t, listTemplatesHintHighConfidence, hint)
	require.Equal(t, topID, recommendedID)
	require.Equal(t, "matches_query", reason)

	personalUsageID := uuid.New()
	hint, recommendedID, reason = selectTemplateRecommendation(
		[]rankedTemplate{
			{
				Template: database.Template{ID: personalUsageID},
				Usage: templateUsage{
					WorkspaceCount: 3,
					LastUsedAt:     now.Add(-180 * 24 * time.Hour),
				},
			},
			{Template: database.Template{ID: uuid.New()}},
		},
		2,
		templateRankingSignalErrors{ActiveDeveloperCounts: enrichmentErr},
		now,
	)
	require.Equal(t, listTemplatesHintHighConfidence, hint)
	require.Equal(t, personalUsageID, recommendedID)
	require.Equal(t, "used_by_you", reason)

	hint, recommendedID, reason = selectTemplateRecommendation(
		[]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, ActiveDevelopers: 2},
			{Template: database.Template{ID: uuid.New()}},
		},
		2,
		templateRankingSignalErrors{Usage: enrichmentErr},
		now,
	)
	require.Equal(t, listTemplatesHintNoConfidence, hint)
	require.Equal(t, uuid.Nil, recommendedID)
	require.Equal(t, "ranking_signals_unavailable", reason)

	hint, recommendedID, reason = selectTemplateRecommendation(
		[]rankedTemplate{
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
			{Template: database.Template{ID: uuid.New()}, QueryScore: queryScoreNamePrefix},
		},
		2,
		enrichmentErrors,
		now,
	)
	require.Equal(t, listTemplatesHintNoConfidence, hint)
	require.Equal(t, uuid.Nil, recommendedID)
	require.Equal(t, "ranking_signals_unavailable", reason)
}
