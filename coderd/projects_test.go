package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestProject(t *testing.T) {
	t.Parallel()

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		_, err := client.Project(context.Background(), project.ID)
		require.NoError(t, err)
	})
}

func TestProjectVersionsByProject(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		versions, err := client.ProjectVersionsByProject(context.Background(), project.ID)
		require.NoError(t, err)
		require.Len(t, versions, 1)
	})
}

func TestProjectVersionByName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		_, err := client.ProjectVersionByName(context.Background(), project.ID, "nothing")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		_, err := client.ProjectVersionByName(context.Background(), project.ID, version.Name)
		require.NoError(t, err)
	})
}
