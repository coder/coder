package messagepartbuffer_test

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestBuffer_CreateEpisodeRejectsDuplicate(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.ErrorIs(t, buffer.CreateEpisode(key), messagepartbuffer.ErrEpisodeExists)
}

func TestBuffer_AddPartAndGetParts(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("hello")))

	parts, err := buffer.GetParts(key)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, int64(1), parts[0].Seq)
	require.Equal(t, codersdk.ChatMessageRoleAssistant, parts[0].Role)
	require.Equal(t, codersdk.ChatMessageText("hello"), parts[0].MessagePart)
}

func TestBuffer_AddPartMissingEpisodeReturnsNotFound(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	err := buffer.AddPart(testEpisodeKey(), codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("hello"))
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeNotFound)
}

func TestBuffer_GetPartsMissingEpisodeReturnsNotFound(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	_, err := buffer.GetParts(testEpisodeKey())
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeNotFound)
}

func TestBuffer_AddPartFullEpisodeReturnsFull(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{MaxEpisodeBytes: 1})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	err := buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("hello"))
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeFull)
	parts, getErr := buffer.GetParts(key)
	require.NoError(t, getErr)
	require.Empty(t, parts)
}

func TestBuffer_CloseEpisodeMissingCreatesClosedEpisode(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CloseEpisode(key))
	parts, err := buffer.GetParts(key)
	require.NoError(t, err)
	require.Empty(t, parts)
	err = buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("tail"))
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeClosed)
}

func TestBuffer_CloseEpisodeIdempotent(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.CloseEpisode(key))
	require.NoError(t, buffer.CloseEpisode(key))
}

func TestBuffer_ModelInvokedAt(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()

	key := testEpisodeKey()
	require.Zero(t, buffer.ModelInvokedAt(key), "unknown episode has no invocation stamp")
	require.ErrorIs(t, buffer.StartModelInvocation(key), messagepartbuffer.ErrEpisodeNotFound)

	require.NoError(t, buffer.CreateEpisode(key))
	require.Zero(t, buffer.ModelInvokedAt(key), "episode without a provider stream has no invocation stamp")
	// Attempt setup happens before the provider stream opens and is
	// not billable.
	clock.Advance(time.Second)
	require.NoError(t, buffer.StartModelInvocation(key))
	invokedAt := buffer.ModelInvokedAt(key)
	require.Equal(t, clock.Now(), invokedAt)

	// A repeat call re-stamps the start so only the most recent
	// invocation is billed.
	clock.Advance(time.Second)
	require.NoError(t, buffer.StartModelInvocation(key))
	require.Equal(t, clock.Now(), buffer.ModelInvokedAt(key))

	// Closing must not move the recorded stamp, and a closed episode
	// no longer accepts an invocation start.
	invokedAt = buffer.ModelInvokedAt(key)
	clock.Advance(1500 * time.Millisecond)
	require.NoError(t, buffer.CloseEpisode(key))
	require.ErrorIs(t, buffer.StartModelInvocation(key), messagepartbuffer.ErrEpisodeClosed)
	require.Equal(t, invokedAt, buffer.ModelInvokedAt(key))

	// Episodes that never open a provider stream, such as local tool
	// execution batches, report no invocation stamp.
	toolBatch := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(toolBatch))
	clock.Advance(time.Second)
	require.NoError(t, buffer.CloseEpisode(toolBatch))
	require.Zero(t, buffer.ModelInvokedAt(toolBatch))

	// Episodes created implicitly by CloseEpisode never started a
	// generation attempt, so they report no invocation stamp.
	implicit := testEpisodeKey()
	require.NoError(t, buffer.CloseEpisode(implicit))
	require.Zero(t, buffer.ModelInvokedAt(implicit))
}

func TestBuffer_ToolCompletions(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()

	key := testEpisodeKey()
	require.Nil(t, buffer.ToolCompletions(key), "unknown episode has no completions")
	require.ErrorIs(t, buffer.RecordToolStart(key, 0, clock.Now()), messagepartbuffer.ErrEpisodeNotFound)
	require.ErrorIs(t, buffer.RecordToolCompletion(key, 0, clock.Now()), messagepartbuffer.ErrEpisodeNotFound)

	require.NoError(t, buffer.CreateEpisode(key))
	require.Empty(t, buffer.ToolCompletions(key), "episode without started tools has no completions")
	require.NoError(t, buffer.RecordToolCompletion(key, 2, clock.Now()))
	require.Empty(t, buffer.ToolCompletions(key), "completion without a start must be dropped")

	clock.Advance(time.Second)
	secondStartedAt := clock.Now()
	require.NoError(t, buffer.RecordToolStart(key, 2, secondStartedAt))
	clock.Advance(time.Second)
	firstStartedAt := clock.Now()
	require.NoError(t, buffer.RecordToolStart(key, 1, firstStartedAt))
	started := map[int]messagepartbuffer.ToolCompletion{
		1: {StartedAt: firstStartedAt},
		2: {StartedAt: secondStartedAt},
	}
	require.Equal(t, started, buffer.ToolCompletions(key))

	clock.Advance(time.Second)
	require.NoError(t, buffer.RecordToolStart(key, 2, clock.Now()))
	require.NoError(t, buffer.RecordToolStart(key, -1, clock.Now()))
	require.Equal(t, started, buffer.ToolCompletions(key), "repeated and invalid starts must not replace or append occurrences")

	clock.Advance(time.Second)
	secondCompletedAt := clock.Now()
	require.NoError(t, buffer.RecordToolCompletion(key, 2, secondCompletedAt))
	clock.Advance(2 * time.Second)
	firstCompletedAt := clock.Now()
	require.NoError(t, buffer.RecordToolCompletion(key, 1, firstCompletedAt))
	completions := buffer.ToolCompletions(key)
	require.Equal(t, map[int]messagepartbuffer.ToolCompletion{
		1: {StartedAt: firstStartedAt, CompletedAt: firstCompletedAt},
		2: {StartedAt: secondStartedAt, CompletedAt: secondCompletedAt},
	}, completions)

	completion := completions[2]
	completion.CompletedAt = clock.Now()
	completions[2] = completion
	require.Equal(t, secondCompletedAt, buffer.ToolCompletions(key)[2].CompletedAt)

	clock.Advance(time.Second)
	require.NoError(t, buffer.RecordToolCompletion(key, 5, clock.Now()))
	require.Len(t, buffer.ToolCompletions(key), 2, "completion without a start must be dropped")

	require.NoError(t, buffer.CloseEpisode(key))
	require.ErrorIs(t, buffer.RecordToolStart(key, 0, clock.Now()), messagepartbuffer.ErrEpisodeClosed)
	require.ErrorIs(t, buffer.RecordToolCompletion(key, 0, clock.Now()), messagepartbuffer.ErrEpisodeClosed)
}

func TestBuffer_SubscribeExistingReplaysThenStreamsLiveParts(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("before")))

	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	defer cancel()
	require.Equal(t, "before", receivePart(t, ch).MessagePart.Text)

	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("after")))
	require.Equal(t, "after", receivePart(t, ch).MessagePart.Text)
}

func TestBuffer_SubscribeClosedEpisodeReplaysThenCloses(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("before")))
	require.NoError(t, buffer.CloseEpisode(key))

	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	defer cancel()
	require.Equal(t, "before", receivePart(t, ch).MessagePart.Text)
	assertChannelClosed(t, ch)
}

func TestBuffer_SubscribeBeforeCreateReturnsAndWaitsWithoutNotFound(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	defer cancel()

	select {
	case part := <-ch:
		t.Fatalf("received part before episode create: %+v", part)
	default:
	}

	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
	require.Equal(t, "live", receivePart(t, ch).MessagePart.Text)
}

func TestBuffer_AddPartAssignsContiguousSeq(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	for i := range 3 {
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(string(rune('a'+i)))))
	}
	parts, err := buffer.GetParts(key)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, []int64{parts[0].Seq, parts[1].Seq, parts[2].Seq})
}

func TestBuffer_EpisodeByteLimitUsesJSONAccounting(t *testing.T) {
	t.Parallel()

	part := codersdk.ChatMessageText("hello")
	limit := serializedPartBytes(t, messagepartbuffer.Part{Seq: 1, Role: codersdk.ChatMessageRoleAssistant, MessagePart: part})
	buffer := messagepartbuffer.New(messagepartbuffer.Options{MaxEpisodeBytes: limit})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, part))
	err := buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("too much"))
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeFull)
}

func TestBuffer_GCClosedEpisodeAfterGraceAndNoSubscribers(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer("message-part-buffer", "subscriber-send")
	defer trap.Close()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{
		Clock:                  clock,
		ClosedEpisodeRetention: time.Minute,
		SubscriberSendTimeout:  10 * time.Minute,
	})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("held")))
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	require.NoError(t, buffer.CloseEpisode(key))
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	clock.Advance(time.Minute).MustWait(ctx)
	clock.Advance(time.Second).MustWait(ctx)
	_, err = buffer.GetParts(key)
	require.NoError(t, err)

	cancel()
	drainUntilClosed(t, ch)
	_, err = buffer.GetParts(key)
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeNotFound)
}

func TestBuffer_GCRetainedSubscribedEpisodeDoesNotBlockOtherExpiredEpisodes(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer("message-part-buffer", "subscriber-send")
	defer trap.Close()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{
		Clock:                  clock,
		ClosedEpisodeRetention: time.Minute,
		SubscriberSendTimeout:  10 * time.Minute,
	})
	defer buffer.Close()
	retainedKey := testEpisodeKey()
	collectedKey := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(retainedKey))
	require.NoError(t, buffer.AddPart(retainedKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("held")))
	require.NoError(t, buffer.CreateEpisode(collectedKey))
	require.NoError(t, buffer.AddPart(collectedKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("collect me")))
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, retainedKey)
	require.NoError(t, err)
	defer cancel()
	require.NoError(t, buffer.CloseEpisode(retainedKey))
	require.NoError(t, buffer.CloseEpisode(collectedKey))
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	clock.Advance(time.Minute).MustWait(ctx)
	clock.Advance(time.Second).MustWait(ctx)

	_, err = buffer.GetParts(retainedKey)
	require.NoError(t, err)
	_, err = buffer.GetParts(collectedKey)
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeNotFound)

	cancel()
	drainUntilClosed(t, ch)
	_, err = buffer.GetParts(retainedKey)
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeNotFound)
}

func TestBuffer_SendWaitingAtCloseTimesOutAfterClose(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const sendTimeout = time.Second
		clock := quartz.NewMock(t)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{
			Clock:                 clock,
			SubscriberSendTimeout: sendTimeout,
		})
		defer buffer.Close()
		key := testEpisodeKey()
		require.NoError(t, buffer.CreateEpisode(key))
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("blocked")))
		ctx := testutil.Context(t, testutil.WaitLong)
		// alive is read just before the timeout, dropped just after.
		alive, cancelAlive, err := buffer.SubscribeToEpisode(ctx, key)
		require.NoError(t, err)
		defer cancelAlive()
		dropped, cancelDropped, err := buffer.SubscribeToEpisode(ctx, key)
		require.NoError(t, err)
		defer cancelDropped()
		synctest.Wait()

		// Time waited while the episode is open does not count.
		clock.Advance(sendTimeout).MustWait(ctx)
		require.NoError(t, buffer.CloseEpisode(key))
		synctest.Wait()

		clock.Advance(sendTimeout - time.Millisecond).MustWait(ctx)
		synctest.Wait()
		require.Equal(t, "blocked", receivePart(t, alive).MessagePart.Text)
		clock.Advance(time.Millisecond).MustWait(ctx)
		synctest.Wait()
		assertChannelClosed(t, dropped)
	})
}

func TestBuffer_SendAfterCloseTimesOut(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const sendTimeout = time.Second
		clock := quartz.NewMock(t)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{
			Clock:                 clock,
			SubscriberSendTimeout: sendTimeout,
		})
		defer buffer.Close()
		key := testEpisodeKey()
		require.NoError(t, buffer.CreateEpisode(key))
		// Both parts exist first, so each subscriber reads them in one batch
		// while the episode is open.
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("first")))
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("second")))
		ctx := testutil.Context(t, testutil.WaitLong)
		alive, cancelAlive, err := buffer.SubscribeToEpisode(ctx, key)
		require.NoError(t, err)
		defer cancelAlive()
		dropped, cancelDropped, err := buffer.SubscribeToEpisode(ctx, key)
		require.NoError(t, err)
		defer cancelDropped()
		synctest.Wait()
		require.NoError(t, buffer.CloseEpisode(key))
		synctest.Wait()

		// The sends of "second" start after the close.
		require.Equal(t, "first", receivePart(t, alive).MessagePart.Text)
		require.Equal(t, "first", receivePart(t, dropped).MessagePart.Text)
		synctest.Wait()

		clock.Advance(sendTimeout - time.Millisecond).MustWait(ctx)
		synctest.Wait()
		require.Equal(t, "second", receivePart(t, alive).MessagePart.Text)
		clock.Advance(time.Millisecond).MustWait(ctx)
		synctest.Wait()
		assertChannelClosed(t, dropped)
	})
}

func TestBuffer_SlowSubscriberOfOpenEpisodeCatchesUp(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const sendTimeout = time.Second
		clock := quartz.NewMock(t)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{
			Clock:                 clock,
			SubscriberSendTimeout: sendTimeout,
		})
		defer buffer.Close()
		key := testEpisodeKey()
		require.NoError(t, buffer.CreateEpisode(key))
		ctx := testutil.Context(t, testutil.WaitLong)
		ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
		require.NoError(t, err)
		defer cancel()
		for i := range 3 {
			require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(string(rune('a'+i)))))
		}
		synctest.Wait()

		clock.Advance(sendTimeout).MustWait(ctx)
		synctest.Wait()
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("d")))
		for _, want := range []string{"a", "b", "c", "d"} {
			require.Equal(t, want, receivePart(t, ch).MessagePart.Text)
		}
	})
}

func TestBuffer_StuckSubscriberRetainsClosedEpisodeUntilSendTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			retention   = 30 * time.Second
			sendTimeout = 75 * time.Second
		)
		clock := quartz.NewMock(t)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{
			Clock:                  clock,
			ClosedEpisodeRetention: retention,
			SubscriberSendTimeout:  sendTimeout,
		})
		defer buffer.Close()
		key := testEpisodeKey()
		require.NoError(t, buffer.CreateEpisode(key))
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("held")))
		ctx := testutil.Context(t, testutil.WaitLong)
		ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
		require.NoError(t, err)
		defer cancel()
		synctest.Wait()

		// Cleanups run every 30s. Close at 20s, drop at 95s, collection at
		// 120s. A timeout counted from the send's start would drop at 75s, and
		// the 90s cleanup would collect the episode.
		advance := func(d time.Duration) {
			clock.Advance(d).MustWait(ctx)
			synctest.Wait()
		}
		advance(20 * time.Second)
		require.NoError(t, buffer.CloseEpisode(key))
		synctest.Wait()
		advance(10 * time.Second)
		advance(30 * time.Second)
		advance(15 * time.Second)
		advance(15 * time.Second)
		require.Equal(t, 1, buffer.EpisodeCount(), "a subscribed closed episode was collected")
		advance(5 * time.Second)
		assertChannelClosed(t, ch)
		require.Equal(t, 1, buffer.EpisodeCount())
		advance(25 * time.Second)
		require.Equal(t, 0, buffer.EpisodeCount(), "the closed episode was not collected after the subscriber was dropped")
	})
}

func TestBuffer_WaitingSendEndsOnShutdownAndCancel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		end  func(buffer *messagepartbuffer.Buffer, cancelSubscription func(), cancelContext context.CancelFunc)
	}{
		{name: "BufferClose", end: func(buffer *messagepartbuffer.Buffer, _ func(), _ context.CancelFunc) { buffer.Close() }},
		{name: "CancelSubscription", end: func(_ *messagepartbuffer.Buffer, cancelSubscription func(), _ context.CancelFunc) {
			cancelSubscription()
		}},
		{name: "CancelContext", end: func(_ *messagepartbuffer.Buffer, _ func(), cancelContext context.CancelFunc) { cancelContext() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: quartz.NewMock(t)})
				defer buffer.Close()
				key := testEpisodeKey()
				require.NoError(t, buffer.CreateEpisode(key))
				require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("blocked")))
				ctx, cancelContext := context.WithCancel(context.Background())
				defer cancelContext()
				ch, cancelSubscription, err := buffer.SubscribeToEpisode(ctx, key)
				require.NoError(t, err)
				defer cancelSubscription()
				// The episode is open, so the send has no deadline.
				synctest.Wait()

				tc.end(buffer, cancelSubscription, cancelContext)
				assertChannelClosed(t, ch)
			})
		})
	}
}

func TestBuffer_BurstyOutputDeliversEveryPartInOrder(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	defer cancel()

	for i := range 8 {
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(string(rune('a'+i)))))
	}
	for i := range 8 {
		part := receivePart(t, ch)
		require.Equal(t, string(rune('a'+i)), part.MessagePart.Text)
	}
}

func TestBuffer_SubscribeCanceledBeforeCreateCanCreateEpisode(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	ctx, cancel := context.WithCancel(context.Background())
	ch, cancelSub, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	cancel()
	drainUntilClosed(t, ch)
	cancelSub()
	require.NoError(t, buffer.CreateEpisode(key))
}

func TestBuffer_SubscribeCanceledWithoutCreateReclaimsEpisode(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancelSub, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	cancelSub()
	// The subscriber goroutine removes itself from the episode before closing
	// the output channel, so cleanup is complete once the channel is closed.
	drainUntilClosed(t, ch)

	_, err = buffer.GetParts(key)
	require.ErrorIs(t, err, messagepartbuffer.ErrEpisodeNotFound)
	require.Equal(t, 0, buffer.EpisodeCount())
}

func TestBuffer_CloseClosesPendingSubscriptionAndRejectsOperations(t *testing.T) {
	t.Parallel()

	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	key := testEpisodeKey()
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	defer cancel()
	buffer.Close()
	assertChannelClosed(t, ch)
	require.ErrorIs(t, buffer.CreateEpisode(key), messagepartbuffer.ErrMessagePartBufferClosed)
}

func testEpisodeKey() messagepartbuffer.Key {
	return messagepartbuffer.Key{ChatID: uuid.New(), HistoryVersion: 1, GenerationAttempt: 1}
}

func receivePart(t *testing.T, ch <-chan messagepartbuffer.Part) messagepartbuffer.Part {
	t.Helper()
	select {
	case part, ok := <-ch:
		require.True(t, ok)
		return part
	case <-time.After(testutil.WaitLong):
		t.Fatal("timed out waiting for buffered part")
		return messagepartbuffer.Part{}
	}
}

func assertChannelClosed[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case _, ok := <-ch:
		require.False(t, ok)
	case <-time.After(testutil.WaitLong):
		t.Fatal("timed out waiting for channel close")
	}
}

func drainUntilClosed[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-time.After(testutil.WaitLong):
			t.Fatal("timed out waiting for channel close")
		}
	}
}

func serializedPartBytes(t *testing.T, part messagepartbuffer.Part) int64 {
	t.Helper()
	data, err := json.Marshal(struct {
		Seq  int64                    `json:"seq"`
		Role codersdk.ChatMessageRole `json:"role"`
		Part codersdk.ChatMessagePart `json:"part"`
	}{
		Seq:  part.Seq,
		Role: part.Role,
		Part: part.MessagePart,
	})
	require.NoError(t, err)
	return int64(len(data))
}
