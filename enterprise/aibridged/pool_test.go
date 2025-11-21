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
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/mcpmock"
	"github.com/coder/coder/v2/enterprise/aibridged"
	mock "github.com/coder/coder/v2/enterprise/aibridged/aibridgedmock"
)

// TestPool validates the published behavior of [aibridged.CachedBridgePool].
// It is not meant to be an exhaustive test of the internal cache's functionality,
// since that is already covered by its library.
func TestPool(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)

	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Second}
	pool, err := aibridged.NewCachedBridgePool(opts, nil, logger)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Shutdown(context.Background()) })

	id, id2, apiKeyID1, apiKeyID2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	clientFn := func() (aibridged.DRPCClient, error) {
		return client, nil
	}

	// Once a pool instance is initialized, it will try setup its MCP proxier(s).
	// This is called exactly once since the instance below is only created once.
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)
	// This is part of the lifecycle.
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	// Acquiring a pool instance will create one the first time it sees an
	// initiator ID...
	inst, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id,
		APIKeyID:    apiKeyID1.String(),
	}, clientFn, newMockMCPFactory(mcpProxy), nil)
	require.NoError(t, err, "acquire pool instance")

	// ...and it will return it when acquired again.
	instB, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id,
		APIKeyID:    apiKeyID1.String(),
	}, clientFn, newMockMCPFactory(mcpProxy), nil)
	require.NoError(t, err, "acquire pool instance")
	require.Same(t, inst, instB)

	metrics := pool.Metrics()
	require.EqualValues(t, 1, metrics.KeysAdded())
	require.EqualValues(t, 0, metrics.KeysEvicted())
	require.EqualValues(t, 1, metrics.Hits())
	require.EqualValues(t, 1, metrics.Misses())

	// This will get called again because a new instance will be created.
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)

	// But that key will be evicted when a new initiator is seen (maxItems=1):
	inst2, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id2,
		APIKeyID:    apiKeyID1.String(),
	}, clientFn, newMockMCPFactory(mcpProxy), nil)
	require.NoError(t, err, "acquire pool instance")
	require.NotSame(t, inst, inst2)

	metrics = pool.Metrics()
	require.EqualValues(t, 2, metrics.KeysAdded())
	require.EqualValues(t, 1, metrics.KeysEvicted())
	require.EqualValues(t, 1, metrics.Hits())
	require.EqualValues(t, 2, metrics.Misses())

	// This will get called again because a new instance will be created.
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)

	// New instance is created for different api key id
	inst2B, err := pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id2,
		APIKeyID:    apiKeyID2.String(),
	}, clientFn, newMockMCPFactory(mcpProxy), nil)
	require.NoError(t, err, "acquire pool instance 2B")
	require.NotSame(t, inst2, inst2B)

	metrics = pool.Metrics()
	require.EqualValues(t, 3, metrics.KeysAdded())
	require.EqualValues(t, 2, metrics.KeysEvicted())
	require.EqualValues(t, 1, metrics.Hits())
	require.EqualValues(t, 3, metrics.Misses())

	// TODO: add test for expiry.
	// This requires Go 1.25's [synctest](https://pkg.go.dev/testing/synctest) since the
	// internal cache lib cannot be tested using coder/quartz.
}

var _ aibridged.MCPProxyBuilder = &mockMCPFactory{}

type mockMCPFactory struct {
	proxy *mcpmock.MockServerProxier
}

func newMockMCPFactory(proxy *mcpmock.MockServerProxier) *mockMCPFactory {
	return &mockMCPFactory{proxy: proxy}
}

func (m *mockMCPFactory) Build(ctx context.Context, req aibridged.Request) (mcp.ServerProxier, error) {
	return m.proxy, nil
}
