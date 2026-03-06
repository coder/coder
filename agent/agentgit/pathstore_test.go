package agentgit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/testutil"
)

func TestPathStore_AddPaths_StoresForChatAndAncestors(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ancestor1 := uuid.New()
	ancestor2 := uuid.New()

	ps.AddPaths([]uuid.UUID{chatID, ancestor1, ancestor2}, []string{"/a", "/b"})

	// All three IDs should see the paths.
	require.Equal(t, []string{"/a", "/b"}, ps.GetPaths(chatID))
	require.Equal(t, []string{"/a", "/b"}, ps.GetPaths(ancestor1))
	require.Equal(t, []string{"/a", "/b"}, ps.GetPaths(ancestor2))

	// An unrelated chat should see nothing.
	require.Nil(t, ps.GetPaths(uuid.New()))
}

func TestPathStore_AddPaths_SkipsNilUUIDs(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()

	// A nil chatID should be a no-op.
	ps.AddPaths([]uuid.UUID{uuid.Nil}, []string{"/x"})
	require.Nil(t, ps.GetPaths(uuid.Nil))

	// A nil ancestor should be silently skipped.
	chatID := uuid.New()
	ps.AddPaths([]uuid.UUID{chatID, uuid.Nil}, []string{"/y"})
	require.Equal(t, []string{"/y"}, ps.GetPaths(chatID))
	require.Nil(t, ps.GetPaths(uuid.Nil))
}

func TestPathStore_GetPaths_DeduplicatedSorted(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	ps.AddPaths([]uuid.UUID{chatID}, []string{"/z", "/a", "/m", "/a", "/z"})
	ps.AddPaths([]uuid.UUID{chatID}, []string{"/a", "/b"})

	got := ps.GetPaths(chatID)
	require.Equal(t, []string{"/a", "/b", "/m", "/z"}, got)
}

func TestPathStore_Subscribe_ReceivesNotification(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	ch, unsub := ps.Subscribe(chatID)
	defer unsub()

	ps.AddPaths([]uuid.UUID{chatID}, []string{"/file"})

	ctx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-ch:
		// Success.
	case <-ctx.Done():
		t.Fatal("timed out waiting for notification")
	}
}

func TestPathStore_Subscribe_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	ch1, unsub1 := ps.Subscribe(chatID)
	defer unsub1()
	ch2, unsub2 := ps.Subscribe(chatID)
	defer unsub2()

	ps.AddPaths([]uuid.UUID{chatID}, []string{"/file"})

	ctx := testutil.Context(t, testutil.WaitShort)
	for i, ch := range []<-chan struct{}{ch1, ch2} {
		select {
		case <-ch:
			// OK
		case <-ctx.Done():
			t.Fatalf("subscriber %d did not receive notification", i)
		}
	}
}

func TestPathStore_Unsubscribe_StopsNotifications(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	ch, unsub := ps.Subscribe(chatID)
	unsub()

	ps.AddPaths([]uuid.UUID{chatID}, []string{"/file"})

	// AddPaths sends synchronously via a non-blocking send to the
	// buffered channel, so if a notification were going to arrive
	// it would already be in the channel by now.
	select {
	case <-ch:
		t.Fatal("received notification after unsubscribe")
	default:
		// Expected: no notification.
	}
}

func TestPathStore_Subscribe_AncestorNotification(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ancestor := uuid.New()

	// Subscribe to the ancestor, then add paths via the child.
	ch, unsub := ps.Subscribe(ancestor)
	defer unsub()

	ps.AddPaths([]uuid.UUID{chatID, ancestor}, []string{"/file"})

	ctx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-ch:
		// Success.
	case <-ctx.Done():
		t.Fatal("ancestor subscriber did not receive notification")
	}
}

func TestPathStore_Notify_NotifiesWithoutAddingPaths(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	ch, unsub := ps.Subscribe(chatID)
	defer unsub()

	ps.Notify([]uuid.UUID{chatID})

	ctx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-ch:
		// Success.
	case <-ctx.Done():
		t.Fatal("timed out waiting for notification")
	}

	require.Nil(t, ps.GetPaths(chatID))
}

func TestPathStore_Notify_SkipsNilUUIDs(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	ch, unsub := ps.Subscribe(chatID)
	defer unsub()

	ps.Notify([]uuid.UUID{uuid.Nil})

	// Notify sends synchronously via a non-blocking send to the
	// buffered channel, so if a notification were going to arrive
	// it would already be in the channel by now.
	select {
	case <-ch:
		t.Fatal("received notification for nil UUID")
	default:
		// Expected: no notification.
	}

	require.Nil(t, ps.GetPaths(chatID))
}

func TestPathStore_Notify_AncestorNotification(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ancestorID := uuid.New()

	// Subscribe to the ancestor, then notify via the child.
	ch, unsub := ps.Subscribe(ancestorID)
	defer unsub()

	ps.Notify([]uuid.UUID{chatID, ancestorID})

	ctx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-ch:
		// Success.
	case <-ctx.Done():
		t.Fatal("ancestor subscriber did not receive notification")
	}

	require.Nil(t, ps.GetPaths(ancestorID))
}

func TestPathStore_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	const goroutines = 20
	const iterations = 50

	chatIDs := make([]uuid.UUID, goroutines)
	for i := range chatIDs {
		chatIDs[i] = uuid.New()
	}

	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // writers + readers

	// Writers.
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			for j := range iterations {
				ancestors := []uuid.UUID{chatIDs[(idx+1)%goroutines]}
				path := []string{
					"/file-" + chatIDs[idx].String() + "-" + time.Now().Format(time.RFC3339Nano),
					"/iter-" + string(rune('0'+j%10)),
				}
				ps.AddPaths(append([]uuid.UUID{chatIDs[idx]}, ancestors...), path)
			}
		}(i)
	}

	// Readers.
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			for range iterations {
				_ = ps.GetPaths(chatIDs[idx])
			}
		}(i)
	}

	wg.Wait()

	// Verify every chat has at least the paths it wrote.
	for _, id := range chatIDs {
		paths := ps.GetPaths(id)
		require.NotEmpty(t, paths, "chat %s should have paths", id)
	}
}
