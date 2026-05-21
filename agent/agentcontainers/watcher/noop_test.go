package watcher_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/testutil"
)

func TestNoopWatcher(t *testing.T) {
	t.Parallel()

	// Create the noop watcher under test.
	wut := watcher.NewNoop()

	// Test adding/removing files (should have no effect).
	err := wut.Add("some-file.txt")
	assert.NoError(t, err, "noop watcher should not return error on Add")

	err = wut.Remove("some-file.txt")
	assert.NoError(t, err, "noop watcher should not return error on Remove")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a goroutine to wait for Next to return.
	errC := make(chan error, 1)
	go func() {
		_, err := wut.Next(ctx)
		errC <- err
	}()

	select {
	case <-errC:
		require.Fail(t, "want Next to block")
	default:
	}

	// Cancel the context and check that Next returns.
	cancel()

	select {
	case err := <-errC:
		assert.Error(t, err, "want Next error when context is canceled")
	case <-time.After(testutil.WaitShort):
		t.Fatal("want Next to return after context was canceled")
	}

	// Test Close.
	err = wut.Close()
	assert.NoError(t, err, "want no error on Close")
}

func TestNoopWatcher_CloseBeforeNext(t *testing.T) {
	t.Parallel()

	wut := watcher.NewNoop()

	err := wut.Close()
	require.NoError(t, err, "close watcher failed")

	ctx := context.Background()
	_, err = wut.Next(ctx)
	assert.Error(t, err, "want Next to return error when watcher is closed")
}
