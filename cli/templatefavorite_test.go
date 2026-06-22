package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateFavoriteUnfavorite(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	// Favorite the template.
	inv, root := clitest.New(t, "templates", "favorite", template.Name)
	clitest.SetupConfig(t, memberClient, root)

	var buf bytes.Buffer
	inv.Stdout = &buf
	err := inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.NoError(t, err)

	// Verify it shows as favorited via the API.
	ctx := testutil.Context(t, testutil.WaitShort)
	updated, err := memberClient.Template(ctx, template.ID)
	require.NoError(t, err)
	require.True(t, updated.Favorite)

	// Unfavorite the template.
	buf.Reset()
	inv, root = clitest.New(t, "templates", "unfavorite", template.Name)
	clitest.SetupConfig(t, memberClient, root)
	inv.Stdout = &buf
	err = inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.NoError(t, err)

	// Verify it is no longer favorited.
	ctx = testutil.Context(t, testutil.WaitShort)
	updated, err = memberClient.Template(ctx, template.ID)
	require.NoError(t, err)
	require.False(t, updated.Favorite)
}
