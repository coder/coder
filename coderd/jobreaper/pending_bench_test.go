package jobreaper_test

import (
	"database/sql"
	"fmt"
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
	for _, size := range []struct {
		historyJobs int
		pendingJobs int
	}{
		{historyJobs: 100_000, pendingJobs: 1_000},
		{historyJobs: 100_000, pendingJobs: 10_000},
		{historyJobs: 2_000_000, pendingJobs: 10_000},
	} {
		b.Run(fmt.Sprintf("History%d/Pending%d", size.historyJobs, size.pendingJobs), func(b *testing.B) {
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
			// Reuse the job history across availability cases. Only daemon
			// heartbeats change between cases, outside the timed query loop.
			_, err := sqlDB.ExecContext(ctx, `
				INSERT INTO provisioner_jobs (
					id, created_at, updated_at, started_at, completed_at, organization_id,
					initiator_id, provisioner, storage_method, file_id, type, input, tags
				)
				SELECT gen_random_uuid(), $1, $1,
					CASE WHEN n > $5 THEN $1::timestamptz ELSE NULL END,
					CASE WHEN n > $5 THEN $1::timestamptz ELSE NULL END,
					$2, $3, 'echo', 'file', $4, 'template_version_import', '{}',
					jsonb_build_object('pool', (n % 100)::text)
				FROM generate_series(1, $5::integer + $6::integer) AS n`,
				now.Add(-35*time.Minute), org.ID, user.ID, file.ID, size.pendingJobs, size.historyJobs)
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
			for _, availability := range []struct {
				name        string
				onlinePools int
			}{
				{name: "AllOnline", onlinePools: 100},
				{name: "Mixed", onlinePools: 99},
				{name: "AllOffline", onlinePools: 0},
			} {
				b.Run(availability.name, func(b *testing.B) {
					_, err := sqlDB.ExecContext(ctx, `
						UPDATE provisioner_daemons
						SET last_seen_at = CASE WHEN (tags->>'pool')::integer < $1
							THEN $2::timestamptz ELSE $3::timestamptz END
						WHERE organization_id = $4`,
						availability.onlinePools, now, now.Add(-5*time.Minute), org.ID)
					require.NoError(b, err)
					_, err = sqlDB.ExecContext(ctx, "ANALYZE provisioner_daemons")
					require.NoError(b, err)
					wantCandidates := min(size.pendingJobs*(100-availability.onlinePools)/100, jobreaper.MaxJobsPerRun)
					for b.Loop() {
						jobs, err := db.GetProvisionerJobsToBeReaped(ctx, params)
						require.NoError(b, err)
						require.Len(b, jobs, wantCandidates)
					}
				})
			}
		})
	}
}
