package database

import "hash/fnv"

// Well-known lock IDs for lock functions in the database. These should not
// change. If locks are deprecated, they should be kept in this list to avoid
// reusing the same ID.
const (
	// Keep the unused iota here so we don't need + 1 every time
	lockIDUnused = iota
	LockIDDeploymentSetup
	LockIDEnterpriseDeploymentSetup
)

// GenLockID generates a unique and consistent lock ID from a given string.
func GenLockID(name string) int64 {
	hash := fnv.New64()
	_, _ = hash.Write([]byte(name))
	return int64(hash.Sum64())
}
