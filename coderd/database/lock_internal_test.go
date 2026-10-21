package database

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestWorkspaceQuotaLockID pins the namespace, UUID order, hash algorithm,
// and signed result so replicas running different versions share one lock.
func TestWorkspaceQuotaLockID(t *testing.T) {
	t.Parallel()

	ownerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	organizationID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	require.Equal(t, int64(-7417162456764879603), WorkspaceQuotaLockID(ownerID, organizationID))
}

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
		"LockIDChatCapacityAdmission":        LockIDChatCapacityAdmission,
		"LockIDNotifyUnpricedAIModels":       LockIDNotifyUnpricedAIModels,
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
