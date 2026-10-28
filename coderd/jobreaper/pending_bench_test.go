package jobreaper_test

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/jobreaper"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
)

func BenchmarkPendingReaperCandidates(b *testing.B) {
	for _, pendingJobs := range []int{1_000, 10_000} {
		b.Run(strconv.Itoa(pendingJobs), func(b *testing.B) {
			db, _, sqlDB := dbtestutil.NewDBWithSQLDB(b)
			ctx := b.Context()
			now := dbtime.Now()
			org := dbgen.Organization(b, db, database.Organization{})
			user := dbgen.User(b, db, database.User{})
			file := dbgen.File(b, db, database.File{})
			for i := range 100 {
				dbgen.ProvisionerDaemon(b, db, database.ProvisionerDaemon{
					OrganizationID: org.ID,
					Tags:           database.StringMap{"pool": strconv.Itoa(i)},
					LastSeenAt:     sql.NullTime{Time: now, Valid: true},
				})
			}
			// Include completed history and a small unschedulable backlog so the
			// query must filter protected jobs before filling its candidate limit.
			_, err := sqlDB.ExecContext(ctx, `
				INSERT INTO provisioner_jobs (
					id, created_at, updated_at, completed_at, organization_id,
					initiator_id, provisioner, storage_method, file_id, type, input, tags
				)
				SELECT gen_random_uuid(), $1, $1,
					CASE WHEN n > $5 THEN $1::timestamptz ELSE NULL END,
					$2, $3, 'echo', 'file', $4, 'template_version_import', '{}',
					jsonb_build_object('pool', CASE WHEN n % 100 = 0 THEN 'missing' ELSE (n % 100)::text END)
				FROM generate_series(1, $5::integer + 100000) AS n`,
				now.Add(-35*time.Minute), org.ID, user.ID, file.ID, pendingJobs)
			require.NoError(b, err)
			// Refresh planner statistics so query plans reflect the seeded data.
			_, err = sqlDB.ExecContext(ctx, "ANALYZE provisioner_jobs, provisioner_daemons")
			require.NoError(b, err)
			params := database.GetProvisionerJobsToBeReapedParams{
				PendingSince:         now.Add(-jobreaper.PendingJobDuration),
				MaxPendingSince:      now.Add(-2 * time.Hour),
				ProvisionerSeenSince: now.Add(-provisionerdserver.StaleInterval),
				HungSince:            now.Add(-jobreaper.HungJobDuration),
				MaxJobs:              jobreaper.MaxJobsPerRun,
			}
			for b.Loop() {
				jobs, err := db.GetProvisionerJobsToBeReaped(ctx, params)
				require.NoError(b, err)
				require.Len(b, jobs, jobreaper.MaxJobsPerRun)
			}
		})
	}
}
