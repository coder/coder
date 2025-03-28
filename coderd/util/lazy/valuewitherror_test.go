package lazy_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/lazy"
)

func TestLazyWithErrorOK(t *testing.T) {
	t.Parallel()

	l := lazy.NewWithError(func() (int, error) {
		return 1, nil
	})

	i, err := l.Load()
	require.NoError(t, err)
	require.Equal(t, 1, i)
}

func TestLazyWithErrorErr(t *testing.T) {
	t.Parallel()

	l := lazy.NewWithError(func() (int, error) {
		return 0, xerrors.New("oh no! everything that could went horribly wrong!")
	})

	i, err := l.Load()
	require.Error(t, err)
	require.Equal(t, 0, i)
}

func TestLazyWithErrorPointers(t *testing.T) {
	t.Parallel()

	a := 1
	l := lazy.NewWithError(func() (*int, error) {
		return &a, nil
	})

	b, err := l.Load()
	require.NoError(t, err)
	c, err := l.Load()
	require.NoError(t, err)

	*b += 1
	*c += 1
	require.Equal(t, 3, a)
}
