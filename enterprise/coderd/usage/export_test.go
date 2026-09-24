package usage

import "time"

func (h *PublishHealth) RecordCycleFailureForTest(now time.Time) {
	h.recordCycleFailure(h.currentEpoch(), now)
}

func (h *PublishHealth) RecordCyclePublishedForTest(now time.Time) {
	h.recordCyclePublished(h.currentEpoch(), now)
}
