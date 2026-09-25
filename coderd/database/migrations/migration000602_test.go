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
	insertConnectionLog000601 = `INSERT INTO connection_logs (id, connect_time, organization_id,
		workspace_owner_id, workspace_id, workspace_name, agent_name, type, slug_or_port)
		VALUES ($1, now(), $2, $3, $4, $5, 'main', $6, $7)`
	insertConnectionLog000602 = `INSERT INTO connection_logs (id, connect_time, organization_id,
		workspace_owner_id, workspace_id, workspace_name, agent_name, source, app_name_or_port)
		VALUES ($1, now(), $2, $3, $4, $5, 'main', $6, $7)`
	selectConnectionLog000601 = `SELECT type::text, slug_or_port FROM connection_logs WHERE id = $1`
	selectConnectionLog000602 = `SELECT source, app_name_or_port FROM connection_logs WHERE id = $1`
)

// The up migration moves every connection type into a source and app name,
// and the down migration folds app names back into families.
func TestMigration000602ConnectionLogsSource(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 601)
	ctx := testutil.Context(t, testutil.WaitLong)

	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: owner.ID})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: owner.ID, TemplateID: tpl.ID})

	// row is (type, slug_or_port) before 000602 and (source, app_name_or_port)
	// after it.
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

	// Each old type and what the up migration turns it into.
	converted := map[row]row{
		{"ssh", sql.NullString{}}:              {"agent", app("ssh")},
		{"vscode", sql.NullString{}}:           {"agent", app("vscode")},
		{"jetbrains", sql.NullString{}}:        {"agent", app("jetbrains")},
		{"reconnecting_pty", sql.NullString{}}: {"agent", app("reconnecting_pty")},
		{"workspace_app", app("code-server")}:  {"workspace_app", app("code-server")},
		{"port_forwarding", app("8080")}:       {"port_forwarding", app("8080")},
		{"tunnel", sql.NullString{}}:           {"tunnel", sql.NullString{}},
	}
	for old := range converted {
		insert(insertConnectionLog000601, old)
	}
	migrate("000602_connection_logs_source_app_name.up.sql")
	for old, want := range converted {
		require.Equal(t, want, read(selectConnectionLog000602, old), old.kind)
	}

	// App names only newer rows hold, and the family down folds each into.
	folded := map[row]row{
		{"agent", app("cursor")}:    {"vscode", sql.NullString{}},
		{"agent", app("zed")}:       {"ssh", sql.NullString{}},
		{"agent", app("a_new_ide")}: {"ssh", sql.NullString{}},
	}
	for current := range folded {
		insert(insertConnectionLog000602, current)
	}
	migrate("000602_connection_logs_source_app_name.down.sql")
	for old := range converted {
		require.Equal(t, old, read(selectConnectionLog000601, old), old.kind)
	}
	for current, want := range folded {
		require.Equal(t, want, read(selectConnectionLog000601, current), current.appNameOrPort.String)
	}
}
