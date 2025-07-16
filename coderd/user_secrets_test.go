package coderd_test

import (
	"github.com/coder/coder/v2/codersdk"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

func TestUserSecrets(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	db, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(
		t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID),
	)
	_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	_, _, _ = ctx, templateAdminClient, member

	// test create API
	userSecretName := "open-ai-api-key"
	userSecretDescription := "api key for open ai"
	userSecret, err := templateAdminClient.CreateUserSecret(ctx, codersdk.CreateUserSecretRequest{
		Name:        userSecretName,
		Description: userSecretDescription,
		Value:       "secretkey",
	})
	require.NoError(t, err)
	//userSecretInJSON, err := json.Marshal(userSecret)
	//require.NoError(t, err)
	//fmt.Printf("userSecretInJSON: %s\n", userSecretInJSON)

	require.NotNil(t, userSecret.ID)
	require.Equal(t, templateAdmin.ID, userSecret.UserID)
	require.Equal(t, userSecretName, userSecret.Name)
	require.Equal(t, userSecretDescription, userSecret.Description)

	// test list API
	userSecretList, err := templateAdminClient.ListUserSecrets(ctx)
	require.NoError(t, err)
	require.Len(t, userSecretList.Secrets, 1)
	//userSecretListInJSON, err := json.Marshal(userSecretList)
	//require.NoError(t, err)
	//fmt.Printf("userSecretListInJSON: %s\n", userSecretListInJSON)

	require.NotNil(t, userSecretList.Secrets[0].ID)
	require.Equal(t, templateAdmin.ID, userSecretList.Secrets[0].UserID)
	require.Equal(t, userSecretName, userSecretList.Secrets[0].Name)
	require.Equal(t, userSecretDescription, userSecretList.Secrets[0].Description)

	// test get API
	userSecret, err = templateAdminClient.GetUserSecret(ctx, userSecretName)
	require.NoError(t, err)
	//userSecretInJSON, err := json.Marshal(userSecret)
	//require.NoError(t, err)
	//fmt.Printf("userSecretInJSON: %s\n", userSecretInJSON)

	require.NotNil(t, userSecret.ID)
	require.Equal(t, templateAdmin.ID, userSecret.UserID)
	require.Equal(t, userSecretName, userSecret.Name)
	require.Equal(t, userSecretDescription, userSecret.Description)
}
