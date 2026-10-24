package agentgit_test

import (
	"fmt"
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

// TestPathStore_AddReadPaths_NotifiesOnlyNewPaths pins that a read
// notifies only when it adds a path. Every notification makes each
// watcher re-resolve the chat's paths, and a read does not change the
// repository.
func TestPathStore_AddReadPaths_NotifiesOnlyNewPaths(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ancestorID := uuid.New()
	chatCh, unsubChat := ps.Subscribe(chatID)
	defer unsubChat()
	ancestorCh, unsubAncestor := ps.Subscribe(ancestorID)
	defer unsubAncestor()

	ps.AddReadPaths([]uuid.UUID{chatID, ancestorID}, []string{"/repo"})
	require.Len(t, chatCh, 1)
	require.Len(t, ancestorCh, 1)
	<-chatCh
	<-ancestorCh

	ps.AddReadPaths([]uuid.UUID{chatID, ancestorID}, []string{"/repo"})
	require.Empty(t, chatCh, "a repeated read must not notify")
	require.Empty(t, ancestorCh, "a repeated read must not notify")

	// Only the chat that gains a path is notified.
	ps.AddReadPaths([]uuid.UUID{ancestorID}, []string{"/other"})
	ps.AddReadPaths([]uuid.UUID{chatID, ancestorID}, []string{"/other"})
	require.Len(t, chatCh, 1)
	require.Len(t, ancestorCh, 1)
	<-chatCh
	<-ancestorCh

	// A write to a known path notifies, because it changes the repo.
	ps.AddPaths([]uuid.UUID{chatID}, []string{"/repo"})
	require.Len(t, chatCh, 1)
	require.Equal(t, []string{"/other", "/repo"}, ps.GetPaths(chatID))
}

// TestPathStore_EvictsReadPathsBeforeWritten pins the per-chat cap:
// over the cap, the least recently used read-only paths go first, and
// written paths survive any number of reads.
func TestPathStore_EvictsReadPathsBeforeWritten(t *testing.T) {
	t.Parallel()

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ids := []uuid.UUID{chatID}

	ps.AddPaths(ids, []string{"/written"})
	// A read of a written path keeps it written.
	ps.AddReadPaths(ids, []string{"/written"})
	// A write of a read path makes it written.
	ps.AddReadPaths(ids, []string{"/upgraded"})
	ps.AddPaths(ids, []string{"/upgraded"})
	ps.AddReadPaths(ids, []string{"/read-old"})
	for i := range agentgit.MaxPathsPerChat {
		ps.AddReadPaths(ids, []string{fmt.Sprintf("/read-%04d", i)})
		if i == agentgit.MaxPathsPerChat/2 {
			// Re-reading refreshes recency.
			ps.AddReadPaths(ids, []string{"/read-old"})
		}
	}

	paths := ps.GetPaths(chatID)
	require.Len(t, paths, agentgit.MaxPathsPerChat)
	require.Contains(t, paths, "/written")
	require.Contains(t, paths, "/upgraded")
	require.Contains(t, paths, "/read-old")
	require.NotContains(t, paths, "/read-0000")
	require.NotContains(t, paths, "/read-0001")
	require.NotContains(t, paths, "/read-0002")
	require.Contains(t, paths, "/read-0003")

	// Written paths are evicted only when nothing read-only is left.
	for i := range agentgit.MaxPathsPerChat {
		ps.AddPaths(ids, []string{fmt.Sprintf("/write-%04d", i)})
	}
	paths = ps.GetPaths(chatID)
	require.Len(t, paths, agentgit.MaxPathsPerChat)
	require.NotContains(t, paths, "/read-old")
	require.NotContains(t, paths, "/written")
	require.Contains(t, paths, "/write-0000")
}
