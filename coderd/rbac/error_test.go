package rbac

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestIsUnauthorizedError(t *testing.T) {
	t.Parallel()
	t.Run("NotWrapped", func(t *testing.T) {
		t.Parallel()
		errFunc := func() error {
			return UnauthorizedError{}
		}

		err := errFunc()
		require.True(t, IsUnauthorizedError(err))
	})

	t.Run("Wrapped", func(t *testing.T) {
		t.Parallel()
		errFunc := func() error {
			return xerrors.Errorf("test error: %w", UnauthorizedError{})
		}
		err := errFunc()
		require.True(t, IsUnauthorizedError(err))
	})
}
