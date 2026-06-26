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
