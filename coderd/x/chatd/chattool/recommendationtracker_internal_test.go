package chattool

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
)

func TestRecommendationTracker_Classify(t *testing.T) {
	t.Parallel()

	t.Run("AcceptedRecommendation", func(t *testing.T) {
		t.Parallel()
		tr := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		chat, rec, other := uuid.New(), uuid.New(), uuid.New()
		tr.Record(chat, rec, []uuid.UUID{rec, other})
		require.Equal(t, recommendationFollowupAccepted, tr.Classify(chat, rec))
	})

	t.Run("OverrodeWithListedTemplate", func(t *testing.T) {
		t.Parallel()
		tr := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		chat, rec, other := uuid.New(), uuid.New(), uuid.New()
		tr.Record(chat, rec, []uuid.UUID{rec, other})
		require.Equal(t, recommendationFollowupOverrodeListed, tr.Classify(chat, other))
	})

	t.Run("ListedWithoutRecommendation", func(t *testing.T) {
		t.Parallel()
		tr := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		chat, listed := uuid.New(), uuid.New()
		// uuid.Nil recommendation: list_templates returned templates but
		// recommended none.
		tr.Record(chat, uuid.Nil, []uuid.UUID{listed})
		require.Equal(t, recommendationFollowupListedNoRec, tr.Classify(chat, listed))
	})

	t.Run("UnlistedTemplate", func(t *testing.T) {
		t.Parallel()
		tr := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		chat, rec, unlisted := uuid.New(), uuid.New(), uuid.New()
		tr.Record(chat, rec, []uuid.UUID{rec})
		require.Equal(t, recommendationFollowupUnlisted, tr.Classify(chat, unlisted))
	})

	t.Run("NoRecord", func(t *testing.T) {
		t.Parallel()
		tr := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		require.Equal(t, recommendationFollowupNoRecord, tr.Classify(uuid.New(), uuid.New()))
	})

	t.Run("ConsumedOnce", func(t *testing.T) {
		t.Parallel()
		tr := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		chat, rec := uuid.New(), uuid.New()
		tr.Record(chat, rec, []uuid.UUID{rec})
		require.Equal(t, recommendationFollowupAccepted, tr.Classify(chat, rec))
		// A second classification finds nothing: the entry was consumed.
		require.Equal(t, recommendationFollowupNoRecord, tr.Classify(chat, rec))
	})

	t.Run("ExpiredByTTL", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		tr := NewRecommendationTracker(clock, time.Minute, 0)
		chat, rec := uuid.New(), uuid.New()
		tr.Record(chat, rec, []uuid.UUID{rec})
		clock.Advance(time.Minute + time.Second)
		require.Equal(t, recommendationFollowupNoRecord, tr.Classify(chat, rec))
	})

	t.Run("NilTrackerAndNilChat", func(t *testing.T) {
		t.Parallel()
		var tr *RecommendationTracker
		require.Equal(t, recommendationFollowupNoRecord, tr.Classify(uuid.New(), uuid.New()))
		// Record on a nil tracker or with a nil chat ID must not panic.
		tr.Record(uuid.New(), uuid.New(), nil)
		live := NewRecommendationTracker(quartz.NewMock(t), 0, 0)
		live.Record(uuid.Nil, uuid.New(), nil)
		require.Equal(t, recommendationFollowupNoRecord, live.Classify(uuid.Nil, uuid.New()))
	})
}

func TestRecommendationTracker_EvictsOldestAtCapacity(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	const maxEntries = 3
	tr := NewRecommendationTracker(clock, time.Hour, maxEntries)

	// Record the oldest entry, then advance so later entries are strictly
	// newer, filling capacity beyond maxEntries.
	oldest := uuid.New()
	oldestRec := uuid.New()
	tr.Record(oldest, oldestRec, []uuid.UUID{oldestRec})

	for i := 0; i < maxEntries; i++ {
		clock.Advance(time.Second)
		chat, rec := uuid.New(), uuid.New()
		tr.Record(chat, rec, []uuid.UUID{rec})
	}

	// The oldest entry was evicted to keep the map bounded; newer entries
	// within TTL remain classifiable.
	require.Equal(t, recommendationFollowupNoRecord, tr.Classify(oldest, oldestRec))
}
