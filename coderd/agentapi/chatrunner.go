package agentapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

type ChatRunnerAPI struct {
	AgentID            uuid.UUID
	Database           database.Store
	Log                slog.Logger
	Experiments        codersdk.Experiments
	OnRequiresAction   func(context.Context, database.Chat) error
	OnChatStatusChange func(context.Context, database.Chat) error
}

func (a *ChatRunnerAPI) ensureEnabled() error {
	if !a.Experiments.Enabled(codersdk.ExperimentAgentChatRunner) {
		return drpcerr.WithCode(
			xerrors.New("agent chat runner not enabled"),
			drpcerr.Unimplemented,
		)
	}
	return nil
}

func (a *ChatRunnerAPI) ReportChatRunnerStatus(ctx context.Context, req *agentproto.ReportChatRunnerStatusRequest) (*agentproto.ReportChatRunnerStatusResponse, error) {
	if err := a.ensureEnabled(); err != nil {
		return nil, err
	}

	// nolint:gocritic // Agent-side system operation.
	err := a.Database.UpdateWorkspaceAgentChatRunnerStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentChatRunnerStatusParams{
		ChatRunnerReady: req.Ready,
		AgentID:         a.AgentID,
	})
	if err != nil {
		return nil, drpcerr.WithCode(
			xerrors.Errorf("update workspace agent chat runner status: %w", err),
			uint64(codes.Internal),
		)
	}

	return &agentproto.ReportChatRunnerStatusResponse{}, nil
}

func (a *ChatRunnerAPI) PollChatWork(ctx context.Context, req *agentproto.PollChatWorkRequest) (*agentproto.PollChatWorkResponse, error) {
	if err := a.ensureEnabled(); err != nil {
		return nil, err
	}
	if req.MaxChats <= 0 {
		return nil, drpcerr.WithCode(
			xerrors.New("max_chats must be greater than zero"),
			uint64(codes.InvalidArgument),
		)
	}

	// nolint:gocritic // Agent-side system operation.
	rows, err := a.Database.GetPendingChatsForAgent(dbauthz.AsSystemRestricted(ctx), database.GetPendingChatsForAgentParams{
		AgentID:  a.AgentID,
		MaxChats: req.MaxChats,
	})
	if err != nil {
		return nil, drpcerr.WithCode(
			xerrors.Errorf("get pending chats for agent: %w", err),
			uint64(codes.Internal),
		)
	}

	items := make([]*agentproto.ChatWorkItem, 0, len(rows))
	for _, row := range rows {
		chatID := row.ID
		items = append(items, &agentproto.ChatWorkItem{
			ChatId:    chatID[:],
			Title:     row.Title,
			CreatedAt: timestamppb.New(row.CreatedAt),
		})
	}

	return &agentproto.PollChatWorkResponse{WorkItems: items}, nil
}

func (a *ChatRunnerAPI) AcquireChatLease(ctx context.Context, req *agentproto.AcquireChatLeaseRequest) (*agentproto.AcquireChatLeaseResponse, error) {
	if err := a.ensureEnabled(); err != nil {
		return nil, err
	}

	chatID, err := parseChatID(req.ChatId)
	if err != nil {
		return nil, err
	}

	// nolint:gocritic // Agent-side system operation.
	chat, err := a.Database.AcquireChatForAgent(dbauthz.AsSystemRestricted(ctx), database.AcquireChatForAgentParams{
		StartedAt: time.Now(),
		AgentID:   a.AgentID,
		ChatID:    chatID,
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, drpcerr.WithCode(
				xerrors.Errorf("acquire chat lease: %w", err),
				uint64(codes.NotFound),
			)
		}
		return nil, drpcerr.WithCode(
			xerrors.Errorf("acquire chat lease: %w", err),
			uint64(codes.Internal),
		)
	}

	return &agentproto.AcquireChatLeaseResponse{
		LeaseEpoch: chat.LeaseEpoch,
		Status:     string(chat.Status),
	}, nil
}

func (a *ChatRunnerAPI) RenewChatLease(ctx context.Context, req *agentproto.RenewChatLeaseRequest) (*agentproto.RenewChatLeaseResponse, error) {
	if err := a.ensureEnabled(); err != nil {
		return nil, err
	}

	chatID, err := parseChatID(req.ChatId)
	if err != nil {
		return nil, err
	}
	if err := validateLeaseEpoch(req.LeaseEpoch); err != nil {
		return nil, err
	}

	// nolint:gocritic // Agent-side system operation.
	_, err = a.Database.RenewChatLeaseByAgent(dbauthz.AsSystemRestricted(ctx), database.RenewChatLeaseByAgentParams{
		Now:        time.Now(),
		ChatID:     chatID,
		AgentID:    a.AgentID,
		LeaseEpoch: req.LeaseEpoch,
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return &agentproto.RenewChatLeaseResponse{Renewed: false}, nil
		}
		return nil, drpcerr.WithCode(
			xerrors.Errorf("renew chat lease: %w", err),
			uint64(codes.Internal),
		)
	}

	return &agentproto.RenewChatLeaseResponse{Renewed: true}, nil
}

func (a *ChatRunnerAPI) ReleaseChatLease(ctx context.Context, req *agentproto.ReleaseChatLeaseRequest) (*agentproto.ReleaseChatLeaseResponse, error) {
	if err := a.ensureEnabled(); err != nil {
		return nil, err
	}

	chatID, err := parseChatID(req.ChatId)
	if err != nil {
		return nil, err
	}
	if err := validateLeaseEpoch(req.LeaseEpoch); err != nil {
		return nil, err
	}

	finalStatus, err := validateFinalStatus(req.FinalStatus)
	if err != nil {
		return nil, err
	}

	// nolint:gocritic // Agent-side system operation.
	chat, err := a.Database.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:          chatID,
		Status:      finalStatus,
		WorkerID:    uuid.NullUUID{},
		RunnerType:  database.NullChatRunnerType{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError: sql.NullString{
			String: req.Error,
			Valid:  req.Error != "",
		},
		LeaseEpoch: sql.NullInt64{Int64: req.LeaseEpoch, Valid: true},
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, drpcerr.WithCode(
				xerrors.Errorf("release chat lease: %w", err),
				uint64(codes.NotFound),
			)
		}
		return nil, drpcerr.WithCode(
			xerrors.Errorf("release chat lease: %w", err),
			uint64(codes.Internal),
		)
	}
	if finalStatus == database.ChatStatusRequiresAction && a.OnRequiresAction != nil {
		if callbackErr := a.OnRequiresAction(ctx, chat); callbackErr != nil {
			a.Log.Warn(ctx,
				"post-commit requires_action publish failed",
				slog.F("chat_id", chatID),
				slog.F("lease_epoch", req.LeaseEpoch),
				slog.Error(callbackErr),
			)
		}
	}
	// For all final statuses (waiting, completed, error,
	// requires_action), broadcast the status change so the frontend can
	// update its UI.
	if a.OnChatStatusChange != nil {
		if callbackErr := a.OnChatStatusChange(ctx, chat); callbackErr != nil {
			a.Log.Warn(ctx, "post-commit status change publish failed",
				slog.F("chat_id", chatID),
				slog.F("status", string(finalStatus)),
				slog.Error(callbackErr),
			)
		}
	}

	return &agentproto.ReleaseChatLeaseResponse{}, nil
}

func parseChatID(chatID []byte) (uuid.UUID, error) {
	if len(chatID) != len(uuid.UUID{}) {
		return uuid.Nil, drpcerr.WithCode(
			xerrors.New("chat_id must be 16 bytes"),
			uint64(codes.InvalidArgument),
		)
	}

	parsed, err := uuid.FromBytes(chatID)
	if err != nil {
		return uuid.Nil, drpcerr.WithCode(
			xerrors.Errorf("parse chat_id: %w", err),
			uint64(codes.InvalidArgument),
		)
	}

	return parsed, nil
}

func validateLeaseEpoch(leaseEpoch int64) error {
	if leaseEpoch <= 0 {
		return drpcerr.WithCode(
			xerrors.New("lease_epoch must be greater than zero"),
			uint64(codes.InvalidArgument),
		)
	}

	return nil
}

func validateFinalStatus(finalStatus string) (database.ChatStatus, error) {
	switch database.ChatStatus(finalStatus) {
	case database.ChatStatusWaiting,
		database.ChatStatusCompleted,
		database.ChatStatusError,
		database.ChatStatusRequiresAction:
		return database.ChatStatus(finalStatus), nil
	default:
		return "", drpcerr.WithCode(
			xerrors.New("final_status must be one of: waiting, completed, error, requires_action"),
			uint64(codes.InvalidArgument),
		)
	}
}
