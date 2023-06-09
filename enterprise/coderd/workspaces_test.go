package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
)

func TestCreateWorkspace(t *testing.T) {
	t.Parallel()

	// Test that a user cannot indirectly access
	// a template they do not have access to.
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				acl.Groups[0].ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		client1, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		req := codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "testme",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		}

		_, err = client1.CreateWorkspace(ctx, user.OrganizationID, user1.ID.String(), req)
		require.Error(t, err)
	})
}

func TestWorkspacesFiltering(t *testing.T) {
	t.Parallel()

	t.Run("FilterQueryHasDeletingByAndLicensed", func(t *testing.T) {
		t.Parallel()

		inactivityTTL := 1 * 24 * time.Hour

		client := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAdvancedTemplateScheduling: 1,
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// update template with inactivity ttl
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		template, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			InactivityTTLMillis: inactivityTTL.Milliseconds(),
		})

		assert.NoError(t, err)
		assert.Equal(t, inactivityTTL.Milliseconds(), template.InactivityTTLMillis)

		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// stop build so workspace is inactive
		stopBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJob(t, client, stopBuild.ID)

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			// adding a second to time.Now() to give some buffer in case test runs quickly
			FilterQuery: fmt.Sprintf("deleting_by:%s", time.Now().Add(time.Second).Add(inactivityTTL).Format("2006-01-02")),
		})
		assert.NoError(t, err)
		assert.Len(t, res.Workspaces, 1)
		assert.Equal(t, workspace.ID, res.Workspaces[0].ID)
	})
}
