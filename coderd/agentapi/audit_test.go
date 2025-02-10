package agentapi_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
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
		name    string
		id      uuid.UUID
		action  *agentproto.Connection_Action
		typ     *agentproto.Connection_Type
		time    time.Time
		discard bool
	}{
		{
			name:   "SSH Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_SSH.Enum(),
			time:   time.Now(),
		},
		{
			name:   "VS Code Connect",
			id:     uuid.New(),
			action: agentproto.Connection_CONNECT.Enum(),
			typ:    agentproto.Connection_VSCODE.Enum(),
			time:   time.Now(),
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

		// Discard disconnects, for now.
		{
			name:    "SSH Disconnect",
			id:      uuid.New(),
			action:  agentproto.Connection_DISCONNECT.Enum(),
			typ:     agentproto.Connection_SSH.Enum(),
			time:    time.Now(),
			discard: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mAudit := audit.NewMock()

			mDB := dbmock.NewMockStore(gomock.NewController(t))
			if !tt.discard {
				mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(workspace, nil)
				mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspace.ID).Return(build, nil)
			}

			api := &agentapi.AuditAPI{
				Auditor:  asAtomicPointer[audit.Auditor](mAudit),
				Database: mDB,
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return agent, nil
				},
			}
			api.ReportConnection(context.Background(), &agentproto.ReportConnectionRequest{
				Connection: &agentproto.Connection{
					Id:        tt.id[:],
					Action:    *tt.action,
					Type:      *tt.typ,
					Timestamp: timestamppb.New(tt.time),
				},
			})

			if tt.discard {
				require.Len(t, mAudit.AuditLogs(), 0)
				return
			}

			mAudit.Contains(t, database.AuditLog{
				Time:           dbtime.Time(tt.time),
				Action:         database.AuditActionConnect,
				OrganizationID: workspace.OrganizationID,
				UserID:         owner.ID,
				RequestID:      tt.id,
				ResourceType:   database.ResourceTypeWorkspaceAgent,
				ResourceID:     agent.ID,
				ResourceTarget: agent.Name,
			})

			// Check some additional fields.
			var m map[string]any
			err := json.Unmarshal(mAudit.AuditLogs()[0].AdditionalFields, &m)
			require.NoError(t, err)
			require.Equal(t, agentProtoConnectionTypeToSDK(t, *tt.typ), agentsdk.ConnectionType((m["connection_type"]).(string)))
		})
	}
}

func agentProtoConnectionTypeToSDK(t *testing.T, typ agentproto.Connection_Type) agentsdk.ConnectionType {
	switch typ {
	case agentproto.Connection_SSH:
		return agentsdk.ConnectionTypeSSH
	case agentproto.Connection_VSCODE:
		return agentsdk.ConnectionTypeVSCode
	case agentproto.Connection_JETBRAINS:
		return agentsdk.ConnectionTypeJetBrains
	case agentproto.Connection_RECONNECTING_PTY:
		return agentsdk.ConnectionTypeReconnectingPTY
	default:
		t.Fatalf("unknown agent connection type %q", typ)
		return ""
	}
}

func asAtomicPointer[T any](v T) *atomic.Pointer[T] {
	var p atomic.Pointer[T]
	p.Store(&v)
	return &p
}
