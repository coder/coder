package dbtestutil

import (
	"sync"
	"testing"

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
	errC := make(chan error)
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
				t.Fatal("InTx called more than once, this is not allowed with the StartTx helper")
			}

			select {
			case _, _ = <-errC:
			}
			// Just return nil. The caller should be checking their own errors.
			return nil
		}, opts)
		finalErr <- err
	}()

	txStore := <-txC
	close(txC)

	return &DBTx{Store: txStore, done: errC, finalErr: finalErr}
}

// Done can only be called once. If you call it twice, it will panic.
func (tx *DBTx) Done() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	close(tx.done)
	return <-tx.finalErr
}
