package provisionerdserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestCompleteJob_ClosesOpenConnectionLogsOnStopOrDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		transition database.WorkspaceTransition
		reason     string
	}{
		{
			name:       "Stop",
			transition: database.WorkspaceTransitionStop,
			reason:     "workspace stopped",
		},
		{
			name:       "Delete",
			transition: database.WorkspaceTransitionDelete,
			reason:     "workspace deleted",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, db, ps, pd := setup(t, false, nil)

			user := dbgen.User(t, db, database.User{})
			template := dbgen.Template(t, db, database.Template{
				Name:           "template",
				CreatedBy:      user.ID,
				Provisioner:    database.ProvisionerTypeEcho,
				OrganizationID: pd.OrganizationID,
			})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			workspaceTable := dbgen.Workspace(t, db, database.WorkspaceTable{
				TemplateID:     template.ID,
				OwnerID:        user.ID,
				OrganizationID: pd.OrganizationID,
			})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: pd.OrganizationID,
				CreatedBy:      user.ID,
				TemplateID: uuid.NullUUID{
					UUID:  template.ID,
					Valid: true,
				},
				JobID: uuid.New(),
			})

			wsBuildID := uuid.New()
			job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				ID:          uuid.New(),
				FileID:      file.ID,
				InitiatorID: user.ID,
				Type:        database.ProvisionerJobTypeWorkspaceBuild,
				Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
					WorkspaceBuildID: wsBuildID,
				})),
				OrganizationID: pd.OrganizationID,
			})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				ID:                wsBuildID,
				JobID:             job.ID,
				WorkspaceID:       workspaceTable.ID,
				TemplateVersionID: version.ID,
				BuildNumber:       2,
				Transition:        tc.transition,
				Reason:            database.BuildReasonInitiator,
			})

			_, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				OrganizationID: pd.OrganizationID,
				WorkerID: uuid.NullUUID{
					UUID:  pd.ID,
					Valid: true,
				},
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: must(json.Marshal(job.Tags)),
				StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
			})
			require.NoError(t, err)

			// Insert an open SSH connection log for the workspace.
			ip := pqtype.Inet{
				Valid: true,
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
			}

			openLog, err := db.UpsertConnectionLog(ctx, database.UpsertConnectionLogParams{
				ID:               uuid.New(),
				Time:             dbtime.Now(),
				OrganizationID:   workspaceTable.OrganizationID,
				WorkspaceOwnerID: workspaceTable.OwnerID,
				WorkspaceID:      workspaceTable.ID,
				WorkspaceName:    workspaceTable.Name,
				AgentName:        "agent",
				Type:             database.ConnectionTypeSsh,
				Ip:               ip,
				ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
				ConnectionStatus: database.ConnectionStatusConnected,
			})
			require.NoError(t, err)
			require.False(t, openLog.DisconnectTime.Valid)

			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
						State:     []byte{},
						Resources: []*sdkproto.Resource{},
					},
				},
			})
			require.NoError(t, err)

			rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{WorkspaceID: workspaceTable.ID})
			require.NoError(t, err)
			require.Len(t, rows, 1)

			updated := rows[0].ConnectionLog
			require.Equal(t, openLog.ID, updated.ID)
			require.True(t, updated.DisconnectTime.Valid)
			require.True(t, updated.DisconnectReason.Valid)
			require.Equal(t, tc.reason, updated.DisconnectReason.String)
			require.False(t, updated.DisconnectTime.Time.Before(updated.ConnectTime), "disconnect_time should never be before connect_time")
		})
	}
}
