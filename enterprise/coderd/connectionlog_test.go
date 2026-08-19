package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
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

	t.Run("WebInfoTunnel", func(t *testing.T) {
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
		ws := createWorkspace(t, db)
		// Tunnel events are written by coderd with the connecting
		// user's identity; they must surface it via WebInfo.
		clog := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now.Add(-time.Hour),
			Type:             database.ConnectionTypeTunnel,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			UserAgent:        sql.NullString{String: "coder-cli/2.0.0", Valid: true},
			Code:             sql.NullInt32{Int32: http.StatusSwitchingProtocols, Valid: true},
			UserID:           uuid.NullUUID{UUID: ws.OwnerID, Valid: true},
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.Nil(t, logs.ConnectionLogs[0].SSHInfo)
		require.NotNil(t, logs.ConnectionLogs[0].WebInfo)
		require.Equal(t, clog.UserAgent.String, logs.ConnectionLogs[0].WebInfo.UserAgent)
		require.NotNil(t, logs.ConnectionLogs[0].WebInfo.User)
		require.Equal(t, ws.OwnerID, logs.ConnectionLogs[0].WebInfo.User.ID)
		require.EqualValues(t, http.StatusSwitchingProtocols, logs.ConnectionLogs[0].WebInfo.StatusCode)
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
			IP: pqtype.Inet{IPNet: net.IPNet{
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

	t.Run("FileTransferInfo", func(t *testing.T) {
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
		// A file-transfer session row and two file operation rows
		// sharing its connection ID. The operations must insert as
		// separate rows despite the shared connection ID.
		session := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now.Add(-time.Hour),
			Type:             database.ConnectionTypeFileTransfer,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
		})
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now.Add(-time.Minute),
			Type:             database.ConnectionTypeFileOperation,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceName:    session.WorkspaceName,
			AgentName:        session.AgentName,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			FileProtocol: database.NullConnectionLogFileProtocol{
				ConnectionLogFileProtocol: database.ConnectionLogFileProtocolSftp,
				Valid:                     true,
			},
			FileAction: database.NullConnectionLogFileAction{
				ConnectionLogFileAction: database.ConnectionLogFileActionDownload,
				Valid:                   true,
			},
			FilePath: sql.NullString{String: "/home/coder/secret.txt", Valid: true},
		})
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			Time:             now,
			Type:             database.ConnectionTypeFileOperation,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceName:    session.WorkspaceName,
			AgentName:        session.AgentName,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			FileProtocol: database.NullConnectionLogFileProtocol{
				ConnectionLogFileProtocol: database.ConnectionLogFileProtocolSftp,
				Valid:                     true,
			},
			FileAction: database.NullConnectionLogFileAction{
				ConnectionLogFileAction: database.ConnectionLogFileActionRename,
				Valid:                   true,
			},
			FilePath:   sql.NullString{String: "/home/coder/a.txt", Valid: true},
			FileTarget: sql.NullString{String: "/home/coder/b.txt", Valid: true},
		})

		// The session and its operations are grouped by connection_id.
		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{
			SearchQuery: "connection_id:" + connID.String(),
		})
		require.NoError(t, err)
		require.Len(t, logs.ConnectionLogs, 3)

		// Results are ordered by connect_time descending.
		renameLog := logs.ConnectionLogs[0]
		require.Equal(t, codersdk.ConnectionTypeFileOperation, renameLog.Type)
		require.Nil(t, renameLog.SSHInfo)
		require.Nil(t, renameLog.WebInfo)
		require.NotNil(t, renameLog.FileTransferInfo)
		require.Equal(t, connID, renameLog.FileTransferInfo.ConnectionID)
		require.Equal(t, codersdk.ConnectionLogFileProtocolSFTP, renameLog.FileTransferInfo.Protocol)
		require.Equal(t, codersdk.ConnectionLogFileActionRename, renameLog.FileTransferInfo.Action)
		require.Equal(t, "/home/coder/a.txt", renameLog.FileTransferInfo.Path)
		require.Equal(t, "/home/coder/b.txt", renameLog.FileTransferInfo.Target)

		readLog := logs.ConnectionLogs[1]
		require.NotNil(t, readLog.FileTransferInfo)
		require.Equal(t, codersdk.ConnectionLogFileActionDownload, readLog.FileTransferInfo.Action)
		require.Equal(t, "/home/coder/secret.txt", readLog.FileTransferInfo.Path)
		require.Empty(t, readLog.FileTransferInfo.Target)

		sessionLog := logs.ConnectionLogs[2]
		require.Equal(t, codersdk.ConnectionTypeFileTransfer, sessionLog.Type)
		require.NotNil(t, sessionLog.SSHInfo)
		require.Nil(t, sessionLog.FileTransferInfo)

		// File operations are point-in-time events and excluded from
		// status filter results.
		logs, err = client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{
			SearchQuery: "status:ongoing connection_id:" + connID.String(),
		})
		require.NoError(t, err)
		require.Len(t, logs.ConnectionLogs, 1)
		require.Equal(t, codersdk.ConnectionTypeFileTransfer, logs.ConnectionLogs[0].Type)
	})
}
