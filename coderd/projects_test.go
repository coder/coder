package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	// t.Run("ListEmpty", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	_ = coderdtest.CreateInitialUser(t, client)
	// 	projects, err := client.Projects(context.Background(), "")
	// 	require.NoError(t, err)
	// 	require.NotNil(t, projects)
	// 	require.Len(t, projects, 0)
	// })

	// t.Run("List", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	_ = coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	projects, err := client.Projects(context.Background(), "")
	// 	require.NoError(t, err)
	// 	require.Len(t, projects, 1)
	// })

	// t.Run("ListWorkspaceOwnerCount", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	coderdtest.NewProvisionerDaemon(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	coderdtest.AwaitProjectImportJob(t, client, user.OrganizationID, job.ID)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
	// 	_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
	// 	projects, err := client.Projects(context.Background(), "")
	// 	require.NoError(t, err)
	// 	require.Len(t, projects, 1)
	// 	require.Equal(t, projects[0].WorkspaceOwnerCount, uint32(1))
	// })
}

func TestProjectByOrganization(t *testing.T) {
	t.Parallel()
	// t.Run("Get", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	_, err := client.Project(context.Background(), user.OrganizationID, project.Name)
	// 	require.NoError(t, err)
	// })
}

func TestPostParametersByProject(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		job := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
		_, err := client.CreateProjectParameter(context.Background(), user.OrganizationID, project.Name, coderd.CreateParameterValueRequest{
			Name:              "somename",
			SourceValue:       "tomato",
			SourceScheme:      database.ParameterSourceSchemeData,
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)
	})
}

func TestParametersByProject(t *testing.T) {
	t.Parallel()
	// t.Run("ListEmpty", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	params, err := client.ProjectParameters(context.Background(), user.OrganizationID, project.Name)
	// 	require.NoError(t, err)
	// 	require.NotNil(t, params)
	// })

	// t.Run("List", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	_, err := client.CreateProjectParameter(context.Background(), user.OrganizationID, project.Name, coderd.CreateParameterValueRequest{
	// 		Name:              "example",
	// 		SourceValue:       "source-value",
	// 		SourceScheme:      database.ParameterSourceSchemeData,
	// 		DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
	// 	})
	// 	require.NoError(t, err)
	// 	params, err := client.ProjectParameters(context.Background(), user.OrganizationID, project.Name)
	// 	require.NoError(t, err)
	// 	require.NotNil(t, params)
	// 	require.Len(t, params, 1)
	// })
}
