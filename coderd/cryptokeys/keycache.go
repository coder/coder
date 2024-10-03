package cryptokeys

import (
	"context"
	"io"

	"golang.org/x/xerrors"
)

var (
	ErrKeyNotFound    = xerrors.New("key not found")
	ErrKeyInvalid     = xerrors.New("key is invalid for use")
	ErrClosed         = xerrors.New("closed")
	ErrInvalidFeature = xerrors.New("invalid feature for this operation")
)

type EncryptionKeycache interface {
	EncryptingKey(ctx context.Context) (id string, key interface{}, err error)
	DecryptingKey(ctx context.Context, id string) (key interface{}, err error)
	io.Closer
}

type SigningKeycache interface {
	SigningKey(ctx context.Context) (id string, key interface{}, err error)
	VerifyingKey(ctx context.Context, id string) (key interface{}, err error)
	io.Closer
}
