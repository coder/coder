package cliutil_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliutil"
)

func TestDiscardAfterClose(t *testing.T) {
	t.Parallel()
	exErr := errors.New("test")
	fwc := &fakeWriteCloser{err: exErr}
	uut := cliutil.DiscardAfterClose(fwc)

	n, err := uut.Write([]byte("one"))
	require.Equal(t, 3, n)
	require.NoError(t, err)

	n, err = uut.Write([]byte("two"))
	require.Equal(t, 3, n)
	require.NoError(t, err)

	err = uut.Close()
	require.Equal(t, exErr, err)

	n, err = uut.Write([]byte("three"))
	require.Equal(t, 5, n)
	require.NoError(t, err)

	require.Len(t, fwc.writes, 2)
	require.EqualValues(t, "one", fwc.writes[0])
	require.EqualValues(t, "two", fwc.writes[1])
}

type fakeWriteCloser struct {
	writes [][]byte
	closed bool
	err    error
}

func (f *fakeWriteCloser) Write(p []byte) (n int, err error) {
	q := make([]byte, len(p))
	copy(q, p)
	f.writes = append(f.writes, q)
	return len(p), nil
}

func (f *fakeWriteCloser) Close() error {
	f.closed = true
	return f.err
}
