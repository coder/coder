package coderd_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostUserSecret(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "github-token",
			Value:       "ghp_xxxxxxxxxxxx",
			Description: "Personal GitHub PAT",
			EnvName:     "GITHUB_TOKEN",
			FilePath:    "~/.github-token",
		})
		require.NoError(t, err)
		assert.Equal(t, "github-token", secret.Name)
		assert.Equal(t, "Personal GitHub PAT", secret.Description)
		assert.Equal(t, "GITHUB_TOKEN", secret.EnvName)
		assert.Equal(t, "~/.github-token", secret.FilePath)
		assert.NotZero(t, secret.ID)
		assert.NotZero(t, secret.CreatedAt)
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Value: "some-value",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "name", "required")
	})

	t.Run("MissingValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name: "missing-value-secret",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "value", "required")
	})

	t.Run("InvalidName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "foo/bar",
			Value: "some-value",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "name", "must not contain")
	})

	t.Run("WhitespaceName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  " github",
			Value: "some-value",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "name", "whitespace")
	})

	t.Run("DuplicateName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "dup-secret",
			Value:   "value1",
			EnvName: "DUP_SECRET_ENV_1",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "dup-secret",
			Value:   "value2",
			EnvName: "DUP_SECRET_ENV_2",
		})
		requireSecretValidationEqualsError(t, err, http.StatusConflict, "name", "name already in use")
	})

	t.Run("DuplicateEnvName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "env-dup-1",
			Value:   "value1",
			EnvName: "DUPLICATE_ENV",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "env-dup-2",
			Value:   "value2",
			EnvName: "DUPLICATE_ENV",
		})
		requireSecretValidationEqualsError(t, err, http.StatusConflict, "env_name", "environment variable already in use")
	})

	t.Run("DuplicateFilePath", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "fp-dup-1",
			Value:    "value1",
			FilePath: "/tmp/dup-file",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "fp-dup-2",
			Value:    "value2",
			FilePath: "/tmp/dup-file",
		})
		requireSecretValidationEqualsError(t, err, http.StatusConflict, "file_path", "file path already in use")
	})

	t.Run("InvalidEnvName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "invalid-env-secret",
			Value:   "value",
			EnvName: "1INVALID",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "must start")
	})

	t.Run("ReservedEnvName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "reserved-env-secret",
			Value:   "value",
			EnvName: "PATH",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "reserved")
	})

	t.Run("CoderPrefixEnvName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "coder-prefix-secret",
			Value:   "value",
			EnvName: "CODER_AGENT_TOKEN",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "CODER_")
	})

	t.Run("InvalidFilePath", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "bad-path-secret",
			Value:    "value",
			FilePath: "relative/path",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "must start")
	})

	t.Run("NullByteInValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "null-byte-secret",
			Value: "before\x00after",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "value", "null bytes")
	})

	t.Run("OversizedValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "oversized-secret",
			Value: strings.Repeat("a", codersdk.MaxUserSecretValueBytes+1),
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "value", "must not exceed")
	})

	t.Run("MissingInjectionTarget", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "missing-target-secret",
			Value: "value",
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "at least one of env_name or file_path")
	})

	t.Run("DisabledByDefault", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		disabled := false
		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "create-disabled",
			Value:   "value",
			EnvName: "CREATE_DISABLED",
			Enabled: &disabled,
		})
		require.NoError(t, err)
		assert.False(t, secret.Enabled)
	})

	t.Run("DisabledWithoutTarget", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		// A disabled secret may omit both injection targets. Bulk
		// imports rely on this for keys that cannot be env-injected.
		disabled := false
		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "create-disabled-no-target",
			Value:   "value",
			Enabled: &disabled,
		})
		require.NoError(t, err)
		assert.False(t, secret.Enabled)
		assert.Empty(t, secret.EnvName)
		assert.Empty(t, secret.FilePath)
	})

	t.Run("EnabledByDefault", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "create-default-enabled",
			Value:   "value",
			EnvName: "CREATE_DEFAULT_ENABLED",
		})
		require.NoError(t, err)
		assert.True(t, secret.Enabled)
	})
}

func TestPostUserSecretForbiddenForAnotherUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := memberClient.CreateUserSecret(ctx, owner.UserID.String(), codersdk.CreateUserSecretRequest{
		Name:    "forbidden",
		Value:   "value",
		EnvName: "FORBIDDEN",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}

func TestGetUserSecrets(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// Verify no secrets exist on a fresh user.
	ctx := testutil.Context(t, testutil.WaitMedium)
	secrets, err := client.UserSecrets(ctx, codersdk.Me)
	require.NoError(t, err)
	assert.Empty(t, secrets)

	t.Run("WithSecrets", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "list-secret-a",
			Value:   "value-a",
			EnvName: "LIST_SECRET_A",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "list-secret-b",
			Value:   "value-b",
			EnvName: "LIST_SECRET_B",
		})
		require.NoError(t, err)

		secrets, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, secrets, 2)
		// Sorted by name.
		assert.Equal(t, "list-secret-a", secrets[0].Name)
		assert.Equal(t, "list-secret-b", secrets[1].Name)
	})
}

func TestGetUserSecret(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		created, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "get-found-secret",
			Value:   "my-value",
			EnvName: "GET_FOUND_SECRET",
		})
		require.NoError(t, err)

		got, err := client.UserSecretByName(ctx, codersdk.Me, "get-found-secret")
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, "get-found-secret", got.Name)
		assert.Equal(t, "GET_FOUND_SECRET", got.EnvName)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.UserSecretByName(ctx, codersdk.Me, "nonexistent")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestPatchUserSecret(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("UpdateDescription", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "patch-desc-secret",
			Value:       "my-value",
			Description: "original",
			EnvName:     "PATCH_DESC_ENV",
		})
		require.NoError(t, err)

		newDesc := "updated"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "patch-desc-secret", codersdk.UpdateUserSecretRequest{
			Description: &newDesc,
		})
		require.NoError(t, err)
		assert.Equal(t, "updated", updated.Description)
		// Other fields unchanged.
		assert.Equal(t, "PATCH_DESC_ENV", updated.EnvName)
	})

	t.Run("NoFields", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "patch-nofields-secret",
			Value:   "my-value",
			EnvName: "PATCH_NOFIELDS_ENV",
		})
		require.NoError(t, err)

		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "patch-nofields-secret", codersdk.UpdateUserSecretRequest{})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		newVal := "new-value"
		_, err := client.UpdateUserSecret(ctx, codersdk.Me, "nonexistent", codersdk.UpdateUserSecretRequest{
			Value: &newVal,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("ConflictEnvName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "conflict-env-1",
			Value:   "value1",
			EnvName: "CONFLICT_TAKEN_ENV",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "conflict-env-2",
			Value:    "value2",
			FilePath: "/tmp/conflict-env-2",
		})
		require.NoError(t, err)

		taken := "CONFLICT_TAKEN_ENV"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "conflict-env-2", codersdk.UpdateUserSecretRequest{
			EnvName: &taken,
		})
		requireSecretValidationEqualsError(t, err, http.StatusConflict, "env_name", "environment variable already in use")
	})

	t.Run("ConflictFilePath", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "conflict-fp-1",
			Value:    "value1",
			FilePath: "/tmp/conflict-taken",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "conflict-fp-2",
			Value:   "value2",
			EnvName: "CONFLICT_FP_2",
		})
		require.NoError(t, err)

		taken := "/tmp/conflict-taken"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "conflict-fp-2", codersdk.UpdateUserSecretRequest{
			FilePath: &taken,
		})
		requireSecretValidationEqualsError(t, err, http.StatusConflict, "file_path", "file path already in use")
	})

	t.Run("InvalidEnvName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "patch-invalid-env",
			Value:    "good-value",
			FilePath: "/tmp/patch-invalid-env",
		})
		require.NoError(t, err)

		badEnvName := "1INVALID"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "patch-invalid-env", codersdk.UpdateUserSecretRequest{
			EnvName: &badEnvName,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "must start")
	})

	t.Run("InvalidFilePath", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "patch-invalid-file-path",
			Value:   "good-value",
			EnvName: "PATCH_INVALID_FILE_PATH",
		})
		require.NoError(t, err)

		badFilePath := "relative/path"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "patch-invalid-file-path", codersdk.UpdateUserSecretRequest{
			FilePath: &badFilePath,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "must start")
	})

	t.Run("InvalidValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "patch-invalid-val",
			Value:   "good-value",
			EnvName: "PATCH_INVALID_VAL",
		})
		require.NoError(t, err)

		badVal := "before\x00after"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "patch-invalid-val", codersdk.UpdateUserSecretRequest{
			Value: &badVal,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "value", "null bytes")
	})

	t.Run("ToggleEnabled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		secret, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "toggle-enabled",
			Value:   "value",
			EnvName: "TOGGLE_ENABLED",
		})
		require.NoError(t, err)
		require.True(t, secret.Enabled)

		disable := false
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "toggle-enabled", codersdk.UpdateUserSecretRequest{
			Enabled: &disable,
		})
		require.NoError(t, err)
		assert.False(t, updated.Enabled)
		// Other fields should be unchanged.
		assert.Equal(t, "TOGGLE_ENABLED", updated.EnvName)

		enable := true
		updated, err = client.UpdateUserSecret(ctx, codersdk.Me, "toggle-enabled", codersdk.UpdateUserSecretRequest{
			Enabled: &enable,
		})
		require.NoError(t, err)
		assert.True(t, updated.Enabled)
	})

	t.Run("ClearingBothTargetsRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "clear-both",
			Value:   "value",
			EnvName: "CLEAR_BOTH_ENV",
		})
		require.NoError(t, err)

		// PATCH that clears env_name while file_path is also empty
		// should be rejected: the row stays enabled but would have no
		// injection target.
		empty := ""
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "clear-both", codersdk.UpdateUserSecretRequest{
			EnvName: &empty,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "at least one of env_name or file_path")
	})

	t.Run("ClearingTargetsWhileDisablingAllowed", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "clear-and-disable",
			Value:   "value",
			EnvName: "CLEAR_AND_DISABLE",
		})
		require.NoError(t, err)

		// Clearing the last target is allowed when the same PATCH also
		// disables the secret: only enabled secrets need a target.
		empty := ""
		disabled := false
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "clear-and-disable", codersdk.UpdateUserSecretRequest{
			EnvName: &empty,
			Enabled: &disabled,
		})
		require.NoError(t, err)
		assert.False(t, updated.Enabled)
		assert.Empty(t, updated.EnvName)

		// Re-enabling without restoring a target is rejected.
		enable := true
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "clear-and-disable", codersdk.UpdateUserSecretRequest{
			Enabled: &enable,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "at least one of env_name or file_path")

		// Re-enabling and restoring a target in one PATCH succeeds.
		envName := "CLEAR_AND_DISABLE"
		updated, err = client.UpdateUserSecret(ctx, codersdk.Me, "clear-and-disable", codersdk.UpdateUserSecretRequest{
			EnvName: &envName,
			Enabled: &enable,
		})
		require.NoError(t, err)
		assert.True(t, updated.Enabled)
		assert.Equal(t, "CLEAR_AND_DISABLE", updated.EnvName)
	})

	t.Run("AtomicEnvFileSwap", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "atomic-swap",
			Value:   "value",
			EnvName: "ATOMIC_SWAP_ENV",
		})
		require.NoError(t, err)

		// Clearing env_name and setting file_path in the same PATCH must
		// succeed: the post-update row still has an injection target.
		empty := ""
		newPath := "/tmp/atomic-swap"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, "atomic-swap", codersdk.UpdateUserSecretRequest{
			EnvName:  &empty,
			FilePath: &newPath,
		})
		require.NoError(t, err)
		assert.Equal(t, "", updated.EnvName)
		assert.Equal(t, "/tmp/atomic-swap", updated.FilePath)
	})
}

func requireSecretValidationContainsError(t *testing.T, err error, status int, field string, detailContains string) {
	t.Helper()
	validation := requireSecretValidation(t, err, status, field)
	assert.Contains(t, validation.Detail, detailContains)
}

func requireSecretValidationEqualsError(t *testing.T, err error, status int, field string, detail string) {
	t.Helper()
	validation := requireSecretValidation(t, err, status, field)
	assert.Equal(t, detail, validation.Detail)
}

func requireSecretValidation(t *testing.T, err error, status int, field string) codersdk.ValidationError {
	t.Helper()

	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	assert.Equal(t, status, sdkErr.StatusCode())
	for _, validation := range sdkErr.Validations {
		if validation.Field == field {
			return validation
		}
	}
	require.Failf(t, "missing validation", "field %q not found in %#v", field, sdkErr.Validations)
	return codersdk.ValidationError{}
}

// TestUserSecretLimits exercises the per-user count and byte caps
// enforced by enforce_user_secrets_per_user_limits across both POST
// (creating a new secret) and PATCH (updating an existing one).
// Each subtest spins up its own server so it can burn the budget
// without affecting other tests.
//
// Each subtest checks three things per cap:
//
//   - POST past the cap is rejected with a 400.
//   - PATCH of an existing row at the cap is accepted; the trigger
//     uses FILTER (WHERE id IS DISTINCT FROM NEW.id) so an UPDATE
//     does not double-count its own row.
//   - A different user's budget is independent; the trigger groups
//     by user_id.
func TestUserSecretLimits(t *testing.T) {
	t.Parallel()

	t.Run("CountLimit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Fill the count budget exactly to the cap.
		var firstSecret codersdk.UserSecret
		for i := 0; i < codersdk.MaxUserSecretsPerUserCount; i++ {
			s, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     fmt.Sprintf("count-limit-%03d", i),
				Value:    "x",
				FilePath: fmt.Sprintf("/tmp/count-limit-%03d", i),
			})
			require.NoError(t, err)
			if i == 0 {
				firstSecret = s
			}
		}

		// POST: the 51st secret is rejected.
		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "one-too-many",
			Value:    "x",
			FilePath: "/tmp/one-too-many",
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "at most")

		// PATCH at the cap: changing the description must succeed.
		// Without the FILTER clause the trigger would re-count
		// firstSecret and reject this UPDATE.
		newDescription := "renamed"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, firstSecret.Name, codersdk.UpdateUserSecretRequest{
			Description: &newDescription,
		})
		require.NoError(t, err)

		// Other-user isolation: the second user's budget is independent.
		_, err = otherClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "other-user-secret",
			Value:    "x",
			FilePath: "/tmp/other-user-secret",
		})
		require.NoError(t, err)
	})

	t.Run("TotalBytesLimit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Pre-fill the total-bytes budget exactly to the cap using
		// max-sized file-only secrets (which don't count against env
		// bytes).
		big := strings.Repeat("a", codersdk.MaxUserSecretValueBytes)
		numBig := codersdk.MaxUserSecretsTotalValueBytes / codersdk.MaxUserSecretValueBytes
		remainder := codersdk.MaxUserSecretsTotalValueBytes % codersdk.MaxUserSecretValueBytes
		var firstSecret codersdk.UserSecret
		for i := 0; i < numBig; i++ {
			s, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     fmt.Sprintf("big-%03d", i),
				Value:    big,
				FilePath: fmt.Sprintf("/tmp/big-%03d", i),
			})
			require.NoError(t, err)
			if i == 0 {
				firstSecret = s
			}
		}
		if remainder > 0 {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     "big-pad",
				Value:    strings.Repeat("a", remainder),
				FilePath: "/tmp/big-pad",
			})
			require.NoError(t, err)
		}

		// POST: one more byte pushes past the total budget.
		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "overflow",
			Value:    "x",
			FilePath: "/tmp/overflow",
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "per-user budget")

		// PATCH at the cap: rewriting the existing row with a value
		// of the same size must succeed. The FILTER clause excludes
		// firstSecret's old bytes from the aggregate so the trigger
		// computes (cap - old) + new = cap, not cap + new.
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, firstSecret.Name, codersdk.UpdateUserSecretRequest{
			Value: &big,
		})
		require.NoError(t, err)

		// Other-user isolation: a fresh user can fill their own
		// total-bytes budget without interference.
		for i := 0; i < numBig; i++ {
			_, err := otherClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     fmt.Sprintf("other-big-%03d", i),
				Value:    big,
				FilePath: fmt.Sprintf("/tmp/other-big-%03d", i),
			})
			require.NoError(t, err)
		}
		if remainder > 0 {
			_, err := otherClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     "other-big-pad",
				Value:    strings.Repeat("a", remainder),
				FilePath: "/tmp/other-big-pad",
			})
			require.NoError(t, err)
		}
	})

	t.Run("EnvBytesLimit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// One env-injected secret consumes nearly the whole env budget.
		envBig, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "env-big",
			Value:   strings.Repeat("a", codersdk.MaxUserSecretValueBytes-16),
			EnvName: "ENV_BIG",
		})
		require.NoError(t, err)

		// POST: another env-injected secret pushes us over the env budget.
		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "env-overflow",
			Value:   strings.Repeat("a", 1024),
			EnvName: "ENV_OVERFLOW",
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "env_name")

		// A same-sized value used purely as a file is fine because
		// file_path secrets do not count against the env budget.
		fileOK, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     "file-ok",
			Value:    strings.Repeat("a", 1024),
			FilePath: "/tmp/file-ok",
		})
		require.NoError(t, err)

		// PATCH at the cap: updating envBig's description must
		// succeed. Without FILTER, the trigger would re-add envBig's
		// 24 KiB to itself and reject the UPDATE.
		newDescription := "renamed"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, envBig.Name, codersdk.UpdateUserSecretRequest{
			Description: &newDescription,
		})
		require.NoError(t, err)

		// PATCH a file_path secret to env mode: moves its 1 KiB into
		// the env budget, which already holds envBig's 24 KiB - 16.
		// new_env_bytes = 24560 + 1024 = 25584 > 24576, rejected.
		envName := "ENV_LATE"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, fileOK.Name, codersdk.UpdateUserSecretRequest{
			EnvName: &envName,
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "env_name")

		// Other-user isolation: a fresh user can create their own
		// near-cap env secret.
		_, err = otherClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "other-env-big",
			Value:   strings.Repeat("a", codersdk.MaxUserSecretValueBytes-16),
			EnvName: "OTHER_ENV_BIG",
		})
		require.NoError(t, err)
	})
}

// requireSecretAPIError asserts a non-validation user-facing error.
// Used for trigger-driven failures (per-user limits) whose responses
// are plain codersdk.Response without ValidationError entries.
func requireSecretAPIError(t *testing.T, err error, status int, detailContains string) {
	t.Helper()
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	assert.Equal(t, status, sdkErr.StatusCode())
	combined := sdkErr.Message + " " + sdkErr.Detail
	assert.Containsf(t, combined, detailContains,
		"expected response to contain %q; got Message=%q Detail=%q",
		detailContains, sdkErr.Message, sdkErr.Detail)
}

func TestDeleteUserSecret(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "delete-me-secret",
			Value:   "my-value",
			EnvName: "DELETE_ME_SECRET",
		})
		require.NoError(t, err)

		err = client.DeleteUserSecret(ctx, codersdk.Me, "delete-me-secret")
		require.NoError(t, err)

		// Verify it's gone.
		_, err = client.UserSecretByName(ctx, codersdk.Me, "delete-me-secret")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		err := client.DeleteUserSecret(ctx, codersdk.Me, "nonexistent")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
