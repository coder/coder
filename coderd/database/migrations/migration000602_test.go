package migrations_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

// TestMigration000602ConnectionLogsSource converts every connection type into
// a source and app name, then folds app names back into families.
func TestMigration000602ConnectionLogsSource(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 601)

	ctx := testutil.Context(t, testutil.WaitLong)
	upSQL, err := os.ReadFile("000602_connection_logs_source_app_name.up.sql")
	require.NoError(t, err)
	downSQL, err := os.ReadFile("000602_connection_logs_source_app_name.down.sql")
	require.NoError(t, err)

	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      owner.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		TemplateID:     tpl.ID,
	})

	// row is a connection log as (type, slug_or_port) before the migration and
	// (source, app_name_or_port) after it.
	type row struct {
		kind          string
		appNameOrPort sql.NullString
	}
	name := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	insert := func(column string, r row) uuid.UUID {
		id := uuid.New()
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO connection_logs (
				id, connect_time, organization_id, workspace_owner_id, workspace_id,
				workspace_name, agent_name, `+column+`
			) VALUES ($1, now(), $2, $3, $4, $5, 'main', $6, $7)
		`, id, org.ID, owner.ID, ws.ID, ws.Name, r.kind, r.appNameOrPort)
		require.NoError(t, err)
		return id
	}
	read := func(query string, id uuid.UUID) row {
		var got row
		require.NoError(t, sqlDB.QueryRowContext(ctx, query, id).Scan(&got.kind, &got.appNameOrPort))
		return got
	}

	up := []struct {
		name string
		old  row
		want row
	}{
		{"SSH", row{"ssh", sql.NullString{}}, row{"agent", name("ssh")}},
		{"VSCode", row{"vscode", sql.NullString{}}, row{"agent", name("vscode")}},
		{"JetBrains", row{"jetbrains", sql.NullString{}}, row{"agent", name("jetbrains")}},
		{"WebTerminal", row{"reconnecting_pty", sql.NullString{}}, row{"agent", name("reconnecting_pty")}},
		{"WorkspaceApp", row{"workspace_app", name("code-server")}, row{"workspace_app", name("code-server")}},
		{"PortForwarding", row{"port_forwarding", name("8080")}, row{"port_forwarding", name("8080")}},
		{"Tunnel", row{"tunnel", sql.NullString{}}, row{"tunnel", sql.NullString{}}},
	}
	upIDs := make([]uuid.UUID, len(up))
	for i, tc := range up {
		upIDs[i] = insert("type, slug_or_port", tc.old)
	}

	_, err = sqlDB.ExecContext(ctx, string(upSQL))
	require.NoError(t, err)

	for i, tc := range up {
		require.Equal(t, tc.want, read(`SELECT source, app_name_or_port FROM connection_logs WHERE id = $1`, upIDs[i]), tc.name)
	}

	down := []struct {
		name string
		new  row
		want row
	}{
		{"RegisteredApp", row{"agent", name("cursor")}, row{"vscode", sql.NullString{}}},
		{"SSHFamilyApp", row{"agent", name("zed")}, row{"ssh", sql.NullString{}}},
		{"UnregisteredApp", row{"agent", name("a_new_ide")}, row{"ssh", sql.NullString{}}},
	}
	downIDs := make([]uuid.UUID, len(down))
	for i, tc := range down {
		downIDs[i] = insert("source, app_name_or_port", tc.new)
	}

	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)

	const readOld = `SELECT type::text, slug_or_port FROM connection_logs WHERE id = $1`
	// The down migration restores every original row.
	for i, tc := range up {
		require.Equal(t, tc.old, read(readOld, upIDs[i]), tc.name)
	}
	for i, tc := range down {
		require.Equal(t, tc.want, read(readOld, downIDs[i]), tc.name)
	}
}
