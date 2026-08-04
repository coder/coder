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
	LockIDReconcilePrebuilds
	LockIDReconcileSystemRoles
	LockIDBoundaryUsageStats
	LockIDAIProvidersEnvSeed
	LockIDChatModelConfigWrites
)

// Per-setting advisory lock IDs for the chat operational settings. These
// derive from the exact site_configs key with GenLockID (FNV-1a 64) instead
// of the sequential LockID* block above, so writers of different settings
// never contend and the IDs cannot collide with any sequentially allocated
// lock ID (different derivation space) or with another subsystem's
// GenLockID output (the key strings are unique to these settings).
var (
	LockIDChatOperationalChatRetentionDays             = GenLockID("agents_chat_retention_days")
	LockIDChatOperationalChatDebugRetentionDays        = GenLockID("agents_chat_debug_retention_days")
	LockIDChatOperationalChatAutoArchiveDays           = GenLockID("agents_chat_auto_archive_days")
	LockIDChatOperationalWorkspaceTTL                  = GenLockID("agents_workspace_ttl")
	LockIDChatOperationalComputerUseProvider           = GenLockID("agents_computer_use_provider")
	LockIDChatOperationalDebugLoggingAllowUsers        = GenLockID("agents_chat_debug_logging_allow_users")
	LockIDChatOperationalPersonalModelOverridesEnabled = GenLockID("agents_chat_personal_model_overrides_enabled")
)

// GenLockID generates a unique and consistent lock ID from a given string.
func GenLockID(name string) int64 {
	hash := fnv.New64()
	_, _ = hash.Write([]byte(name))
	// #nosec G115 - Safe conversion as FNV hash should be treated as random value and both uint64/int64 have the same range of unique values
	return int64(hash.Sum64())
}
