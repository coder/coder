package coderd_test

import (
	"testing"
)

func TestProjectVersionsByOrganization(t *testing.T) {
	t.Parallel()
	// t.Run("List", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	versions, err := client.ProjectVersions(context.Background(), user.OrganizationID, project.Name)
	// 	require.NoError(t, err)
	// 	require.NotNil(t, versions)
	// 	require.Len(t, versions, 1)
	// })
}

func TestProjectVersionByOrganizationAndName(t *testing.T) {
	t.Parallel()
	// t.Run("Get", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	_, err := client.ProjectVersion(context.Background(), user.OrganizationID, project.Name, project.ActiveVersionID.String())
	// 	require.NoError(t, err)
	// })
}

func TestPostProjectVersionByOrganization(t *testing.T) {
	t.Parallel()
	// t.Run("Create", func(t *testing.T) {
	// 	t.Parallel()
	// 	client := coderdtest.New(t, nil)
	// 	user := coderdtest.CreateInitialUser(t, client)
	// 	job := coderdtest.CreateProjectImportJob(t, client, user.OrganizationID, nil)
	// 	project := coderdtest.CreateProject(t, client, user.OrganizationID, job.ID)
	// 	_, err := client.CreateProjectVersion(context.Background(), user.OrganizationID, project.Name, coderd.CreateProjectVersionRequest{
	// 		ImportJobID: job.ID,
	// 	})
	// 	require.NoError(t, err)
	// })
}
