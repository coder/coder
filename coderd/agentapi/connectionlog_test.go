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
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
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
		// agentBuildID is the build that created the agent.
		agentBuildID = uuid.New()
	)

	type testCase struct {
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
		rxBytes         *int64
		txBytes         *int64
		build           *database.WorkspaceBuild
		jobStatus       database.ProvisionerJobStatus
		buildErr        error
		jobErr          error
		wantReason      string
	}
	tests := []testCase{
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
		{
			name:       "Method And App Connect",
			id:         uuid.New(),
			action:     agentproto.Connection_CONNECT.Enum(),
			typ:        agentproto.Connection_TYPE_UNSPECIFIED.Enum(),
			reqMethod:  agentproto.Connection_METHOD_SSH,
			reqAppName: "Some-IDE",
			method:     database.ConnectionLogMethodSSH,
			appName:    "some_ide",
			time:       dbtime.Now(),
		},
		{
			name:    "Bytes Transferred Disconnect",
			id:      uuid.New(),
			action:  agentproto.Connection_DISCONNECT.Enum(),
			typ:     agentproto.Connection_SSH.Enum(),
			method:  database.ConnectionLogMethodSSH,
			time:    dbtime.Now(),
			rxBytes: new(int64(10)),
			txBytes: new(int64(50)),
		},
		{
			name:    "Zero Bytes Transferred Disconnect",
			id:      uuid.New(),
			action:  agentproto.Connection_DISCONNECT.Enum(),
			typ:     agentproto.Connection_SSH.Enum(),
			method:  database.ConnectionLogMethodSSH,
			time:    dbtime.Now(),
			rxBytes: new(int64(0)),
			txBytes: new(int64(0)),
		},
	}
	for _, transition := range []database.WorkspaceTransition{
		database.WorkspaceTransitionStart, database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete,
	} {
		for _, status := range []database.ProvisionerJobStatus{
			database.ProvisionerJobStatusPending, database.ProvisionerJobStatusRunning,
			database.ProvisionerJobStatusSucceeded, database.ProvisionerJobStatusCanceling,
			database.ProvisionerJobStatusCanceled, database.ProvisionerJobStatusFailed,
		} {
			tt := testCase{
				name: string(transition) + "/" + string(status), id: uuid.New(),
				action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_SSH.Enum(), method: database.ConnectionLogMethodSSH,
				time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
				clientSessionID: "0123456789abcdef0123456789abcdef",
				build: &database.WorkspaceBuild{
					ID: uuid.New(), JobID: uuid.New(), Transition: transition,
					Reason: database.BuildReasonAutostop, CreatedAt: dbtime.Now().Add(-time.Minute),
				}, jobStatus: status,
			}
			// Any newer build replaced the agent once its job runs.
			if status == database.ProvisionerJobStatusRunning || status == database.ProvisionerJobStatusSucceeded {
				tt.wantReason = string(codersdk.DisconnectReasonWorkspaceStopped)
			}
			tests = append(tests, tt)
		}
	}
	for _, reason := range []codersdk.DisconnectReason{
		codersdk.DisconnectReasonClientClosed, codersdk.DisconnectReasonNetworkError, codersdk.DisconnectReasonGraceful,
	} {
		tests = append(tests, testCase{
			name: "Preserve/" + string(reason), id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_SSH.Enum(), method: database.ConnectionLogMethodSSH,
			time: dbtime.Now(), reason: string(reason),
		})
	}
	queryErr := xerrors.New("database unavailable")
	tests = append(tests,
		testCase{
			name: "BuildLookupError", id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_SSH.Enum(), method: database.ConnectionLogMethodSSH,
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown), buildErr: queryErr,
		},
		testCase{
			name: "JobLookupError", id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_JETBRAINS.Enum(), method: database.ConnectionLogMethodSSH, appName: "jetbrains",
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
			build: &database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), Transition: database.WorkspaceTransitionStop}, jobErr: queryErr,
		},
		testCase{
			// The agent's own build is still the latest, so no build ended it.
			name: "AgentBuildIsLatest", id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_SSH.Enum(), method: database.ConnectionLogMethodSSH,
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
			build: &database.WorkspaceBuild{ID: agentBuildID, JobID: uuid.New(), Transition: database.WorkspaceTransitionStart, CreatedAt: dbtime.Now().Add(-time.Hour)},
		},
		testCase{
			name: "LaterStopBuild", id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_SSH.Enum(), method: database.ConnectionLogMethodSSH,
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
			build: &database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), Transition: database.WorkspaceTransitionStop, CreatedAt: dbtime.Now().Add(time.Minute)},
		},
		testCase{
			name: "ConnectWithShutdownReason", id: uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(), typ: agentproto.Connection_SSH.Enum(), method: database.ConnectionLogMethodSSH,
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
		},
		testCase{
			name: "JetBrainsShutdown", id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_JETBRAINS.Enum(), method: database.ConnectionLogMethodSSH, appName: "jetbrains",
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
			build: &database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), Transition: database.WorkspaceTransitionStop}, jobStatus: database.ProvisionerJobStatusRunning,
			wantReason: string(codersdk.DisconnectReasonWorkspaceStopped),
		},
		testCase{
			name: "ReconnectingPTYShutdown", id: uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(), typ: agentproto.Connection_RECONNECTING_PTY.Enum(), method: database.ConnectionLogMethodReconnectingPTY,
			time: dbtime.Now(), reason: string(codersdk.DisconnectReasonServerShutdown),
			clientSessionID: "0123456789abcdef0123456789abcdef",
			build:           &database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), Transition: database.WorkspaceTransitionDelete}, jobStatus: database.ProvisionerJobStatusSucceeded,
			wantReason: string(codersdk.DisconnectReasonWorkspaceStopped),
		},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			connLogger := connectionlog.NewFake()

			mDB := dbmock.NewMockStore(gomock.NewController(t))
			mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)

			if *tt.action == agentproto.Connection_DISCONNECT && tt.reason == string(codersdk.DisconnectReasonServerShutdown) {
				build := database.WorkspaceBuild{}
				if tt.build != nil {
					build = *tt.build
				}
				mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspace.ID).Return(build, tt.buildErr)
				if tt.buildErr == nil && !build.CreatedAt.After(tt.time) && build.ID != agentBuildID {
					mDB.EXPECT().GetProvisionerJobByID(gomock.Any(), build.JobID).Return(database.ProvisionerJob{JobStatus: tt.jobStatus}, tt.jobErr)
				}
			}

			api := &agentapi.ConnLogAPI{
				ConnectionLogger: asAtomicPointer[connectionlog.ConnectionLogger](connLogger),
				Database:         mDB,
				AgentID:          agent.ID,
				AgentName:        agent.Name,
				WorkspaceBuildID: agentBuildID,
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
					RxBytes:          tt.rxBytes,
					TxBytes:          tt.txBytes,
				},
			})
			require.NoError(t, err)
			if tt.wantReason == "" {
				tt.wantReason = tt.reason
			}

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
					String: tt.wantReason,
					Valid:  tt.wantReason != "",
				},
				ConnectionID: uuid.NullUUID{
					UUID:  tt.id,
					Valid: tt.id != uuid.Nil,
				},
				ClientSessionID: sql.NullString{
					String: tt.clientSessionID,
					Valid:  tt.clientSessionID != "",
				},
				RxBytes: sql.NullInt64{
					Int64: ptr.NilToEmpty(tt.rxBytes),
					Valid: tt.rxBytes != nil,
				},
				TxBytes: sql.NullInt64{
					Int64: ptr.NilToEmpty(tt.txBytes),
					Valid: tt.txBytes != nil,
				},
			}, logs[0])
		})
	}
}

// An agent connection type this coderd does not know is rejected rather than
// logged with a guessed method.
func TestConnectionLogRejectsUnknownType(t *testing.T) {
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
			Id:        id[:],
			Action:    agentproto.Connection_CONNECT,
			Type:      agentproto.Connection_Type(1000),
			Timestamp: timestamppb.New(dbtime.Now()),
		},
	})
	require.Error(t, err)
	require.Empty(t, connLogger.ConnectionLogs())
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
