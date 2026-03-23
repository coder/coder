package chatd

// WaitUntilIdleForTest waits for background chat work tracked by the server to
// finish. Tests use this to assert final database state only after asynchronous
// chat processing has completed.
func WaitUntilIdleForTest(server *Server) {
	server.inflight.Wait()
}
