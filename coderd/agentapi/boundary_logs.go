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
		}

		switch r := l.Resource.(type) {
		case *agentproto.BoundaryLog_HttpRequest_:
			if r.HttpRequest == nil {
				a.Log.Warn(ctx, "empty http request resource")
				continue
			}

			fields := []slog.Field{
				slog.F("decision", allowBoolToString(l.Allowed)),
				slog.F("workspace_id", a.WorkspaceID.String()),
				slog.F("http_method", r.HttpRequest.Method),
				slog.F("http_url", r.HttpRequest.Url),
				slog.F("event_time", logTime.Format(time.RFC3339Nano)),
			}
			if l.Allowed {
				fields = append(fields, slog.F("matched_rule", r.HttpRequest.MatchedRule))
			}

			a.Log.With(fields...).Info(ctx, "boundary_request")
		default:
			a.Log.Warn(ctx, "unexpected resource type", slog.F("type", r))
		}
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
