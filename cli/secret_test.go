package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestSecretCreate(t *testing.T) {
	t.Parallel()

	t.Run("MissingValue", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "create", "api-key")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "secret value must be provided by exactly one of --value or non-interactive stdin (pipe or redirect)")
	})

	t.Run("MissingValueOnTTY", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "--force-tty", "secret", "create", "api-key")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "secret value must be provided with --value or stdin via pipe or redirect")
	})

	t.Run("SuccessWithValueFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(
			t,
			"secret",
			"create",
			"api-key",
			"--value", "super-secret-value",
			"--description", "API key for workspace tools",
			"--env", "API_KEY",
			"--file", "~/.api-key",
		)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "api-key")

		secret, err := client.UserSecretByName(ctx, codersdk.Me, "api-key")
		require.NoError(t, err)
		require.Equal(t, "api-key", secret.Name)
		require.Equal(t, "API key for workspace tools", secret.Description)
		require.Equal(t, "API_KEY", secret.EnvName)
		require.Equal(t, "~/.api-key", secret.FilePath)
	})

	t.Run("ValueFlagConflictsWithStdin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(
			t,
			"secret",
			"create",
			"api-key",
			"--value", "super-secret-value",
		)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("different-value")

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "secret value may be provided by only one source, got --value, stdin")
	})

	t.Run("SuccessWithStdin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(
			t,
			"secret",
			"create",
			"api-key",
			"--description", "API key for workspace tools",
			"--env", "API_KEY",
		)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("super-secret-value")

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "api-key")

		secret, err := client.UserSecretByName(ctx, codersdk.Me, "api-key")
		require.NoError(t, err)
		require.Equal(t, "api-key", secret.Name)
		require.Equal(t, "API key for workspace tools", secret.Description)
		require.Equal(t, "API_KEY", secret.EnvName)
	})

	t.Run("StdinTrailingNewlineWarnsAndPreservesValue", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, ownerClient)
		client, user := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		inv, root := clitest.New(
			t,
			"secret",
			"create",
			"api-key",
			"--description", "API key for workspace tools",
			"--env", "API_KEY",
		)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("super-secret-value\n")

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "api-key")
		require.Contains(t, output.Stderr(), "secret value from stdin ends with a trailing newline")

		secret, err := db.GetUserSecretByUserIDAndName(
			dbauthz.AsSystemRestricted(ctx),
			database.GetUserSecretByUserIDAndNameParams{
				UserID: user.ID,
				Name:   "api-key",
			},
		)
		require.NoError(t, err)
		require.Equal(t, "super-secret-value\n", secret.Value)
	})

	t.Run("EmptyStdinIsNotProvided", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "create", "api-key")
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("")

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "secret value must be provided by exactly one of --value or non-interactive stdin (pipe or redirect)")
	})
}

func TestSecretUpdate(t *testing.T) {
	t.Parallel()

	t.Run("ServerValidationError", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "my-secret",
			Value: "original-value",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "update", "my-secret")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "At least one field must be provided")
	})

	t.Run("AllowsClearingFields", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "my-secret",
			Value:       "original-value",
			Description: "original description",
			EnvName:     "MY_SECRET",
			FilePath:    "~/.my-secret",
		})
		require.NoError(t, err)

		inv, root := clitest.New(
			t,
			"secret",
			"update",
			"my-secret",
			"--value", "rotated-secret",
			"--description", "",
			"--env", "",
			"--file", "",
		)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "my-secret")

		secret, err := client.UserSecretByName(ctx, codersdk.Me, "my-secret")
		require.NoError(t, err)
		require.Equal(t, "", secret.Description)
		require.Equal(t, "", secret.EnvName)
		require.Equal(t, "", secret.FilePath)
	})

	t.Run("UpdatesValueFromEmptyFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "my-secret",
			Value: "original-value",
		})
		require.NoError(t, err)

		inv, root := clitest.New(
			t,
			"secret",
			"update",
			"my-secret",
			"--value", "",
		)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "my-secret")
	})

	t.Run("UpdatesValueFromStdin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "my-secret",
			Value: "original-value",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "update", "my-secret")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("rotated-secret")

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "my-secret")
	})

	t.Run("ValueFlagConflictsWithStdin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "my-secret",
			Value: "original-value",
		})
		require.NoError(t, err)

		inv, root := clitest.New(
			t,
			"secret",
			"update",
			"my-secret",
			"--value", "rotated-secret",
		)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("different-value")

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "secret value may be provided by only one source, got --value, stdin")
	})
}

func TestSecretList(t *testing.T) {
	t.Parallel()

	t.Run("TableOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "tool-config",
			Value:       "config-value",
			Description: "Tool configuration",
			FilePath:    "~/.config/tool/config.json",
		})
		require.NoError(t, err)
		_, err = client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "service-token",
			Value:       "service-token-value",
			Description: "Service access token",
			EnvName:     "SERVICE_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "list")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		out := output.Stdout()
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "CREATED")
		assert.Contains(t, out, "UPDATED")
		assert.Contains(t, out, "ENV")
		assert.Contains(t, out, "FILE")
		assert.Contains(t, out, "DESCRIPTION")
		assert.Contains(t, out, "service-token")
		assert.Contains(t, out, "SERVICE_TOKEN")
		assert.Contains(t, out, "tool-config")
		assert.Contains(t, out, "~/.config/tool/config.json")
	})

	t.Run("JSONOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		created, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "service-token",
			Value:       "service-token-value",
			Description: "Service access token",
			EnvName:     "SERVICE_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "list", "--output=json")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var got []codersdk.UserSecret
		require.NoError(t, json.Unmarshal([]byte(output.Stdout()), &got))
		require.Len(t, got, 1)
		require.Equal(t, created, got[0])
	})

	t.Run("SingleSecretTableOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "tool-config",
			Value:       "config-value",
			Description: "Tool configuration",
			FilePath:    "~/.config/tool/config.json",
		})
		require.NoError(t, err)
		_, err = client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "service-token",
			Value:       "service-token-value",
			Description: "Service access token",
			EnvName:     "SERVICE_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "list", "service-token")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		out := output.Stdout()
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "CREATED")
		assert.Contains(t, out, "UPDATED")
		assert.Contains(t, out, "ENV")
		assert.Contains(t, out, "FILE")
		assert.Contains(t, out, "DESCRIPTION")
		assert.Contains(t, out, "service-token")
		assert.Contains(t, out, "SERVICE_TOKEN")
		assert.NotContains(t, out, "tool-config")
		assert.NotContains(t, out, "~/.config/tool/config.json")
	})

	t.Run("SingleSecretJSONOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		created, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "service-token",
			Value:       "service-token-value",
			Description: "Service access token",
			EnvName:     "SERVICE_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "list", "service-token", "--output=json")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var got []codersdk.UserSecret
		require.NoError(t, json.Unmarshal([]byte(output.Stdout()), &got))
		require.Len(t, got, 1)
		require.Equal(t, created, got[0])
	})

	t.Run("EmptyState", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "list")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		assert.Contains(t, output.Stderr(), "No secrets found.")
	})
}

func TestSecretDelete(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "service-token",
			Value: "service-token-value",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "delete", "service-token")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		waiter := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Delete secret")
		pty.ExpectMatchContext(ctx, "service-token")
		pty.WriteLine("yes")
		pty.ExpectMatchContext(ctx, "Deleted secret")

		require.NoError(t, waiter.Wait())

		_, err = client.UserSecretByName(setupCtx, codersdk.Me, "service-token")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("YesSkipsPrompt", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "service-token",
			Value: "service-token-value",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "delete", "service-token", "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "Deleted secret")
		require.NotContains(t, output.Stdout(), "Delete secret")
		require.Empty(t, output.Stderr())

		_, err = client.UserSecretByName(setupCtx, codersdk.Me, "service-token")
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "delete", "missing-secret")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		waiter := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Delete secret")
		pty.ExpectMatchContext(ctx, "missing-secret")
		pty.WriteLine("yes")

		err := waiter.Wait()
		require.ErrorContains(t, err, `delete secret "missing-secret"`)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
