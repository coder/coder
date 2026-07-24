package xnet_test

import (
	"context"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/xnet"
)

func TestIsTimeoutError(t *testing.T) {
	t.Parallel()

	require.False(t, xnet.IsTimeoutError(nil))
	require.False(t, xnet.IsTimeoutError(xerrors.New("other")))
	require.True(t, xnet.IsTimeoutError(context.DeadlineExceeded))
	require.True(t, xnet.IsTimeoutError(xerrors.Errorf("dial: %w", syscall.ETIMEDOUT)))
	require.True(t, xnet.IsTimeoutError(&net.OpError{Op: "dial", Err: syscall.ETIMEDOUT}))
}

func TestIsConnectionError(t *testing.T) {
	t.Parallel()

	require.False(t, xnet.IsConnectionError(nil))
	require.False(t, xnet.IsConnectionError(context.DeadlineExceeded))
	require.True(t, xnet.IsConnectionError(io.EOF))
	require.True(t, xnet.IsConnectionError(io.ErrUnexpectedEOF))
	require.True(t, xnet.IsConnectionError(xerrors.Errorf("write: %w", syscall.EPIPE)))
	require.True(t, xnet.IsConnectionError(&net.OpError{Op: "read", Err: syscall.ECONNRESET}))
}
