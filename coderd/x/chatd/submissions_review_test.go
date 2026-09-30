package chatd_test

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/testutil"
)

type submissionCommitFailureStore struct {
	database.Store
	calls  atomic.Int32
	commit bool
}

func (s *submissionCommitFailureStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	if s.calls.Add(1) != 2 {
		return s.Store.InTx(fn, opts)
	}
	if s.commit {
		if err := s.Store.InTx(fn, opts); err != nil {
			return err
		}
	}
	return xerrors.New("transaction outcome unavailable")
}

func TestSubmissionUnknownCommitDoesNotBecomeRejection(t *testing.T) {
	t.Parallel()
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprint(commit), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			user, org, model := seedChatDependencies(t, db)
			var calls atomic.Int32
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = w.Write([]byte("{}")) }))
			t.Cleanup(consumer.Close)
			store := &submissionCommitFailureStore{Store: db, commit: commit}
			server := newHookTestServer(t, store, ps, consumer)
			opts := createHookOptions(t, db, user.ID, org.ID, model.ID, "uncertain commit")
			digest := sha256.Sum256([]byte("uncertain commit"))
			opts.Submission = &chatd.SubmissionOptions{RequestID: uuid.New(), ActorID: user.ID, InputDigest: digest[:]}
			store.calls.Store(0)
			_, err := server.CreateChat(ctx, opts)
			require.ErrorContains(t, err, "transaction outcome unavailable")
			restarted := newHookTestServer(t, db, ps, consumer)
			chat, err := restarted.CreateChat(ctx, opts)
			if commit {
				require.NoError(t, err)
				require.Equal(t, opts.Submission.Receipt.ChatID, chat.ID)
				require.Equal(t, "accepted", opts.Submission.Receipt.State)
			} else {
				var unresolved *chatd.SubmissionError
				require.ErrorAs(t, err, &unresolved)
				require.Equal(t, "uncertain", unresolved.Receipt.State)
			}
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
