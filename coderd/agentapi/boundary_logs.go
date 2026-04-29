package agentapi

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

type BoundaryLogsAPI struct {
	Log                  slog.Logger
	Database             database.Store
	AgentID              uuid.UUID
	WorkspaceID          uuid.UUID
	OwnerID              uuid.UUID
	TemplateID           uuid.UUID
	TemplateVersionID    uuid.UUID
	BoundaryUsageTracker *boundaryusage.Tracker

	// knownSessions tracks session IDs that have already been created
	// in the database during this connection's lifetime, avoiding
	// redundant lookups on every batch.
	knownSessionsMu sync.Mutex
	knownSessions   map[uuid.UUID]struct{}
}

func (a *BoundaryLogsAPI) ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	var allowed, denied int64

	// Parse the session ID from the request. If absent or invalid,
	// fall back to log-only mode for backwards compatibility.
	sessionID, err := uuid.Parse(req.GetSessionId())
	persistEnabled := err == nil && a.Database != nil

	authCtx := ctx
	if persistEnabled {
		authCtx = dbauthz.AsAgentAPIHandler(ctx)
	}

	now := dbtime.Now()

	// Lazy-create the boundary session on first log arrival.
	if persistEnabled {
		if sessionErr := a.ensureSession(authCtx, sessionID, req.GetConfinedProcessName(), now); sessionErr != nil {
			a.Log.Error(ctx, "failed to ensure boundary session",
				slog.F("session_id", sessionID.String()),
				slog.Error(sessionErr))
			// Continue processing logs even if session creation
			// fails, so that structured logging and usage tracking
			// still work.
		}
	}

	for _, l := range req.Logs {
		var logTime time.Time
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

			if persistEnabled {
				insertErr := a.insertHTTPLog(authCtx, sessionID, l, r.HttpRequest, now, logTime)
				if insertErr != nil {
					a.Log.Error(ctx, "failed to insert boundary log",
						slog.F("session_id", sessionID.String()),
						slog.F("sequence_number", l.SequenceNumber),
						slog.Error(insertErr))
				}
			}
		default:
			a.Log.Warn(ctx, "unknown resource type",
				slog.F("workspace_id", a.WorkspaceID.String()))
		}
	}

	if a.BoundaryUsageTracker != nil && (allowed > 0 || denied > 0) {
		a.BoundaryUsageTracker.Track(a.WorkspaceID, a.OwnerID, allowed, denied)
	}

	return &agentproto.ReportBoundaryLogsResponse{}, nil
}

// ensureSession creates the boundary_sessions row if it does not
// already exist. The check is cached in-memory so that subsequent
// batches for the same session skip the database round-trip.
func (a *BoundaryLogsAPI) ensureSession(ctx context.Context, sessionID uuid.UUID, confinedProcess string, now time.Time) error {
	a.knownSessionsMu.Lock()
	if a.knownSessions == nil {
		a.knownSessions = make(map[uuid.UUID]struct{})
	}
	_, known := a.knownSessions[sessionID]
	a.knownSessionsMu.Unlock()

	if known {
		return nil
	}

	// Check the database in case another replica or reconnection
	// already created this session.
	_, err := a.Database.GetBoundarySessionByID(ctx, sessionID)
	if err == nil {
		a.knownSessionsMu.Lock()
		a.knownSessions[sessionID] = struct{}{}
		a.knownSessionsMu.Unlock()
		return nil
	}

	// Session does not exist; create it. started_at is the time
	// the first log is received by coderd, per the RFC.
	_, err = a.Database.InsertBoundarySession(ctx, database.InsertBoundarySessionParams{
		ID:               sessionID,
		WorkspaceAgentID: a.AgentID,
		ConfinedProcess:  confinedProcess,
		StartedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		// If another goroutine or replica raced us, the insert
		// will fail with a unique constraint violation. Treat
		// that as success.
		if database.IsUniqueViolation(err, database.UniqueBoundarySessionsPkey) {
			a.knownSessionsMu.Lock()
			a.knownSessions[sessionID] = struct{}{}
			a.knownSessionsMu.Unlock()
			return nil
		}
		return xerrors.Errorf("insert boundary session: %w", err)
	}

	a.knownSessionsMu.Lock()
	a.knownSessions[sessionID] = struct{}{}
	a.knownSessionsMu.Unlock()
	return nil
}

// insertHTTPLog persists a single HTTP boundary log entry.
func (a *BoundaryLogsAPI) insertHTTPLog(
	ctx context.Context,
	sessionID uuid.UUID,
	l *agentproto.BoundaryLog,
	httpReq *agentproto.BoundaryLog_HttpRequest,
	capturedAt time.Time,
	createdAt time.Time,
) error {
	var matchedRule sql.NullString
	if l.Allowed && httpReq.MatchedRule != "" {
		matchedRule = sql.NullString{String: httpReq.MatchedRule, Valid: true}
	}

	_, err := a.Database.InsertBoundaryLog(ctx, database.InsertBoundaryLogParams{
		ID:             uuid.New(),
		SessionID:      sessionID,
		SequenceNumber: int64(l.SequenceNumber),
		Allowed:        l.Allowed,
		CapturedAt:     capturedAt,
		CreatedAt:      createdAt,
		Proto:          "http",
		Method:         httpReq.Method,
		Detail:         httpReq.Url,
		MatchedRule:    matchedRule,
	})
	if err != nil {
		return xerrors.Errorf("insert boundary log: %w", err)
	}
	return nil
}

//nolint:revive // This stringifies the boolean argument.
func allowBoolToString(b bool) string {
	if b {
		return "allow"
	}
	return "deny"
}
