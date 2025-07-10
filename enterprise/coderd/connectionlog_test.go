package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

func TestConnectionLogs(t *testing.T) {
	t.Parallel()

	createWorkspace := func(t *testing.T, db database.Store) database.WorkspaceTable {
		u := dbgen.User(t, db, database.User{})
		o := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		return dbgen.Workspace(t, db, database.WorkspaceTable{
			ID:               uuid.New(),
			OwnerID:          u.ID,
			OrganizationID:   o.ID,
			AutomaticUpdates: database.AutomaticUpdatesNever,
			TemplateID:       tpl.ID,
		})
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client, db, _ := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			ConnectionLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog:      1,
					codersdk.FeatureConnectionLog: 1,
				},
			},
		})

		ws := createWorkspace(t, db)
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Type:             database.ConnectionTypeSsh,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.Equal(t, codersdk.ConnectionTypeSSH, logs.ConnectionLogs[0].Type)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client, _, _ := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			ConnectionLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog:      1,
					codersdk.FeatureConnectionLog: 1,
				},
			},
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)
		require.EqualValues(t, 0, logs.Count)
		require.Len(t, logs.ConnectionLogs, 0)
	})

	t.Run("ByOrganizationIDAndName", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client, db, _ := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			ConnectionLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog:      1,
					codersdk.FeatureConnectionLog: 1,
				},
			},
		})

		org := dbgen.Organization(t, db, database.Organization{})
		ws := createWorkspace(t, db)
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Type:             database.ConnectionTypeSsh,
			WorkspaceID:      ws.ID,
			OrganizationID:   org.ID,
			WorkspaceOwnerID: ws.OwnerID,
		})
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Type:             database.ConnectionTypeSsh,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
		})

		// By name
		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{
			SearchQuery: fmt.Sprintf("organization:%s", org.Name),
		})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.Equal(t, org.ID, logs.ConnectionLogs[0].Organization.ID)

		// By ID
		logs, err = client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{
			SearchQuery: fmt.Sprintf("organization:%s", ws.OrganizationID),
		})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.Equal(t, ws.OrganizationID, logs.ConnectionLogs[0].Organization.ID)
	})

	t.Run("WebInfo", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client, db, _ := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			ConnectionLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog:      1,
					codersdk.FeatureConnectionLog: 1,
				},
			},
		})

		now := dbtime.Now()
		connID := uuid.New()
		ws := createWorkspace(t, db)
		clog := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now.Add(-time.Hour),
			Type:             database.ConnectionTypeWorkspaceApp,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			UserAgent:        sql.NullString{String: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36", Valid: true},
			UserID:           uuid.NullUUID{UUID: ws.OwnerID, Valid: true},
			SlugOrPort:       sql.NullString{String: "code-server", Valid: true},
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.NotNil(t, logs.ConnectionLogs[0].WebInfo)
		require.Equal(t, clog.SlugOrPort.String, logs.ConnectionLogs[0].WebInfo.SlugOrPort)
		require.Equal(t, clog.UserAgent.String, logs.ConnectionLogs[0].WebInfo.UserAgent)
		require.Equal(t, ws.OwnerID, logs.ConnectionLogs[0].WebInfo.User.ID)
	})

	t.Run("SSHInfo", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client, db, _ := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			ConnectionLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog:      1,
					codersdk.FeatureConnectionLog: 1,
				},
			},
		})

		now := dbtime.Now()
		connID := uuid.New()
		ws := createWorkspace(t, db)
		clog := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now.Add(-time.Hour),
			Type:             database.ConnectionTypeSsh,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.NotNil(t, logs.ConnectionLogs[0].SSHInfo)
		require.Empty(t, logs.ConnectionLogs[0].WebInfo)
		require.Empty(t, logs.ConnectionLogs[0].SSHInfo.ExitCode)
		require.Empty(t, logs.ConnectionLogs[0].SSHInfo.DisconnectTime)
		require.Empty(t, logs.ConnectionLogs[0].SSHInfo.DisconnectReason)

		// Mark log as closed
		updatedClog := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now,
			OrganizationID:   clog.OrganizationID,
			Type:             clog.Type,
			WorkspaceID:      clog.WorkspaceID,
			WorkspaceOwnerID: clog.WorkspaceOwnerID,
			WorkspaceName:    clog.WorkspaceName,
			AgentName:        clog.AgentName,
			Code: sql.NullInt32{
				Int32: 0,
				Valid: false,
			},
			Ip: pqtype.Inet{IPNet: net.IPNet{
				IP:   net.ParseIP("192.168.0.1"),
				Mask: net.CIDRMask(8, 32),
			}, Valid: true},

			ConnectionID:     clog.ConnectionID,
			ConnectionStatus: database.ConnectionStatusDisconnected,
			DisconnectReason: sql.NullString{
				String: "example close reason",
				Valid:  true,
			},
		})

		logs, err = client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.NotNil(t, logs.ConnectionLogs[0].SSHInfo)
		require.Nil(t, logs.ConnectionLogs[0].WebInfo)
		require.Equal(t, codersdk.ConnectionTypeSSH, logs.ConnectionLogs[0].Type)
		require.Equal(t, clog.ConnectionID.UUID, logs.ConnectionLogs[0].SSHInfo.ConnectionID)
		require.True(t, logs.ConnectionLogs[0].SSHInfo.DisconnectTime.Equal(now))
		require.Equal(t, updatedClog.DisconnectReason.String, logs.ConnectionLogs[0].SSHInfo.DisconnectReason)
	})
}
