package objstore

import (
	"context"
	"io"
	"iter"
	"time"

	"golang.org/x/xerrors"
)

// Sentinel errors.
var (
	// ErrNotFound is returned when a Read or Delete targets a key
	// that does not exist.
	ErrNotFound = xerrors.New("object not found")

	// ErrClosed is returned when an operation is attempted on a
	// closed store.
	ErrClosed = xerrors.New("object store closed")
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	// Key is the object's key within its namespace.
	Key string
	// Size is the object's size in bytes. May be -1 if unknown.
	Size int64
	// LastModified is the time the object was last written.
	LastModified time.Time
}

// Store provides namespace-scoped CRUD operations on opaque binary
// objects. Namespaces are implicit string prefixes created on first
// write; they require no registration.
//
// Implementations must be safe for concurrent use.
type Store interface {
	// Read returns a reader for the object at namespace/key. The
	// caller MUST close the returned ReadCloser when done. Returns
	// ErrNotFound if the object does not exist.
	Read(ctx context.Context, namespace, key string) (io.ReadCloser, ObjectInfo, error)

	// Write stores data at namespace/key. Semantics are
	// unconditional put: last writer wins.
	Write(ctx context.Context, namespace, key string, data []byte) error

	// List returns an iterator over objects in the given namespace
	// whose keys start with prefix. Pass "" for prefix to list all
	// objects in the namespace.
	//
	// The iterator is lazy and fetches pages on demand. Context
	// cancellation is respected between page fetches.
	List(ctx context.Context, namespace, prefix string) iter.Seq2[ObjectInfo, error]

	// Delete removes the object at namespace/key. Returns
	// ErrNotFound if the object does not exist.
	Delete(ctx context.Context, namespace, key string) error

	// Close releases any resources held by the store.
	// Operations after Close return ErrClosed.
	io.Closer
}
