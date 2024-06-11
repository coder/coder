package system_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/system"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestNotifyWorkspaceDeleted tests the "public" interface for enqueueing notifications.
// Calling system.NotifyWorkspaceDeleted uses the Enqueuer singleton to enqueue the notification.
func TestNotifyWorkspaceDeleted(t *testing.T) {
	// given
	manager := newFakeEnqueuer()
	notifications.RegisterInstance(manager)

	// when
	system.NotifyWorkspaceDeleted(context.Background(), uuid.New(), "test", "reason", "test")

	// then
	select {
	case ok := <-manager.enqueued:
		require.True(t, ok)
	case <-time.After(testutil.WaitShort):
		t.Fatalf("timed out")
	}
}

type fakeEnqueuer struct {
	enqueued chan bool
}

func newFakeEnqueuer() *fakeEnqueuer {
	return &fakeEnqueuer{enqueued: make(chan bool, 1)}
}

func (f *fakeEnqueuer) Enqueue(context.Context, uuid.UUID, uuid.UUID, types.Labels, string, ...uuid.UUID) (*uuid.UUID, error) {
	f.enqueued <- true
	return nil, nil
}
