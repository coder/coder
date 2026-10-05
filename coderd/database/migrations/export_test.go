package migrations

// UpWithFSAndLogger exposes runUp so tests can run custom migrations with
// a capturing logger.
var UpWithFSAndLogger = runUp

// LockID is the advisory lock held while migrations run.
const LockID = lockID
