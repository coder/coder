package messagepartbuffer_test

import (
	"context"
	"encoding/json"
	"testing"
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

func TestBuffer_ToolBatchStartedAt(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()

	key := testEpisodeKey()
	require.Zero(t, buffer.ToolBatchStartedAt(key), "unknown episode has no batch stamp")
	require.ErrorIs(t, buffer.StartToolBatch(key, []string{"call-1"}), messagepartbuffer.ErrEpisodeNotFound)

	require.NoError(t, buffer.CreateEpisode(key))
	require.Zero(t, buffer.ToolBatchStartedAt(key), "episode without a tool batch has no batch stamp")
	// Attempt setup happens before tools start executing and is not
	// billable.
	clock.Advance(time.Second)
	require.NoError(t, buffer.StartToolBatch(key, []string{"call-1"}))
	startedAt := buffer.ToolBatchStartedAt(key)
	require.Equal(t, clock.Now(), startedAt)

	// Closing must not move the recorded stamp, and a closed episode
	// no longer accepts a batch start.
	clock.Advance(1500 * time.Millisecond)
	require.NoError(t, buffer.CloseEpisode(key))
	require.ErrorIs(t, buffer.StartToolBatch(key, []string{"call-1"}), messagepartbuffer.ErrEpisodeClosed)
	require.Equal(t, startedAt, buffer.ToolBatchStartedAt(key))

	// Episodes that never execute local tools, such as model
	// invocations without tool calls, report no batch stamp.
	modelOnly := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(modelOnly))
	require.NoError(t, buffer.StartModelInvocation(modelOnly))
	clock.Advance(time.Second)
	require.NoError(t, buffer.CloseEpisode(modelOnly))
	require.Zero(t, buffer.ToolBatchStartedAt(modelOnly))
}

func TestBuffer_ToolCompletions(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()

	key := testEpisodeKey()
	require.Nil(t, buffer.ToolCompletions(key), "unknown episode has no completions")
	require.ErrorIs(t, buffer.RecordToolCompletion(key, "call-1", clock.Now()), messagepartbuffer.ErrEpisodeNotFound)

	require.NoError(t, buffer.CreateEpisode(key))
	require.Empty(t, buffer.ToolCompletions(key), "episode without a tool batch has no completions")
	// Starting the batch seeds one still-running occurrence per
	// dispatched call, in dispatch order. Duplicate IDs keep distinct
	// occurrences instead of collapsing into one shared state.
	require.NoError(t, buffer.StartToolBatch(key, []string{"call-1", "call-dup", "call-dup"}))
	require.Equal(t, []messagepartbuffer.ToolCompletion{
		{ToolCallID: "call-1"},
		{ToolCallID: "call-dup"},
		{ToolCallID: "call-dup"},
	}, buffer.ToolCompletions(key))
	// A completion stamps the first still-running occurrence with its
	// ID; the duplicate's second occurrence stays running.
	clock.Advance(time.Second)
	firstCompletedAt := clock.Now()
	require.NoError(t, buffer.RecordToolCompletion(key, "call-dup", firstCompletedAt))
	require.Equal(t, []messagepartbuffer.ToolCompletion{
		{ToolCallID: "call-1"},
		{ToolCallID: "call-dup", CompletedAt: firstCompletedAt},
		{ToolCallID: "call-dup"},
	}, buffer.ToolCompletions(key))
	clock.Advance(2 * time.Second)
	secondCompletedAt := clock.Now()
	require.NoError(t, buffer.RecordToolCompletion(key, "call-dup", secondCompletedAt))
	completions := buffer.ToolCompletions(key)
	require.Equal(t, []messagepartbuffer.ToolCompletion{
		{ToolCallID: "call-1"},
		{ToolCallID: "call-dup", CompletedAt: firstCompletedAt},
		{ToolCallID: "call-dup", CompletedAt: secondCompletedAt},
	}, completions)

	// The returned slice is a copy: mutating it must not corrupt the
	// episode's recorded completions.
	completions[0].CompletedAt = clock.Now()
	require.True(t, buffer.ToolCompletions(key)[0].CompletedAt.IsZero())

	// A closed episode keeps its recorded completions but accepts no
	// more, matching the batch-start stamp's lifecycle.
	require.NoError(t, buffer.CloseEpisode(key))
	require.ErrorIs(t, buffer.RecordToolCompletion(key, "call-1", clock.Now()), messagepartbuffer.ErrEpisodeClosed)
	require.True(t, buffer.ToolCompletions(key)[0].CompletedAt.IsZero())
}

func TestBuffer_CloseEpisodeForBilling(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()

	// Closing an unknown episode creates it closed, like CloseEpisode,
	// and reports empty billing.
	unknown := testEpisodeKey()
	billing, err := buffer.CloseEpisodeForBilling(unknown)
	require.NoError(t, err)
	require.Zero(t, billing.ModelInvokedAt)
	require.Zero(t, billing.ToolBatchStartedAt)
	require.Empty(t, billing.ToolCompletions)

	// The snapshot carries every stamp accepted before closure, and
	// stamps are rejected afterwards, so nothing can land in a gap
	// between reading and closing.
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	clock.Advance(time.Second)
	batchStartedAt := clock.Now()
	require.NoError(t, buffer.StartToolBatch(key, []string{"call-1", "call-2"}))
	clock.Advance(time.Second)
	completedAt := clock.Now()
	require.NoError(t, buffer.RecordToolCompletion(key, "call-1", completedAt))
	billing, err = buffer.CloseEpisodeForBilling(key)
	require.NoError(t, err)
	require.Zero(t, billing.ModelInvokedAt)
	require.Equal(t, batchStartedAt, billing.ToolBatchStartedAt)
	require.Equal(t, []messagepartbuffer.ToolCompletion{
		{ToolCallID: "call-1", CompletedAt: completedAt},
		{ToolCallID: "call-2"},
	}, billing.ToolCompletions)
	require.ErrorIs(t, buffer.RecordToolCompletion(key, "call-2", clock.Now()), messagepartbuffer.ErrEpisodeClosed)

	// Closing an already-closed episode still reports its billing, so
	// an interrupt racing the generation task's own close loses
	// nothing.
	again, err := buffer.CloseEpisodeForBilling(key)
	require.NoError(t, err)
	require.Equal(t, billing, again)
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

func TestBuffer_SlowSubscriberClosed(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer("message-part-buffer", "subscriber-send")
	defer trap.Close()
	stopTrap := clock.Trap().TimerStop()
	defer stopTrap.Close()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{
		Clock:                 clock,
		SubscriberSendTimeout: time.Second,
	})
	defer buffer.Close()
	key := testEpisodeKey()
	require.NoError(t, buffer.CreateEpisode(key))
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancel, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	defer cancel()

	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("blocked")))
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	clock.Advance(time.Second).MustWait(ctx)
	stopCall := stopTrap.MustWait(ctx)
	stopCall.MustRelease(ctx)
	assertChannelClosed(t, ch)
}

func TestBuffer_BurstyOutputDoesNotCloseSubscriberBeforeSendTimeout(t *testing.T) {
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
