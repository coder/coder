package database

import "context"

// NewInMemory returns an in-memory store of the database.
func NewInMemory() Store {
	return &memoryQuerier{}
}

type memoryQuerier struct{}

// InTx doesn't rollback data properly for in-memory yet.
func (q *memoryQuerier) InTx(ctx context.Context, fn func(Store) error) error {
	return fn(q)
}

func (q *memoryQuerier) ExampleQuery(ctx context.Context) error {
	return nil
}
