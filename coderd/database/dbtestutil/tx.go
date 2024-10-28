package dbtestutil

import (
	"sync"
	"testing"

	"github.com/coder/coder/v2/coderd/database"
)

type DBTx struct {
	database.Store
	mu       sync.Mutex
	err      error
	errC     chan error
	finalErr chan error
}

// StartTx starts a transaction and returns a DBTx object. This allows running
// 2 transactions concurrently in a test more easily.
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
				t.Logf("InTx called more than once: %d", count)
			}
			return <-errC
		}, opts)
		finalErr <- err
	}()

	txStore := <-txC
	close(txC)

	return &DBTx{Store: txStore, errC: errC, finalErr: finalErr}
}

func (tx *DBTx) SetError(err error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.err = err
}

// Done can only be called once. If you call it twice, it will panic.
func (tx *DBTx) Done() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	tx.errC <- tx.err
	close(tx.errC)
	return <-tx.finalErr
}
