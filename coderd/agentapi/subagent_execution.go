package agentapi

import (
	"context"
	"errors"

	"storj.io/drpc/drpcerr"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

// TODO(CODAGT): implement subagent execution control server-side.

// AcquireSubagentExecution is not implemented yet. It returns a drpc
// Unimplemented error so clients can detect the missing capability and
// fall back.
func (*API) AcquireSubagentExecution(context.Context, *agentproto.AcquireSubagentExecutionRequest) (*agentproto.AcquireSubagentExecutionResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}

// ReportSubagentExecutionStatus is not implemented yet. It returns a drpc
// Unimplemented error so clients can detect the missing capability and
// fall back.
func (*API) ReportSubagentExecutionStatus(context.Context, *agentproto.ReportSubagentExecutionStatusRequest) (*agentproto.ReportSubagentExecutionStatusResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}
