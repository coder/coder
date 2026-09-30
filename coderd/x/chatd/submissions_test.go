package chatd_test

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateSubmissionDoesNotRepeatHooks(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			user, org, model := seedChatDependencies(t, db)
			var calls atomic.Int32
			requestID := uuid.New()
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if status != http.StatusOK {
					journal, err := db.GetChatSubmission(ctx, database.GetChatSubmissionParams{OrganizationID: org.ID, ActorID: user.ID, RequestID: requestID})
					require.NoError(t, err)
					require.Equal(t, "reserved", journal.State)
				}
				if status == http.StatusForbidden {
					_, _ = w.Write([]byte(`{"permission":{"decision":"deny"},"user_message":"blocked"}`))
					return
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("{}"))
			}))
			t.Cleanup(consumer.Close)
			server := newHookTestServer(t, db, ps, consumer)
			digest := sha256.Sum256([]byte("same creation"))
			opts := createHookOptions(t, db, user.ID, org.ID, model.ID, "same creation")
			opts.Submission = &chatd.SubmissionOptions{RequestID: requestID, ActorID: user.ID, InputDigest: digest[:]}
			first, err := server.CreateChat(ctx, opts)
			if status == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			callsBeforeReplay := calls.Load()
			require.Positive(t, callsBeforeReplay)
			// A separate server instance has no in-memory knowledge of the first call.
			restarted := newHookTestServer(t, db, ps, consumer)
			second, err := restarted.CreateChat(ctx, opts)
			if status == http.StatusOK {
				require.NoError(t, err)
				require.Equal(t, first.ID, second.ID)
			} else {
				var uncertain *chatd.SubmissionError
				require.ErrorAs(t, err, &uncertain)
				if status == http.StatusForbidden {
					require.Equal(t, "rejected", uncertain.Receipt.State)
					require.Contains(t, uncertain.Receipt.Error, "blocked")
				} else {
					require.Equal(t, "uncertain", uncertain.Receipt.State)
				}
			}
			require.Equal(t, callsBeforeReplay, calls.Load())
			changed := sha256.Sum256([]byte("changed creation"))
			opts.Submission.InputDigest = changed[:]
			_, err = restarted.CreateChat(ctx, opts)
			var conflict *chatd.SubmissionError
			require.ErrorAs(t, err, &conflict)
			require.True(t, conflict.Conflict)
			require.Equal(t, callsBeforeReplay, calls.Load())
		})
	}
}

func TestMessageSubmissionConcurrentHookReservation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(consumer.Close)
	server := newHookTestServer(t, db, ps, consumer)
	digest := sha256.Sum256([]byte("same message"))
	requestID := uuid.New()
	options := func() chatd.SendMessageOptions {
		return chatd.SendMessageOptions{
			ChatID: chat.ID, CreatedBy: user.ID, Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("same message")},
			Submission: &chatd.SubmissionOptions{RequestID: requestID, ActorID: user.ID, InputDigest: digest[:]},
		}
	}
	first := options()
	done := make(chan error, 1)
	go func() { _, err := server.SendMessage(ctx, first); done <- err }()
	testutil.RequireReceive(ctx, t, entered)
	_, err := server.SendMessage(ctx, options())
	var uncertain *chatd.SubmissionError
	require.ErrorAs(t, err, &uncertain)
	require.Equal(t, "reserved", uncertain.Receipt.State)
	close(release)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
	retry := options()
	_, err = server.SendMessage(ctx, retry)
	require.NoError(t, err)
	require.Equal(t, first.Submission.Receipt.ID, retry.Submission.Receipt.ID)
	require.NotZero(t, retry.Submission.Receipt.MessageID)
	select {
	case <-entered:
		t.Fatal("replayed admission hook")
	default:
	}
}

// The admission callback must share the receipt/mutation transaction, while
// external hooks run after the durable reservation commits and releases locks.
