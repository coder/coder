package aibridgedserver

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
)

var _ proto.DRPCRecorderServer = &Server{}

type Server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx context.Context
	store        database.Store
	logger       slog.Logger
}

func NewServer(lifecycleCtx context.Context, store database.Store, logger slog.Logger) (*Server, error) {
	return &Server{
		lifecycleCtx: lifecycleCtx,
		store:        store,
		logger:       logger.Named("aibridgedserver"),
	}, nil
}

func (s *Server) RecordSession(ctx context.Context, in *proto.RecordSessionRequest) (*proto.RecordSessionResponse, error) {
	sessID, err := uuid.Parse(in.GetSessionId())
	if err != nil {
		return nil, xerrors.Errorf("invalid session ID %q: %w", in.GetSessionId(), err)
	}
	initID, err := uuid.Parse(in.GetInitiatorId())
	if err != nil {
		return nil, xerrors.Errorf("invalid initiator ID %q: %w", in.GetInitiatorId(), err)
	}

	_, err = s.store.InsertAIBridgeSession(ctx, database.InsertAIBridgeSessionParams{
		ID:          sessID,
		InitiatorID: initID,
		Provider:    in.Provider,
		Model:       in.Model,
	})
	if err != nil {
		return nil, xerrors.Errorf("start session: %w", err)
	}

	return &proto.RecordSessionResponse{}, nil
}

func (s *Server) RecordTokenUsage(ctx context.Context, in *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
	sessID, err := uuid.Parse(in.GetSessionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse session_id %q: %w", in.GetSessionId(), err)
	}

	err = s.store.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
		ID:           uuid.New(),
		SessionID:    sessID,
		ProviderID:   in.GetMsgId(),
		InputTokens:  in.GetInputTokens(),
		OutputTokens: in.GetOutputTokens(),
		Metadata:     s.marshalMetadata(in.GetMetadata()),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert token usage: %w", err)
	}
	return &proto.RecordTokenUsageResponse{}, nil
}

func (s *Server) RecordPromptUsage(ctx context.Context, in *proto.RecordPromptUsageRequest) (*proto.RecordPromptUsageResponse, error) {
	sessID, err := uuid.Parse(in.GetSessionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse session_id %q: %w", in.GetSessionId(), err)
	}

	err = s.store.InsertAIBridgeUserPrompt(ctx, database.InsertAIBridgeUserPromptParams{
		ID:         uuid.New(),
		SessionID:  sessID,
		ProviderID: in.GetMsgId(),
		Prompt:     in.GetPrompt(),
		Metadata:   s.marshalMetadata(in.GetMetadata()),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert user prompt: %w", err)
	}
	return &proto.RecordPromptUsageResponse{}, nil
}

func (s *Server) RecordToolUsage(ctx context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
	sessID, err := uuid.Parse(in.GetSessionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse session_id %q: %w", in.GetSessionId(), err)
	}

	err = s.store.InsertAIBridgeToolUsage(ctx, database.InsertAIBridgeToolUsageParams{
		ID:         uuid.New(),
		SessionID:  sessID,
		ProviderID: in.GetMsgId(),
		Tool:       in.GetTool(),
		Input:      in.GetInput(),
		Injected:   in.GetInjected(),
		Metadata:   s.marshalMetadata(in.GetMetadata()),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert tool usage: %w", err)
	}
	return &proto.RecordToolUsageResponse{}, nil
}

func (s *Server) marshalMetadata(in map[string]*anypb.Any) []byte {
	mdMap := make(map[string]any, len(in))
	for k, v := range in {
		if v == nil {
			continue
		}
		var sv structpb.Value
		if err := v.UnmarshalTo(&sv); err == nil {
			mdMap[k] = sv.AsInterface()
		}
	}
	out, err := json.Marshal(mdMap)
	if err != nil {
		s.logger.Warn(s.lifecycleCtx, "failed to marshal metadata", slog.Error(err))
	}
	return out
}
