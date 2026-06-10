package coderd_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	coderpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAIProvidersChangedPubsub asserts that the CRUD handlers publish
// notifications for the operations that affect the runtime provider set.
// Subscribers (aibridged, aibridgeproxyd, chatd) depend on these
// notifications to refresh their provider state.
//
// The handlers publish best-effort, so we assert "at least one event per
// mutation" via counters.
func TestAIProvidersChangedPubsub(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	var providerChangedCount atomic.Int64
	unsubscribeProviderChanged, err := api.Pubsub.Subscribe(coderpubsub.AIProvidersChangedChannel, func(_ context.Context, _ []byte) {
		providerChangedCount.Add(1)
	})
	require.NoError(t, err)
	t.Cleanup(unsubscribeProviderChanged)

	var chatConfigProviderCount atomic.Int64
	var chatConfigUnexpectedCount atomic.Int64
	unsubscribeChatConfig, err := api.Pubsub.SubscribeWithErr(
		coderpubsub.ChatConfigEventChannel,
		coderpubsub.HandleChatConfigEvent(func(_ context.Context, event coderpubsub.ChatConfigEvent, err error) {
			if err != nil || event.Kind != coderpubsub.ChatConfigEventProviders || event.EntityID != uuid.Nil {
				chatConfigUnexpectedCount.Add(1)
				return
			}
			chatConfigProviderCount.Add(1)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(unsubscribeChatConfig)

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
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		return providerChangedCount.Load() >= 1 && chatConfigProviderCount.Load() >= 1
	}, testutil.IntervalFast)
	require.Zero(t, chatConfigUnexpectedCount.Load())

	// Update.
	newKey := "k2"
	_, err = client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
		APIKeys: &[]codersdk.AIProviderKeyMutation{{APIKey: &newKey}},
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		return providerChangedCount.Load() >= 2 && chatConfigProviderCount.Load() >= 2
	}, testutil.IntervalFast)
	require.Zero(t, chatConfigUnexpectedCount.Load())

	// Delete.
	err = client.DeleteAIProvider(ctx, created.ID.String())
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		return providerChangedCount.Load() >= 3 && chatConfigProviderCount.Load() >= 3
	}, testutil.IntervalFast)
	require.Zero(t, chatConfigUnexpectedCount.Load())
}
