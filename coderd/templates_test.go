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
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
	})

	t.Run("WorkspaceCount", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleOwner())
		memberWithDeleted := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleOwner())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// Create 3 workspaces with 3 users. 2 workspaces exist, 1 is deleted
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		memberWorkspace := coderdtest.CreateWorkspace(t, member, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, member, memberWorkspace.LatestBuild.ID)

		deletedWorkspace := coderdtest.CreateWorkspace(t, memberWithDeleted, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, deletedWorkspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, deletedWorkspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		template, err = client.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, 2, int(template.WorkspaceOwnerCount), "workspace count")
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		got, err := client.Template(ctx, expected.ID)
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, uuid.New(), codersdk.CreateTemplateRequest{
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateByName(ctx, user.OrganizationID, "something")
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateByName(ctx, user.OrganizationID, template.Name)
		require.NoError(t, err)
	})
}

func TestPatchTemplateMeta(t *testing.T) {
	t.Parallel()

	t.Run("Modified", func(t *testing.T) {
		t.Parallel()

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
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, req.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)

		// Extra paranoid: did it _really_ happen?
		updated, err = client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, req.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})

	t.Run("NotModified", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

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
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, template.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.UpdateTemplateMeta{
			MaxTTLMillis:               -int64(time.Hour),
			MinAutostartIntervalMillis: -int64(time.Hour),
		}
		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Contains(t, apiErr.Message, "Invalid request")
		require.Len(t, apiErr.Validations, 2)
		assert.Equal(t, apiErr.Validations[0].Field, "max_ttl_ms")
		assert.Equal(t, apiErr.Validations[1].Field, "min_autostart_interval_ms")

		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.WithinDuration(t, template.UpdatedAt, updated.UpdatedAt, time.Minute)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, template.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteTemplate(ctx, template.ID)
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteTemplate(ctx, template.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
}
