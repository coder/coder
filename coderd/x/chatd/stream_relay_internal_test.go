package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type fakeDialError struct {
	unrecoverable bool
}

func (fakeDialError) Error() string           { return "fake dial error" }
func (e fakeDialError) IsUnrecoverable() bool { return e.unrecoverable }

func TestStreamPartsDialUnrecoverable(t *testing.T) {
	t.Parallel()

	require.False(t, streamPartsDialUnrecoverable(nil))
	require.False(t, streamPartsDialUnrecoverable(xerrors.New("plain error")))
	require.False(t, streamPartsDialUnrecoverable(fakeDialError{unrecoverable: false}))
	require.True(t, streamPartsDialUnrecoverable(fakeDialError{unrecoverable: true}))
	require.True(t, streamPartsDialUnrecoverable(xerrors.Errorf("wrapped: %w", fakeDialError{unrecoverable: true})))
}
