package agentapi_test

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestAuditReport(t *testing.T) {
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
		build = database.WorkspaceBuild{
			ID:          uuid.New(),
			WorkspaceID: workspace.ID,
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
			time:   time.Now(),
			ip:     "127.0.0.1",
			status: 200,
		},
		{
			name:   "VS Code Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_VSCODE.Enum(),
			time:   time.Now(),
			ip:     "8.8.8.8",
		},
		{
			name:   "JetBrains Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_JETBRAINS.Enum(),
			time:   time.Now(),
		},
		{
			name:   "Reconnecting PTY Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_RECONNECTING_PTY.Enum(),
			time:   time.Now(),
		},
		{
			name:   "SSH Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			time:   time.Now(),
		},
		{
			name:   "SSH Disconnect",
			id:     uuid.New(),
			action: agentproto.Connection_DISCONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			time:   time.Now(),
			status: 500,
			reason: "because error says so",
		},
	}
	//nolint:paralleltest // No longer necessary to reinitialise the variable tt.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mAudit := audit.NewMock()

			mDB := dbmock.NewMockStore(gomock.NewController(t))
			mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)
			mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspace.ID).Return(build, nil)

			api := &agentapi.AuditAPI{
				Auditor:  asAtomicPointer[audit.Auditor](mAudit),
				Database: mDB,
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return agent, nil
				},
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

			mAudit.Contains(t, database.AuditLog{
				Time:           dbtime.Time(tt.time).In(time.UTC),
				Action:         agentProtoConnectionActionToAudit(t, *tt.action),
				OrganizationID: workspace.OrganizationID,
				UserID:         uuid.Nil,
				RequestID:      tt.id,
				ResourceType:   database.ResourceTypeWorkspaceAgent,
				ResourceID:     agent.ID,
				ResourceTarget: agent.Name,
				Ip:             pqtype.Inet{Valid: true, IPNet: net.IPNet{IP: net.ParseIP(tt.ip), Mask: net.CIDRMask(32, 32)}},
				StatusCode:     tt.status,
			})

			// Check some additional fields.
			var m map[string]any
			err := json.Unmarshal(mAudit.AuditLogs()[0].AdditionalFields, &m)
			require.NoError(t, err)
			require.Equal(t, string(agentProtoConnectionTypeToSDK(t, *tt.typ)), m["connection_type"].(string))
			if tt.reason != "" {
				require.Equal(t, tt.reason, m["reason"])
			}
		})
	}
}

func agentProtoConnectionActionToAudit(t *testing.T, action agentproto.Connection_Action) database.AuditAction {
	a, err := db2sdk.AuditActionFromAgentProtoConnectionAction(action)
	require.NoError(t, err)
	return a
}

func agentProtoConnectionTypeToSDK(t *testing.T, typ agentproto.Connection_Type) agentsdk.ConnectionType {
	action, err := agentsdk.ConnectionTypeFromProto(typ)
	require.NoError(t, err)
	return action
}

func asAtomicPointer[T any](v T) *atomic.Pointer[T] {
	var p atomic.Pointer[T]
	p.Store(&v)
	return &p
}
