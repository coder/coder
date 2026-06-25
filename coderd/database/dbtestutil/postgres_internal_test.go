package dbtestutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// dialRefusedErr builds the *net.OpError chain that net.Dial, sql.Exec
// (via pq), and pubsub.New all produce on a refused TCP connection.
func dialRefusedErr(errno syscall.Errno) *net.OpError {
	return &net.OpError{
		Op:   "dial",
		Net:  "tcp",
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5432},
		Err:  &os.SyscallError{Syscall: "connect", Err: errno},
	}
}

func TestIsTransientConnectError(t *testing.T) {
	t.Parallel()

	// Iterate the platform slice so the fixture errno is real on every OS.
	t.Run("PositiveCases", func(t *testing.T) {
		t.Parallel()
		require.NotEmpty(t, transientConnectErrnos, "platform must declare at least one transient errno")
		for _, target := range transientConnectErrnos {
			var errno syscall.Errno
			require.Truef(t, errors.As(target, &errno), "transientConnectErrnos entry %v is not a syscall.Errno", target)
			label := fmt.Sprintf("errno=%d", uintptr(errno))

			t.Run(label+"/bare", func(t *testing.T) {
				t.Parallel()
				require.True(t, isTransientConnectError(errno))
			})
			t.Run(label+"/net_op_error", func(t *testing.T) {
				t.Parallel()
				require.True(t, isTransientConnectError(dialRefusedErr(errno)))
			})
			t.Run(label+"/xerrors_wrap", func(t *testing.T) {
				t.Parallel()
				wrapped := xerrors.Errorf("create pq listener: %w", dialRefusedErr(errno))
				require.True(t, isTransientConnectError(wrapped))
			})
		}
	})

	// Negative cases stay platform-agnostic.
	t.Run("NegativeCases", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			err  error
		}{
			{"nil", nil},
			{"unrelated_pq_error", xerrors.New("pq: relation \"users\" does not exist")},
			{"context_deadline", context.DeadlineExceeded},
			// String lookalike: documents that callers must preserve %w.
			{"bare_string_lookalike", xerrors.New("dial tcp 127.0.0.1:5432: connect: connection refused")},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				require.False(t, isTransientConnectError(c.err))
			})
		}
	})
}

func TestRetryTransientConnect(t *testing.T) {
	t.Parallel()

	// Use the platform's first errno so the fixture is real on every OS.
	require.NotEmpty(t, transientConnectErrnos)
	var transientErrno syscall.Errno
	require.True(t, errors.As(transientConnectErrnos[0], &transientErrno))

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var calls atomic.Int32
		err := retryTransientConnect(ctx, func() error {
			calls.Add(1)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("TransientThenSuccess", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var calls atomic.Int32
		transient := dialRefusedErr(transientErrno)
		err := retryTransientConnect(ctx, func() error {
			if calls.Add(1) < 3 {
				return transient
			}
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, int32(3), calls.Load())
	})

	t.Run("NonTransientReturnsImmediately", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var calls atomic.Int32
		fatal := xerrors.New("pq: syntax error")
		err := retryTransientConnect(ctx, func() error {
			calls.Add(1)
			return fatal
		})
		require.ErrorIs(t, err, fatal)
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("BudgetExhausted", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		var calls atomic.Int32
		transient := dialRefusedErr(transientErrno)
		err := retryTransientConnect(ctx, func() error {
			calls.Add(1)
			return transient
		})
		require.ErrorIs(t, err, transient)
		require.GreaterOrEqual(t, calls.Load(), int32(2), "expected at least one retry before the budget exhausted")
	})
}
