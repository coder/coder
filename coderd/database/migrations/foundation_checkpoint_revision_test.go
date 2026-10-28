package migrations_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

func TestFoundationCheckpointRevisionMigration(t *testing.T) {
	t.Parallel()
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
	type fixture struct {
		foundationID uuid.UUID
		revisionID   uuid.UUID
		status       string
	}
	var fixtures []fixture
	for _, status := range []string{"applied", "unknown"} {
		foundation := dbgen.Foundation(t, db, database.Foundation{OrganizationID: org.ID})
		revisionID := uuid.New()
		_, err := sqlDB.ExecContext(t.Context(), `
			INSERT INTO template_resource_revisions (id, foundation_id, organization_id, file_id, source, created_by, created_at)
			VALUES ($1, $2, $3, $4, '', $5, now())`, revisionID, foundation.ID, org.ID, file.ID, user.ID)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(t.Context(), `
			INSERT INTO template_resource_states (foundation_id, organization_id, applied_revision_id, status, state, outputs, updated_at)
			VALUES ($1, $2, $3, $4, 'checkpoint', '{"retained":true}', now())`, foundation.ID, org.ID, revisionID, status)
		require.NoError(t, err)
		fixtures = append(fixtures, fixture{foundationID: foundation.ID, revisionID: revisionID, status: status})
	}
	// Reapply only this migration to rows written with the preceding schema.
	down, err := os.ReadFile("000638_foundation_checkpoint_revision.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), string(down))
	require.NoError(t, err)
	up, err := os.ReadFile("000638_foundation_checkpoint_revision.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), string(up))
	require.NoError(t, err)
	for _, fixture := range fixtures {
		var checkpointRevision uuid.NullUUID
		var status, outputs string
		var state []byte
		require.NoError(t, sqlDB.QueryRowContext(t.Context(), `
			SELECT checkpoint_revision_id, status, state, outputs::text
			FROM template_resource_states WHERE foundation_id = $1`, fixture.foundationID).
			Scan(&checkpointRevision, &status, &state, &outputs))
		require.Equal(t, fixture.status, status)
		require.Equal(t, []byte("checkpoint"), state)
		require.JSONEq(t, `{"retained":true}`, outputs)
		if fixture.status == "unknown" {
			require.False(t, checkpointRevision.Valid, "migration must not manufacture authority for quarantined state")
		} else {
			require.Equal(t, uuid.NullUUID{UUID: fixture.revisionID, Valid: true}, checkpointRevision)
		}
	}
	_, err = sqlDB.ExecContext(t.Context(), `UPDATE template_resource_states SET checkpoint_revision_id = $1 WHERE foundation_id = $2`, fixtures[0].revisionID, fixtures[1].foundationID)
	require.Error(t, err, "checkpoint provenance must belong to the same Foundation")
}
