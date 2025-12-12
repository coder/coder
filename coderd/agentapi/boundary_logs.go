package agentapi

import (
	"context"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

type BoundaryLogsAPI struct {
	Log         slog.Logger
	WorkspaceID uuid.UUID
}

func (a *BoundaryLogsAPI) ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	for _, l := range req.Logs {
		var logTime time.Time
		if l.Time != nil {
			logTime = l.Time.AsTime()
		} else {
			logTime = time.Now()
		}

		// Handle different resource types
		switch r := l.Resource.(type) {
		case *agentproto.BoundaryLog_HttpRequest_:
			if r.HttpRequest == nil {
				continue
			}
			if l.Allowed {
				a.Log.Info(ctx, "boundary request allowed",
					slog.F("workspace_id", a.WorkspaceID.String()),
					slog.F("http_method", r.HttpRequest.Method),
					slog.F("http_url", r.HttpRequest.Url),
					slog.F("event_time", logTime.Format(time.RFC3339Nano)),
				)
			} else {
				a.Log.Warn(ctx, "boundary request denied",
					slog.F("workspace_id", a.WorkspaceID.String()),
					slog.F("http_method", r.HttpRequest.Method),
					slog.F("http_url", r.HttpRequest.Url),
					slog.F("event_time", logTime.Format(time.RFC3339Nano)),
					slog.F("matched_rule", r.HttpRequest.MatchedRule),
				)
			}
		default:
			a.Log.Warn(ctx, "unknown boundary log resource type",
				slog.F("workspace_id", a.WorkspaceID.String()),
				slog.F("event_time", logTime.Format(time.RFC3339Nano)),
			)
		}
	}

	return &agentproto.ReportBoundaryLogsResponse{}, nil
}
