package agent
import (
	"errors"
	"testing"
	"github.com/stretchr/testify/require"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)
func TestCheckpoint_CompleteWait(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	uut := newCheckpoint(logger)
	err := errors.New("test")
	uut.complete(err)
	got := uut.wait(ctx)
	require.Equal(t, err, got)
}
func TestCheckpoint_CompleteTwice(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctx := testutil.Context(t, testutil.WaitShort)
	uut := newCheckpoint(logger)
	err := errors.New("test")
	uut.complete(err)
	uut.complete(nil) // drops CRITICAL log
	got := uut.wait(ctx)
	require.Equal(t, err, got)
}
func TestCheckpoint_WaitComplete(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	uut := newCheckpoint(logger)
	err := errors.New("test")
	errCh := make(chan error, 1)
	go func() {
		errCh <- uut.wait(ctx)
	}()
	uut.complete(err)
	got := testutil.RequireRecvCtx(ctx, t, errCh)
	require.Equal(t, err, got)
}
