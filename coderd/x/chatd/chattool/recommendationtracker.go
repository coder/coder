package chattool

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/coder/quartz"
)

// Recommendation follow-up outcomes describe how a create_workspace call
// related to the most recent list_templates recommendation for the same chat.
// They are the label values for template_recommendation_followup_total.
const (
	// recommendationFollowupAccepted: the chosen template is the one
	// list_templates recommended.
	recommendationFollowupAccepted = "accepted_recommendation"
	// recommendationFollowupOverrodeListed: a recommendation existed but the
	// agent built a different template that was still on the shown page.
	recommendationFollowupOverrodeListed = "overrode_with_listed_template"
	// recommendationFollowupListedNoRecommendation: no recommendation was made,
	// and the agent built a template from the shown page.
	recommendationFollowupListedNoRecommendation = "created_listed_without_recommendation"
	// recommendationFollowupUnlisted: the agent built a template that was not
	// on the shown page (e.g. user named it, or an older list result).
	recommendationFollowupUnlisted = "created_unlisted_template"
	// recommendationFollowupNoRecord: no fresh list_templates result is known
	// for the chat (restart, replica handoff, expiry, or none was called).
	recommendationFollowupNoRecord = "no_recent_list_templates"
)

const (
	// defaultRecommendationTTL bounds how long a recorded recommendation stays
	// eligible for follow-up classification.
	defaultRecommendationTTL = 30 * time.Minute
	// defaultRecommendationMaxEntries bounds the tracker's memory footprint.
	defaultRecommendationMaxEntries = 4096
)

// RecommendationTracker correlates the most recent list_templates result per
// chat with the template that create_workspace later builds, so we can measure
// whether the agent followed the recommendation.
//
// State is in-memory and best-effort: it is lost on restart and is not shared
// across replicas, so a follow-up handled elsewhere classifies as
// "no_recent_list_templates". The durable source of truth for offline analysis
// is the persisted chat transcript (the list_templates result and the
// create_workspace call) plus the chats.workspace_id binding; this tracker
// exists only to surface a live, aggregate acceptance signal.
type RecommendationTracker struct {
	clock      quartz.Clock
	ttl        time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[uuid.UUID]recommendationEntry
}

type recommendationEntry struct {
	recommendedID uuid.UUID
	listed        map[uuid.UUID]struct{}
	recordedAt    time.Time
}

// NewRecommendationTracker constructs a tracker. A nil clock defaults to a real
// clock; non-positive ttl or maxEntries fall back to defaults.
func NewRecommendationTracker(clock quartz.Clock, ttl time.Duration, maxEntries int) *RecommendationTracker {
	if clock == nil {
		clock = quartz.NewReal()
	}
	if ttl <= 0 {
		ttl = defaultRecommendationTTL
	}
	if maxEntries <= 0 {
		maxEntries = defaultRecommendationMaxEntries
	}
	return &RecommendationTracker{
		clock:      clock,
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[uuid.UUID]recommendationEntry),
	}
}

// Record stores the latest list_templates outcome for a chat. recommendedID may
// be uuid.Nil when no template was recommended. listedIDs are the template IDs
// shown on the returned page (what the agent actually saw). page is the 1-based
// page number: page 1 starts a fresh record, while later pages of the same
// result accumulate their listed IDs so a follow-up build from any shown page
// still classifies as "listed" rather than "unlisted". No-op when t is nil or
// chatID is uuid.Nil.
func (t *RecommendationTracker) Record(chatID, recommendedID uuid.UUID, listedIDs []uuid.UUID, page int) {
	if t == nil || chatID == uuid.Nil {
		return
	}
	now := t.clock.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Later pages of the same result continue an existing record, so union
	// their listed IDs instead of overwriting. Page 1, a missing entry, or a
	// changed recommendation starts fresh.
	if page > 1 {
		if entry, ok := t.entries[chatID]; ok && entry.recommendedID == recommendedID {
			for _, id := range listedIDs {
				if id != uuid.Nil {
					entry.listed[id] = struct{}{}
				}
			}
			entry.recordedAt = now
			t.entries[chatID] = entry
			return
		}
	}

	listed := make(map[uuid.UUID]struct{}, len(listedIDs))
	for _, id := range listedIDs {
		if id != uuid.Nil {
			listed[id] = struct{}{}
		}
	}
	t.evictLocked(now)
	t.entries[chatID] = recommendationEntry{
		recommendedID: recommendedID,
		listed:        listed,
		recordedAt:    now,
	}
}

// Classify consumes the recorded recommendation for a chat and reports how the
// chosen template relates to it. The entry is removed so a single creation is
// counted once. Returns recommendationFollowupNoRecord when t is nil, chatID is
// uuid.Nil, or no fresh record exists.
func (t *RecommendationTracker) Classify(chatID, chosenID uuid.UUID) string {
	if t == nil || chatID == uuid.Nil {
		return recommendationFollowupNoRecord
	}
	now := t.clock.Now()

	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[chatID]
	if !ok {
		return recommendationFollowupNoRecord
	}
	delete(t.entries, chatID)
	if now.Sub(entry.recordedAt) > t.ttl {
		return recommendationFollowupNoRecord
	}

	_, listed := entry.listed[chosenID]
	switch {
	case entry.recommendedID != uuid.Nil && chosenID == entry.recommendedID:
		return recommendationFollowupAccepted
	case listed && entry.recommendedID != uuid.Nil:
		return recommendationFollowupOverrodeListed
	case listed:
		return recommendationFollowupListedNoRecommendation
	default:
		return recommendationFollowupUnlisted
	}
}

// evictLocked drops expired entries and, if still at capacity, the oldest
// remaining entry to make room for one more. Callers must hold t.mu.
func (t *RecommendationTracker) evictLocked(now time.Time) {
	for id, e := range t.entries {
		if now.Sub(e.recordedAt) > t.ttl {
			delete(t.entries, id)
		}
	}
	if len(t.entries) < t.maxEntries {
		return
	}
	var (
		oldestID uuid.UUID
		oldestAt time.Time
		found    bool
	)
	for id, e := range t.entries {
		if !found || e.recordedAt.Before(oldestAt) {
			oldestID, oldestAt, found = id, e.recordedAt, true
		}
	}
	if found {
		delete(t.entries, oldestID)
	}
}
