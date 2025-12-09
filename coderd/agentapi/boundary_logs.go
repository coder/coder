package agentapi

import (
	"context"

	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

type BoundaryLogsAPI struct{}

func (*BoundaryLogsAPI) ReportBoundaryLogs(context.Context, *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	return nil, xerrors.New("not implemented")
}
