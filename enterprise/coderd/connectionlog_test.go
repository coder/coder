package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"slices"
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
	"github.com/coder/coder/v2/testutil"
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
			ConnectionMethod: database.ConnectionLogMethodSSH,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.Equal(t, string(codersdk.ConnectionTypeSSH), logs.ConnectionLogs[0].Type)
		require.Equal(t, codersdk.ConnectionLogMethodSSH, logs.ConnectionLogs[0].ConnectionMethod)
		require.Empty(t, logs.ConnectionLogs[0].AppName)
		require.Empty(t, logs.ConnectionLogs[0].AppDisplayName, "no display name is made up for a missing app")
	})

	// Method and app identity are separate filters that intersect with each
	// other and with the deprecated type filter.
	t.Run("AppName", func(t *testing.T) {
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
		insert := func(method database.ConnectionLogMethod, appNameOrPort string) {
			agent := method == database.ConnectionLogMethodSSH || method == database.ConnectionLogMethodReconnectingPTY
			_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
				ConnectionMethod: method,
				AppNameOrPort:    sql.NullString{String: appNameOrPort, Valid: appNameOrPort != ""},
				WorkspaceID:      ws.ID,
				OrganizationID:   ws.OrganizationID,
				WorkspaceOwnerID: ws.OwnerID,
				ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: agent},
			})
		}
		insert(database.ConnectionLogMethodSSH, "")
		insert(database.ConnectionLogMethodSSH, "cursor")
		insert(database.ConnectionLogMethodSSH, "vscode")
		insert(database.ConnectionLogMethodSSH, "goland")
		insert(database.ConnectionLogMethodSSH, "an_unregistered_ide")
		insert(database.ConnectionLogMethodReconnectingPTY, "")
		insert(database.ConnectionLogMethodReconnectingPTY, "cursor")
		// A workspace app slugged like an IDE is still a workspace app.
		insert(database.ConnectionLogMethodWorkspaceApp, "cursor")
		insert(database.ConnectionLogMethodPortForwarding, "8080")
		insert(database.ConnectionLogMethodTunnel, "")

		type got struct {
			Type       string
			Method     codersdk.ConnectionLogMethod
			AppName    string
			SlugOrPort string
		}
		query := func(q string) []got {
			logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{SearchQuery: q})
			require.NoError(t, err, q)
			require.EqualValues(t, len(logs.ConnectionLogs), logs.Count, "count must match the listed logs for %q", q)
			var out []got
			for _, l := range logs.ConnectionLogs {
				g := got{Type: l.Type, Method: l.ConnectionMethod, AppName: l.AppName}
				if l.WebInfo != nil {
					g.SlugOrPort = l.WebInfo.SlugOrPort
				}
				out = append(out, g)
			}
			return out
		}

		sshPlain := got{"ssh", codersdk.ConnectionLogMethodSSH, "", ""}
		sshCursor := got{"vscode", codersdk.ConnectionLogMethodSSH, "cursor", ""}
		sshVSCode := got{"vscode", codersdk.ConnectionLogMethodSSH, "vscode", ""}
		sshGoLand := got{"jetbrains", codersdk.ConnectionLogMethodSSH, "goland", ""}
		sshUnknown := got{"ssh", codersdk.ConnectionLogMethodSSH, "an_unregistered_ide", ""}
		pty := got{"reconnecting_pty", codersdk.ConnectionLogMethodReconnectingPTY, "", ""}
		ptyCursor := got{"reconnecting_pty", codersdk.ConnectionLogMethodReconnectingPTY, "cursor", ""}
		// Destinations are reported in WebInfo only.
		webApp := got{"workspace_app", codersdk.ConnectionLogMethodWorkspaceApp, "", "cursor"}
		port := got{"port_forwarding", codersdk.ConnectionLogMethodPortForwarding, "", "8080"}
		tunnel := got{"tunnel", codersdk.ConnectionLogMethodTunnel, "", ""}
		agentRows := []got{sshPlain, sshCursor, sshVSCode, sshGoLand, sshUnknown, pty, ptyCursor}

		require.ElementsMatch(t, append(slices.Clone(agentRows), webApp, port, tunnel), query(""))

		// Deprecated type filter.
		require.ElementsMatch(t, []got{sshCursor, sshVSCode}, query("type:vscode"))
		require.ElementsMatch(t, []got{sshGoLand}, query("type:jetbrains"))
		require.ElementsMatch(t, []got{sshPlain, sshUnknown}, query("type:ssh"))
		require.ElementsMatch(t, []got{pty, ptyCursor}, query("type:reconnecting_pty"))
		require.ElementsMatch(t, []got{webApp}, query("type:workspace_app"))
		require.ElementsMatch(t, []got{port}, query("type:port_forwarding"))
		require.ElementsMatch(t, []got{tunnel}, query("type:tunnel"))

		// Method filter.
		require.ElementsMatch(t, []got{sshPlain, sshCursor, sshVSCode, sshGoLand, sshUnknown}, query("method:ssh"))
		require.ElementsMatch(t, []got{pty, ptyCursor}, query("method:reconnecting_pty"))
		require.ElementsMatch(t, []got{webApp}, query("method:workspace_app"))

		// The app filter matches client identity, never a destination.
		require.ElementsMatch(t, []got{sshCursor, ptyCursor}, query("app:cursor"))
		require.ElementsMatch(t, []got{sshVSCode}, query("app:vscode"))
		require.Empty(t, query("app:8080"))
		require.Empty(t, query("method:workspace_app app:cursor"))

		// Combined constraints intersect.
		require.ElementsMatch(t, []got{sshCursor}, query("method:ssh app:cursor"))
		require.ElementsMatch(t, []got{ptyCursor}, query("method:reconnecting_pty app:cursor"))
		require.ElementsMatch(t, []got{sshCursor}, query("type:vscode app:cursor"))
		require.Empty(t, query("method:reconnecting_pty type:vscode"))
		require.Empty(t, query("method:tunnel type:ssh"))

		// Status only applies to agent connections, which have a lifecycle.
		require.ElementsMatch(t, agentRows, query("status:ongoing"))
		require.Empty(t, query("status:completed"))

		// Removed and unknown values are rejected.
		for _, q := range []string{"type:unknown", "type:cursor", "method:vscode"} {
			_, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{SearchQuery: q})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, q)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), q)
		}
	})

	// An absent app is omitted from the JSON, and the deprecated type is a
	// plain string.
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
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
		ssh := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			ConnectionMethod: database.ConnectionLogMethodSSH,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			ConnectionID:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
		webApp := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			ConnectionMethod: database.ConnectionLogMethodWorkspaceApp,
			AppNameOrPort:    sql.NullString{String: "code-server", Valid: true},
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
		})

		res, err := client.Request(ctx, http.MethodGet, "/api/v2/connectionlog", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var body struct {
			ConnectionLogs []map[string]any `json:"connection_logs"`
		}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		require.Len(t, body.ConnectionLogs, 2)

		byID := map[string]map[string]any{}
		for _, l := range body.ConnectionLogs {
			byID[l["id"].(string)] = l
		}
		sshJSON := byID[ssh.ID.String()]
		require.NotNil(t, sshJSON)
		require.Equal(t, "ssh", sshJSON["type"])
		require.Equal(t, "ssh", sshJSON["connection_method"])
		require.NotContains(t, sshJSON, "app_name")
		require.NotContains(t, sshJSON, "app_display_name")
		require.NotContains(t, sshJSON, "web_info")

		webAppJSON := byID[webApp.ID.String()]
		require.NotNil(t, webAppJSON)
		require.Equal(t, "workspace_app", webAppJSON["type"])
		require.Equal(t, "workspace_app", webAppJSON["connection_method"])
		require.NotContains(t, webAppJSON, "app_name")
		require.NotContains(t, webAppJSON, "app_display_name")
		require.NotContains(t, webAppJSON, "ssh_info")
		webInfo, ok := webAppJSON["web_info"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "code-server", webInfo["slug_or_port"])
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
			ConnectionMethod: database.ConnectionLogMethodSSH,
			WorkspaceID:      ws.ID,
			OrganizationID:   org.ID,
			WorkspaceOwnerID: ws.OwnerID,
		})
		_ = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			ConnectionMethod: database.ConnectionLogMethodSSH,
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
			ConnectionMethod: database.ConnectionLogMethodWorkspaceApp,
			WorkspaceID:      ws.ID,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			ConnectionID:     uuid.NullUUID{UUID: connID, Valid: true},
			UserAgent:        sql.NullString{String: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36", Valid: true},
			UserID:           uuid.NullUUID{UUID: ws.OwnerID, Valid: true},
			AppNameOrPort:    sql.NullString{String: "code-server", Valid: true},
		})

		logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{})
		require.NoError(t, err)

		require.Len(t, logs.ConnectionLogs, 1)
		require.EqualValues(t, 1, logs.Count)
		require.NotNil(t, logs.ConnectionLogs[0].WebInfo)
		require.Equal(t, clog.AppNameOrPort.String, logs.ConnectionLogs[0].WebInfo.SlugOrPort)
		require.Empty(t, logs.ConnectionLogs[0].AppName, "a destination is not a client app")
		require.Empty(t, logs.ConnectionLogs[0].AppDisplayName)
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
			ConnectionMethod: database.ConnectionLogMethodTunnel,
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
			ConnectionMethod: database.ConnectionLogMethodSSH,
			AppNameOrPort:    sql.NullString{String: "cursor", Valid: true},
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
			ConnectionMethod: clog.ConnectionMethod,
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
		require.Equal(t, string(codersdk.ConnectionTypeVSCode), logs.ConnectionLogs[0].Type)
		require.Equal(t, codersdk.ConnectionLogMethodSSH, logs.ConnectionLogs[0].ConnectionMethod)
		require.Equal(t, "cursor", logs.ConnectionLogs[0].AppName)
		require.Equal(t, "Cursor", logs.ConnectionLogs[0].AppDisplayName)
		require.Equal(t, clog.ConnectionID.UUID, logs.ConnectionLogs[0].SSHInfo.ConnectionID)
		require.True(t, logs.ConnectionLogs[0].SSHInfo.DisconnectTime.Equal(now))
		require.Equal(t, updatedClog.DisconnectReason.String, logs.ConnectionLogs[0].SSHInfo.DisconnectReason)
	})
}
