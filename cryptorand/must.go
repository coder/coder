package cryptorand

import "golang.org/x/xerrors"

// must is a utility function that panics with the given error if
// err is non-nil.
func must(err error) {
	if err != nil {
		panic(xerrors.Errorf("crand: %w", err))
	}
}
