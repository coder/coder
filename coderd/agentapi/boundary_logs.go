package agentapi

import (
	"context"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/boundaryusage"
)

type BoundaryLogsAPI struct {
	Log                  slog.Logger
	WorkspaceID          uuid.UUID
	OwnerID              uuid.UUID
	TemplateID           uuid.UUID
	TemplateVersionID    uuid.UUID
	BoundaryUsageTracker *boundaryusage.Tracker
}

func (a *BoundaryLogsAPI) ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	var allowed, denied int64

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

//nolint:revive // This stringifies the boolean argument.
func allowBoolToString(b bool) string {
	if b {
		return "allow"
	}
	return "deny"
}
