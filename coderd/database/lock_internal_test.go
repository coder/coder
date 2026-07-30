package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The chat operational settings advisory locks derive from GenLockID
// (FNV-1a 64 of the site_configs key) rather than the sequential LockID*
// block. This test proves they are pairwise distinct and distinct from every
// enumerated sequential constant. The sequential constants are not
// programmatically enumerable, so the list is explicit: when a new LockID*
// constant is added to lock.go, add it here so the collision check covers it
// (review catches the addition, not a production deadlock).
func TestChatOperationalLockIDsDistinct(t *testing.T) {
	t.Parallel()

	sequential := []int64{
		LockIDDeploymentSetup,
		LockIDEnterpriseDeploymentSetup,
		LockIDDBRollup,
		LockIDDBPurge,
		LockIDNotificationsReportGenerator,
		LockIDCryptoKeyRotation,
		LockIDReconcilePrebuilds,
		LockIDReconcileSystemRoles,
		LockIDBoundaryUsageStats,
		LockIDAIProvidersEnvSeed,
		LockIDChatModelConfigWrites,
	}
	operational := []int64{
		LockIDChatOperationalChatRetentionDays,
		LockIDChatOperationalChatDebugRetentionDays,
		LockIDChatOperationalChatAutoArchiveDays,
		LockIDChatOperationalWorkspaceTTL,
		LockIDChatOperationalComputerUseProvider,
		LockIDChatOperationalDebugLoggingAllowUsers,
		LockIDChatOperationalPersonalModelOverridesEnabled,
	}

	seen := map[int64]string{}
	for _, id := range sequential {
		require.NotContains(t, seen, id, "duplicate sequential lock ID")
		seen[id] = "sequential"
	}
	for _, id := range operational {
		require.NotContains(t, seen, id, "chat operational lock ID collides with an existing lock ID")
		seen[id] = "operational"
	}
	require.Len(t, seen, len(sequential)+len(operational))
}
