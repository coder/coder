package dbtestutil

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type DBTx struct {
	database.Store
	mu       sync.Mutex
	done     chan error
	finalErr chan error
}

// StartTx starts a transaction and returns a DBTx object. This allows running
// 2 transactions concurrently in a test more easily.
// Example:
//
//	a := StartTx(t, db, opts)
//	b := StartTx(t, db, opts)
//
//	a.GetUsers(...)
//	b.GetUsers(...)
//
//	require.NoError(t, a.Done()
func StartTx(t *testing.T, db database.Store, opts *database.TxOptions) *DBTx {
	done := make(chan error)
	finalErr := make(chan error)
	txC := make(chan database.Store)

	go func() {
		t.Helper()
		once := sync.Once{}
		count := 0

		err := db.InTx(func(store database.Store) error {
			// InTx can be retried
			once.Do(func() {
				txC <- store
			})
			count++
			if count > 1 {
				// If you recursively call InTx, then don't use this.
				t.Logf("InTx called more than once: %d", count)
				assert.NoError(t, xerrors.New("InTx called more than once, this is not allowed with the StartTx helper"))
			}

			<-done
			// Just return nil. The caller should be checking their own errors.
			return nil
		}, opts)
		finalErr <- err
	}()

	txStore := <-txC
	close(txC)

	return &DBTx{Store: txStore, done: done, finalErr: finalErr}
}

// Done can only be called once. If you call it twice, it will panic.
func (tx *DBTx) Done() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	close(tx.done)
	return <-tx.finalErr
}

var errRollbackTestTx = xerrors.New("roll back test transaction")

// StartRolledBackTx returns a Store bound to a transaction that is rolled back
// when the test finishes. It lets parallel subtests that only need query
// isolation share one database instead of creating one each.
func StartRolledBackTx(t testing.TB, db database.Store) database.Store {
	t.Helper()
	done := make(chan struct{})
	txC := make(chan database.Store, 1)
	errC := make(chan error, 1)

	go func() {
		errC <- db.InTx(func(tx database.Store) error {
			txC <- tx
			<-done
			return errRollbackTestTx
		}, nil)
	}()

	var tx database.Store
	select {
	case tx = <-txC:
	case err := <-errC:
		require.NoError(t, err, "start transaction")
	}
	t.Cleanup(func() {
		close(done)
		assert.ErrorIs(t, <-errC, errRollbackTestTx)
	})
	return tx
}
