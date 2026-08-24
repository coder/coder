package coderd_test

import (
	"net/http"
	"testing"

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

func TestUserSecretFilePathDisabledHandlers(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	dv := coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
		dv.DisableUserSecretFilePath = true
	})
	db, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db, Pubsub: ps, DeploymentValues: dv, Auditor: auditor,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name: "file-secret", Value: "value", EnvName: "FILE_SECRET", FilePath: "/tmp/file-secret",
	})
	requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")

	imported, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format: codersdk.SecretsFileFormatEnv, Content: "IMPORTED=value\nPATH=disabled\n",
	})
	require.NoError(t, err)
	require.Len(t, imported, 2)

	legacy := dbgen.UserSecret(t, db, database.UserSecret{UserID: owner.UserID, Name: "legacy"},
		func(p *database.CreateUserSecretParams) {
			p.EnvName, p.FilePath, p.Enabled = "", "/tmp/legacy", true
		})

	description := "updated"
	updated, err := client.UpdateUserSecret(ctx, codersdk.Me, legacy.Name, codersdk.UpdateUserSecretRequest{
		Description: &description,
	})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/legacy", updated.FilePath)
	assert.True(t, updated.Enabled)

	empty := ""
	_, err = client.UpdateUserSecret(ctx, codersdk.Me, legacy.Name, codersdk.UpdateUserSecretRequest{FilePath: &empty})
	requireSecretValidationContainsError(t, err, http.StatusBadRequest, "env_name", "Add env_name")

	disabled := false
	updated, err = client.UpdateUserSecret(ctx, codersdk.Me, legacy.Name, codersdk.UpdateUserSecretRequest{Enabled: &disabled})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/legacy", updated.FilePath)

	auditor.ResetLogs()
	newPath := "/tmp/other"
	_, err = client.UpdateUserSecret(ctx, codersdk.Me, legacy.Name, codersdk.UpdateUserSecretRequest{
		FilePath: &newPath,
	})
	requireSecretValidationContainsError(t, err, http.StatusBadRequest, "file_path", "disabled")

	logs := auditor.AuditLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, database.AuditActionWrite, logs[0].Action)
	assert.EqualValues(t, http.StatusBadRequest, logs[0].StatusCode)
	assert.Equal(t, legacy.ID, logs[0].ResourceID)
	assert.JSONEq(t, "{}", string(logs[0].Diff))
}
