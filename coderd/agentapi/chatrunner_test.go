package agentapi_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/codes"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestReportChatRunnerStatus(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, codersdk.Experiments{})

		resp, err := api.ReportChatRunnerStatus(context.Background(), &agentproto.ReportChatRunnerStatusRequest{Ready: true})
		requireDRPCCode(t, err, drpcerr.Unimplemented)
		require.Nil(t, resp)
	})

	t.Run("OK_Ready", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentChatRunnerStatus(gomock.Any(), database.UpdateWorkspaceAgentChatRunnerStatusParams{
			ChatRunnerReady: true,
			AgentID:         agentID,
		}).Return(nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReportChatRunnerStatus(context.Background(), &agentproto.ReportChatRunnerStatusRequest{Ready: true})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReportChatRunnerStatusResponse{}, resp)
	})

	t.Run("OK_NotReady", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentChatRunnerStatus(gomock.Any(), database.UpdateWorkspaceAgentChatRunnerStatusParams{
			ChatRunnerReady: false,
			AgentID:         agentID,
		}).Return(nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReportChatRunnerStatus(context.Background(), &agentproto.ReportChatRunnerStatusRequest{Ready: false})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReportChatRunnerStatusResponse{}, resp)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbErr := xerrors.New("db boom")
		dbM.EXPECT().UpdateWorkspaceAgentChatRunnerStatus(gomock.Any(), database.UpdateWorkspaceAgentChatRunnerStatusParams{
			ChatRunnerReady: true,
			AgentID:         agentID,
		}).Return(dbErr)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReportChatRunnerStatus(context.Background(), &agentproto.ReportChatRunnerStatusRequest{Ready: true})
		requireDRPCCode(t, err, uint64(codes.Internal))
		require.Nil(t, resp)
	})
}

func TestPollChatWork(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, codersdk.Experiments{})

		resp, err := api.PollChatWork(context.Background(), &agentproto.PollChatWorkRequest{MaxChats: 1})
		requireDRPCCode(t, err, drpcerr.Unimplemented)
		require.Nil(t, resp)
	})

	t.Run("InvalidMaxChats", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.PollChatWork(context.Background(), &agentproto.PollChatWorkRequest{MaxChats: 0})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("NegativeMaxChats", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.PollChatWork(context.Background(), &agentproto.PollChatWorkRequest{MaxChats: -1})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("OK_Empty", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetPendingChatsForAgent(gomock.Any(), database.GetPendingChatsForAgentParams{
			AgentID:  agentID,
			MaxChats: 2,
		}).Return([]database.GetPendingChatsForAgentRow{}, nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.PollChatWork(context.Background(), &agentproto.PollChatWorkRequest{MaxChats: 2})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Empty(t, resp.WorkItems)
	})

	t.Run("OK_WithItems", func(t *testing.T) {
		t.Parallel()

		chatID1 := uuid.New()
		chatID2 := uuid.New()
		createdAt1 := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
		createdAt2 := time.Date(2026, time.January, 3, 4, 5, 6, 0, time.UTC)
		rows := []database.GetPendingChatsForAgentRow{
			{ID: chatID1, Title: "First chat", CreatedAt: createdAt1},
			{ID: chatID2, Title: "Second chat", CreatedAt: createdAt2},
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetPendingChatsForAgent(gomock.Any(), database.GetPendingChatsForAgentParams{
			AgentID:  agentID,
			MaxChats: 2,
		}).Return(rows, nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.PollChatWork(context.Background(), &agentproto.PollChatWorkRequest{MaxChats: 2})
		require.NoError(t, err)
		require.Len(t, resp.WorkItems, 2)

		require.Equal(t, chatID1[:], resp.WorkItems[0].ChatId)
		require.Equal(t, rows[0].Title, resp.WorkItems[0].Title)
		require.NotNil(t, resp.WorkItems[0].CreatedAt)
		require.True(t, createdAt1.Equal(resp.WorkItems[0].CreatedAt.AsTime()))

		require.Equal(t, chatID2[:], resp.WorkItems[1].ChatId)
		require.Equal(t, rows[1].Title, resp.WorkItems[1].Title)
		require.NotNil(t, resp.WorkItems[1].CreatedAt)
		require.True(t, createdAt2.Equal(resp.WorkItems[1].CreatedAt.AsTime()))
	})

	t.Run("DatabaseError", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbErr := xerrors.New("db boom")
		dbM.EXPECT().GetPendingChatsForAgent(gomock.Any(), database.GetPendingChatsForAgentParams{
			AgentID:  agentID,
			MaxChats: 2,
		}).Return(nil, dbErr)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.PollChatWork(context.Background(), &agentproto.PollChatWorkRequest{MaxChats: 2})
		requireDRPCCode(t, err, uint64(codes.Internal))
		require.Nil(t, resp)
	})
}

func TestAcquireChatLease(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, codersdk.Experiments{})

		chatID := uuid.New()
		resp, err := api.AcquireChatLease(context.Background(), &agentproto.AcquireChatLeaseRequest{ChatId: chatID[:]})
		requireDRPCCode(t, err, drpcerr.Unimplemented)
		require.Nil(t, resp)
	})

	t.Run("InvalidChatID", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.AcquireChatLease(context.Background(), &agentproto.AcquireChatLeaseRequest{ChatId: []byte{1, 2, 3}})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().AcquireChatForAgent(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params database.AcquireChatForAgentParams) (database.Chat, error) {
			require.Equal(t, agentID, params.AgentID)
			require.Equal(t, chatID, params.ChatID)
			require.False(t, params.StartedAt.IsZero())
			return database.Chat{LeaseEpoch: 2, Status: database.ChatStatusRunning}, nil
		})

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.AcquireChatLease(context.Background(), &agentproto.AcquireChatLeaseRequest{ChatId: chatID[:]})
		require.NoError(t, err)
		require.Equal(t, int64(2), resp.LeaseEpoch)
		require.Equal(t, string(database.ChatStatusRunning), resp.Status)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().AcquireChatForAgent(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params database.AcquireChatForAgentParams) (database.Chat, error) {
			require.Equal(t, agentID, params.AgentID)
			require.Equal(t, chatID, params.ChatID)
			require.False(t, params.StartedAt.IsZero())
			return database.Chat{}, sql.ErrNoRows
		})

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.AcquireChatLease(context.Background(), &agentproto.AcquireChatLeaseRequest{ChatId: chatID[:]})
		requireDRPCCode(t, err, uint64(codes.NotFound))
		require.Nil(t, resp)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbErr := xerrors.New("db boom")
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().AcquireChatForAgent(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params database.AcquireChatForAgentParams) (database.Chat, error) {
			require.Equal(t, agentID, params.AgentID)
			require.Equal(t, chatID, params.ChatID)
			require.False(t, params.StartedAt.IsZero())
			return database.Chat{}, dbErr
		})

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.AcquireChatLease(context.Background(), &agentproto.AcquireChatLeaseRequest{ChatId: chatID[:]})
		requireDRPCCode(t, err, uint64(codes.Internal))
		require.Nil(t, resp)
	})
}

func TestRenewChatLease(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, codersdk.Experiments{})

		chatID := uuid.New()
		resp, err := api.RenewChatLease(context.Background(), &agentproto.RenewChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 1})
		requireDRPCCode(t, err, drpcerr.Unimplemented)
		require.Nil(t, resp)
	})

	t.Run("InvalidChatID", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.RenewChatLease(context.Background(), &agentproto.RenewChatLeaseRequest{ChatId: []byte{1, 2, 3}, LeaseEpoch: 1})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("InvalidLeaseEpoch", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.RenewChatLease(context.Background(), &agentproto.RenewChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 0})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().RenewChatLeaseByAgent(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params database.RenewChatLeaseByAgentParams) (uuid.UUID, error) {
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, agentID, params.AgentID)
			require.Equal(t, int64(7), params.LeaseEpoch)
			require.False(t, params.Now.IsZero())
			return chatID, nil
		})

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.RenewChatLease(context.Background(), &agentproto.RenewChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 7})
		require.NoError(t, err)
		require.Equal(t, &agentproto.RenewChatLeaseResponse{Renewed: true}, resp)
	})

	t.Run("StaleEpoch", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().RenewChatLeaseByAgent(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params database.RenewChatLeaseByAgentParams) (uuid.UUID, error) {
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, agentID, params.AgentID)
			require.Equal(t, int64(7), params.LeaseEpoch)
			require.False(t, params.Now.IsZero())
			return uuid.Nil, sql.ErrNoRows
		})

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.RenewChatLease(context.Background(), &agentproto.RenewChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 7})
		require.NoError(t, err)
		require.Equal(t, &agentproto.RenewChatLeaseResponse{Renewed: false}, resp)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbErr := xerrors.New("db boom")
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().RenewChatLeaseByAgent(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, params database.RenewChatLeaseByAgentParams) (uuid.UUID, error) {
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, agentID, params.AgentID)
			require.Equal(t, int64(7), params.LeaseEpoch)
			require.False(t, params.Now.IsZero())
			return uuid.Nil, dbErr
		})

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.RenewChatLease(context.Background(), &agentproto.RenewChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 7})
		requireDRPCCode(t, err, uint64(codes.Internal))
		require.Nil(t, resp)
	})
}

func TestReleaseChatLease(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, codersdk.Experiments{})

		chatID := uuid.New()
		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 1, FinalStatus: string(database.ChatStatusCompleted)})
		requireDRPCCode(t, err, drpcerr.Unimplemented)
		require.Nil(t, resp)
	})

	t.Run("InvalidChatID", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: []byte{1, 2, 3}, LeaseEpoch: 1, FinalStatus: string(database.ChatStatusCompleted)})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("InvalidLeaseEpoch", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 0, FinalStatus: string(database.ChatStatusCompleted)})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("InvalidFinalStatus", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 1, FinalStatus: string(database.ChatStatusRunning)})
		requireDRPCCode(t, err, uint64(codes.InvalidArgument))
		require.Nil(t, resp)
	})

	t.Run("OK_Completed", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 3, Valid: true},
		}).Return(database.Chat{}, nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 3, FinalStatus: string(database.ChatStatusCompleted)})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
	})

	t.Run("OK_Error", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusError,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{String: "something broke", Valid: true},
			LeaseEpoch:  sql.NullInt64{Int64: 4, Valid: true},
		}).Return(database.Chat{}, nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 4, FinalStatus: string(database.ChatStatusError), Error: "something broke"})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
	})

	t.Run("OK_Waiting", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusWaiting,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 5, Valid: true},
		}).Return(database.Chat{}, nil)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 5, FinalStatus: string(database.ChatStatusWaiting)})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
	})

	t.Run("OK_RequiresAction_Callback", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		updatedChat := database.Chat{ID: chatID, Status: database.ChatStatusRequiresAction, LeaseEpoch: 6}
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusRequiresAction,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 6, Valid: true},
		}).Return(updatedChat, nil)

		called := false
		var gotChat database.Chat
		api := &agentapi.ChatRunnerAPI{
			AgentID:     agentID,
			Database:    dbM,
			Log:         slogtest.Make(t, nil),
			Experiments: enabledChatRunnerExperiments(),
			OnRequiresAction: func(ctx context.Context, chat database.Chat) error {
				called = true
				gotChat = chat
				return nil
			},
		}

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 6, FinalStatus: string(database.ChatStatusRequiresAction)})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
		require.True(t, called)
		require.Equal(t, updatedChat, gotChat)
	})

	t.Run("StatusChangeCallback_InvokedForFinalStatuses", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			status     database.ChatStatus
			errorMsg   string
			leaseEpoch int64
		}{
			{name: "Waiting", status: database.ChatStatusWaiting, leaseEpoch: 11},
			{name: "Completed", status: database.ChatStatusCompleted, leaseEpoch: 12},
			{name: "Error", status: database.ChatStatusError, errorMsg: "something broke", leaseEpoch: 13},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				chatID := uuid.New()
				updatedChat := database.Chat{ID: chatID, Status: tt.status, LeaseEpoch: tt.leaseEpoch}
				dbM := dbmock.NewMockStore(gomock.NewController(t))
				dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
					ID:          chatID,
					Status:      tt.status,
					WorkerID:    uuid.NullUUID{},
					StartedAt:   sql.NullTime{},
					HeartbeatAt: sql.NullTime{},
					LastError:   sql.NullString{String: tt.errorMsg, Valid: tt.errorMsg != ""},
					LeaseEpoch:  sql.NullInt64{Int64: tt.leaseEpoch, Valid: true},
				}).Return(updatedChat, nil)

				called := false
				var gotChat database.Chat
				api := &agentapi.ChatRunnerAPI{
					AgentID:     agentID,
					Database:    dbM,
					Log:         slogtest.Make(t, nil),
					Experiments: enabledChatRunnerExperiments(),
					OnChatStatusChange: func(ctx context.Context, chat database.Chat) error {
						called = true
						gotChat = chat
						return nil
					},
				}

				resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: tt.leaseEpoch, FinalStatus: string(tt.status), Error: tt.errorMsg})
				require.NoError(t, err)
				require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
				require.True(t, called)
				require.Equal(t, updatedChat, gotChat)
			})
		}
	})

	t.Run("RequiresAction_InvokesBothCallbacks", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		updatedChat := database.Chat{ID: chatID, Status: database.ChatStatusRequiresAction, LeaseEpoch: 14}
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusRequiresAction,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 14, Valid: true},
		}).Return(updatedChat, nil)

		requiresActionCalled := false
		statusChangeCalled := false
		var requiresActionChat database.Chat
		var statusChangeChat database.Chat
		api := &agentapi.ChatRunnerAPI{
			AgentID:     agentID,
			Database:    dbM,
			Log:         slogtest.Make(t, nil),
			Experiments: enabledChatRunnerExperiments(),
			OnRequiresAction: func(ctx context.Context, chat database.Chat) error {
				requiresActionCalled = true
				requiresActionChat = chat
				return nil
			},
			OnChatStatusChange: func(ctx context.Context, chat database.Chat) error {
				statusChangeCalled = true
				statusChangeChat = chat
				return nil
			},
		}

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 14, FinalStatus: string(database.ChatStatusRequiresAction)})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
		require.True(t, requiresActionCalled)
		require.True(t, statusChangeCalled)
		require.Equal(t, updatedChat, requiresActionChat)
		require.Equal(t, updatedChat, statusChangeChat)
	})

	t.Run("StatusChangeCallbackFailure_IsBestEffort", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 15, Valid: true},
		}).Return(database.Chat{ID: chatID, Status: database.ChatStatusCompleted}, nil)

		sink := testutil.NewFakeSink(t)
		api := &agentapi.ChatRunnerAPI{
			AgentID:     agentID,
			Database:    dbM,
			Log:         sink.Logger(),
			Experiments: enabledChatRunnerExperiments(),
			OnChatStatusChange: func(ctx context.Context, chat database.Chat) error {
				return xerrors.New("publish boom")
			},
		}

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 15, FinalStatus: string(database.ChatStatusCompleted)})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
		warns := sink.Entries(func(entry slog.SinkEntry) bool {
			return entry.Level == slog.LevelWarn && entry.Message == "post-commit status change publish failed"
		})
		require.Len(t, warns, 1)
	})

	t.Run("NonRequiresAction_DoesNotInvokeCallback", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			status     database.ChatStatus
			errorMsg   string
			leaseEpoch int64
		}{
			{name: "Waiting", status: database.ChatStatusWaiting, leaseEpoch: 7},
			{name: "Completed", status: database.ChatStatusCompleted, leaseEpoch: 8},
			{name: "Error", status: database.ChatStatusError, errorMsg: "something broke", leaseEpoch: 9},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				chatID := uuid.New()
				dbM := dbmock.NewMockStore(gomock.NewController(t))
				dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
					ID:          chatID,
					Status:      tt.status,
					WorkerID:    uuid.NullUUID{},
					StartedAt:   sql.NullTime{},
					HeartbeatAt: sql.NullTime{},
					LastError:   sql.NullString{String: tt.errorMsg, Valid: tt.errorMsg != ""},
					LeaseEpoch:  sql.NullInt64{Int64: tt.leaseEpoch, Valid: true},
				}).Return(database.Chat{ID: chatID, Status: tt.status}, nil)

				invoked := false
				api := &agentapi.ChatRunnerAPI{
					AgentID:     agentID,
					Database:    dbM,
					Log:         slogtest.Make(t, nil),
					Experiments: enabledChatRunnerExperiments(),
					OnRequiresAction: func(ctx context.Context, chat database.Chat) error {
						invoked = true
						return nil
					},
				}

				resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: tt.leaseEpoch, FinalStatus: string(tt.status), Error: tt.errorMsg})
				require.NoError(t, err)
				require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
				require.False(t, invoked)
			})
		}
	})

	t.Run("RequiresActionCallbackFailure_IsBestEffort", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusRequiresAction,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 10, Valid: true},
		}).Return(database.Chat{ID: chatID, Status: database.ChatStatusRequiresAction}, nil)

		sink := testutil.NewFakeSink(t)
		api := &agentapi.ChatRunnerAPI{
			AgentID:     agentID,
			Database:    dbM,
			Log:         sink.Logger(),
			Experiments: enabledChatRunnerExperiments(),
			OnRequiresAction: func(ctx context.Context, chat database.Chat) error {
				return xerrors.New("publish boom")
			},
		}

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 10, FinalStatus: string(database.ChatStatusRequiresAction)})
		require.NoError(t, err)
		require.Equal(t, &agentproto.ReleaseChatLeaseResponse{}, resp)
		warns := sink.Entries(func(entry slog.SinkEntry) bool {
			return entry.Level == slog.LevelWarn && entry.Message == "post-commit requires_action publish failed"
		})
		require.Len(t, warns, 1)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 6, Valid: true},
		}).Return(database.Chat{}, sql.ErrNoRows)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 6, FinalStatus: string(database.ChatStatusCompleted)})
		requireDRPCCode(t, err, uint64(codes.NotFound))
		require.Nil(t, resp)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbErr := xerrors.New("db boom")
		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateChatStatus(gomock.Any(), database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
			LeaseEpoch:  sql.NullInt64{Int64: 6, Valid: true},
		}).Return(database.Chat{}, dbErr)

		api := newChatRunnerAPI(t, agentID, dbM, enabledChatRunnerExperiments())

		resp, err := api.ReleaseChatLease(context.Background(), &agentproto.ReleaseChatLeaseRequest{ChatId: chatID[:], LeaseEpoch: 6, FinalStatus: string(database.ChatStatusCompleted)})
		requireDRPCCode(t, err, uint64(codes.Internal))
		require.Nil(t, resp)
	})
}

func enabledChatRunnerExperiments() codersdk.Experiments {
	return codersdk.Experiments{codersdk.ExperimentAgentChatRunner}
}

func newChatRunnerAPI(t *testing.T, agentID uuid.UUID, db database.Store, experiments codersdk.Experiments) *agentapi.ChatRunnerAPI {
	t.Helper()

	return &agentapi.ChatRunnerAPI{
		AgentID:     agentID,
		Database:    db,
		Log:         slogtest.Make(t, nil),
		Experiments: experiments,
	}
}

func requireDRPCCode(t *testing.T, err error, wantCode uint64) {
	t.Helper()

	require.Error(t, err)
	require.Equal(t, wantCode, drpcerr.Code(err))
}
