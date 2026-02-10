package agentapi_test

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/wspubsub"
)

func TestConnectionLog(t *testing.T) {
	t.Parallel()

	var (
		owner = database.User{
			ID:       uuid.New(),
			Username: "cool-user",
		}
		workspace = database.Workspace{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			OwnerID:        owner.ID,
			Name:           "cool-workspace",
		}
		agent = database.WorkspaceAgent{
			ID: uuid.New(),
		}
	)

	tests := []struct {
		name       string
		id         uuid.UUID
		action     *agentproto.Connection_Action
		typ        *agentproto.Connection_Type
		time       time.Time
		ip         string
		status     int32
		reason     string
		slugOrPort string
	}{
		{
			name:   "SSH Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			time:   dbtime.Now(),
			ip:     "127.0.0.1",
			status: 200,
		},
		{
			name:   "VS Code Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_VSCODE.Enum(),
			time:   dbtime.Now(),
			ip:     "8.8.8.8",
		},
		{
			name:   "JetBrains Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_JETBRAINS.Enum(),
			time:   dbtime.Now(),
			// Sometimes, JetBrains clients report as localhost, see
			// https://github.com/coder/coder/issues/20194
			ip: "localhost",
		},
		{
			name:   "Reconnecting PTY Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_RECONNECTING_PTY.Enum(),
			time:   dbtime.Now(),
		},
		{
			name:       "Port Forwarding Connect",
			id:         uuid.New(),
			action:     agentproto.Connection_CONNECT.Enum(),
			typ:        agentproto.Connection_PORT_FORWARDING.Enum(),
			time:       dbtime.Now(),
			ip:         "192.168.1.1",
			slugOrPort: "8080",
		},
		{
			name:       "Port Forwarding Disconnect",
			id:         uuid.New(),
			action:     agentproto.Connection_DISCONNECT.Enum(),
			typ:        agentproto.Connection_PORT_FORWARDING.Enum(),
			time:       dbtime.Now(),
			ip:         "192.168.1.1",
			status:     200,
			slugOrPort: "8080",
		},
		{
			name:       "Workspace App Connect",
			id:         uuid.New(),
			action:     agentproto.Connection_CONNECT.Enum(),
			typ:        agentproto.Connection_WORKSPACE_APP.Enum(),
			time:       dbtime.Now(),
			ip:         "10.0.0.1",
			slugOrPort: "my-app",
		},
		{
			name:   "SSH Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			time:   dbtime.Now(),
		},
		{
			name:   "SSH Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			time:   dbtime.Now(),
			status: 500,
			reason: "because error says so",
		},
	}
	//nolint:paralleltest // No longer necessary to reinitialise the variable tt.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			connLogger := connectionlog.NewFake()

			mDB := dbmock.NewMockStore(gomock.NewController(t))
			mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)

			api := &agentapi.ConnLogAPI{
				ConnectionLogger: asAtomicPointer[connectionlog.ConnectionLogger](connLogger),
				Database:         mDB,
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return agent, nil
				},
				Workspace: &agentapi.CachedWorkspaceFields{},
			}
			api.ReportConnection(context.Background(), &agentproto.ReportConnectionRequest{
				Connection: &agentproto.Connection{
					Id:         tt.id[:],
					Action:     *tt.action,
					Type:       *tt.typ,
					Timestamp:  timestamppb.New(tt.time),
					Ip:         tt.ip,
					StatusCode: tt.status,
					Reason:     &tt.reason,
					SlugOrPort: &tt.slugOrPort,
				},
			})

			expectedIPRaw := tt.ip
			if expectedIPRaw == "localhost" {
				expectedIPRaw = "127.0.0.1"
			}
			expectedIP := database.ParseIP(expectedIPRaw)

			require.True(t, connLogger.Contains(t, database.UpsertConnectionLogParams{
				Time:             dbtime.Time(tt.time).In(time.UTC),
				OrganizationID:   workspace.OrganizationID,
				WorkspaceOwnerID: workspace.OwnerID,
				WorkspaceID:      workspace.ID,
				WorkspaceName:    workspace.Name,
				AgentName:        agent.Name,
				AgentID:          uuid.NullUUID{UUID: agent.ID, Valid: true},
				UserID: uuid.NullUUID{
					UUID:  uuid.Nil,
					Valid: false,
				},
				ConnectionStatus: agentProtoConnectionActionToConnectionLog(t, *tt.action),

				Code: sql.NullInt32{
					Int32: tt.status,
					Valid: *tt.action == agentproto.Connection_DISCONNECT,
				},
				Ip:   expectedIP,
				Type: agentProtoConnectionTypeToConnectionLog(t, *tt.typ),
				DisconnectReason: sql.NullString{
					String: tt.reason,
					Valid:  tt.reason != "",
				},
				ConnectionID: uuid.NullUUID{
					UUID:  tt.id,
					Valid: tt.id != uuid.Nil,
				},
				SlugOrPort: sql.NullString{
					String: tt.slugOrPort,
					Valid:  tt.slugOrPort != "",
				},
			}))
		})
	}
}

func TestConnectionLogPublishesWorkspaceUpdate(t *testing.T) {
	t.Parallel()

	var (
		owner     = database.User{ID: uuid.New(), Username: "cool-user"}
		workspace = database.Workspace{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			OwnerID:        owner.ID,
			Name:           "cool-workspace",
		}
		agent = database.WorkspaceAgent{ID: uuid.New()}
	)

	connLogger := connectionlog.NewFake()

	mDB := dbmock.NewMockStore(gomock.NewController(t))
	mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)

	var (
		called   int
		gotKind  wspubsub.WorkspaceEventKind
		gotAgent uuid.UUID
	)

	api := &agentapi.ConnLogAPI{
		ConnectionLogger: asAtomicPointer[connectionlog.ConnectionLogger](connLogger),
		Database:         mDB,
		AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
			return agent, nil
		},
		Workspace: &agentapi.CachedWorkspaceFields{},
		PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
			called++
			gotKind = kind
			gotAgent = agent.ID
			return nil
		},
	}

	id := uuid.New()
	_, err := api.ReportConnection(context.Background(), &agentproto.ReportConnectionRequest{
		Connection: &agentproto.Connection{
			Id:        id[:],
			Action:    agentproto.Connection_CONNECT,
			Type:      agentproto.Connection_SSH,
			Timestamp: timestamppb.New(dbtime.Now()),
			Ip:        "127.0.0.1",
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, called)
	require.Equal(t, wspubsub.WorkspaceEventKindConnectionLogUpdate, gotKind)
	require.Equal(t, agent.ID, gotAgent)
}

func agentProtoConnectionTypeToConnectionLog(t *testing.T, typ agentproto.Connection_Type) database.ConnectionType {
	a, err := db2sdk.ConnectionLogConnectionTypeFromAgentProtoConnectionType(typ)
	require.NoError(t, err)
	return a
}

func agentProtoConnectionActionToConnectionLog(t *testing.T, action agentproto.Connection_Action) database.ConnectionStatus {
	a, err := db2sdk.ConnectionLogStatusFromAgentProtoConnectionAction(action)
	require.NoError(t, err)
	return a
}

func asAtomicPointer[T any](v T) *atomic.Pointer[T] {
	var p atomic.Pointer[T]
	p.Store(&v)
	return &p
}
