package aibridged_test

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/enterprise/x/aibridged"
	mock "github.com/coder/coder/v2/enterprise/x/aibridged/aibridgedmock"
)

// TestPool validates the published behavior of [aibridged.CachedBridgePool].
// It is not meant to be an exhaustive test of the internal cache's functionality,
// since that is already covered by its library.
func TestPool(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)

	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Second}
	pool, err := aibridged.NewCachedBridgePool(opts, nil, logger)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Shutdown(context.Background()) })

	id, id2 := uuid.New(), uuid.New()
	clientFn := func() (aibridged.DRPCClient, error) {
		return client, nil
	}

	// Acquiring a pool instance will create one the first time it sees an
	// initiator ID...
	inst, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id,
	}, clientFn)
	require.NoError(t, err, "acquire pool instance")

	// ...and it will return it when acquired again.
	instB, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id,
	}, clientFn)
	require.NoError(t, err, "acquire pool instance")
	require.Same(t, inst, instB)

	metrics := pool.Metrics()
	require.EqualValues(t, 1, metrics.KeysAdded())
	require.EqualValues(t, 0, metrics.KeysEvicted())
	require.EqualValues(t, 1, metrics.Hits())
	require.EqualValues(t, 1, metrics.Misses())

	// But that key will be evicted when a new initiator is seen (maxItems=1):
	inst2, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id2,
	}, clientFn)
	require.NoError(t, err, "acquire pool instance")
	require.NotSame(t, inst, inst2)

	metrics = pool.Metrics()
	require.EqualValues(t, 2, metrics.KeysAdded())
	require.EqualValues(t, 1, metrics.KeysEvicted())
	require.EqualValues(t, 1, metrics.Hits())
	require.EqualValues(t, 2, metrics.Misses())

	// TODO: add test for expiry.
	// This requires Go 1.25's [synctest](https://pkg.go.dev/testing/synctest) since the
	// internal cache lib cannot be tested using coder/quartz.
}
