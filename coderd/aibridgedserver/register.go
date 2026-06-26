package aibridgedserver

import (
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"

	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

// Register registers the Recorder, MCPConfigurator, Authorizer, and
// ProviderConfigurator DRPC services backed by srv onto mux. It is shared by
// the embedded in-memory server and the standalone /api/v2/ai-gateway/serve
// WebSocket handler so both expose an identical service set.
func Register(mux *drpcmux.Mux, srv *Server) error {
	if err := proto.DRPCRegisterRecorder(mux, srv); err != nil {
		return xerrors.Errorf("register recorder service: %w", err)
	}
	if err := proto.DRPCRegisterMCPConfigurator(mux, srv); err != nil {
		return xerrors.Errorf("register MCP configurator service: %w", err)
	}
	if err := proto.DRPCRegisterAuthorizer(mux, srv); err != nil {
		return xerrors.Errorf("register authorizer service: %w", err)
	}
	if err := proto.DRPCRegisterProviderConfigurator(mux, srv); err != nil {
		return xerrors.Errorf("register provider configurator service: %w", err)
	}
	return nil
}
