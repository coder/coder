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

	tests := []struct {
		name     string
		fromTTL  time.Duration
		toTTL    time.Duration
		expected sql.NullInt64
	}{
		{
			name:     "ModifyTTLDurationDown",
			fromTTL:  24 * time.Hour,
			toTTL:    1 * time.Hour,
			expected: sql.NullInt64{Valid: true, Int64: int64(1 * time.Hour)},
		},
		{
			name:     "ModifyTTLDurationUp",
			fromTTL:  24 * time.Hour,
			toTTL:    36 * time.Hour,
			expected: sql.NullInt64{Valid: true, Int64: int64(36 * time.Hour)},
		},
		{
			name:     "ModifyTTLDurationSame",
			fromTTL:  24 * time.Hour,
			toTTL:    24 * time.Hour,
			expected: sql.NullInt64{Valid: true, Int64: int64(24 * time.Hour)},
		},
		{
			name:     "DisableTTL",
			fromTTL:  24 * time.Hour,
			toTTL:    0,
			expected: sql.NullInt64{},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				db, _ = dbtestutil.NewDB(t)
				ctx   = testutil.Context(t, testutil.WaitLong)
				user  = dbgen.User(t, db, database.User{})
				file  = dbgen.File(t, db, database.File{CreatedBy: user.ID})
				// Create first template
				templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					FileID:      file.ID,
					InitiatorID: user.ID,
					Tags:        database.StringMap{"foo": "bar"},
				})
				templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					JobID:          templateJob.ID,
					OrganizationID: templateJob.OrganizationID,
				})
				template = dbgen.Template(t, db, database.Template{
					ActiveVersionID: templateVersion.ID,
					CreatedBy:       user.ID,
					OrganizationID:  templateJob.OrganizationID,
				})
				// Create second template
				otherTTL         = tt.fromTTL + 6*time.Hour
				otherTemplateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					FileID:      file.ID,
					InitiatorID: user.ID,
					Tags:        database.StringMap{"foo": "bar"},
				})
				otherTemplateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					JobID:          otherTemplateJob.ID,
					OrganizationID: otherTemplateJob.OrganizationID,
				})
				otherTemplate = dbgen.Template(t, db, database.Template{
					ActiveVersionID: otherTemplateVersion.ID,
					CreatedBy:       user.ID,
					OrganizationID:  otherTemplateJob.OrganizationID,
				})
			)

			templateScheduleStore := schedule.NewAGPLTemplateScheduleStore()

			// Set both template's default TTL
			template, err := templateScheduleStore.Set(ctx, db, template, schedule.TemplateScheduleOptions{
				DefaultTTL: tt.fromTTL,
			})
			require.NoError(t, err)
			otherTemplate, err = templateScheduleStore.Set(ctx, db, otherTemplate, schedule.TemplateScheduleOptions{
				DefaultTTL: otherTTL,
			})
			require.NoError(t, err)

			// We create two workspaces here, one with the template we're modifying, the
			// other with a different template. We want to ensure we only modify one
			// of the workspaces.
			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     template.ID,
				OrganizationID: templateJob.OrganizationID,
				LastUsedAt:     dbtime.Now(),
				Ttl:            sql.NullInt64{Valid: true, Int64: int64(tt.fromTTL)},
			})
			otherWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     otherTemplate.ID,
				OrganizationID: otherTemplateJob.OrganizationID,
				LastUsedAt:     dbtime.Now(),
				Ttl:            sql.NullInt64{Valid: true, Int64: int64(otherTTL)},
			})

			// Ensure the workspace's start with the correct TTLs
			require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(tt.fromTTL)}, workspace.Ttl)
			require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(otherTTL)}, otherWorkspace.Ttl)

			// Update _only_ the primary template's TTL
			_, err = templateScheduleStore.Set(ctx, db, template, schedule.TemplateScheduleOptions{
				DefaultTTL: tt.toTTL,
			})
			require.NoError(t, err)

			// Verify the primary workspace's TTL has been updated.
			ws, err := db.GetWorkspaceByID(ctx, workspace.ID)
			require.NoError(t, err)
			require.Equal(t, tt.expected, ws.Ttl)

			// Verify that the other workspace's TTL has not been touched.
			ws, err = db.GetWorkspaceByID(ctx, otherWorkspace.ID)
			require.NoError(t, err)
			require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(otherTTL)}, ws.Ttl)
		})
	}
}
