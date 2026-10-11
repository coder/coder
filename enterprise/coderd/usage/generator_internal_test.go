package usage

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestGenerateBucketLostRace pins that a bucket whose event another writer
// already recorded resolves as complete without writing a rollup, whether
// the insert reports the duplicate as zero rows or, in the narrow
// speculative-insertion race, as a unique violation on the bucket index.
// TestGeneratorConcurrentReplicas also reaches these paths, but only when its
// goroutines actually interleave; these cases cannot pass by scheduling
// accident.
func TestGenerateBucketLostRace(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		rows int64
		err  error
	}{
		{name: "DuplicateID", rows: 0},
		{name: "UniqueViolation", err: &pq.Error{
			Code:       "23505", // unique_violation
			Constraint: string(database.UniqueIndexUsageEventsAgentRuntime),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

			mDB.EXPECT().
				InTx(gomock.Any(), gomock.Any()).
				DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
					return fn(mDB)
				})
			mDB.EXPECT().
				GetAgentRuntimeHourlyUsage(gomock.Any(), gomock.Any()).
				Return([]database.GetAgentRuntimeHourlyUsageRow{{
					OrganizationID: uuid.New(),
					GroupID:        uuid.New(),
					UserID:         uuid.New(),
					RuntimeMs:      1000,
				}}, nil)
			mDB.EXPECT().
				InsertUsageEvent(gomock.Any(), gomock.Any()).
				Return(tc.rows, tc.err)
			// The mock fails the test on an unexpected
			// InsertAgentRuntimeHourlyUsage call.

			bucket := time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC)
			require.NoError(t, gen.generateBucket(ctx, bucket))
		})
	}
}
