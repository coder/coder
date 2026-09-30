package responsesws_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/extract"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy/responsesws"
	codertestutil "github.com/coder/coder/v2/testutil"
)

// TestBlockedRecorderDoesNotStallFrames requires that frames keep flowing
// while a record call blocks, and that accounting catches up afterwards with
// each response's usage recorded once.
func TestBlockedRecorderDoesNotStallFrames(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)
	lanes := []string{"a", "b", "c"}
	for _, l := range lanes {
		require.NotNil(t, h.send(create(l, "model-"+l, "prompt "+l)))
	}

	h.forward(created("a", "resp_a", "model-a"), completed("a", "resp_a"))
	// The accountant is blocked recording resp_a's usage.
	_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
	frames := []string{created("b", "resp_b", "model-b"), created("c", "resp_c", "model-c")}
	for range 18 {
		for _, l := range lanes {
			frames = append(frames, delta(l))
		}
	}
	frames = append(frames, completed("b", "resp_b"), completed("c", "resp_c"))
	h.forward(frames...)
	require.Empty(t, h.rec.RecordedTokenUsages())

	close(gate)
	require.NoError(t, responsesws.Drain(ctx, h.sess))
	want := map[string][]string{}
	for _, l := range lanes {
		want["resp_"+l] = []string{h.interceptionFor("model-" + l).ID}
	}
	require.Equal(t, want, usagesByResponse(h))
	for _, ic := range h.rec.RecordedInterceptions() {
		require.Equal(t, 1, h.rec.endCount(ic.ID), ic.Model)
	}
}

// TestAccountingOverloadIsLogged fills a small accounting queue behind a
// blocked record call. Frames still flow, every dropped job is logged, and
// interceptions that lost a job end with the overload error.
func TestAccountingOverloadIsLogged(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	responsesws.SetQueueBounds(h.sess, 4, extract.MaxEventBytes)
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)

	h.forward(created("z", "resp_0", "model-0"), completed("z", "resp_0"))
	// The accountant is blocked on resp_0's usage with an empty queue.
	_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
	// Two unexplained responses fill the queue with their start and
	// created jobs; everything after them is dropped.
	h.forward(created("y", "resp_1", "model-1"), created("x", "resp_2", "model-2"))
	h.forward(completed("y", "resp_1"), completed("x", "resp_2"), created("w", "resp_3", "model-3"))
	require.Equal(t, 3, h.logs.count("accounting queue full: dropped event"))

	close(gate)
	require.NoError(t, responsesws.Drain(ctx, h.sess))
	require.NoError(t, h.sess.Close(nil))
	require.Equal(t, map[string][]string{"resp_0": {h.interceptionFor("model-0").ID}}, usagesByResponse(h))
	require.Len(t, h.rec.RecordedInterceptions(), 3)
	require.Empty(t, h.rec.RecordedInterceptionEnd(h.interceptionFor("model-0").ID).ErrorType)
	for _, model := range []string{"model-1", "model-2"} {
		end := h.rec.RecordedInterceptionEnd(h.interceptionFor(model).ID)
		require.NotNil(t, end, model)
		require.Equal(t, recorder.ErrorTypeUnknown, end.ErrorType, model)
		require.Contains(t, end.ErrorMessage, "accounting queue overloaded", model)
	}
}

// TestRacedErrorKeepsCause covers events that race a create's write: an
// error event is the create's answer and becomes its end cause, while a
// response.created that opened its own interception retires the create
// without an error.
func TestRacedErrorKeepsCause(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		event   string
		errType recorder.ErrorType
		message string
	}{
		{
			name:    "ErrorEvent",
			event:   `{"type":"error","status":400,"error":{"type":"invalid_request_error","code":"invalid_value","message":"bad input"}}`,
			errType: recorder.ErrorTypeBadRequest, message: "bad input",
		},
		{name: "ResponseCreated", event: created("", "resp_1", "model-server")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitShort)
			h := newHarness(ctx, t, nil)
			entered, release := make(chan struct{}), make(chan struct{})
			write := func(ctx context.Context) error {
				close(entered)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			h.conn.onWrite.Store(&write)
			sent := make(chan error, 1)
			go func() { sent <- h.sess.Send(ctx, []byte(create("", "model-create", "hi"))) }()
			_ = codertestutil.TryReceive(ctx, t, entered)
			h.relay(tc.event)
			close(release)
			require.NoError(t, codertestutil.TryReceive(ctx, t, sent))

			id := h.interceptionFor("model-create").ID
			end := h.rec.RecordedInterceptionEnd(id)
			require.NotNil(t, end)
			require.Equal(t, tc.errType, end.ErrorType)
			require.Equal(t, tc.message, end.ErrorMessage)
			require.Equal(t, 1, h.rec.endCount(id))
		})
	}
}

// TestUnknownFramesUnchanged requires that frames the extractor cannot use,
// including invalid JSON and oversized events, are forwarded byte for byte
// while recording is blocked, and produce parse notes but no records.
func TestUnknownFramesUnchanged(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)
	require.NotNil(t, h.send(create("", "gpt-6", "hi")))
	h.relay(created("", "resp_1", "gpt-6"))
	prompts := len(h.rec.RecordedPromptUsages())

	huge := `{"type":"response.in_progress","response":{"id":"resp_1","pad":"` + strings.Repeat("x", extract.MaxEventBytes) + `"}}`
	h.forward(
		`{"type":"response.future_event","opaque":{"a":[1,2]}}`,
		`not json at all`,
		`{"type":"response.steer.accepted","steer":{"previous_response_id":"resp_1"}}`,
		huge,
		fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q}`, strings.Repeat("y", 1024)),
	)
	require.NoError(t, responsesws.Drain(ctx, h.sess))
	// Invalid JSON and the oversized event reach the extractor as notes.
	require.Equal(t, 2, h.logs.count("extractor parse note"))
	require.Len(t, h.rec.RecordedPromptUsages(), prompts)
	require.Empty(t, h.rec.RecordedTokenUsages())
	require.Empty(t, h.rec.RecordedToolUsages())
	require.Empty(t, h.rec.RecordedModelThoughts())
	close(gate)
}

// TestEndRecordGetsFreshContext requires that an interception's end record
// gets a full bound of its own after the extractor finished recording.
func TestEndRecordGetsFreshContext(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)
	require.NotNil(t, h.send(create("", "gpt-6", "hi")))
	h.relay(created("", "resp_1", "gpt-6"))
	h.forward(completed("", "resp_1"))
	_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
	close(gate)
	require.NoError(t, responsesws.Drain(ctx, h.sess))

	id := h.interceptionFor("gpt-6").ID
	require.Equal(t, 1, h.rec.endCount(id))
	h.rec.mu.Lock()
	deadline, returned := h.rec.endDeadlines[id], h.rec.usageReturned
	h.rec.mu.Unlock()
	require.False(t, returned.IsZero())
	require.False(t, deadline.Before(returned.Add(recorder.DefaultAsyncTimeout)),
		"end deadline %s is not a full bound after usage returned at %s", deadline, returned)
}

// TestLossyInterceptionEndsWithOverload requires that an interception that
// lost a non-terminal accounting job ends with the overload error, even
// when its terminal event is recorded.
func TestLossyInterceptionEndsWithOverload(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	require.NotNil(t, h.send(create("a", "model-a", "hi")))
	h.relay(created("a", "resp_a", "model-a"))
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)
	h.forward(created("z", "resp_0", "model-0"), completed("z", "resp_0"))
	// The accountant is blocked on resp_0's usage with an empty queue.
	_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
	responsesws.SetQueueBounds(h.sess, 1, extract.MaxEventBytes)
	inProgress := `{"type":"response.in_progress","stream_id":"a","response":{"id":"resp_a"}}`
	h.forward(inProgress, inProgress)
	require.Equal(t, 1, h.logs.count("accounting queue full: dropped event"))

	close(gate)
	require.NoError(t, responsesws.Drain(ctx, h.sess))
	h.relay(completed("a", "resp_a"))
	id := h.interceptionFor("model-a").ID
	require.Equal(t, []string{id}, usagesByResponse(h)["resp_a"])
	end := h.rec.RecordedInterceptionEnd(id)
	require.NotNil(t, end)
	require.Equal(t, recorder.ErrorTypeUnknown, end.ErrorType)
	require.Contains(t, end.ErrorMessage, "accounting queue overloaded")
	require.Equal(t, 1, h.rec.endCount(id))
}

// TestServerOpenedStartedAtIsArrival requires that an interception the
// server opened starts when its frame arrived, not when the accountant got
// to record it.
func TestServerOpenedStartedAtIsArrival(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)
	h.forward(created("z", "resp_0", "model-0"), completed("z", "resp_0"))
	_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)

	// resp_1's start is queued behind the blocked usage record.
	h.forward(created("y", "resp_1", "model-1"))
	arrived := time.Now()
	close(gate)
	require.NoError(t, responsesws.Drain(ctx, h.sess))
	require.False(t, h.interceptionFor("model-1").StartedAt.After(arrived))
}

// TestFailedStartIsRetried requires that a failed start of an interception
// the server opened does not end it: the start is retried before its later
// events, so its terminal usage lands on that same interception whatever
// the timing. If the start never succeeds, the response is dropped with a
// log and no state is left behind.
func TestFailedStartIsRetried(t *testing.T) {
	t.Parallel()
	t.Run("Recovers", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		h.rec.failStart.Store(true)
		h.relay(created("z", "resp_0", "model-0"))
		require.Empty(t, h.rec.RecordedInterceptions())

		h.rec.failStart.Store(false)
		// The terminal event carries no model, so a new interception
		// would be recorded without one.
		h.relay(completed("z", "resp_0"))
		ics := h.rec.RecordedInterceptions()
		require.Len(t, ics, 1)
		require.Equal(t, "model-0", ics[0].Model)
		require.Equal(t, map[string][]string{"resp_0": {ics[0].ID}}, usagesByResponse(h))
		require.Equal(t, 1, h.rec.endCount(ics[0].ID))
	})
	t.Run("NeverStarts", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		h.rec.failStart.Store(true)
		h.relay(created("z", "resp_0", "model-0"), completed("z", "resp_0"))
		require.Empty(t, h.rec.RecordedInterceptions())
		require.Empty(t, h.rec.RecordedTokenUsages())
		require.Equal(t, 1, h.logs.count("dropped response: its interception could not be recorded"))
		require.Equal(t, map[string]int{}, responsesws.StateSizes(h.sess))
	})
}
