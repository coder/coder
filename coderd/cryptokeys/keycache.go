package cryptokeys

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

var ErrKeyNotFound = xerrors.New("key not found")

var ErrKeyInvalid = xerrors.New("key is invalid for use")

// Keycache provides an abstraction for fetching signing keys.
type Keycache interface {
	Signing(ctx context.Context) (codersdk.CryptoKey, error)
	Verifying(ctx context.Context, sequence int32) (codersdk.CryptoKey, error)
}
