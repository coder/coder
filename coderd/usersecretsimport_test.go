package coderd_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestImportUserSecrets(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)
		auditor.ResetLogs()

		secrets, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "ALPHA=a\nBETA=b\nPATH=c\n",
		})
		require.NoError(t, err)
		require.Len(t, secrets, 3)
		// Valid keys are env-injected, while reserved names are imported
		// without env injection.
		assert.Equal(t, "ALPHA", secrets[0].Name)
		assert.Equal(t, "ALPHA", secrets[0].EnvName)
		assert.Equal(t, "PATH", secrets[2].Name)
		assert.Empty(t, secrets[2].EnvName)

		listed, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		names := make([]string, 0, len(listed))
		for _, s := range listed {
			names = append(names, s.Name)
		}
		assert.ElementsMatch(t, []string{"ALPHA", "BETA", "PATH"}, names)

		// Exactly one create audit log per imported secret.
		logs := auditor.AuditLogs()
		require.Len(t, logs, 3)
		resourceIDs := make([]string, 0, len(logs))
		resourceTargets := make([]string, 0, len(logs))
		for _, l := range logs {
			assert.Equal(t, database.AuditActionCreate, l.Action)
			assert.EqualValues(t, http.StatusCreated, l.StatusCode)
			resourceIDs = append(resourceIDs, l.ResourceID.String())
			resourceTargets = append(resourceTargets, l.ResourceTarget)
		}
		assert.ElementsMatch(t, []string{
			secrets[0].ID.String(), secrets[1].ID.String(), secrets[2].ID.String(),
		}, resourceIDs)
		assert.ElementsMatch(t, []string{"ALPHA", "BETA", "PATH"}, resourceTargets)
	})

	t.Run("ValuesNotInResponse", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		const secretValue = "super-secret-sentinel-value-123"
		res, err := client.Request(ctx, http.MethodPost,
			fmt.Sprintf("/api/v2/users/%s/secrets/batch", codersdk.Me),
			codersdk.ImportUserSecretsRequest{
				Format:  codersdk.SecretsFileFormatEnv,
				Content: "LEAKY=" + secretValue,
			})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(body), secretValue)
	})
}

func TestImportUserSecretsForbiddenForAnotherUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := memberClient.ImportUserSecrets(ctx, owner.UserID.String(), codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: "FORBIDDEN=value",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}

func TestImportUserSecretsBodyTooLarge(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: strings.Repeat("a", 8*codersdk.MaxSecretsFileBytes),
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusRequestEntityTooLarge, sdkErr.StatusCode())
}

// TestImportUserSecretsValidationRollback verifies that a single
// invalid entry rejects the whole batch: nothing is created and no
// audit log is written. The valid sibling entry must not leak through.
func TestImportUserSecretsValidationRollback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		badLine string
	}{
		// Empty values are always invalid; this is the canonical rollback case.
		{name: "EmptyValue", badLine: "EMPTY_ONE="},
		{name: "OversizedValue", badLine: "BIG=" + strings.Repeat("a", codersdk.MaxUserSecretValueBytes+1)},
		// A slash in the name is invalid regardless of env-name handling.
		{name: "NameWithSlash", badLine: "bad/name=value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			auditor := audit.NewMock()
			client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
			_ = coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitMedium)
			auditor.ResetLogs()

			_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
				Format:  codersdk.SecretsFileFormatEnv,
				Content: "GOOD_ENTRY=fine\n" + tc.badLine,
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			// Errors are attributed to the offending entry (index 1).
			require.NotEmpty(t, sdkErr.Validations)
			for _, v := range sdkErr.Validations {
				assert.Truef(t, strings.HasPrefix(v.Field, "secrets[1]."),
					"unexpected field %q", v.Field)
			}

			listed, err := client.UserSecrets(ctx, codersdk.Me)
			require.NoError(t, err)
			assert.Empty(t, listed)

			assert.Empty(t, auditor.AuditLogs())
		})
	}
}

// TestImportUserSecretsConflict verifies that a batch containing an
// already-existing secret name aborts entirely: the new entry is not
// created and no audit log is written.
func TestImportUserSecretsConflict(t *testing.T) {
	t.Parallel()
	auditor := audit.NewMock()
	client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name:  "EXISTING",
		Value: "original",
	})
	require.NoError(t, err)
	auditor.ResetLogs()

	_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: "BRANDNEW=x\nEXISTING=collision",
	})
	validation := requireSecretValidation(t, err, http.StatusConflict, "secrets[1].name")
	assert.Equal(t, "name already in use", validation.Detail)

	// Only the pre-existing secret should remain; BRANDNEW must not be created.
	listed, err := client.UserSecrets(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "EXISTING", listed[0].Name)

	assert.Empty(t, auditor.AuditLogs())
}

// TestImportUserSecretsLimits exercises each per-user cap. A cap
// tripped mid-batch must roll back every row in the import and, because
// audit logs are emitted only after the transaction commits, write no
// import audit logs.
func TestImportUserSecretsLimits(t *testing.T) {
	t.Parallel()

	t.Run("CountLimit", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		for i := 0; i < codersdk.MaxUserSecretsPerUserCount-1; i++ {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:  fmt.Sprintf("prefill-%03d", i),
				Value: "original",
			})
			require.NoError(t, err)
		}
		before, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, before, codersdk.MaxUserSecretsPerUserCount-1)

		auditor.ResetLogs()
		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "COUNT_FIRST=x\nCOUNT_SECOND=y\n",
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "secrets[1]")

		after, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, after, len(before))
		beforeNames := make([]string, 0, len(before))
		afterNames := make([]string, 0, len(after))
		for _, secret := range before {
			beforeNames = append(beforeNames, secret.Name)
		}
		for _, secret := range after {
			afterNames = append(afterNames, secret.Name)
		}
		assert.ElementsMatch(t, beforeNames, afterNames)
		assert.Empty(t, auditor.AuditLogs())
	})

	t.Run("EnvBytesLimit", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Every imported secret is env-injected, so two values that are
		// each within the per-value cap can still exceed the env-bytes
		// aggregate together.
		content := fmt.Sprintf("ENV_A=%s\nENV_B=%s\n",
			strings.Repeat("a", codersdk.MaxUserSecretValueBytes-16),
			strings.Repeat("a", 1024))
		auditor.ResetLogs()
		_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "env_name")

		listed, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Empty(t, listed)
		assert.Empty(t, auditor.AuditLogs())
	})

	t.Run("TotalBytesLimit", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Pre-fill the total-bytes budget to the cap using file-only
		// secrets (no env_name), which do not count against the smaller
		// env budget. Creating them via CreateUserSecret directly avoids
		// going through the import parser.
		big := strings.Repeat("a", codersdk.MaxUserSecretValueBytes)
		numBig := codersdk.MaxUserSecretsTotalValueBytes / codersdk.MaxUserSecretValueBytes
		remainder := codersdk.MaxUserSecretsTotalValueBytes % codersdk.MaxUserSecretValueBytes
		for i := 0; i < numBig; i++ {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     fmt.Sprintf("prefill-%03d", i),
				Value:    big,
				FilePath: fmt.Sprintf("/tmp/prefill-%03d", i),
			})
			require.NoError(t, err)
		}
		if remainder > 0 {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     "prefill-pad",
				Value:    strings.Repeat("a", remainder),
				FilePath: "/tmp/prefill-pad",
			})
			require.NoError(t, err)
		}

		before, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)

		// Reset after the prefill (which legitimately emits create audit
		// logs) so the assertion below only sees logs from the rolled-back
		// import.
		auditor.ResetLogs()
		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "OVERFLOW=x",
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "per-user budget")

		after, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Len(t, after, len(before))
		assert.Empty(t, auditor.AuditLogs())
	})
}

func TestImportUserSecretsParseErrors(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	// Parse-error variety is covered by the parser unit tests; this only
	// asserts the endpoint maps a parse failure to 400.
	_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatJSON,
		Content: "{not json",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
}
