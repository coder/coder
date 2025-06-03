package aibridgedserver

import (
	"context"

	"github.com/coder/coder/v2/aibridged/proto"
)

type Server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx context.Context
}

func (s *Server) AuditPrompt(_ context.Context, req *proto.AuditPromptRequest) (*proto.AuditPromptResponse, error) {
	return &proto.AuditPromptResponse{}, nil
}

func NewServer(lifecycleCtx context.Context) (*Server, error) {
	return &Server{
		lifecycleCtx: lifecycleCtx,
	}, nil
}
