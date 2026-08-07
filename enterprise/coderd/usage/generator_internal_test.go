package usage

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestGenerateBucketUniqueViolation deterministically pins every outcome of
// generateBucket's unique-violation branch. TestGeneratorConcurrentReplicas
// also reaches the other-replica-won path, but only when its goroutines
// actually interleave; these cases cannot pass by scheduling accident.
func TestGenerateBucketUniqueViolation(t *testing.T) {
	t.Parallel()

	bucket := time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		// exists and existsErr are returned by the UsageEventExistsByID
		// check that follows the unique violation.
		exists    bool
		existsErr error

		// expectedError is empty when the bucket must resolve as complete.
		expectedError string
	}{
		{
			// The bucket's row landed under our deterministic id: the
			// other replica won the race and the bucket is complete.
			name:   "OtherReplicaWon",
			exists: true,
		},
		{
			// A non-generator writer holds the bucket under a different
			// id, which is what the unique index exists to surface.
			name:          "ForeignID",
			expectedError: "bucket already recorded under a different id",
		},
		{
			// The exists check itself failed: neither state is
			// established, so the error must carry both causes instead of
			// accusing a non-generator writer.
			name:          "ExistsCheckFails",
			existsErr:     xerrors.New("exists check kaboom"),
			expectedError: "check bucket owner after unique violation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

			uniqueViolation := &pq.Error{
				Code:       "23505", // unique_violation
				Constraint: string(database.UniqueIndexUsageEventsAgentRuntime),
			}
			mDB.EXPECT().
				GetTotalChatMessageRuntimeMsInRange(gomock.Any(), gomock.Any()).
				Return(int64(1000), nil)
			mDB.EXPECT().
				InsertUsageEvent(gomock.Any(), gomock.Any()).
				Return(uniqueViolation)
			mDB.EXPECT().
				UsageEventExistsByID(gomock.Any(), gomock.Any()).
				Return(tc.exists, tc.existsErr)

			err := gen.generateBucket(ctx, bucket)
			if tc.expectedError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.expectedError)
			// The unique violation is preserved for the caller's log.
			require.ErrorIs(t, err, uniqueViolation)
			if tc.existsErr != nil {
				// The exists-check failure must not be discarded.
				require.ErrorIs(t, err, tc.existsErr)
			}
		})
	}
}
