package coderd_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	coderpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAIProvidersChangedPubsub asserts that the CRUD handlers publish
// on AIProvidersChangedChannel for the operations that affect the
// runtime provider set. Subscribers (aibridged, aibridgeproxyd) depend
// on these notifications to trigger their pool reload.
//
// The handlers publish best-effort and the payload is empty, so we
// assert "at least one event per mutation" via a counter.
func TestAIProvidersChangedPubsub(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	var count atomic.Int64
	unsubscribe, err := api.Pubsub.Subscribe(coderpubsub.AIProvidersChangedChannel, func(_ context.Context, _ []byte) {
		count.Add(1)
	})
	require.NoError(t, err)
	t.Cleanup(unsubscribe)

	// Create.
	req := codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "pubsub-openai",
		Enabled: true,
		BaseURL: "https://api.openai.com/v1/",
		APIKeys: []string{"k1"},
	}
	//nolint:gocritic // Owner role is the audience for this endpoint.
	created, err := client.CreateAIProvider(ctx, req)
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(_ context.Context) bool { return count.Load() >= 1 }, testutil.IntervalFast)

	// Update.
	newKey := "k2"
	_, err = client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
		APIKeys: &[]codersdk.AIProviderKeyMutation{{APIKey: &newKey}},
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(_ context.Context) bool { return count.Load() >= 2 }, testutil.IntervalFast)

	// Delete.
	err = client.DeleteAIProvider(ctx, created.ID.String())
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(_ context.Context) bool { return count.Load() >= 3 }, testutil.IntervalFast)
}
