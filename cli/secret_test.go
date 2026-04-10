package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
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

		inv, root := clitest.New(t, "secret", "create", "openai-key")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "Missing values for the required flags: value")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(
			t,
			"secret",
			"create",
			"openai-key",
			"--value", "super-secret-value",
			"--description", "Personal OPENAI_API key",
			"--inject-env", "OPEN_AI_KEY",
			"--inject-file", "~/.openai-key",
		)
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "openai-key")

		secret, err := client.UserSecretByName(ctx, codersdk.Me, "openai-key")
		require.NoError(t, err)
		require.Equal(t, "openai-key", secret.Name)
		require.Equal(t, "Personal OPENAI_API key", secret.Description)
		require.Equal(t, "OPEN_AI_KEY", secret.EnvName)
		require.Equal(t, "~/.openai-key", secret.FilePath)
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
			"--inject-env", "",
			"--inject-file", "",
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
}

func TestSecretList(t *testing.T) {
	t.Parallel()

	t.Run("TableOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "aws-creds",
			Value:       "aws-value",
			Description: "AWS credentials",
			FilePath:    "~/.aws/creds",
		})
		require.NoError(t, err)
		_, err = client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "github-token",
			Value:       "ghp_xxxxxxxxxxxx",
			Description: "Personal GitHub access token",
			EnvName:     "GITHUB_TOKEN",
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
		assert.Contains(t, out, "UPDATED")
		assert.Contains(t, out, "ENV")
		assert.Contains(t, out, "FILE")
		assert.Contains(t, out, "DESCRIPTION")
		assert.Contains(t, out, "github-token")
		assert.Contains(t, out, "GITHUB_TOKEN")
		assert.Contains(t, out, "aws-creds")
		assert.Contains(t, out, "~/.aws/creds")
	})

	t.Run("JSONOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		created, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "github-token",
			Value:       "ghp_xxxxxxxxxxxx",
			Description: "Personal GitHub access token",
			EnvName:     "GITHUB_TOKEN",
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
			Name:        "aws-creds",
			Value:       "aws-value",
			Description: "AWS credentials",
			FilePath:    "~/.aws/creds",
		})
		require.NoError(t, err)
		_, err = client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "github-token",
			Value:       "ghp_xxxxxxxxxxxx",
			Description: "Personal GitHub access token",
			EnvName:     "GITHUB_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "list", "github-token")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		out := output.Stdout()
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "UPDATED")
		assert.Contains(t, out, "ENV")
		assert.Contains(t, out, "FILE")
		assert.Contains(t, out, "DESCRIPTION")
		assert.Contains(t, out, "github-token")
		assert.Contains(t, out, "GITHUB_TOKEN")
		assert.NotContains(t, out, "aws-creds")
		assert.NotContains(t, out, "~/.aws/creds")
	})

	t.Run("SingleSecretJSONOutput", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		setupCtx := testutil.Context(t, testutil.WaitMedium)
		created, err := client.CreateUserSecret(setupCtx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:        "github-token",
			Value:       "ghp_xxxxxxxxxxxx",
			Description: "Personal GitHub access token",
			EnvName:     "GITHUB_TOKEN",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "list", "github-token", "--output=json")
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
			Name:  "github-token",
			Value: "ghp_xxxxxxxxxxxx",
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "secret", "delete", "github-token")
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		waiter := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Delete secret")
		pty.ExpectMatchContext(ctx, "github-token")
		pty.WriteLine("yes")
		pty.ExpectMatchContext(ctx, "Deleted secret")

		require.NoError(t, waiter.Wait())

		_, err = client.UserSecretByName(setupCtx, codersdk.Me, "github-token")
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
