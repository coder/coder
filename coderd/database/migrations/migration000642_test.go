package migrations_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

// TestMigration000642PinsOwnedProjects checks that the upgrade pins each
// owner's projects in sidebar order and leaves shared projects unpinned.
func TestMigration000642PinsOwnedProjects(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 641)
	ctx := testutil.Context(t, testutil.WaitLong)

	db := database.New(sqlDB)
	orgID := insertMigrationTestOrganization(ctx, t, sqlDB, []string{})
	first := dbgen.User(t, db, database.User{})
	second := dbgen.User(t, db, database.User{})
	insertProject := func(ownerID, id uuid.UUID, name, userACL string) {
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO chat_projects (id, organization_id, owner_id, name, user_acl) VALUES ($1, $2, $3, $4, $5::jsonb)`,
			id, orgID, ownerID, name, userACL)
		require.NoError(t, err)
	}
	// The sidebar sorts by lower(name), then ID. Neither byte order
	// ("Banana" < "apple") nor a collation that puts lowercase first
	// ("banana" < "Banana") gives that order.
	insertProject(first.ID, uuid.New(), "cherry", "{}")
	insertProject(first.ID, uuid.New(), "apple", "{}")
	insertProject(first.ID, uuid.MustParse("00000000-0000-4000-8000-000000000002"), "banana", "{}")
	insertProject(first.ID, uuid.MustParse("00000000-0000-4000-8000-000000000001"), "Banana", "{}")
	insertProject(second.ID, uuid.New(), "Shared", `{"`+first.ID.String()+`":{"permissions":["read"]}}`)

	up, err := os.ReadFile("000642_chat_project_pins.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(up))
	require.NoError(t, err)

	pins := func(userID uuid.UUID) []string {
		var got pq.StringArray
		require.NoError(t, sqlDB.QueryRowContext(ctx, `
			SELECT array_agg(chat_projects.name || ':' || pin_order || ':' || source ORDER BY pin_order)
			FROM chat_project_pins JOIN chat_projects ON chat_projects.id = chat_project_pins.project_id
			WHERE user_id = $1 AND unpinned_at IS NULL`, userID).Scan(&got))
		return got
	}
	require.Equal(t, []string{"apple:1:migration", "Banana:2:migration", "banana:3:migration", "cherry:4:migration"}, pins(first.ID))
	require.Equal(t, []string{"Shared:1:migration"}, pins(second.ID))
}
