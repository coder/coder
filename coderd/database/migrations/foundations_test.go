package migrations_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testMigration000637Foundations(t *testing.T, sqlDB *sql.DB, next migrationStepper) {
	// The fixture is deliberately written against the template-owned schema.
	fixture, err := os.ReadFile("testdata/fixtures/000636_template_resources.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), string(fixture))
	require.NoError(t, err)
	var templateID, orgID uuid.UUID
	var checkpoint []byte
	var outputs string
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT template_id, organization_id, state, outputs::text FROM template_resource_states WHERE applied_revision_id = '06360000-0000-0000-0000-000000000006'`).Scan(&templateID, &orgID, &checkpoint, &outputs))
	deletedTemplateID := uuid.New()
	_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO templates SELECT (jsonb_populate_record(NULL::templates, to_jsonb(t) || jsonb_build_object('id', $2::uuid, 'deleted', true))).* FROM templates t WHERE id = $1`, templateID, deletedTemplateID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO template_resource_states (template_id, organization_id, updated_at) VALUES ($1, $2, now())`, deletedTemplateID, orgID)
	require.NoError(t, err)
	var snapshotCount int
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM workspace_builds WHERE template_resource_revision_id = '06360000-0000-0000-0000-000000000006'`).Scan(&snapshotCount))
	require.Positive(t, snapshotCount, "fixture must exercise pinned workspace snapshots")
	version, _, err := next()
	require.NoError(t, err)
	require.EqualValues(t, 637, version)
	var foundationID, boundTemplateID, foundationOrgID, revisionID, runRevisionID uuid.UUID
	var retainedState []byte
	var retainedOutputs string
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `
 SELECT f.id, f.template_id, f.organization_id, s.state, s.outputs::text, r.id, run.revision_id
 FROM foundations f
 JOIN template_resource_states s ON s.foundation_id = f.id
 JOIN template_resource_revisions r ON r.id = s.applied_revision_id
 JOIN template_resource_runs run ON run.foundation_id = f.id
 WHERE r.id = '06360000-0000-0000-0000-000000000006'
 `).Scan(&foundationID, &boundTemplateID, &foundationOrgID, &retainedState, &retainedOutputs, &revisionID, &runRevisionID))
	require.NotEqual(t, templateID, foundationID)
	require.Equal(t, templateID, boundTemplateID)
	require.Equal(t, orgID, foundationOrgID)
	require.Equal(t, checkpoint, retainedState)
	require.JSONEq(t, outputs, retainedOutputs)
	require.Equal(t, uuid.MustParse("06360000-0000-0000-0000-000000000006"), revisionID)
	require.Equal(t, revisionID, runRevisionID)
	var distinctNames int
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT count(DISTINCT lower(name)) FROM foundations WHERE template_id IN ($1, $2)`, templateID, deletedTemplateID).Scan(&distinctNames))
	require.Equal(t, 2, distinctNames, "deleted template names must not prevent migration")
	var retainedSnapshotCount int
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM workspace_builds WHERE template_resource_revision_id = $1`, revisionID).Scan(&retainedSnapshotCount))
	require.Equal(t, snapshotCount, retainedSnapshotCount)
	var invalidSnapshots int
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM workspace_builds WHERE template_resource_revision_id = $1 AND template_resource_outputs != '{"value":"retained"}'::jsonb`, revisionID).Scan(&invalidSnapshots))
	require.Zero(t, invalidSnapshots)
}
