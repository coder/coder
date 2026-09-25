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
		name    string
		id      uuid.UUID
		action  *agentproto.Connection_Action
		typ     *agentproto.Connection_Type
		kind    database.ConnectionKind
		appName string
		time    time.Time
		ip      string
		status  int32
		reason  string
	}{
		{
			name:    "SSH Connect",
			id:      uuid.New(),
			action:  agentproto.Connection_CONNECT.Enum(),
			typ:     agentproto.Connection_SSH.Enum(),
			kind:    database.ConnectionKindSSH,
			appName: "ssh",
			time:    dbtime.Now(),
			ip:      "127.0.0.1",
			status:  200,
		},
		{
			name:    "VS Code Connect",
			id:      uuid.New(),
			action:  agentproto.Connection_CONNECT.Enum(),
			typ:     agentproto.Connection_VSCODE.Enum(),
			kind:    database.ConnectionKindSSH,
			appName: "vscode",
			time:    dbtime.Now(),
			ip:      "8.8.8.8",
		},
		{
			name:    "JetBrains Connect",
			id:      uuid.New(),
			action:  agentproto.Connection_CONNECT.Enum(),
			typ:     agentproto.Connection_JETBRAINS.Enum(),
			kind:    database.ConnectionKindSSH,
			appName: "jetbrains",
			time:    dbtime.Now(),
			// Sometimes, JetBrains clients report as localhost, see
			// https://github.com/coder/coder/issues/20194
			ip: "localhost",
		},
		{
			name:    "Reconnecting PTY Connect",
			id:      uuid.New(),
			action:  agentproto.Connection_CONNECT.Enum(),
			typ:     agentproto.Connection_RECONNECTING_PTY.Enum(),
			kind:    database.ConnectionKindReconnectingPTY,
			appName: "reconnecting_pty",
			time:    dbtime.Now(),
		},
		{
			name:    "SSH Disconnect",
			id:      uuid.New(),
			action:  agentproto.Connection_DISCONNECT.Enum(),
			typ:     agentproto.Connection_SSH.Enum(),
			kind:    database.ConnectionKindSSH,
			appName: "ssh",
			time:    dbtime.Now(),
		},
		{
			name:    "SSH Disconnect",
			id:      uuid.New(),
			action:  agentproto.Connection_DISCONNECT.Enum(),
			typ:     agentproto.Connection_SSH.Enum(),
			kind:    database.ConnectionKindSSH,
			appName: "ssh",
			time:    dbtime.Now(),
			status:  500,
			reason:  "because error says so",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			connLogger := connectionlog.NewFake()

			mDB := dbmock.NewMockStore(gomock.NewController(t))
			mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)

			api := &agentapi.ConnLogAPI{
				ConnectionLogger: asAtomicPointer[connectionlog.ConnectionLogger](connLogger),
				Database:         mDB,
				AgentID:          agent.ID,
				AgentName:        agent.Name,
				Workspace:        &agentapi.CachedWorkspaceFields{},
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
				UserID: uuid.NullUUID{
					UUID:  uuid.Nil,
					Valid: false,
				},
				ConnectionStatus: agentProtoConnectionActionToConnectionLog(t, *tt.action),

				Code: sql.NullInt32{
					Int32: tt.status,
					Valid: *tt.action == agentproto.Connection_DISCONNECT,
				},
				IP:            expectedIP,
				Kind:          tt.kind,
				AppNameOrPort: sql.NullString{String: tt.appName, Valid: true},
				DisconnectReason: sql.NullString{
					String: tt.reason,
					Valid:  tt.reason != "",
				},
				ConnectionID: uuid.NullUUID{
					UUID:  tt.id,
					Valid: tt.id != uuid.Nil,
				},
			}))
		})
	}
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
