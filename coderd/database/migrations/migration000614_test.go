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

const (
	insertConnectionLog000613 = `INSERT INTO connection_logs (id, connect_time, organization_id,
		workspace_owner_id, workspace_id, workspace_name, agent_name, type, slug_or_port)
		VALUES ($1, now(), $2, $3, $4, $5, 'main', $6, $7)`
	insertConnectionLog000614 = `INSERT INTO connection_logs (id, connect_time, organization_id,
		workspace_owner_id, workspace_id, workspace_name, agent_name, connection_method, app_name_or_port)
		VALUES ($1, now(), $2, $3, $4, $5, 'main', $6, $7)`
	selectConnectionLog000613 = `SELECT type::text, slug_or_port FROM connection_logs WHERE id = $1`
	selectConnectionLog000614 = `SELECT connection_method::text, app_name_or_port FROM connection_logs WHERE id = $1`
)

// A pending migration batch can add tunnel to the old enum and then replace
// that enum. Comparisons must not materialize its uncommitted new value.
func TestMigration000614ConnectionLogsMethodInSingleTxn(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	applyMigrationsInTxn(ctx, t, sqlDB, 1, 556)
	orgID, ownerID, templateID, workspaceID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO organizations (id, name, description, display_name, created_at, updated_at, default_org_member_roles)
		VALUES ($1, 'migration-org', '', '', now(), now(), '{}');
	`, orgID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO users (id, email, username, hashed_password, created_at, updated_at) VALUES ($1, 'migration@example.com', 'migration-user', '', now(), now());
	`, ownerID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO templates (id, created_at, updated_at, organization_id, name, provisioner, active_version_id, created_by)
		VALUES ($1, now(), now(), $2, 'migration-template', 'echo', $3, $4);
	`, templateID, orgID, uuid.New(), ownerID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, name)
		VALUES ($1, now(), now(), $2, $3, $4, 'migration-workspace');
	`, workspaceID, ownerID, orgID, templateID)
	require.NoError(t, err)
	id := uuid.New()
	_, err = sqlDB.ExecContext(ctx, insertConnectionLog000613, id, orgID, ownerID, workspaceID, "migration-workspace", "vscode", nil)
	require.NoError(t, err)

	applyMigrationsInTxn(ctx, t, sqlDB, 557, 614)
	var method string
	var appName sql.NullString
	require.NoError(t, sqlDB.QueryRowContext(ctx, selectConnectionLog000614, id).Scan(&method, &appName))
	require.Equal(t, "ssh", method)
	require.Equal(t, sql.NullString{String: "vscode", Valid: true}, appName)
}

// The up migration splits each connection type into a method and a client
// identity, and the down migration folds identities back into types.
func TestMigration000614ConnectionLogsMethod(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 613)
	ctx := testutil.Context(t, testutil.WaitLong)

	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: owner.ID})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: owner.ID, TemplateID: tpl.ID})

	// row is (type, slug_or_port) before 000614 and
	// (connection_method, app_name_or_port) after it.
	type row struct {
		kind          string
		appNameOrPort sql.NullString
	}
	app := func(name string) sql.NullString { return sql.NullString{String: name, Valid: true} }
	ids := map[row]uuid.UUID{}
	insert := func(query string, r row) {
		ids[r] = uuid.New()
		_, err := sqlDB.ExecContext(ctx, query, ids[r], org.ID, owner.ID, ws.ID, ws.Name, r.kind, r.appNameOrPort)
		require.NoError(t, err)
	}
	read := func(query string, r row) (got row) {
		require.NoError(t, sqlDB.QueryRowContext(ctx, query, ids[r]).Scan(&got.kind, &got.appNameOrPort))
		return got
	}
	migrate := func(file string) {
		migration, err := os.ReadFile(file)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}

	// Each released type and what the up migration turns it into.
	converted := map[row]row{
		{"ssh", sql.NullString{}}:              {"ssh", sql.NullString{}},
		{"vscode", sql.NullString{}}:           {"ssh", app("vscode")},
		{"jetbrains", sql.NullString{}}:        {"ssh", app("jetbrains")},
		{"reconnecting_pty", sql.NullString{}}: {"reconnecting_pty", sql.NullString{}},
		{"workspace_app", app("code-server")}:  {"workspace_app", app("code-server")},
		{"port_forwarding", app("8080")}:       {"port_forwarding", app("8080")},
		{"tunnel", sql.NullString{}}:           {"tunnel", sql.NullString{}},
	}
	for old := range converted {
		insert(insertConnectionLog000613, old)
	}
	migrate("000614_connection_logs_method_app_name.up.sql")
	for old, want := range converted {
		require.Equal(t, want, read(selectConnectionLog000614, old), old.kind)
	}

	// Identities only newer rows hold, and the type down folds each into
	// using its snapshot of the app registry.
	folded := map[row]row{
		{"ssh", app("cursor")}:              {"vscode", sql.NullString{}},
		{"ssh", app("trae_cn")}:             {"vscode", sql.NullString{}},
		{"ssh", app("goland")}:              {"jetbrains", sql.NullString{}},
		{"ssh", app("zed")}:                 {"ssh", sql.NullString{}},
		{"ssh", app("a_new_ide")}:           {"ssh", sql.NullString{}},
		{"reconnecting_pty", app("cursor")}: {"reconnecting_pty", sql.NullString{}},
	}
	for current := range folded {
		insert(insertConnectionLog000614, current)
	}
	migrate("000614_connection_logs_method_app_name.down.sql")
	for old := range converted {
		require.Equal(t, old, read(selectConnectionLog000613, old), old.kind)
	}
	for current, want := range folded {
		require.Equal(t, want, read(selectConnectionLog000613, current), current.kind+" "+current.appNameOrPort.String)
	}
}
