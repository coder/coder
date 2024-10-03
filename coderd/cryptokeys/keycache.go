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
	// EncryptingKey returns the latest valid key for encrypting payloads. A valid
	// key is one that is both past its start time and before its deletion time.
	EncryptingKey(ctx context.Context) (id string, key interface{}, err error)
	// DecryptingKey returns the key with the provided id which maps to its sequence
	// number. The key is valid for decryption as long as it is not deleted or past
	// its deletion date. We must allow for keys prior to their start time to
	// account for clock skew between peers (one key may be past its start time on
	// one machine while another is not).
	DecryptingKey(ctx context.Context, id string) (key interface{}, err error)
	io.Closer
}

type SigningKeycache interface {
	// SigningKey returns the latest valid key for signing. A valid key is one
	// that is both past its start time and before its deletion time.
	SigningKey(ctx context.Context) (id string, key interface{}, err error)
	// VerifyingKey returns the key with the provided id which should map to its
	// sequence number. The key is valid for verifying as long as it is not deleted
	// or past its deletion date. We must allow for keys prior to their start time
	// to account for clock skew between peers (one key may be past its start time
	// on one machine while another is not).
	VerifyingKey(ctx context.Context, id string) (key interface{}, err error)
	io.Closer
}
