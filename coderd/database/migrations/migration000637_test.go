package migrations_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestMigration000637WorkspaceBuildDeadlineReason(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 636)

	ctx := testutil.Context(t, testutil.WaitLong)
	db := database.New(sqlDB)
	org, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID})

	// GIVEN: A build whose deadline was set before the migration. (Avoid
	// dbgen.WorkspaceBuild: its view query expects the new columns.)
	buildID := uuid.New()
	deadline := dbtime.Now().Add(time.Hour)
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO workspace_builds
		(id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, job_id, deadline)
		VALUES ($1, NOW(), NOW(), $2, $3, 1, 'start', $4, $5, $6)`,
		buildID, workspace.ID, version.ID, user.ID, job.ID, deadline)
	require.NoError(t, err)

	migration, err := os.ReadFile("000637_workspace_build_deadline_reason.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	var (
		gotDeadline time.Time
		explained   bool
		sources     string
	)
	err = sqlDB.QueryRowContext(ctx, `SELECT deadline,
		deadline_reason IS NOT NULL OR deadline_changed_at IS NOT NULL OR deadline_changed_by IS NOT NULL,
		deadline_sources::text
		FROM workspace_builds WHERE id = $1`, buildID).Scan(&gotDeadline, &explained, &sources)
	require.NoError(t, err)
	require.True(t, deadline.Equal(gotDeadline), "deadline changed from %s to %s", deadline, gotDeadline)
	require.False(t, explained, "explanation backfilled")
	require.Equal(t, "[]", sources)
}
