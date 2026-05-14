package coderd_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
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
			Name:  "dup-secret",
			Value: "value1",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "dup-secret",
			Value: "value2",
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
			Value: strings.Repeat("a", codersdk.MaxSecretValueSize+1),
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "value", "must not exceed")
	})
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
			Name:  "list-secret-a",
			Value: "value-a",
		})
		require.NoError(t, err)

		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "list-secret-b",
			Value: "value-b",
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
			Name:  "patch-nofields-secret",
			Value: "my-value",
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
			Name:  "conflict-env-2",
			Value: "value2",
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
			Name:  "conflict-fp-2",
			Value: "value2",
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
			Name:  "patch-invalid-env",
			Value: "good-value",
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
			Name:  "patch-invalid-file-path",
			Value: "good-value",
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
			Name:  "patch-invalid-val",
			Value: "good-value",
		})
		require.NoError(t, err)

		badVal := "before\x00after"
		_, err = client.UpdateUserSecret(ctx, codersdk.Me, "patch-invalid-val", codersdk.UpdateUserSecretRequest{
			Value: &badVal,
		})
		requireSecretValidationContainsError(t, err, http.StatusBadRequest, "value", "null bytes")
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

func TestDeleteUserSecret(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "delete-me-secret",
			Value: "my-value",
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

func TestWatchUserSecrets(t *testing.T) {
	t.Parallel()

	t.Run("ReceivesEvents", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		stream, err := client.WatchUserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		defer stream.Close(websocket.StatusNormalClosure)
		events := stream.Chan()

		name := strings.ReplaceAll(t.Name(), "/", "-")
		created, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:     name,
			Value:    "secret-value",
			EnvName:  "WATCH_SECRET",
			FilePath: "~/watch-secret",
		})
		require.NoError(t, err)

		event := testutil.RequireReceive(ctx, t, events)
		require.Equal(t, codersdk.UserSecretEventKindCreated, event.Kind)
		require.Equal(t, firstUser.UserID, event.UserID)
		require.Equal(t, created.Name, event.Name)
		require.Equal(t, created.EnvName, event.EnvName)
		require.Equal(t, created.FilePath, event.FilePath)

		description := "rotated"
		updated, err := client.UpdateUserSecret(ctx, codersdk.Me, name, codersdk.UpdateUserSecretRequest{
			Description: &description,
		})
		require.NoError(t, err)

		event = testutil.RequireReceive(ctx, t, events)
		require.Equal(t, codersdk.UserSecretEventKindUpdated, event.Kind)
		require.Equal(t, firstUser.UserID, event.UserID)
		require.Equal(t, updated.Name, event.Name)
		require.Equal(t, updated.EnvName, event.EnvName)
		require.Equal(t, updated.FilePath, event.FilePath)

		err = client.DeleteUserSecret(ctx, codersdk.Me, name)
		require.NoError(t, err)

		event = testutil.RequireReceive(ctx, t, events)
		require.Equal(t, codersdk.UserSecretEventKindDeleted, event.Kind)
		require.Equal(t, firstUser.UserID, event.UserID)
		require.Equal(t, name, event.Name)
		require.Empty(t, event.EnvName)
		require.Empty(t, event.FilePath)
	})

	t.Run("RejectsUnauthorizedUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		_, err := otherClient.WatchUserSecrets(ctx, firstUser.UserID.String())
		require.Error(t, err)
	})

	t.Run("OnlyReceivesWatchedUserEvents", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		stream, err := client.WatchUserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		defer stream.Close(websocket.StatusNormalClosure)
		events := stream.Chan()

		_, err = otherClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  strings.ReplaceAll(t.Name(), "/", "-") + "-other",
			Value: "secret-value",
		})
		require.NoError(t, err)

		ownName := strings.ReplaceAll(t.Name(), "/", "-") + "-own"
		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  ownName,
			Value: "secret-value",
		})
		require.NoError(t, err)

		event := testutil.RequireReceive(ctx, t, events)
		require.Equal(t, codersdk.UserSecretEventKindCreated, event.Kind)
		require.Equal(t, firstUser.UserID, event.UserID)
		require.Equal(t, ownName, event.Name)
	})
}
