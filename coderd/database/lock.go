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
	// Deprecated: Reserved to prevent reuse. Do not use at runtime.
	LockIDChatModelConfigWrites
	LockIDChatCapacityAdmission
	LockIDAgentRuntimeHistoricalBackfill
)

// Per-setting advisory lock IDs for the chat instruction settings. These
// derive from the exact site_configs key with GenLockID (FNV-1a 64) instead
// of the sequential LockID* block above, so writers of different settings
// never contend and the IDs cannot collide with any sequentially allocated
// lock ID (different derivation space) or with another subsystem's
// GenLockID output (the key strings are unique to these settings).
var (
	LockIDChatInstructionSystemPrompt = GenLockID("agents_chat_system_prompt")
	LockIDChatInstructionPlanMode     = GenLockID("agents_chat_plan_mode_instructions")
)

// GenLockID generates a unique and consistent lock ID from a given string.
func GenLockID(name string) int64 {
	hash := fnv.New64()
	_, _ = hash.Write([]byte(name))
	// #nosec G115 - Safe conversion as FNV hash should be treated as random value and both uint64/int64 have the same range of unique values
	return int64(hash.Sum64())
}
