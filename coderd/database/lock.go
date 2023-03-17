package database

// Well-known lock IDs for lock functions in the database. These should not
// change. If locks are deprecated, they should be kept to avoid reusing the
// same ID.
const (
	LockIDDeploymentSetup = iota + 1
)
