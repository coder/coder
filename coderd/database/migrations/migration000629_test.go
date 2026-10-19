package migrations_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

func TestMigration000629DropsOutOfOrgChatProjectGrants(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 628)
	ctx := testutil.Context(t, testutil.WaitLong)

	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	member := dbgen.User(t, db, database.User{})
	departed := dbgen.User(t, db, database.User{})
	for _, user := range []database.User{owner, member} {
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	}
	// Membership elsewhere must not keep a grant here.
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: departed.ID, OrganizationID: otherOrg.ID})
	projectID := uuid.New()
	grant := `{"permissions":["read"]}`
	userACL := `{"` + member.ID.String() + `":` + grant + `,"` + departed.ID.String() + `":` + grant + `}`
	_, err := sqlDB.ExecContext(ctx, `INSERT INTO chat_projects (id, organization_id, owner_id, name, user_acl) VALUES ($1, $2, $3, 'project', $4::jsonb)`,
		projectID, org.ID, owner.ID, userACL)
	require.NoError(t, err)

	migration, err := os.ReadFile("000629_chat_project_acl_org_member_delete.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	var raw []byte
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT user_acl FROM chat_projects WHERE id = $1", projectID).Scan(&raw))
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Len(t, got, 1, "only the in-org grant remains")
	require.Contains(t, got, member.ID.String())
}
