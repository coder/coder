package schedule_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateTTL(t *testing.T) {
	t.Parallel()

	t.Run("ModifiesWorkspaceTTL", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			ctx         = testutil.Context(t, testutil.WaitLong)
			user        = dbgen.User(t, db, database.User{})
			file        = dbgen.File(t, db, database.File{CreatedBy: user.ID})
			templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				FileID:      file.ID,
				InitiatorID: user.ID,
				Tags:        database.StringMap{"foo": "bar"},
			})
			defaultTTL      = 24 * time.Hour
			templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				JobID:          templateJob.ID,
				OrganizationID: templateJob.OrganizationID,
			})
			template = dbgen.Template(t, db, database.Template{
				ActiveVersionID: templateVersion.ID,
				CreatedBy:       user.ID,
				OrganizationID:  templateJob.OrganizationID,
				DefaultTTL:      int64(defaultTTL),
			})
			workspace = dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     template.ID,
				OrganizationID: templateJob.OrganizationID,
				LastUsedAt:     dbtime.Now(),
				Ttl:            sql.NullInt64{Valid: true, Int64: int64(defaultTTL)},
			})
		)

		templateScheduleStore := schedule.NewAGPLTemplateScheduleStore()

		// We've created a template with a TTL of 24 hours, so we expect our
		// workspace to have a TTL of 24 hours.
		require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(defaultTTL)}, workspace.Ttl)

		// We expect an AGPL template schedule store to always update
		// the TTL of existing workspaces.
		_, err := templateScheduleStore.Set(ctx, db, template, schedule.TemplateScheduleOptions{
			DefaultTTL: 1 * time.Hour,
		})
		require.NoError(t, err)

		// Verify that the workspace's TTL has been updated.
		ws, err := db.GetWorkspaceByID(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(1 * time.Hour)}, ws.Ttl)
	})
}
