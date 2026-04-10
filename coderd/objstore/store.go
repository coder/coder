package objstore

import (
	"context"
	"io"
	"iter"
	"path"
	"sync/atomic"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"golang.org/x/xerrors"
)

// bucketStore implements Store on top of a gocloud.dev/blob.Bucket.
// Namespaces are mapped to key prefixes separated by "/".
type bucketStore struct {
	bucket *blob.Bucket
	closed atomic.Bool
}

// New wraps a gocloud.dev/blob.Bucket as a Store. The caller
// retains no ownership of the bucket after this call; Close on
// the returned Store will close the underlying bucket.
func New(bucket *blob.Bucket) Store {
	return &bucketStore{bucket: bucket}
}

func (s *bucketStore) Read(ctx context.Context, namespace, key string) (io.ReadCloser, ObjectInfo, error) {
	if s.closed.Load() {
		return nil, ObjectInfo{}, ErrClosed
	}

	objKey := objectKey(namespace, key)

	// Fetch attributes first so we can populate ObjectInfo
	// before handing back the reader.
	attrs, err := s.bucket.Attributes(ctx, objKey)
	if err != nil {
		return nil, ObjectInfo{}, mapError(err, namespace, key)
	}

	reader, err := s.bucket.NewReader(ctx, objKey, nil)
	if err != nil {
		return nil, ObjectInfo{}, mapError(err, namespace, key)
	}

	info := ObjectInfo{
		Key:          key,
		Size:         attrs.Size,
		LastModified: attrs.ModTime,
	}
	return reader, info, nil
}

func (s *bucketStore) Write(ctx context.Context, namespace, key string, data []byte) error {
	if s.closed.Load() {
		return ErrClosed
	}

	return mapError(
		s.bucket.WriteAll(ctx, objectKey(namespace, key), data, nil),
		namespace, key,
	)
}

func (s *bucketStore) List(ctx context.Context, namespace, prefix string) iter.Seq2[ObjectInfo, error] {
	return func(yield func(ObjectInfo, error) bool) {
		if s.closed.Load() {
			yield(ObjectInfo{}, ErrClosed)
			return
		}

		fullPrefix := namespace + "/"
		if prefix != "" {
			fullPrefix += prefix
		}

		it := s.bucket.List(&blob.ListOptions{
			Prefix: fullPrefix,
		})

		for {
			obj, err := it.Next(ctx)
			if err != nil {
				if err == io.EOF {
					return
				}
				if !yield(ObjectInfo{}, xerrors.Errorf("list %q/%q: %w", namespace, prefix, err)) {
					return
				}
				return
			}
			if obj.IsDir {
				continue
			}

			// Strip namespace prefix from key to return
			// namespace-relative keys.
			relKey := obj.Key[len(namespace)+1:]

			info := ObjectInfo{
				Key:          relKey,
				Size:         obj.Size,
				LastModified: obj.ModTime,
			}
			if !yield(info, nil) {
				return
			}
		}
	}
}

func (s *bucketStore) Delete(ctx context.Context, namespace, key string) error {
	if s.closed.Load() {
		return ErrClosed
	}

	return mapError(
		s.bucket.Delete(ctx, objectKey(namespace, key)),
		namespace, key,
	)
}

func (s *bucketStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	return s.bucket.Close()
}

// objectKey builds the full key from namespace and key.
func objectKey(namespace, key string) string {
	return path.Join(namespace, key)
}

// mapError translates gocloud error codes into our sentinel
// errors.
func mapError(err error, namespace, key string) error {
	if err == nil {
		return nil
	}
	if gcerrors.Code(err) == gcerrors.NotFound {
		return xerrors.Errorf("%s/%s: %w", namespace, key, ErrNotFound)
	}
	return err
}

// Compile-time check.
var _ Store = (*bucketStore)(nil)
