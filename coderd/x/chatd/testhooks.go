package chatd

import (
	"context"
	"time"

	"golang.org/x/xerrors"
)

// WaitUntilIdleForTest waits for background chat work tracked by the server to
// finish without shutting the server down. Tests use this to assert final
// database state only after asynchronous chat processing has completed.
// Close waits for the same tracked work, but also stops the server. It returns
// an error if ctx ends first.
func WaitUntilIdleForTest(ctx context.Context, server *Server) error {
	idle := func() bool {
		if server.chatWorker == nil {
			return true
		}
		server.chatWorker.mu.Lock()
		manager := server.chatWorker.manager
		server.chatWorker.mu.Unlock()
		return manager == nil || manager.idle()
	}
	// A real ticker rather than the server clock, because tests often run the
	// server on a mock clock that never fires on its own.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !idle() {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return xerrors.Errorf("wait for the chat worker to go idle: %w", ctx.Err())
		}
	}
	drained := make(chan struct{})
	go func() {
		server.drainInflight()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return xerrors.Errorf("wait for tracked chat work to finish: %w", ctx.Err())
	}
}
