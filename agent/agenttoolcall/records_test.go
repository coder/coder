package agenttoolcall_test

import (
	"cmp"
	"context"
	"crypto/sha256"
	"math"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

var (
	inputA   = sha256.Sum256([]byte("a"))
	inputB   = sha256.Sum256([]byte("b"))
	errStart = xerrors.New("start failed")
)

// ageMargin is the margin the spec requires for request transit time.
const ageMargin = 2 * time.Second

func TestStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// uptime is how long the agent has been running. Zero means an
		// hour, long enough for every setup request to be provable.
		uptime time.Duration
		// setup sends earlier requests for the chat, each with age zero.
		setup       func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID)
		messageID   int64
		age         time.Duration
		input       [sha256.Size]byte
		wantStart   bool
		wantValue   string
		wantErr     error
		wantCurrent bool
	}{
		{
			name:        "NoRecordStarts",
			messageID:   1,
			input:       inputA,
			wantStart:   true,
			wantValue:   "new",
			wantCurrent: true,
		},
		{
			name: "RecordReturnsValue",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "first", nil)
			},
			messageID:   1,
			input:       inputA,
			wantValue:   "first",
			wantCurrent: true,
		},
		{
			name: "RecordReturnsError",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "", errStart)
			},
			messageID:   1,
			input:       inputA,
			wantErr:     errStart,
			wantCurrent: true,
		},
		{
			name: "RecordIgnoresAge",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "first", nil)
			},
			messageID:   1,
			age:         2 * time.Hour,
			input:       inputA,
			wantValue:   "first",
			wantCurrent: true,
		},
		{
			name: "InputMismatch",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "first", nil)
			},
			messageID:   1,
			input:       inputB,
			wantErr:     agenttoolcall.ErrInputMismatch,
			wantCurrent: true,
		},
		{
			name: "CanceledRecord",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireCancel(t, r, key(chatID, 1, "call"), "", false, nil)
			},
			messageID:   1,
			input:       inputA,
			wantErr:     agenttoolcall.ErrToolCallCanceled,
			wantCurrent: true,
		},
		{
			name: "StaleMessage",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 2, "other"), inputA, "other", nil)
			},
			messageID: 1,
			input:     inputA,
			wantErr:   agenttoolcall.ErrStaleToolCall,
		},
		{
			name: "StaleBeforeAgentStartedAfterToolCall",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 2, "other"), inputA, "other", nil)
			},
			messageID: 1,
			age:       2 * time.Hour,
			input:     inputA,
			wantErr:   agenttoolcall.ErrStaleToolCall,
		},
		{
			name: "OtherToolCallInLatestMessage",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "other"), inputA, "other", nil)
			},
			messageID:   1,
			input:       inputA,
			wantStart:   true,
			wantValue:   "new",
			wantCurrent: true,
		},
		{
			name: "SameToolCallIDInNewerMessageStarts",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "first", nil)
			},
			messageID:   2,
			input:       inputB,
			wantStart:   true,
			wantValue:   "new",
			wantCurrent: true,
		},
		{
			name:      "AgentStartedAfterToolCall",
			uptime:    10 * time.Second,
			messageID: 1,
			age:       time.Minute,
			input:     inputA,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
		{
			name:      "AgentStartedAtMargin",
			uptime:    10 * time.Second,
			messageID: 1,
			age:       10*time.Second - ageMargin,
			input:     inputA,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
		{
			name:        "AgentStartedPastMargin",
			uptime:      10 * time.Second,
			messageID:   1,
			age:         10*time.Second - ageMargin - time.Nanosecond,
			input:       inputA,
			wantStart:   true,
			wantValue:   "new",
			wantCurrent: true,
		},
		{
			name:      "AgentStartedAfterMaxAge",
			messageID: 1,
			age:       math.MaxInt64,
			input:     inputA,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
		{
			name:      "NegativeAgeCountsAsZero",
			uptime:    time.Second,
			messageID: 1,
			age:       -time.Hour,
			input:     inputA,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRecords(t, cmp.Or(tt.uptime, time.Hour))
			chatID := uuid.New()
			if tt.setup != nil {
				tt.setup(t, r, chatID)
			}

			k := key(chatID, tt.messageID, "call")
			started := false
			got, err := r.Start(t.Context(), k, tt.age, tt.input, func() (string, error) {
				started = true
				return "new", nil
			})
			requireErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantValue, got)
			assert.Equal(t, tt.wantStart, started)
			assert.Equal(t, tt.wantCurrent, r.Current(k))
		})
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// uptime is how long the agent has been running. Zero means an
		// hour, long enough for every setup request to be provable.
		uptime time.Duration
		// setup sends earlier requests for the chat, each with age zero.
		setup       func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID)
		messageID   int64
		age         time.Duration
		wantValue   string
		wantStarted bool
		wantErr     error
		wantCurrent bool
	}{
		{
			name:        "NoRecordRecordsCanceled",
			messageID:   1,
			wantCurrent: true,
		},
		{
			name: "CanceledRecordIsIdempotent",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireCancel(t, r, key(chatID, 1, "call"), "", false, nil)
			},
			messageID:   1,
			wantCurrent: true,
		},
		{
			name: "StartedRecordReturnsValue",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "first", nil)
			},
			messageID:   1,
			wantValue:   "first",
			wantStarted: true,
			wantCurrent: true,
		},
		{
			name: "StartedRecordReturnsError",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "", errStart)
			},
			messageID:   1,
			wantStarted: true,
			wantErr:     errStart,
			wantCurrent: true,
		},
		{
			name: "StartedRecordIgnoresAge",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 1, "call"), inputA, "first", nil)
			},
			messageID:   1,
			age:         2 * time.Hour,
			wantValue:   "first",
			wantStarted: true,
			wantCurrent: true,
		},
		{
			name: "StaleMessage",
			setup: func(t *testing.T, r *agenttoolcall.Records[string], chatID uuid.UUID) {
				requireStart(t, r, key(chatID, 2, "other"), inputA, "other", nil)
			},
			messageID: 1,
			wantErr:   agenttoolcall.ErrStaleToolCall,
		},
		{
			name:      "AgentStartedAtMargin",
			uptime:    10 * time.Second,
			messageID: 1,
			age:       10*time.Second - ageMargin,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
		{
			name:        "AgentStartedPastMargin",
			uptime:      10 * time.Second,
			messageID:   1,
			age:         10*time.Second - ageMargin - time.Nanosecond,
			wantCurrent: true,
		},
		{
			name:      "AgentStartedAfterMaxAge",
			messageID: 1,
			age:       math.MaxInt64,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
		{
			name:      "NegativeAgeCountsAsZero",
			uptime:    time.Second,
			messageID: 1,
			age:       -time.Hour,
			wantErr:   agenttoolcall.ErrAgentStartedAfterToolCall,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRecords(t, cmp.Or(tt.uptime, time.Hour))
			chatID := uuid.New()
			if tt.setup != nil {
				tt.setup(t, r, chatID)
			}

			k := key(chatID, tt.messageID, "call")
			got, started, err := r.Cancel(t.Context(), k, tt.age)
			requireErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantValue, got)
			assert.Equal(t, tt.wantStarted, started)
			assert.Equal(t, tt.wantCurrent, r.Current(k))
		})
	}
}

func TestNewerMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// send sends a request for newer, a tool call in a newer message.
		send        func(t *testing.T, r *agenttoolcall.Records[string], newer agenttoolcall.Key)
		wantDropped bool
	}{
		{
			name: "StartDropsOlderRecords",
			send: func(t *testing.T, r *agenttoolcall.Records[string], newer agenttoolcall.Key) {
				requireStart(t, r, newer, inputA, "newer", nil)
			},
			wantDropped: true,
		},
		{
			name: "CancelDropsOlderRecords",
			send: func(t *testing.T, r *agenttoolcall.Records[string], newer agenttoolcall.Key) {
				requireCancel(t, r, newer, "", false, nil)
			},
			wantDropped: true,
		},
		{
			name: "AgentStartedAfterStartKeepsRecords",
			send: func(t *testing.T, r *agenttoolcall.Records[string], newer agenttoolcall.Key) {
				_, err := r.Start(t.Context(), newer, 2*time.Hour, inputA, notStart(t))
				require.ErrorIs(t, err, agenttoolcall.ErrAgentStartedAfterToolCall)
			},
		},
		{
			name: "AgentStartedAfterCancelKeepsRecords",
			send: func(t *testing.T, r *agenttoolcall.Records[string], newer agenttoolcall.Key) {
				_, _, err := r.Cancel(t.Context(), newer, 2*time.Hour)
				require.ErrorIs(t, err, agenttoolcall.ErrAgentStartedAfterToolCall)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRecords(t, time.Hour)
			chatID := uuid.New()
			first := key(chatID, 1, "first")
			second := key(chatID, 1, "second")
			requireStart(t, r, first, inputA, "first", nil)
			requireCancel(t, r, second, "", false, nil)
			require.True(t, r.Current(first))
			require.True(t, r.Current(second))

			newer := key(chatID, 2, "newer")
			tt.send(t, r, newer)

			assert.Equal(t, tt.wantDropped, r.Current(newer))
			if !tt.wantDropped {
				assert.True(t, r.Current(first))
				assert.True(t, r.Current(second))
				requireStart(t, r, first, inputA, "first", nil)
				return
			}
			assert.False(t, r.Current(first))
			assert.False(t, r.Current(second))
			_, err := r.Start(t.Context(), first, 0, inputA, notStart(t))
			require.ErrorIs(t, err, agenttoolcall.ErrStaleToolCall)
			_, _, err = r.Cancel(t.Context(), second, 0)
			require.ErrorIs(t, err, agenttoolcall.ErrStaleToolCall)
		})
	}
}

func TestChatsAreIndependent(t *testing.T) {
	t.Parallel()

	r := newRecords(t, time.Hour)
	chatA := key(uuid.New(), 5, "call")
	chatB := key(uuid.New(), 1, "call")

	requireStart(t, r, chatA, inputA, "a", nil)
	requireStart(t, r, chatB, inputB, "b", nil)

	assert.True(t, r.Current(chatA))
	assert.True(t, r.Current(chatB))
	requireStart(t, r, chatA, inputA, "a", nil)
}

func TestDoneContext(t *testing.T) {
	t.Parallel()

	t.Run("NoRecordRunsStart", func(t *testing.T) {
		t.Parallel()

		r := newRecords(t, time.Hour)
		k := key(uuid.New(), 1, "call")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		got, err := r.Start(ctx, k, 0, inputA, func() (string, error) { return "new", nil })
		require.NoError(t, err)
		require.Equal(t, "new", got)
		require.True(t, r.Current(k))
	})

	// A done ctx and a published record are both ready, so the record must
	// be checked first. Repeating makes a random choice between the two
	// fail with near certainty.
	t.Run("PublishedRecordWins", func(t *testing.T) {
		t.Parallel()

		r := newRecords(t, time.Hour)
		k := key(uuid.New(), 1, "call")
		requireStart(t, r, k, inputA, "first", nil)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		for range 50 {
			got, err := r.Start(ctx, k, 0, inputA, notStart(t))
			require.NoError(t, err)
			require.Equal(t, "first", got)

			got, started, err := r.Cancel(ctx, k, 0)
			require.NoError(t, err)
			require.True(t, started)
			require.Equal(t, "first", got)
		}
	})
}

func TestStartConcurrentRunsStartOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := newRecords(t, time.Hour)
		k := key(uuid.New(), 1, "call")

		var calls atomic.Int32
		entered := make(chan struct{})
		release := make(chan struct{})
		start := func() (string, error) {
			if calls.Add(1) == 1 {
				close(entered)
			}
			<-release
			return "value", nil
		}

		results := []<-chan startResult{goStart(t.Context(), r, k, inputA, start)}
		<-entered
		for range 7 {
			results = append(results, goStart(t.Context(), r, k, inputA, start))
		}
		synctest.Wait()
		for _, res := range results {
			require.Empty(t, res, "no caller may return before start does")
		}
		require.True(t, r.Current(k), "a pending record is current")

		close(release)
		for _, res := range results {
			got := <-res
			require.NoError(t, got.err)
			require.Equal(t, "value", got.value)
		}
		require.EqualValues(t, 1, calls.Load())
	})
}

func TestStartInputMismatchDoesNotWait(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := newRecords(t, time.Hour)
		k := key(uuid.New(), 1, "call")
		start, entered, release := blockingStart("value", nil)

		started := goStart(t.Context(), r, k, inputA, start)
		<-entered
		// Waiting for the pending start here would deadlock the bubble.
		_, err := r.Start(t.Context(), k, 0, inputB, notStart(t))
		require.ErrorIs(t, err, agenttoolcall.ErrInputMismatch)

		close(release)
		res := <-started
		require.NoError(t, res.err)
		require.Equal(t, "value", res.value)
	})
}

func TestCancelWaitsForPendingStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		err   error
	}{
		{name: "Value", value: "value"},
		{name: "Error", err: errStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				r := newRecords(t, time.Hour)
				k := key(uuid.New(), 1, "call")
				start, entered, release := blockingStart(tt.value, tt.err)

				started := goStart(t.Context(), r, k, inputA, start)
				<-entered
				canceled := goCancel(t.Context(), r, k)
				synctest.Wait()
				require.Empty(t, canceled, "cancel must wait for a pending start")

				close(release)
				got := <-canceled
				requireErrorIs(t, got.err, tt.err)
				assert.True(t, got.started)
				assert.Equal(t, tt.value, got.value)
				res := <-started
				requireErrorIs(t, res.err, tt.err)
				assert.Equal(t, tt.value, res.value)
			})
		})
	}
}

func TestWaiterContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := newRecords(t, time.Hour)
		k := key(uuid.New(), 1, "call")
		start, entered, release := blockingStart("value", nil)

		started := goStart(t.Context(), r, k, inputA, start)
		<-entered
		ctx, cancel := context.WithCancel(t.Context())
		startWaiter := goStart(ctx, r, k, inputA, notStart(t))
		cancelWaiter := goCancel(ctx, r, k)
		synctest.Wait()

		cancel()
		sw := <-startWaiter
		require.ErrorIs(t, sw.err, context.Canceled)
		cw := <-cancelWaiter
		require.ErrorIs(t, cw.err, context.Canceled)
		assert.True(t, cw.started, "a pending record means the agent received the tool call")

		close(release)
		res := <-started
		require.NoError(t, res.err)
		require.Equal(t, "value", res.value)
		requireStart(t, r, k, inputA, "value", nil)
	})
}

func TestStartPanics(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := newRecords(t, time.Hour)
		k := key(uuid.New(), 1, "call")
		entered := make(chan struct{})
		release := make(chan struct{})

		panicked := make(chan any, 1)
		go func() {
			defer func() { panicked <- recover() }()
			_, _ = r.Start(t.Context(), k, 0, inputA, func() (string, error) {
				close(entered)
				<-release
				panic("boom")
			})
		}()
		<-entered
		startWaiter := goStart(t.Context(), r, k, inputA, notStart(t))
		cancelWaiter := goCancel(t.Context(), r, k)
		synctest.Wait()

		close(release)
		require.Equal(t, "boom", <-panicked)
		sw := <-startWaiter
		require.ErrorContains(t, sw.err, "panicked")
		cw := <-cancelWaiter
		require.ErrorContains(t, cw.err, "panicked")
		assert.True(t, cw.started)

		_, err := r.Start(t.Context(), k, 0, inputA, notStart(t))
		require.ErrorContains(t, err, "panicked")
	})
}

func TestDroppedPendingRecordWakesWaiters(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := newRecords(t, time.Hour)
		chatID := uuid.New()
		older := key(chatID, 1, "call")
		newer := key(chatID, 2, "call")
		start, entered, release := blockingStart("older", nil)

		started := goStart(t.Context(), r, older, inputA, start)
		<-entered
		startWaiter := goStart(t.Context(), r, older, inputA, notStart(t))
		cancelWaiter := goCancel(t.Context(), r, older)
		synctest.Wait()

		requireStart(t, r, newer, inputA, "newer", nil)
		require.False(t, r.Current(older))

		close(release)
		for _, res := range []startResult{<-started, <-startWaiter} {
			require.NoError(t, res.err)
			require.Equal(t, "older", res.value)
		}
		cw := <-cancelWaiter
		require.NoError(t, cw.err)
		assert.True(t, cw.started)
		assert.Equal(t, "older", cw.value)

		assert.True(t, r.Current(newer), "publishing a dropped record must not touch current records")
		_, err := r.Start(t.Context(), older, 0, inputA, notStart(t))
		require.ErrorIs(t, err, agenttoolcall.ErrStaleToolCall)
	})
}

type startResult struct {
	value string
	err   error
}

type cancelResult struct {
	value   string
	started bool
	err     error
}

// newRecords returns Records for an agent that has been running for uptime.
func newRecords(t *testing.T, uptime time.Duration) *agenttoolcall.Records[string] {
	t.Helper()
	clock := quartz.NewMock(t)
	r := agenttoolcall.NewRecords[string](agenttoolcall.NewChats(clock))
	clock.Advance(uptime).MustWait(testutil.Context(t, testutil.WaitShort))
	return r
}

func key(chatID uuid.UUID, messageID int64, toolCallID string) agenttoolcall.Key {
	return agenttoolcall.Key{ChatID: chatID, MessageID: messageID, ToolCallID: toolCallID}
}

func requireErrorIs(t *testing.T, err, want error) {
	t.Helper()
	if want == nil {
		require.NoError(t, err)
		return
	}
	require.ErrorIs(t, err, want)
}

// requireStart sends a Start with age zero and requires that it returns
// value and err, running start itself when the key has no record.
func requireStart(t *testing.T, r *agenttoolcall.Records[string], k agenttoolcall.Key, input [sha256.Size]byte, value string, err error) {
	t.Helper()
	got, gotErr := r.Start(t.Context(), k, 0, input, func() (string, error) { return value, err })
	requireErrorIs(t, gotErr, err)
	require.Equal(t, value, got)
}

// requireCancel sends a Cancel with age zero and requires its result.
func requireCancel(t *testing.T, r *agenttoolcall.Records[string], k agenttoolcall.Key, value string, started bool, err error) {
	t.Helper()
	got, gotStarted, gotErr := r.Cancel(t.Context(), k, 0)
	requireErrorIs(t, gotErr, err)
	require.Equal(t, value, got)
	require.Equal(t, started, gotStarted)
}

// notStart returns a start function that fails the test when called.
func notStart(t *testing.T) func() (string, error) {
	return func() (string, error) {
		t.Error("start called for a tool call that already has a record")
		return "", nil
	}
}

// blockingStart returns a start function that closes entered when called
// and returns value and err after release is closed.
func blockingStart(value string, err error) (start func() (string, error), entered <-chan struct{}, release chan<- struct{}) {
	enteredCh := make(chan struct{})
	releaseCh := make(chan struct{})
	start = func() (string, error) {
		close(enteredCh)
		<-releaseCh
		return value, err
	}
	return start, enteredCh, releaseCh
}

func goStart(ctx context.Context, r *agenttoolcall.Records[string], k agenttoolcall.Key, input [sha256.Size]byte, start func() (string, error)) <-chan startResult {
	res := make(chan startResult, 1)
	go func() {
		v, err := r.Start(ctx, k, 0, input, start)
		res <- startResult{value: v, err: err}
	}()
	return res
}

func goCancel(ctx context.Context, r *agenttoolcall.Records[string], k agenttoolcall.Key) <-chan cancelResult {
	res := make(chan cancelResult, 1)
	go func() {
		v, started, err := r.Cancel(ctx, k, 0)
		res <- cancelResult{value: v, started: started, err: err}
	}()
	return res
}

func TestErrorResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantCode    workspacesdk.ToolCallErrorCode
		wantMessage string
		wantOK      bool
	}{
		{name: "Stale", err: agenttoolcall.ErrStaleToolCall, wantCode: workspacesdk.ToolCallErrorStale, wantMessage: "The tool call is in an older message than the chat's latest message.", wantOK: true},
		{name: "AgentStartedAfterToolCall", err: agenttoolcall.ErrAgentStartedAfterToolCall, wantCode: workspacesdk.ToolCallErrorAgentStartedAfterToolCall, wantMessage: "The workspace agent started after the tool call was committed.", wantOK: true},
		{name: "InputMismatch", err: agenttoolcall.ErrInputMismatch, wantCode: workspacesdk.ToolCallErrorInputMismatch, wantMessage: "The request differs from the recorded request for this tool call.", wantOK: true},
		{name: "Canceled", err: agenttoolcall.ErrToolCallCanceled, wantCode: workspacesdk.ToolCallErrorCanceled, wantMessage: "The tool call was canceled.", wantOK: true},
		{name: "Wrapped", err: xerrors.Errorf("start: %w", agenttoolcall.ErrStaleToolCall), wantCode: workspacesdk.ToolCallErrorStale, wantMessage: "The tool call is in an older message than the chat's latest message.", wantOK: true},
		{name: "Nil"},
		{name: "Other", err: xerrors.New("spawn failed")},
		{name: "ContextCanceled", err: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, ok := agenttoolcall.ErrorResponse(tt.err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantCode, resp.Code)
			assert.Equal(t, tt.wantMessage, resp.Message)
		})
	}
}

// TestSharedChats covers record tables built with one Chats, as the
// process and file tables of an agent are: a newer message through one
// table makes older tool calls stale in both, and both use one agent
// start.
func TestSharedChats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// newer sends a request for a tool call in message 2 to the other
		// table.
		newer func(t *testing.T, other *agenttoolcall.Records[string], k agenttoolcall.Key)
	}{
		{
			name: "Start",
			newer: func(t *testing.T, other *agenttoolcall.Records[string], k agenttoolcall.Key) {
				requireStart(t, other, k, inputA, "newer", nil)
			},
		},
		{
			name: "Cancel",
			newer: func(t *testing.T, other *agenttoolcall.Records[string], k agenttoolcall.Key) {
				requireCancel(t, other, k, "", false, nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			chats := agenttoolcall.NewChats(clock)
			processes := agenttoolcall.NewRecords[string](chats)
			files := agenttoolcall.NewRecords[string](chats)
			clock.Advance(time.Hour).MustWait(testutil.Context(t, testutil.WaitShort))
			chatID := uuid.New()
			recorded := key(chatID, 1, "recorded")
			requireStart(t, files, recorded, inputA, "recorded", nil)

			tt.newer(t, processes, key(chatID, 2, "newer"))

			assert.False(t, files.Current(recorded), "a newer message in the other table ends the record")
			_, err := files.Start(t.Context(), recorded, 0, inputA, notStart(t))
			require.ErrorIs(t, err, agenttoolcall.ErrStaleToolCall)
			_, _, err = files.Cancel(t.Context(), key(chatID, 1, "never-received"), 0)
			require.ErrorIs(t, err, agenttoolcall.ErrStaleToolCall)
			// The newer message's tool calls are new to this table.
			requireStart(t, files, key(chatID, 2, "file"), inputA, "file", nil)
			assert.True(t, processes.Current(key(chatID, 2, "newer")))
		})
	}

	t.Run("OneAgentStart", func(t *testing.T) {
		t.Parallel()

		// A table built an hour after the agent started still knows the
		// agent has run for an hour.
		clock := quartz.NewMock(t)
		chats := agenttoolcall.NewChats(clock)
		clock.Advance(time.Hour).MustWait(testutil.Context(t, testutil.WaitShort))
		late := agenttoolcall.NewRecords[string](chats)
		requireStart(t, late, key(uuid.New(), 1, "call"), inputA, "value", nil)
		_, err := late.Start(t.Context(), key(uuid.New(), 1, "call"), 2*time.Hour, inputA, notStart(t))
		require.ErrorIs(t, err, agenttoolcall.ErrAgentStartedAfterToolCall)
	})
}
