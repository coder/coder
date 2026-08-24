package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// filePathDisabledDeployment returns a deployment that has file-based user
// secrets turned off, along with the raw store and the owner's user ID so
// tests can seed rows that predate the policy. The store is unwrapped on
// purpose: seeding a file-only secret is exactly what the API refuses.
func filePathDisabledDeployment(t *testing.T, auditor audit.Auditor) (*codersdk.Client, database.Store, uuid.UUID) {
	t.Helper()
	dv := coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
		dv.DisableUserSecretFilePath = true
	})
	db, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database:         db,
		Pubsub:           ps,
		DeploymentValues: dv,
		Auditor:          auditor,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	return client, db, owner.UserID
}

// seedLegacySecret inserts a secret directly so tests can build states the
// policy refuses to create over the API, such as a file-only target.
func seedLegacySecret(t *testing.T, db database.Store, userID uuid.UUID, name, envName, filePath string, enabled bool) database.UserSecret {
	t.Helper()
	return dbgen.UserSecret(t, db, database.UserSecret{
		UserID: userID,
		Name:   name,
	}, func(params *database.CreateUserSecretParams) {
		params.EnvName = envName
		params.FilePath = filePath
		params.Enabled = enabled
	})
}

func TestPostUserSecretFilePathDisabled(t *testing.T) {
	t.Parallel()

	t.Run("RejectsFilePath", func(t *testing.T) {
		t.Parallel()
		client, _, _ := filePathDisabledDeployment(t, nil)
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "file-secret",
			Value:    "value",
			EnvName:  "FILE_SECRET",
			FilePath: "/tmp/file-secret",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")
	})

	t.Run("RejectsFilePathOnDisabledSecret", func(t *testing.T) {
		t.Parallel()
		client, _, _ := filePathDisabledDeployment(t, nil)
		ctx := testutil.Context(t, testutil.WaitMedium)

		disabled := false
		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "file-secret-disabled",
			Value:    "value",
			FilePath: "/tmp/file-secret-disabled",
			Enabled:  &disabled,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")
	})

	t.Run("AllowsEnvOnlySecret", func(t *testing.T) {
		t.Parallel()
		client, _, _ := filePathDisabledDeployment(t, nil)
		ctx := testutil.Context(t, testutil.WaitMedium)

		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "env-secret",
			Value:   "value",
			EnvName: "ENV_SECRET",
		})
		require.NoError(t, err)
		assert.Equal(t, "ENV_SECRET", secret.EnvName)
		assert.Empty(t, secret.FilePath)
	})

	t.Run("PolicyOffStillAllowsFilePath", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "policy-off-file",
			Value:    "value",
			FilePath: "/tmp/policy-off-file",
		})
		require.NoError(t, err)
		assert.Equal(t, "/tmp/policy-off-file", secret.FilePath)
	})
}

func TestImportUserSecretsFilePathDisabled(t *testing.T) {
	t.Parallel()

	// Parsed secrets files cannot carry a file path today, so importing
	// under the policy must keep working unchanged.
	client, _, _ := filePathDisabledDeployment(t, nil)
	ctx := testutil.Context(t, testutil.WaitMedium)

	secrets, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: "ALPHA=a\nPATH=c\n",
	})
	require.NoError(t, err)
	require.Len(t, secrets, 2)
	assert.Equal(t, "ALPHA", secrets[0].EnvName)
	assert.Empty(t, secrets[0].FilePath)
	// Reserved keys import disabled and without any injection target.
	assert.Empty(t, secrets[1].EnvName)
	assert.False(t, secrets[1].Enabled)
}

func TestPatchUserSecretFilePathDisabled(t *testing.T) {
	t.Parallel()

	t.Run("UnrelatedEditOnLegacyFileOnlySecretAllowed", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-unrelated", "", "/tmp/legacy-unrelated", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		description := "updated description"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-unrelated", codersdk.UpdateUserSecretRequest{
			Description: &description,
		})
		require.NoError(t, err)
		assert.Equal(t, "updated description", updated.Description)
		// The legacy file path is preserved untouched.
		assert.Equal(t, "/tmp/legacy-unrelated", updated.FilePath)
		assert.True(t, updated.Enabled)
	})

	t.Run("UnchangedFilePathResubmitAllowed", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-resubmit", "", "/tmp/legacy-resubmit", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Re-sending the current path is neither an add nor a change.
		samePath := "/tmp/legacy-resubmit"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-resubmit", codersdk.UpdateUserSecretRequest{
			FilePath: &samePath,
		})
		require.NoError(t, err)
		assert.Equal(t, samePath, updated.FilePath)
	})

	t.Run("DisableLegacySecretAllowed", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-disable", "", "/tmp/legacy-disable", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		disabled := false
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-disable", codersdk.UpdateUserSecretRequest{
			Enabled: &disabled,
		})
		require.NoError(t, err)
		assert.False(t, updated.Enabled)
		assert.Equal(t, "/tmp/legacy-disable", updated.FilePath)
	})

	t.Run("DisableDoesNotClearStoredFilePath", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-keep-path", "", "/tmp/legacy-keep-path", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Disabling only flips the flag. The stored path survives so it
		// becomes effective again if the deployment turns the policy
		// back off.
		disabled := false
		_, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-keep-path", codersdk.UpdateUserSecretRequest{
			Enabled: &disabled,
		})
		require.NoError(t, err)

		stored, err := client.UserSecretByName(ctx, codersdk.Me, "legacy-keep-path")
		require.NoError(t, err)
		assert.False(t, stored.Enabled)
		assert.Equal(t, "/tmp/legacy-keep-path", stored.FilePath)

		// Only an explicit clear removes it.
		empty := ""
		cleared, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-keep-path", codersdk.UpdateUserSecretRequest{
			FilePath: &empty,
		})
		require.NoError(t, err)
		assert.Empty(t, cleared.FilePath)
	})

	t.Run("CleanupClearingFilePathAllowed", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-cleanup", "", "/tmp/legacy-cleanup", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Clearing the last target is allowed when the same PATCH also
		// disables the secret.
		empty := ""
		disabled := false
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-cleanup", codersdk.UpdateUserSecretRequest{
			FilePath: &empty,
			Enabled:  &disabled,
		})
		require.NoError(t, err)
		assert.Empty(t, updated.FilePath)
		assert.False(t, updated.Enabled)
	})

	t.Run("MigrateLegacySecretToEnvAllowed", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-migrate", "", "/tmp/legacy-migrate", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		empty := ""
		envName := "LEGACY_MIGRATE_ENV"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-migrate", codersdk.UpdateUserSecretRequest{
			EnvName:  &envName,
			FilePath: &empty,
		})
		require.NoError(t, err)
		assert.Equal(t, envName, updated.EnvName)
		assert.Empty(t, updated.FilePath)
		assert.True(t, updated.Enabled)
	})

	t.Run("AddingFilePathRejected", func(t *testing.T) {
		t.Parallel()
		client, _, _ := filePathDisabledDeployment(t, nil)
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "patch-add-file",
			Value:   "value",
			EnvName: "PATCH_ADD_FILE",
		})
		require.NoError(t, err)

		newPath := "/tmp/patch-add-file"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "patch-add-file", codersdk.UpdateUserSecretRequest{
			FilePath: &newPath,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")
	})

	t.Run("ChangingLegacyFilePathRejected", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-change", "", "/tmp/legacy-change", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		newPath := "/tmp/legacy-change-other"
		_, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-change", codersdk.UpdateUserSecretRequest{
			FilePath: &newPath,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")
	})

	t.Run("ClearingEnvNameOnFileTargetRejected", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-clear-env", "LEGACY_CLEAR_ENV", "/tmp/legacy-clear-env", true)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// The row keeps a file path, so the base injection-target
		// invariant still holds, but a file path is not an effective
		// target under the policy.
		empty := ""
		_, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-clear-env", codersdk.UpdateUserSecretRequest{
			EnvName: &empty,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "disabled")
	})

	t.Run("ReEnablingFileOnlySecretRejected", func(t *testing.T) {
		t.Parallel()
		client, db, userID := filePathDisabledDeployment(t, nil)
		seedLegacySecret(t, db, userID, "legacy-reenable", "", "/tmp/legacy-reenable", false)
		ctx := testutil.Context(t, testutil.WaitMedium)

		enabled := true
		_, err := client.UpdateUserSecret(ctx, codersdk.Me, "legacy-reenable", codersdk.UpdateUserSecretRequest{
			Enabled: &enabled,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "disabled")
	})

	t.Run("PolicyOffAllowsAddingFilePath", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "policy-off-patch",
			Value:   "value",
			EnvName: "POLICY_OFF_PATCH",
		})
		require.NoError(t, err)

		newPath := "/tmp/policy-off-patch"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "policy-off-patch", codersdk.UpdateUserSecretRequest{
			FilePath: &newPath,
		})
		require.NoError(t, err)
		assert.Equal(t, newPath, updated.FilePath)
	})
}

func TestPatchUserSecretFilePathDisabledAudit(t *testing.T) {
	t.Parallel()

	t.Run("RejectedPatchAuditsFailure", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client, _, _ := filePathDisabledDeployment(t, auditor)
		ctx := testutil.Context(t, testutil.WaitMedium)

		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "audit-reject",
			Value:   "value",
			EnvName: "AUDIT_REJECT",
		})
		require.NoError(t, err)
		auditor.ResetLogs()

		newPath := "/tmp/audit-reject"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "audit-reject", codersdk.UpdateUserSecretRequest{
			FilePath: &newPath,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")

		logs := auditor.AuditLogs()
		require.Len(t, logs, 1)
		assert.Equal(t, database.AuditActionWrite, logs[0].Action)
		assert.EqualValues(t, http.StatusBadRequest, logs[0].StatusCode)
		assert.Equal(t, secret.ID, logs[0].ResourceID)
		// Failed requests never carry a diff.
		assert.JSONEq(t, "{}", string(logs[0].Diff))
	})

	t.Run("AllowedPatchAuditsSuccess", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client, _, _ := filePathDisabledDeployment(t, auditor)
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "audit-allow",
			Value:   "value",
			EnvName: "AUDIT_ALLOW",
		})
		require.NoError(t, err)
		auditor.ResetLogs()

		description := "new description"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "audit-allow", codersdk.UpdateUserSecretRequest{
			Description: &description,
		})
		require.NoError(t, err)

		logs := auditor.AuditLogs()
		require.Len(t, logs, 1)
		assert.Equal(t, database.AuditActionWrite, logs[0].Action)
		assert.EqualValues(t, http.StatusOK, logs[0].StatusCode)
		assert.NotContains(t, string(logs[0].Diff), "file_path")
	})
}
