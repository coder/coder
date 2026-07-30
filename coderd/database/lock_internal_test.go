package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChatInstructionLockIDsDistinct proves the per-setting advisory lock IDs
// for the chat instruction settings cannot collide with each other or with
// any sequentially allocated LockID* constant. The constants are listed
// explicitly rather than enumerated programmatically (there is no registry of
// iota constants), so a future LockID* addition that collides fails here in
// review, not in a production deadlock.
func TestChatInstructionLockIDsDistinct(t *testing.T) {
	t.Parallel()

	generated := map[string]int64{
		"LockIDChatInstructionSystemPrompt": LockIDChatInstructionSystemPrompt,
		"LockIDChatInstructionPlanMode":     LockIDChatInstructionPlanMode,
	}

	sequential := map[string]int64{
		"LockIDDeploymentSetup":              LockIDDeploymentSetup,
		"LockIDEnterpriseDeploymentSetup":    LockIDEnterpriseDeploymentSetup,
		"LockIDDBRollup":                     LockIDDBRollup,
		"LockIDDBPurge":                      LockIDDBPurge,
		"LockIDNotificationsReportGenerator": LockIDNotificationsReportGenerator,
		"LockIDCryptoKeyRotation":            LockIDCryptoKeyRotation,
		"LockIDReconcilePrebuilds":           LockIDReconcilePrebuilds,
		"LockIDReconcileSystemRoles":         LockIDReconcileSystemRoles,
		"LockIDBoundaryUsageStats":           LockIDBoundaryUsageStats,
		"LockIDAIProvidersEnvSeed":           LockIDAIProvidersEnvSeed,
		"LockIDChatModelConfigWrites":        LockIDChatModelConfigWrites,
	}

	// The two generated IDs are pairwise distinct.
	require.NotEqual(t,
		LockIDChatInstructionSystemPrompt,
		LockIDChatInstructionPlanMode,
		"per-setting lock IDs must differ from each other")

	// Neither generated ID collides with any sequential constant.
	for name, id := range generated {
		for seqName, seqID := range sequential {
			require.NotEqualf(t, seqID, id,
				"%s (%d) collides with sequential constant %s", name, id, seqName)
		}
	}
}
