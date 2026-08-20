package confine

import (
	"context"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// SessionReporter retains a confinement session and its egress decisions
// server-side for callers that own the egress proxy directly, such as the
// embedded microVM sandbox command. The reporting credential must belong to
// the confined, AI-bound agent itself: sessions are posted with an empty
// ChildAgentID, so coderd resolves attribution from the reporter's own AI
// identity binding.
type SessionReporter struct {
	client      AgentClient
	logger      slog.Logger
	enforcement codersdk.AISandboxEgressEnforcement
	sessionID   uuid.UUID
	batcher     *eventBatcher
	startedAt   time.Time
}

// NewSessionReporter builds a reporter for one confinement session. The
// session ID is generated here so events can reference the session before
// the create round-trip completes and retries stay idempotent.
func NewSessionReporter(client AgentClient, logger slog.Logger, enforcement codersdk.AISandboxEgressEnforcement) *SessionReporter {
	sessionID := uuid.New()
	return &SessionReporter{
		client:      client,
		logger:      logger,
		enforcement: enforcement,
		sessionID:   sessionID,
		batcher:     newEventBatcher(client, logger, sessionID, eventQueueSize),
	}
}

// Start opens the session server-side and begins periodic event flushing
// until ctx is canceled. It must complete before the sandbox can observe
// egress: coderd rejects events for a session that does not exist yet, and
// the batcher drops rejected flushes.
func (r *SessionReporter) Start(ctx context.Context) {
	r.startedAt = time.Now()
	r.postSession(ctx, nil)
	go r.batcher.Run(ctx, eventFlushPeriod)
}

// Record queues one egress decision for the next periodic flush.
func (r *SessionReporter) Record(event NetworkEvent) {
	r.batcher.Add(event)
}

// Close flushes queued events and marks the session ended. It uses a fresh
// context because it typically runs during shutdown, after the run context
// is already canceled.
func (r *SessionReporter) Close() {
	r.batcher.Flush()
	endedAt := time.Now()
	r.postSession(context.Background(), &endedAt)
}

func (r *SessionReporter) postSession(ctx context.Context, endedAt *time.Time) {
	request := agentsdk.PostAISandboxSessionRequest{
		ID:                r.sessionID,
		EgressEnforcement: r.enforcement,
		StartedAt:         r.startedAt,
		EndedAt:           endedAt,
	}
	backoff := time.Second
	var lastErr error
	for attempt := 1; attempt <= sessionReportAttempts; attempt++ {
		reportCtx, cancel := context.WithTimeout(ctx, reportTimeout)
		lastErr = r.client.PostAISandboxSession(reportCtx, request)
		cancel()
		if lastErr == nil || isNotFound(lastErr) {
			return
		}
		if attempt == sessionReportAttempts || !waitBackoff(ctx, backoff) {
			break
		}
		backoff *= 2
	}
	r.logger.Warn(context.Background(), "report AI sandbox session", slog.Error(lastErr))
}
