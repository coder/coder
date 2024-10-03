package cryptokeys

import (
	"context"

	"golang.org/x/xerrors"
)

var (
	ErrKeyNotFound = xerrors.New("key not found")
	ErrKeyInvalid  = xerrors.New("key is invalid for use")
	ErrClosed      = xerrors.New("closed")
)

// Keycache provides an abstraction for fetching cryptographic keys used for signing or encrypting payloads.
type Keycache interface {
	SigningKeycache
	EncryptionKeycache
}

type EncryptionKeycache interface {
	EncryptingKey(ctx context.Context) (id string, key interface{}, err error)
	DecryptingKey(ctx context.Context, id string) (key interface{}, err error)
}

type SigningKeycache interface {
	SigningKey(ctx context.Context) (id string, key interface{}, err error)
	VerifyingKey(ctx context.Context, id string) (key interface{}, err error)
}
