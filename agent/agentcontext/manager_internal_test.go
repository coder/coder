package agentcontext

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestManager_RunOnce(t *testing.T) {
	t.Parallel()
	m := NewManager(ManagerOptions{
		Logger:     testutil.Logger(t).Named("agentcontext-test"),
		WorkingDir: func() string { return t.TempDir() },
	})
	t.Cleanup(func() { _ = m.Close() })
	m.SetReady()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	// Wait for Run to claim the running flag, then verify the
	// second call rejects with a deterministic error rather than
	// racing the scheduler.
	select {
	case <-m.started():
	case <-ctx.Done():
		t.Fatalf("manager never started: %v", ctx.Err())
	}

	err := m.Run(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than once")
	cancel()
	_ = m.Close()
}
