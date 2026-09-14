package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
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
			Name:    "my-secret",
			Value:   "original-value",
			EnvName: "MY_SECRET",
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

		// Clearing env_name and description while leaving file_path
		// keeps the secret well-formed (still has an injection
		// target). Trying to clear both env_name and file_path is
		// covered by the server-side test below.
		inv, root := clitest.New(
			t,
			"secret",
			"update",
			"my-secret",
			"--value", "rotated-secret",
			"--description", "",
			"--env", "",
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
		require.Equal(t, "~/.my-secret", secret.FilePath)
	})

	t.Run("ClearingBothTargetsRejected", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "my-secret",
			Value:   "original-value",
			EnvName: "MY_SECRET",
		})
		require.NoError(t, err)

		inv, root := clitest.New(
			t,
			"secret",
			"update",
			"my-secret",
			"--env", "",
		)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "env_name", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "at least one of env_name or file_path")
	})

	t.Run("UpdatesValueFromEmptyFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "my-secret",
			Value:   "original-value",
			EnvName: "MY_SECRET",
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
			Name:    "my-secret",
			Value:   "original-value",
			EnvName: "MY_SECRET",
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
			Name:    "my-secret",
			Value:   "original-value",
			EnvName: "MY_SECRET",
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

		logger := testutil.Logger(t)
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "service-token",
			Value:   "service-token-value",
			EnvName: "SERVICE_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "delete", "service-token")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)
		waiter := clitest.StartWithWaiter(t, inv)
		stdout.ExpectMatch(ctx, "Delete secret")
		stdout.ExpectMatch(ctx, "service-token")
		stdin.WriteLine("yes")
		stdout.ExpectMatch(ctx, "Deleted secret")

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
			Name:    "service-token",
			Value:   "service-token-value",
			EnvName: "SERVICE_TOKEN",
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

		logger := testutil.Logger(t)
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "delete", "missing-secret")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)
		waiter := clitest.StartWithWaiter(t, inv)
		stdout.ExpectMatch(ctx, "Delete secret")
		stdout.ExpectMatch(ctx, "missing-secret")
		stdin.WriteLine("yes")

		err := waiter.Wait()
		require.ErrorContains(t, err, `delete secret "missing-secret"`)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestSecretImport(t *testing.T) {
	t.Parallel()

	writeSecretsFile := func(t *testing.T, name, content string) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	t.Run("InfersFormatFromExtension", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			file    string
			content string
		}{
			{name: "Env", file: "secrets.env", content: "ALPHA=a\nBETA=b\n"},
			{name: "JSON", file: "secrets.json", content: `{"ALPHA":"a","BETA":"b"}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, nil)
				_ = coderdtest.CreateFirstUser(t, client)

				inv, root := clitest.New(t, "secret", "import", writeSecretsFile(t, tt.file, tt.content))
				output := clitest.Capture(inv)
				clitest.SetupConfig(t, client, root)

				ctx := testutil.Context(t, testutil.WaitMedium)
				require.NoError(t, inv.WithContext(ctx).Run())
				require.Contains(t, output.Stdout(), "Imported 2 secrets.")
				require.NotContains(t, output.Stderr(), "without an environment variable name")

				secret, err := client.UserSecretByName(ctx, codersdk.Me, "ALPHA")
				require.NoError(t, err)
				require.Equal(t, "ALPHA", secret.EnvName)
			})
		}
	})

	// The flag wins over the extension, and its value is matched
	// case-insensitively.
	t.Run("InputFormatOverridesExtension", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		path := writeSecretsFile(t, "secrets.json", "ALPHA=a\n")
		inv, root := clitest.New(t, "secret", "import", path, "--input-format", "ENV")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		require.NoError(t, inv.WithContext(ctx).Run())
		require.Contains(t, output.Stdout(), "Imported 1 secret.")

		_, err := client.UserSecretByName(ctx, codersdk.Me, "ALPHA")
		require.NoError(t, err)
	})

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "import", "-", "--input-format", "env")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("ALPHA=a\n")

		ctx := testutil.Context(t, testutil.WaitMedium)
		require.NoError(t, inv.WithContext(ctx).Run())
		require.Contains(t, output.Stdout(), "Imported 1 secret.")

		_, err := client.UserSecretByName(ctx, codersdk.Me, "ALPHA")
		require.NoError(t, err)
	})

	t.Run("UnknownExtension", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		path := writeSecretsFile(t, "secrets.txt", "ALPHA=a\n")
		inv, root := clitest.New(t, "secret", "import", path)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "set --input-format to one of: env, json, yaml")
	})

	t.Run("RejectsLocalErrorsBeforeSending", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			content []byte
			wantErr string
		}{
			{name: "Malformed", content: []byte("ALPHA=a\nNOEQUALS\n"), wantErr: "line 2: expected KEY=VALUE"},
			{name: "InvalidUTF8", content: []byte{'A', 'L', 'P', 'H', 'A', '=', 0xff}, wantErr: "must contain valid UTF-8"},
			{name: "InvalidEntry", content: []byte("ALPHA=a\nBETA=\n"), wantErr: `secret 2 ("BETA") value: Value is required.`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()

				client := codersdk.New(must(url.Parse(server.URL)))
				client.SetSessionToken("test-token")
				path := filepath.Join(t.TempDir(), "secrets.env")
				require.NoError(t, os.WriteFile(path, tt.content, 0o600))
				inv, root := clitest.New(t, "secret", "import", path)
				clitest.SetupConfig(t, client, root)

				err := inv.Run()
				require.ErrorContains(t, err, tt.wantErr)
				require.Zero(t, requests.Load(), "expected no API request")
			})
		}
	})

	t.Run("RejectsTTYStdin", func(t *testing.T) {
		t.Parallel()

		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := codersdk.New(must(url.Parse(server.URL)))
		client.SetSessionToken("test-token")
		inv, root := clitest.New(t, "--force-tty", "secret", "import", "-", "--input-format", "env")
		clitest.SetupConfig(t, client, root)

		err := inv.Run()
		require.ErrorContains(t, err, "secrets file must be provided via non-interactive stdin (pipe or redirect)")
		require.Zero(t, requests.Load(), "expected no API request")
	})

	t.Run("WarnsAboutKeysWithoutEnvNames", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// PATH is reserved and MY-TOKEN is not a valid identifier, so both are
		// imported without an env name.
		require.Error(t, codersdk.UserSecretEnvNameValid("PATH"))
		path := writeSecretsFile(t, "secrets.env", "ALPHA=a\nPATH=b\nMY-TOKEN=c\n")
		inv, root := clitest.New(t, "secret", "import", path)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		require.NoError(t, inv.WithContext(ctx).Run())
		require.Contains(t, output.Stdout(), "Imported 3 secrets.")
		require.Contains(t, output.Stderr(), `2 secrets imported without an environment variable name: "PATH", "MY-TOKEN"`)

		secret, err := client.UserSecretByName(ctx, codersdk.Me, "PATH")
		require.NoError(t, err)
		require.Empty(t, secret.EnvName)
	})
}

func TestSecretEnableDisable(t *testing.T) {
	t.Parallel()

	t.Run("Disable", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		created, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "service-token",
			Value:   "service-token-value",
			EnvName: "SERVICE_TOKEN",
		})
		require.NoError(t, err)
		require.True(t, created.Enabled)

		inv, root := clitest.New(t, "secret", "disable", "service-token")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "Disabled secret")
		require.Contains(t, output.Stdout(), "service-token")

		got, err := client.UserSecretByName(setupCtx, codersdk.Me, "service-token")
		require.NoError(t, err)
		assert.False(t, got.Enabled)
	})

	t.Run("Enable", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		disabled := false
		created, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "service-token",
			Value:   "service-token-value",
			EnvName: "SERVICE_TOKEN",
			Enabled: &disabled,
		})
		require.NoError(t, err)
		require.False(t, created.Enabled)

		inv, root := clitest.New(t, "secret", "enable", "service-token")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "Enabled secret")
		require.Contains(t, output.Stdout(), "service-token")

		got, err := client.UserSecretByName(setupCtx, codersdk.Me, "service-token")
		require.NoError(t, err)
		assert.True(t, got.Enabled)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "secret", "disable", "missing-secret")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
