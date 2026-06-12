package agentapi

import (
	"context"

	"storj.io/drpc/drpcerr"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

// PushContextState is the server-side stub for the v2.10
// PushContextState RPC. Coderd does not yet persist context
// snapshots; the chatd integration that consumes pushes lives
// in a follow-up change.
//
// Returning Unimplemented signals the agent to stop pushing for
// the remainder of the connection. The agent.Manager.RunPush
// loop translates this into a clean shutdown rather than a
// retry storm.
func (*API) PushContextState(_ context.Context, _ *agentproto.PushContextStateRequest) (*agentproto.PushContextStateResponse, error) {
	return nil, drpcerr.WithCode(errPushContextStateUnimplemented, drpcerr.Unimplemented)
}

// errPushContextStateUnimplemented is the static error returned
// by PushContextState before the chatd integration lands.
var errPushContextStateUnimplemented = stringError("agentapi: PushContextState is not implemented yet")

type stringError string

func (e stringError) Error() string { return string(e) }
