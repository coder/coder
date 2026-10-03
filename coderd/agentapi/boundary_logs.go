package agentapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const maxBoundaryLogsPerBatch = 1000

// ErrBatchSizeExceeded matches any BatchSizeExceededError via errors.Is.
var ErrBatchSizeExceeded = xerrors.New("boundary logs batch size exceeded")

// BatchSizeExceededError is returned when a ReportBoundaryLogs request
// exceeds maxBoundaryLogsPerBatch. Match it with errors.As for the sizes,
// or errors.Is(err, ErrBatchSizeExceeded) for the category.
type BatchSizeExceededError struct {
	BatchSize int
	MaxSize   int
}

func (e BatchSizeExceededError) Error() string {
	return fmt.Sprintf("batch size %d exceeds maximum of %d", e.BatchSize, e.MaxSize)
}

func (BatchSizeExceededError) Is(target error) bool {
	return target == ErrBatchSizeExceeded
}

type BoundaryLogsAPI struct {
	Log                  slog.Logger
	Database             database.Store
	AgentID              uuid.UUID
	WorkspaceID          uuid.UUID
	OwnerID              uuid.UUID
	TemplateID           uuid.UUID
	TemplateVersionID    uuid.UUID
	BoundaryUsageTracker *boundaryusage.Tracker

	// mu guards ensuredSessions, which records session IDs already persisted
	// by this connection so repeated batches skip the existence check and
	// insert. The API is one instance per agent connection, so this lives for
	// the session's lifetime.
	mu              sync.Mutex
	ensuredSessions map[uuid.UUID]struct{}
}

func (a *BoundaryLogsAPI) ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	var allowed, denied int64

	if len(req.Logs) == 0 {
		a.Log.Debug(ctx, "empty boundary logs request, skipping")
		return &agentproto.ReportBoundaryLogsResponse{}, nil
	}

	if len(req.Logs) > maxBoundaryLogsPerBatch {
		return nil, BatchSizeExceededError{BatchSize: len(req.Logs), MaxSize: maxBoundaryLogsPerBatch}
	}

	now := dbtime.Now()

	// Parse session_id if present. Old boundary clients may not send it,
	// so a missing or invalid session_id disables DB persistence but
	// structured logging and usage tracking still run.
	var sessionID uuid.UUID
	persistEnabled := false
	if raw := req.GetSessionId(); raw != "" {
		parsed, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			a.Log.Warn(ctx, "invalid session_id, persistence disabled for this batch",
				slog.F("raw_session_id", raw),
				slog.Error(parseErr))
		} else {
			sessionID = parsed
			persistEnabled = true
		}
	}

	if persistEnabled && !a.sessionEnsured(sessionID) {
		// Lazy-create the boundary session on first log arrival.
		// If this fails (transient DB error), we continue so that
		// logs are still persisted. The session will be created on
		// a subsequent batch since every request carries the session
		// details. On success we record the session so later batches
		// skip the existence check and insert entirely.
		if sessionErr := a.ensureSession(ctx, sessionID, req.GetConfinedProcessName(), now); sessionErr != nil {
			a.Log.Error(ctx, "failed to ensure boundary session",
				slog.F("session_id", sessionID.String()),
				slog.Error(sessionErr))
		} else {
			a.markSessionEnsured(sessionID)
		}
	}

	// Collect batch insert params while iterating.
	batch := database.InsertBoundaryLogsParams{
		SessionID:      sessionID,
		OwnerID:        a.OwnerID,
		ID:             nil,
		SequenceNumber: nil,
		CapturedAt:     nil,
		CreatedAt:      nil,
		Proto:          nil,
		Method:         nil,
		Detail:         nil,
		MatchedRule:    nil,
	}

	for _, l := range req.Logs {
		logTime := now
		if l.Time != nil {
			logTime = l.Time.AsTime()
		}

		switch r := l.Resource.(type) {
		case *agentproto.BoundaryLog_HttpRequest_:
			if r.HttpRequest == nil {
				a.Log.Warn(ctx, "empty http request resource",
					slog.F("workspace_id", a.WorkspaceID.String()))
				continue
			}

			if l.Allowed {
				allowed++
			} else {
				denied++
			}

			fields := []slog.Field{
				slog.F("decision", allowBoolToString(l.Allowed)),
				slog.F("session_id", req.SessionId),
				slog.F("sequence_number", l.SequenceNumber),
				slog.F("workspace_id", a.WorkspaceID.String()),
				slog.F("template_id", a.TemplateID.String()),
				slog.F("template_version_id", a.TemplateVersionID.String()),
				slog.F("http_method", r.HttpRequest.Method),
				slog.F("http_url", r.HttpRequest.Url),
				slog.F("event_time", logTime.Format(time.RFC3339Nano)),
			}
			if l.Allowed {
				fields = append(fields, slog.F("matched_rule", r.HttpRequest.MatchedRule))
			}

			a.Log.With(fields...).Info(ctx, "boundary_request")

			var matchedRule string
			if l.Allowed && r.HttpRequest.MatchedRule != "" {
				matchedRule = r.HttpRequest.MatchedRule
			}
			batch.ID = append(batch.ID, uuid.New())
			batch.SequenceNumber = append(batch.SequenceNumber, l.SequenceNumber)
			batch.CapturedAt = append(batch.CapturedAt, now)
			batch.CreatedAt = append(batch.CreatedAt, logTime)
			batch.Proto = append(batch.Proto, "http")
			batch.Method = append(batch.Method, r.HttpRequest.Method)
			batch.Detail = append(batch.Detail, r.HttpRequest.Url)
			batch.MatchedRule = append(batch.MatchedRule, matchedRule)
		default:
			a.Log.Warn(ctx, "unknown resource type",
				slog.F("workspace_id", a.WorkspaceID.String()))
		}
	}

	// Batch-insert all collected logs in a single query.
	if persistEnabled && len(batch.ID) > 0 {
		if insertErr := a.insertLogs(ctx, batch); insertErr != nil {
			a.Log.Error(ctx, "failed to insert boundary logs",
				slog.F("session_id", sessionID.String()),
				slog.F("count", len(batch.ID)),
				slog.Error(insertErr))
		}
	}

	if a.BoundaryUsageTracker != nil && (allowed > 0 || denied > 0) {
		a.BoundaryUsageTracker.Track(a.WorkspaceID, a.OwnerID, allowed, denied)
	}

	return &agentproto.ReportBoundaryLogsResponse{}, nil
}

// sessionEnsured reports whether this connection has already persisted the
// session, letting repeated batches skip the database round-trip.
func (a *BoundaryLogsAPI) sessionEnsured(sessionID uuid.UUID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.ensuredSessions[sessionID]
	return ok
}

// markSessionEnsured records that the session has been persisted.
func (a *BoundaryLogsAPI) markSessionEnsured(sessionID uuid.UUID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ensuredSessions == nil {
		a.ensuredSessions = make(map[uuid.UUID]struct{})
	}
	a.ensuredSessions[sessionID] = struct{}{}
}

// ensureSession creates the boundary_sessions row if it does not
// already exist.
func (a *BoundaryLogsAPI) ensureSession(ctx context.Context, sessionID uuid.UUID, confinedProcess string, now time.Time) error {
	if a.Database == nil {
		return nil
	}

	_, err := a.Database.InsertBoundarySession(ctx, database.InsertBoundarySessionParams{
		ID:                  sessionID,
		WorkspaceAgentID:    a.AgentID,
		OwnerID:             uuid.NullUUID{UUID: a.OwnerID, Valid: true},
		ConfinedProcessName: confinedProcess,
		StartedAt:           now,
		UpdatedAt:           now,
	})
	if err != nil {
		if database.IsUniqueViolation(err, database.UniqueBoundarySessionsPkey) {
			a.Log.Debug(ctx, "boundary session already created",
				slog.F("session_id", sessionID.String()))
			return nil
		}
		return xerrors.Errorf("insert boundary session: %w", err)
	}

	return nil
}

// insertLogs persists a batch of boundary log entries.
func (a *BoundaryLogsAPI) insertLogs(ctx context.Context, batch database.InsertBoundaryLogsParams) error {
	if a.Database == nil {
		return nil
	}
	_, err := a.Database.InsertBoundaryLogs(ctx, batch)
	return err
}

//nolint:revive // This stringifies the boolean argument.
func allowBoolToString(b bool) string {
	if b {
		return "allow"
	}
	return "deny"
}
