package coderd_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/githubrunners"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGitHubRunnerPoolAPI(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	setHandler, cancel, serverURL, opts := coderdtest.NewOptions(t, &coderdtest.Options{Clock: quartz.NewMock(t)})
	opts.GitHubRunnerClient = func(database.GithubRunnerPool) (githubrunners.Client, error) {
		return nil, xerrors.New("GitHub disabled in API test")
	}
	api := coderd.New(opts)
	setHandler(api.RootHandler)
	t.Cleanup(func() { cancel(); _ = api.Close() })
	client := codersdk.New(serverURL)
	first := coderdtest.CreateFirstUser(t, client)
	template := dbgen.Template(t, api.Database, database.Template{OrganizationID: first.OrganizationID, CreatedBy: first.UserID})
	req := codersdk.CreateGitHubRunnerPoolRequest{Name: "test-pool", TemplateID: template.ID, GitHubConfigURL: "https://github.com/example/repo", GitHubToken: "never-return-this-token", MaxRunners: 2}
	pool, err := client.CreateGitHubRunnerPool(ctx, first.OrganizationID, req)
	require.NoError(t, err)
	require.Equal(t, first.OrganizationID, pool.OrganizationID)
	require.Equal(t, first.UserID, pool.OwnerID)
	// Check the wire response as well as the SDK's write-only credential type.
	res, err := client.Request(ctx, http.MethodGet, "/api/v2/organizations/"+first.OrganizationID.String()+"/github-runner-pools/"+pool.ID.String(), nil)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	_ = res.Body.Close()
	body := string(bodyBytes)
	require.NotContains(t, body, req.GitHubToken)
	require.NotContains(t, body, "github_token")
	pools, err := client.GitHubRunnerPools(ctx, first.OrganizationID)
	require.NoError(t, err)
	require.Len(t, pools, 1)
	_, err = client.UpdateGitHubRunnerPool(ctx, first.OrganizationID, pool.ID, codersdk.UpdateGitHubRunnerPoolRequest{State: "paused"})
	require.NoError(t, err)
	runners, err := client.GitHubRunners(ctx, first.OrganizationID, pool.ID)
	require.NoError(t, err)
	require.Empty(t, runners)
	member, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
	_, err = member.CreateGitHubRunnerPool(ctx, first.OrganizationID, req)
	require.Error(t, err)
	_, err = member.GitHubRunnerPool(ctx, first.OrganizationID, pool.ID)
	require.Error(t, err)
	_, err = member.UpdateGitHubRunnerPool(ctx, first.OrganizationID, pool.ID, codersdk.UpdateGitHubRunnerPoolRequest{State: "active"})
	require.Error(t, err)
	_, err = client.GitHubRunnerPool(ctx, uuid.New(), pool.ID)
	require.Error(t, err)
	otherOrg := dbgen.Organization(t, api.Database, database.Organization{})
	otherTemplate := dbgen.Template(t, api.Database, database.Template{OrganizationID: otherOrg.ID, CreatedBy: first.UserID})
	req.TemplateID = otherTemplate.ID
	req.Name = "wrong-org"
	_, err = client.CreateGitHubRunnerPool(ctx, first.OrganizationID, req)
	require.Error(t, err)
	require.NoError(t, client.DeleteGitHubRunnerPool(ctx, first.OrganizationID, pool.ID))
	_, err = client.UpdateGitHubRunnerPool(ctx, first.OrganizationID, pool.ID, codersdk.UpdateGitHubRunnerPoolRequest{State: "active"})
	require.Error(t, err)
}
