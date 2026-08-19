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
		name   string
		id     uuid.UUID
		action *agentproto.Connection_Action
		typ    *agentproto.Connection_Type
		time   time.Time
		ip     string
		status int32
		reason string
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
		{
			name:   "File Transfer Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_FILE_TRANSFER.Enum(),
			time:   dbtime.Now(),
			ip:     "127.0.0.1",
		},
		{
			name:   "File Transfer Blocked Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_FILE_TRANSFER.Enum(),
			time:   dbtime.Now(),
			ip:     "127.0.0.1",
			status: 65,
			reason: "file transfer blocked",
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
				IP:   expectedIP,
				Type: agentProtoConnectionTypeToConnectionLog(t, *tt.typ),
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

func TestReportFileOperations(t *testing.T) {
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
			ID:   uuid.New(),
			Name: "cool-agent",
		}
	)

	newAPI := func(t *testing.T, connLogger connectionlog.ConnectionLogger, expectDBLookup bool) *agentapi.ConnLogAPI {
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		if expectDBLookup {
			mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)
		}
		return &agentapi.ConnLogAPI{
			ConnectionLogger: asAtomicPointer(connLogger),
			Database:         mDB,
			AgentID:          agent.ID,
			AgentName:        agent.Name,
			Workspace:        &agentapi.CachedWorkspaceFields{},
		}
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		connLogger := connectionlog.NewFake()
		api := newAPI(t, connLogger, true)

		sessionID := uuid.New()
		now := dbtime.Now()
		target := "/home/coder/b.txt"
		_, err := api.ReportFileOperations(context.Background(), &agentproto.ReportFileOperationsRequest{
			Operations: []*agentproto.FileTransferOperation{
				{
					ConnectionId: sessionID[:],
					Protocol:     agentproto.FileTransferOperation_SFTP,
					Action:       agentproto.FileTransferOperation_DOWNLOAD,
					Path:         "/home/coder/secret.txt",
					Timestamp:    timestamppb.New(now),
				},
				{
					ConnectionId: sessionID[:],
					Protocol:     agentproto.FileTransferOperation_SFTP,
					Action:       agentproto.FileTransferOperation_RENAME,
					Path:         "/home/coder/a.txt",
					Target:       &target,
					Timestamp:    timestamppb.New(now),
				},
			},
		})
		require.NoError(t, err)

		require.Len(t, connLogger.ConnectionLogs(), 2)
		require.True(t, connLogger.Contains(t, database.UpsertConnectionLogParams{
			Time:             dbtime.Time(now).In(time.UTC),
			OrganizationID:   workspace.OrganizationID,
			WorkspaceOwnerID: workspace.OwnerID,
			WorkspaceID:      workspace.ID,
			WorkspaceName:    workspace.Name,
			AgentName:        agent.Name,
			Type:             database.ConnectionTypeFileOperation,
			ConnectionStatus: database.ConnectionStatusConnected,
			ConnectionID:     uuid.NullUUID{UUID: sessionID, Valid: true},
			FileProtocol: database.NullConnectionLogFileProtocol{
				ConnectionLogFileProtocol: database.ConnectionLogFileProtocolSftp,
				Valid:                     true,
			},
			FileAction: database.NullConnectionLogFileAction{
				ConnectionLogFileAction: database.ConnectionLogFileActionDownload,
				Valid:                   true,
			},
			FilePath: sql.NullString{String: "/home/coder/secret.txt", Valid: true},
		}))
		require.True(t, connLogger.Contains(t, database.UpsertConnectionLogParams{
			Type: database.ConnectionTypeFileOperation,
			FileAction: database.NullConnectionLogFileAction{
				ConnectionLogFileAction: database.ConnectionLogFileActionRename,
				Valid:                   true,
			},
			FilePath:   sql.NullString{String: "/home/coder/a.txt", Valid: true},
			FileTarget: sql.NullString{String: target, Valid: true},
		}))
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		connLogger := connectionlog.NewFake()
		api := newAPI(t, connLogger, false)

		_, err := api.ReportFileOperations(context.Background(), &agentproto.ReportFileOperationsRequest{})
		require.NoError(t, err)
		require.Empty(t, connLogger.ConnectionLogs())
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		sessionID := uuid.New()
		now := dbtime.Now()
		tests := []struct {
			name string
			op   *agentproto.FileTransferOperation
		}{
			{
				name: "NilConnectionID",
				op: &agentproto.FileTransferOperation{
					ConnectionId: uuid.Nil[:],
					Protocol:     agentproto.FileTransferOperation_SFTP,
					Action:       agentproto.FileTransferOperation_DOWNLOAD,
					Path:         "/etc/passwd",
					Timestamp:    timestamppb.New(now),
				},
			},
			{
				name: "UnspecifiedProtocol",
				op: &agentproto.FileTransferOperation{
					ConnectionId: sessionID[:],
					Protocol:     agentproto.FileTransferOperation_PROTOCOL_UNSPECIFIED,
					Action:       agentproto.FileTransferOperation_DOWNLOAD,
					Path:         "/etc/passwd",
					Timestamp:    timestamppb.New(now),
				},
			},
			{
				name: "UnspecifiedAction",
				op: &agentproto.FileTransferOperation{
					ConnectionId: sessionID[:],
					Protocol:     agentproto.FileTransferOperation_SFTP,
					Action:       agentproto.FileTransferOperation_ACTION_UNSPECIFIED,
					Path:         "/etc/passwd",
					Timestamp:    timestamppb.New(now),
				},
			},
			{
				name: "EmptyPath",
				op: &agentproto.FileTransferOperation{
					ConnectionId: sessionID[:],
					Protocol:     agentproto.FileTransferOperation_SFTP,
					Action:       agentproto.FileTransferOperation_DOWNLOAD,
					Path:         "",
					Timestamp:    timestamppb.New(now),
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				connLogger := connectionlog.NewFake()
				api := newAPI(t, connLogger, true)

				_, err := api.ReportFileOperations(context.Background(), &agentproto.ReportFileOperationsRequest{
					Operations: []*agentproto.FileTransferOperation{tt.op},
				})
				require.Error(t, err)
				require.Empty(t, connLogger.ConnectionLogs())
			})
		}
	})
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
