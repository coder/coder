package aibridgedserver

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type Server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx context.Context
	store        database.Store
}

func (s *Server) TrackTokenUsage(ctx context.Context, in *proto.TrackTokenUsageRequest) (*proto.TrackTokenUsageResponse, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, xerrors.Errorf("marshal event: %w", err)
	}

	err = s.store.InsertWormholeEvent(ctx, database.InsertWormholeEventParams{Event: raw, EventType: "token_usage"})
	if err != nil {
		return nil, xerrors.Errorf("store event: %w", err)
	}

	return &proto.TrackTokenUsageResponse{}, nil
}

func (s *Server) TrackUserPrompt(ctx context.Context, in *proto.TrackUserPromptRequest) (*proto.TrackUserPromptResponse, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, xerrors.Errorf("marshal event: %w", err)
	}

	err = s.store.InsertWormholeEvent(ctx, database.InsertWormholeEventParams{Event: raw, EventType: "user_prompt"})
	if err != nil {
		return nil, xerrors.Errorf("store event: %w", err)
	}

	return &proto.TrackUserPromptResponse{}, nil
}

func (s *Server) TrackToolUsage(ctx context.Context, in *proto.TrackToolUsageRequest) (*proto.TrackToolUsageResponse, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, xerrors.Errorf("marshal event: %w", err)
	}

	err = s.store.InsertWormholeEvent(ctx, database.InsertWormholeEventParams{Event: raw, EventType: "tool_usage"})
	if err != nil {
		return nil, xerrors.Errorf("store event: %w", err)
	}

	return &proto.TrackToolUsageResponse{}, nil
}

func NewServer(lifecycleCtx context.Context, store database.Store) (*Server, error) {
	return &Server{
		lifecycleCtx: lifecycleCtx,
		store:        store,
	}, nil
}
