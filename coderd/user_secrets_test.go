package coderd_test

import (
	"encoding/json"
	"fmt"
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

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
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

	userSecretName := "open-ai-api-key"
	userSecretDescription := "api key for open ai"
	userSecret, err := templateAdminClient.CreateUserSecret(ctx, codersdk.CreateUserSecretRequest{
		Name:        userSecretName,
		Description: userSecretDescription,
		Value:       "secretkey",
	})
	require.NoError(t, err)
	userSecretInJSON, err := json.Marshal(userSecret)
	require.NoError(t, err)
	fmt.Printf("userSecretInJSON: %s\n", userSecretInJSON)

	require.NotNil(t, userSecret.ID)
	require.Equal(t, userSecret.UserID, templateAdmin.ID)
	require.Equal(t, userSecret.Name, userSecretName)
	require.Equal(t, userSecret.Description, userSecretDescription)
}
