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
		workspaceID, err := uuid.FromBytes(l.WorkspaceId)
		if err != nil {
			workspaceID = a.WorkspaceID
		}

		var logTime time.Time
		if l.Time != nil {
			logTime = l.Time.AsTime()
		} else {
			logTime = time.Now()
		}

		if l.Allowed {
			a.Log.Info(ctx, "boundary request allowed",
				slog.F("workspace_id", workspaceID.String()),
				slog.F("http_method", l.HttpMethod),
				slog.F("http_url", l.HttpUrl),
				slog.F("event_time", logTime.Format(time.RFC3339Nano)),
				slog.F("matched_rule", l.MatchedRule),
			)
		} else {
			a.Log.Warn(ctx, "boundary request denied",
				slog.F("workspace_id", workspaceID.String()),
				slog.F("http_method", l.HttpMethod),
				slog.F("http_url", l.HttpUrl),
				slog.F("event_time", logTime.Format(time.RFC3339Nano)),
				slog.F("matched_rule", l.MatchedRule),
			)
		}
	}

	return &agentproto.ReportBoundaryLogsResponse{}, nil
}
