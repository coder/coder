package migrations_test

import (
	"database/sql"
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

func TestMigration000647WorkspaceBuildDeadlineReason(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 646)

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

	migration, err := os.ReadFile("000647_workspace_build_deadline_reason.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	type snapshot struct {
		Deadline  time.Time
		Reason    string
		ChangedAt sql.NullTime
		ChangedBy uuid.NullUUID
		Sources   string
	}
	get := func() snapshot {
		var got snapshot
		err := sqlDB.QueryRowContext(ctx, `SELECT deadline, deadline_reason, deadline_changed_at,
			deadline_changed_by, deadline_sources::text
			FROM workspace_builds WHERE id = $1`, buildID).Scan(&got.Deadline, &got.Reason, &got.ChangedAt, &got.ChangedBy, &got.Sources)
		require.NoError(t, err)
		return got
	}

	// THEN: The build has an unknown snapshot without a time, and its
	// deadline is unchanged.
	got := get()
	require.True(t, deadline.Equal(got.Deadline), "deadline changed from %s to %s", deadline, got.Deadline)
	require.Equal(t, snapshot{Deadline: got.Deadline, Reason: "unknown", Sources: "[]"}, got)

	// WHEN: A previous server changes the deadline without a snapshot.
	_, err = sqlDB.ExecContext(ctx, `UPDATE workspace_builds SET deadline = deadline + interval '1 hour', updated_at = NOW() WHERE id = $1`, buildID)
	require.NoError(t, err)

	// THEN: The change is unknown, with the time of the change.
	got = get()
	require.True(t, got.ChangedAt.Valid, "unknown change must record its time")
	require.WithinDuration(t, time.Now(), got.ChangedAt.Time, testutil.WaitLong)
	require.Equal(t, snapshot{Deadline: got.Deadline, Reason: "unknown", ChangedAt: got.ChangedAt, Sources: "[]"}, got)
}
