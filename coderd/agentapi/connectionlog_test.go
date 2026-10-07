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
		name            string
		id              uuid.UUID
		action          *agentproto.Connection_Action
		typ             *agentproto.Connection_Type
		reqMethod       agentproto.Connection_Method
		reqAppName      string
		method          database.ConnectionLogMethod
		appName         string
		time            time.Time
		ip              string
		status          int32
		reason          string
		clientSessionID string
	}{
		{
			name:            "SSH Connect",
			id:              uuid.New(),
			action:          agentproto.Connection_CONNECT.Enum(),
			typ:             agentproto.Connection_SSH.Enum(),
			method:          database.ConnectionLogMethodSSH,
			time:            dbtime.Now(),
			ip:              "127.0.0.1",
			status:          200,
			clientSessionID: "0123456789abcdef0123456789abcdef",
		},
		{
			name:    "VS Code Connect",
			id:      uuid.New(),
			action:  agentproto.Connection_CONNECT.Enum(),
			typ:     agentproto.Connection_VSCODE.Enum(),
			method:  database.ConnectionLogMethodSSH,
			appName: "vscode",
			time:    dbtime.Now(),
			ip:      "8.8.8.8",
		},
		{
			name:    "JetBrains Connect",
			id:      uuid.New(),
			action:  agentproto.Connection_CONNECT.Enum(),
			typ:     agentproto.Connection_JETBRAINS.Enum(),
			method:  database.ConnectionLogMethodSSH,
			appName: "jetbrains",
			time:    dbtime.Now(),
			// Sometimes, JetBrains clients report as localhost, see
			// https://github.com/coder/coder/issues/20194
			ip: "localhost",
		},
		{
			name:   "Reconnecting PTY Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_RECONNECTING_PTY.Enum(),
			method: database.ConnectionLogMethodReconnectingPTY,
			time:   dbtime.Now(),
		},
		{
			// SSH handlers reported unfamiliar apps as unspecified.
			name:   "Unspecified Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_TYPE_UNSPECIFIED.Enum(),
			method: database.ConnectionLogMethodSSH,
			time:   dbtime.Now(),
		},
		{
			name:   "SSH Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			method: database.ConnectionLogMethodSSH,
			time:   dbtime.Now(),
		},
		{
			name:   "SSH Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			method: database.ConnectionLogMethodSSH,
			time:   dbtime.Now(),
			status: 500,
			reason: "because error says so",
		},
		{
			name:    "VS Code Disconnect",
			id:      uuid.New(),
			action:  agentproto.Connection_DISCONNECT.Enum(),
			typ:     agentproto.Connection_VSCODE.Enum(),
			method:  database.ConnectionLogMethodSSH,
			appName: "vscode",
			time:    dbtime.Now(),
		},
		// Disconnects repeat the method and app.
		{
			name:            "Unfamiliar App Connect",
			id:              uuid.New(),
			action:          agentproto.Connection_CONNECT.Enum(),
			typ:             agentproto.Connection_TYPE_UNSPECIFIED.Enum(),
			reqMethod:       agentproto.Connection_METHOD_SSH,
			reqAppName:      "Some-New-IDE",
			method:          database.ConnectionLogMethodSSH,
			appName:         "some_new_ide",
			time:            dbtime.Now(),
			clientSessionID: "0123456789abcdef0123456789abcdef",
		},
		{
			name:            "Unfamiliar App Disconnect",
			id:              uuid.New(),
			action:          agentproto.Connection_DISCONNECT.Enum(),
			typ:             agentproto.Connection_TYPE_UNSPECIFIED.Enum(),
			reqMethod:       agentproto.Connection_METHOD_SSH,
			reqAppName:      "Some-New-IDE",
			method:          database.ConnectionLogMethodSSH,
			appName:         "some_new_ide",
			time:            dbtime.Now(),
			reason:          "because",
			clientSessionID: "0123456789abcdef0123456789abcdef",
		},
		// A forward's column holds its destination, not an app.
		{
			name:            "Port Forwarding Connect",
			id:              uuid.New(),
			action:          agentproto.Connection_CONNECT.Enum(),
			typ:             agentproto.Connection_TYPE_UNSPECIFIED.Enum(),
			reqMethod:       agentproto.Connection_METHOD_PORT_FORWARDING,
			reqAppName:      "Some-App",
			method:          database.ConnectionLogMethodPortForwarding,
			time:            dbtime.Now(),
			clientSessionID: "0123456789abcdef0123456789abcdef",
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
			_, err := api.ReportConnection(context.Background(), &agentproto.ReportConnectionRequest{
				Connection: &agentproto.Connection{
					Id:               tt.id[:],
					Action:           *tt.action,
					Type:             *tt.typ,
					Timestamp:        timestamppb.New(tt.time),
					Ip:               tt.ip,
					StatusCode:       tt.status,
					Reason:           &tt.reason,
					ClientSessionId:  tt.clientSessionID,
					AppName:          tt.reqAppName,
					ConnectionMethod: tt.reqMethod,
				},
			})
			require.NoError(t, err)

			expectedIPRaw := tt.ip
			if expectedIPRaw == "localhost" {
				expectedIPRaw = "127.0.0.1"
			}
			expectedIP := database.ParseIP(expectedIPRaw)
			// Only disconnects carry a status code.
			var expectedCode sql.NullInt32
			if *tt.action == agentproto.Connection_DISCONNECT {
				expectedCode = sql.NullInt32{Int32: tt.status, Valid: true}
			}

			// Compare every field, since Contains skips fields left unset.
			logs := connLogger.ConnectionLogs()
			require.Len(t, logs, 1)
			require.Equal(t, database.UpsertConnectionLogParams{
				// The ID is generated per report.
				ID:               logs[0].ID,
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

				Code:             expectedCode,
				IP:               expectedIP,
				ConnectionMethod: tt.method,
				AppNameOrPort:    sql.NullString{String: tt.appName, Valid: tt.appName != ""},
				DisconnectReason: sql.NullString{
					String: tt.reason,
					Valid:  tt.reason != "",
				},
				ConnectionID: uuid.NullUUID{
					UUID:  tt.id,
					Valid: tt.id != uuid.Nil,
				},
				ClientSessionID: sql.NullString{
					String: tt.clientSessionID,
					Valid:  tt.clientSessionID != "",
				},
			}, logs[0])
		})
	}
}

// Unknown types and methods are rejected, not logged with a guess.
func TestConnectionLogRejectsUnknownType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		typ    agentproto.Connection_Type
		method agentproto.Connection_Method
	}{
		{"UnknownType", agentproto.Connection_Type(1000), agentproto.Connection_METHOD_UNSPECIFIED},
		{"UnknownMethod", agentproto.Connection_SSH, agentproto.Connection_Method(42)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			connLogger := connectionlog.NewFake()
			api := &agentapi.ConnLogAPI{
				ConnectionLogger: asAtomicPointer[connectionlog.ConnectionLogger](connLogger),
				Database:         dbmock.NewMockStore(gomock.NewController(t)),
				AgentID:          uuid.New(),
				Workspace:        &agentapi.CachedWorkspaceFields{},
			}
			id := uuid.New()
			_, err := api.ReportConnection(context.Background(), &agentproto.ReportConnectionRequest{
				Connection: &agentproto.Connection{
					Id:               id[:],
					Action:           agentproto.Connection_CONNECT,
					Type:             tc.typ,
					ConnectionMethod: tc.method,
					Timestamp:        timestamppb.New(dbtime.Now()),
				},
			})
			require.Error(t, err)
			require.Empty(t, connLogger.ConnectionLogs())
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
