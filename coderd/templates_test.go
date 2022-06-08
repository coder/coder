package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func TestTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
	})
}

func TestPostTemplateByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		expected := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		got, err := client.Template(context.Background(), expected.ID)
		require.NoError(t, err)

		assert.Equal(t, expected.Name, got.Name)
		assert.Equal(t, expected.Description, got.Description)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_, err := client.CreateTemplate(context.Background(), user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:      template.Name,
			VersionID: version.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateTemplate(context.Background(), uuid.New(), codersdk.CreateTemplateRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, err.Error(), "Try logging in using 'coder login <url>'.")
	})

	t.Run("NoVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateTemplate(context.Background(), user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
}

func TestTemplatesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		templates, err := client.TemplatesByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, templates)
		require.Len(t, templates, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		templates, err := client.TemplatesByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, templates, 1)
	})
	t.Run("ListMultiple", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		templates, err := client.TemplatesByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, templates, 2)
	})
}

func TestTemplateByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.TemplateByName(context.Background(), user.OrganizationID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_, err := client.TemplateByName(context.Background(), user.OrganizationID, template.Name)
		require.NoError(t, err)
	})
}

func TestPatchTemplateMeta(t *testing.T) {
	t.Parallel()

	t.Run("Modified", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})
		req := codersdk.UpdateTemplateMeta{
			Description:                "lorem ipsum dolor sit amet et cetera",
			MaxTTLMillis:               12 * time.Hour.Milliseconds(),
			MinAutostartIntervalMillis: time.Minute.Milliseconds(),
		}
		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, req.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})

	t.Run("NotModified", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})
		req := codersdk.UpdateTemplateMeta{
			Description:                template.Description,
			MaxTTLMillis:               template.MaxTTLMillis,
			MinAutostartIntervalMillis: template.MinAutostartIntervalMillis,
		}
		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.ErrorContains(t, err, "not modified")
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, req.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})
}

func TestDeleteTemplate(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspaces", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		err := client.DeleteTemplate(context.Background(), template.ID)
		require.NoError(t, err)
	})

	t.Run("Workspaces", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		err := client.DeleteTemplate(context.Background(), template.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
}
