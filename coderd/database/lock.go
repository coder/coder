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
	LockIDNotifyUnpricedAIModels
)

// Per-setting advisory lock IDs for the chat instruction settings. These
// derive from the exact site_configs key with GenLockID (FNV-1a 64) instead
// of the sequential LockID* block above, so writers of different settings
// never contend. All advisory lock IDs share one flat bigint keyspace, so
// collision with the block above or any other derived ID is possible in
// principle but vanishingly unlikely (about 2^-64 per pair); the derivations
// are registered here for discoverability.
var (
	LockIDChatInstructionSystemPrompt = GenLockID("agents_chat_system_prompt")
	LockIDChatInstructionPlanMode     = GenLockID("agents_chat_plan_mode_instructions")
)

// Trigger-side advisory locks, registered here for discoverability only.
// The per-user cap triggers serialize on transaction-scoped advisory locks
// derived in SQL, never from Go code. The per-user cap triggers on
// user_secrets and user_skills (migration 000590) take
//
//	pg_advisory_xact_lock(hashtextextended('user_secrets_cap:' || user_id, 0))
//	pg_advisory_xact_lock(hashtextextended('user_skills_cap:'  || user_id, 0))
//
// TestUserCapAdvisoryLocks pins these key prefixes against the live trigger
// function definitions so a rename in a future migration fails a test
// instead of leaving this registry silently stale. hashtextextended shares
// the same flat bigint keyspace as the IDs above; collision is vanishingly
// unlikely, not impossible.

// GenLockID generates a unique and consistent lock ID from a given string.
func GenLockID(name string) int64 {
	hash := fnv.New64()
	_, _ = hash.Write([]byte(name))
	// #nosec G115 - Safe conversion as FNV hash should be treated as random value and both uint64/int64 have the same range of unique values
	return int64(hash.Sum64())
}
