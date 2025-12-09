package agentapi

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

type BoundaryLogsAPI struct {
	WorkspaceID uuid.UUID
}

func (a *BoundaryLogsAPI) ReportBoundaryLogs(_ context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	for _, log := range req.Logs {
		workspaceID, err := uuid.FromBytes(log.WorkspaceId)
		if err != nil {
			workspaceID = a.WorkspaceID
		}

		decision := "allow"
		level := "info"
		if !log.Allowed {
			decision = "deny"
			level = "warn"
		}

		var logTime time.Time
		if log.Time != nil {
			logTime = log.Time.AsTime()
		} else {
			logTime = time.Now()
		}

		// Format: [API] 2025-12-08 20:58:46.093 [warn] boundary: workspace.id=... decision=deny http.method="GET" http.url="..." time="..."
		_, _ = fmt.Fprintf(os.Stderr, "[API] %s [%s] boundary: workspace.id=%s decision=%s http.method=%q http.url=%q time=%q\n",
			logTime.Format("2006-01-02 15:04:05.000"),
			level,
			workspaceID.String(),
			decision,
			log.HttpMethod,
			log.HttpUrl,
			logTime.Format(time.RFC3339Nano),
		)
	}

	return &agentproto.ReportBoundaryLogsResponse{}, nil
}
