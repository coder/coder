package agentapi

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto/boundary_logs"
)

type BoundaryLogsAPI struct{}

func (*BoundaryLogsAPI) ReportBoundaryLogs(context.Context, *boundary_logs.ReportResourceAccessLogsRequest) (*boundary_logs.ReportResourceAccessLogsResponse, error) {
	return nil, xerrors.New("not implemented")
}
