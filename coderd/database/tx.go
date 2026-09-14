package database

import (
	"database/sql"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

const maxRetries = 5

// ReadModifyUpdate is a helper function to run a db transaction that reads some
// object(s), modifies some of the data, and writes the modified object(s) back
// to the database.  It is run in a transaction at RepeatableRead isolation so
// that if another database client also modifies the data we are writing and
// commits, then the transaction is rolled back and restarted.
//
// This is needed because we typically read all object columns, modify some
// subset, and then write all columns.  Consider an object with columns A, B and
// initial values A=1, B=1.  Two database clients work simultaneously, with one
// client attempting to set A=2, and another attempting to set B=2.  They both
// initially read A=1, B=1, and then one writes A=2, B=1, and the other writes
// A=1, B=2.  With default PostgreSQL isolation of ReadCommitted, both of these
// transactions would succeed and we end up with either A=2, B=1 or A=1, B=2.
// One or other client gets their transaction wiped out even though the data
// they wanted to change didn't conflict.
//
// If we run at RepeatableRead isolation, then one or other transaction will
// fail.  Let's say the transaction that sets A=2 succeeds.  Then the first B=2
// transaction fails, but here we retry.  The second attempt we read A=2, B=1,
// then write A=2, B=2 as desired, and this succeeds.
func ReadModifyUpdate(db Store, f func(tx Store) error,
) error {
	var err error
	for retries := 0; retries < maxRetries; retries++ {
		err = db.InTx(f, &TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		var pqe *pq.Error
		if xerrors.As(err, &pqe) {
			if pqe.Code == "40001" {
				// serialization error, retry
				continue
			}
		}
		return err
	}
	return xerrors.Errorf("too many errors; last error: %w", err)
}
