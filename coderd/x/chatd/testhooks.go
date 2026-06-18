package chatd

import "context"

// WaitUntilIdleForTest waits for background chat work tracked by the server to
// finish without shutting the server down. Tests use this to assert final
// database state only after asynchronous chat processing has completed.
// Close waits for the same tracked work, but also stops the server.
func WaitUntilIdleForTest(ctx context.Context, server *Server) error {
	if server.chatWorker != nil {
		if err := server.chatWorker.WaitIdle(ctx); err != nil {
			return err
		}
	}
	server.drainInflight()
	return nil
}
