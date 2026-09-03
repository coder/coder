package usage

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestGenerateBucketUniqueViolation pins that a unique violation on the
// bucket index resolves the bucket as complete: another writer already
// recorded it. TestGeneratorConcurrentReplicas also reaches this path, but
// only when its goroutines actually interleave; this case cannot pass by
// scheduling accident.
func TestGenerateBucketUniqueViolation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

	mDB.EXPECT().
		GetTotalChatMessageRuntimeMsInRange(gomock.Any(), gomock.Any()).
		Return(int64(1000), nil)
	mDB.EXPECT().
		InsertUsageEvent(gomock.Any(), gomock.Any()).
		Return(&pq.Error{
			Code:       "23505", // unique_violation
			Constraint: string(database.UniqueIndexUsageEventsAgentRuntime),
		})

	bucket := time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC)
	require.NoError(t, gen.generateBucket(ctx, bucket))
}
