package database

import "hash/fnv"

// Well-known lock IDs for lock functions in the database. These should not
// change. If locks are deprecated, they should be kept in this list to avoid
// reusing the same ID.
const (
	LockIDDeploymentSetup = iota + 1
	LockIDEnterpriseDeploymentSetup
	LockIDDBRollup
	LockIDDBPurge
	LockIDNotificationsReportGenerator
	LockIDCryptoKeyRotation
	LockIDReconcileTemplatePrebuilds
	LockIDDeterminePrebuildsState
)

// GenLockID generates a unique and consistent lock ID from a given string.
func GenLockID(name string) int64 {
	hash := fnv.New64()
	_, _ = hash.Write([]byte(name))
	// #nosec G115 - Safe conversion as FNV hash should be treated as random value and both uint64/int64 have the same range of unique values
	return int64(hash.Sum64())
}
