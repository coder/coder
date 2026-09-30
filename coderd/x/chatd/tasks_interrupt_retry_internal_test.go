//nolint:testpackage // Exercises the package-private interrupt task.
package chatd

import (
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// failNthTxStore rolls back the nth transaction opened through it after its
// callback succeeds, as a transient database failure would.
type failNthTxStore struct {
	database.Store
	calls atomic.Int32
	failN int32
}

var errInjectedTxFailure = xerrors.New("injected transient transaction failure")

func (s *failNthTxStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	n := s.calls.Add(1)
	return s.Store.InTx(func(tx database.Store) error {
		if err := fn(tx); err != nil {
			return err
		}
		if n == s.failN {
			return errInjectedTxFailure
		}
		return nil
	}, opts)
}

// TestInterruptTask_RetryKeepsFirstAttemptSnapshot fails the first
// interrupt attempt's FinishInterruption transaction and retries it, as
// runTaskWithRetry would. The retry must commit the partial answer and the
// model runtime the first attempt observed, whether or not the buffer's
// closed-episode retention (15 s) expired in between. Tool cancellation
// alone can take up to 30 s before the commit.
func TestInterruptTask_RetryKeepsFirstAttemptSnapshot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// advances run between the attempts. The episode closes at 1.5 s
		// and the buffer cleanup ticks every 15 s.
		advances []time.Duration
	}{
		{
			name:     "WithinRetention",
			advances: []time.Duration{13500 * time.Millisecond},
		},
		{
			// The retry at 18 s collects the episode closed at 1.5 s.
			name:     "AfterRetention",
			advances: []time.Duration{13500 * time.Millisecond, 3 * time.Second},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newTaskTestFixture(t)
			chat := f.createRunningChat(t)
			workerID := uuid.New()
			runnerID := uuid.New()
			acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
			recorder := newTaskSideEffectRecorder()
			clock := quartz.NewMock(t)
			starter := newTestTaskStarterWithClock(t, f, recorder, clock)
			// Transaction 1 is the interrupt's ReadLock, transaction 2 its
			// FinishInterruption update.
			starter.opts.Store = &failNthTxStore{Store: f.db, failN: 2}
			buffer := starter.opts.MessagePartBuffer
			key := messagepartbuffer.Key{
				ChatID:            chat.ID,
				HistoryVersion:    acquired.HistoryVersion,
				GenerationAttempt: acquired.GenerationAttempt,
			}
			require.NoError(t, buffer.CreateEpisode(key))
			require.NoError(t, buffer.StartModelInvocation(key))
			require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("partial answer")))
			clock.Advance(1500 * time.Millisecond)
			interrupting := f.interruptChat(t, chat.ID)
			// runner.runTask shares one state across the attempts of a task.
			input := chatWorkerTaskStartInput{
				ChatID:            chat.ID,
				WorkerID:          workerID,
				RunnerID:          runnerID,
				HistoryVersion:    interrupting.HistoryVersion,
				GenerationAttempt: interrupting.GenerationAttempt,
				Status:            database.ChatStatusInterrupting,
				Interrupt:         &interruptTaskState{},
			}

			err := starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), input)
			require.ErrorIs(t, err, errTaskRetryable)
			require.ErrorIs(t, err, errInjectedTxFailure)

			ctx := testutil.Context(t, testutil.WaitShort)
			for _, d := range tc.advances {
				clock.Advance(d).MustWait(ctx)
			}

			require.NoError(t, starter.StartInterrupt(testutil.Context(t, testutil.WaitLong), input))

			messages, err := f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
			require.NoError(t, err)
			var assistant *database.ChatMessage
			for i := range messages {
				if messages[i].Role == database.ChatMessageRoleAssistant {
					assistant = &messages[i]
				}
			}
			require.NotNil(t, assistant, "partial assistant message was not persisted after the interrupt retry")
			parts, err := chatprompt.ParseContent(*assistant)
			require.NoError(t, err)
			require.Len(t, parts, 1)
			require.Equal(t, "partial answer", parts[0].Text)
			// Billed from model invocation to the first attempt's interrupt
			// instant; retry delay is not billed.
			require.Equal(t, sql.NullInt64{Int64: 1500, Valid: true}, assistant.RuntimeMs)
		})
	}
}
