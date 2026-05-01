package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAIProvidersPubsubPublish verifies that mutating an AI provider
// publishes on the AIProvidersChangedChannel so each replica's
// RequestBridge pool can invalidate.
func TestAIProvidersPubsubPublish(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	notified := make(chan struct{}, 4)
	cancel, err := ps.Subscribe(coderd.AIProvidersChangedChannel, func(_ context.Context, _ []byte) {
		select {
		case notified <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(cancel)

	// Create publishes.
	//nolint:gocritic // Owner role is the audience for this endpoint.
	created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "pubsub-test",
		Enabled: true,
		BaseURL: "https://api.openai.com/v1",
	})
	require.NoError(t, err)
	select {
	case <-notified:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for pubsub notify after create")
	}

	// Update publishes.
	display := "Renamed"
	//nolint:gocritic // Owner role is the audience for this endpoint.
	_, err = client.UpdateAIProvider(ctx, created.Name, codersdk.UpdateAIProviderRequest{
		DisplayName: &display,
	})
	require.NoError(t, err)
	select {
	case <-notified:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for pubsub notify after update")
	}

	// Delete publishes.
	//nolint:gocritic // Owner role is the audience for this endpoint.
	require.NoError(t, client.DeleteAIProvider(ctx, created.Name))
	select {
	case <-notified:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for pubsub notify after delete")
	}
}
