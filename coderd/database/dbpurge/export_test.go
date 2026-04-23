package dbpurge

// SetChatAutoArchiveBatchSizeForTest lets tests shrink the batch size
// used by AutoArchiveInactiveChats so they can drive multi-tick
// pagination without inserting thousands of rows. The returned
// restore function resets the original value and must be called via
// t.Cleanup.
//
// Defined in an _test.go file so the symbol is only compiled into
// the test binary.
func SetChatAutoArchiveBatchSizeForTest(n int32) (restore func()) {
	old := chatAutoArchiveBatchSize
	chatAutoArchiveBatchSize = n
	return func() { chatAutoArchiveBatchSize = old }
}
