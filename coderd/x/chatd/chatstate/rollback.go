package chatstate

import "github.com/coder/coder/v2/coderd/database"

// RolledBackError means the mutation callback failed before commit was attempted.
// A commit transport error deliberately does not carry this guarantee.
type RolledBackError struct{ Cause error }

func (e *RolledBackError) Error() string { return e.Cause.Error() }
func (e *RolledBackError) Unwrap() error { return e.Cause }

func mutationInTx(store database.Store, fn func(database.Store) error) error {
	var callbackErr error
	err := store.InTx(func(tx database.Store) error {
		callbackErr = fn(tx)
		return callbackErr
	}, nil)
	if err != nil && callbackErr != nil {
		return &RolledBackError{Cause: err}
	}
	return err
}
