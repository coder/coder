package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ws, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		require.Equal(t, user.UserID, ws.LatestBuild.InitiatorID)
		require.Equal(t, codersdk.BuildReasonInitiator, ws.LatestBuild.Reason)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// Getting with deleted=true should still work.
		_, err := client.DeletedWorkspace(context.Background(), workspace.ID)
		require.NoError(t, err)

		// Delete the workspace
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// Getting with deleted=true should work.
		workspaceNew, err := client.DeletedWorkspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Getting with deleted=false should not work.
		_, err = client.Workspace(context.Background(), workspace.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "410") // gone
	})
}

func TestAdminViewAllWorkspaces(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	_, err := client.Workspace(context.Background(), workspace.ID)
	require.NoError(t, err)

	otherOrg, err := client.CreateOrganization(context.Background(), codersdk.CreateOrganizationRequest{
		Name: "default-test",
	})
	require.NoError(t, err, "create other org")

	// This other user is not in the first user's org. Since other is an admin, they can
	// still see the "first" user's workspace.
	other := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.RoleAdmin())
	otherWorkspaces, err := other.Workspaces(context.Background(), codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(other) fetch workspaces")

	firstWorkspaces, err := other.Workspaces(context.Background(), codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(first) fetch workspaces")

	require.ElementsMatch(t, otherWorkspaces, firstWorkspaces)
}

func TestPostWorkspacesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("InvalidTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
			TemplateID: uuid.New(),
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("NoTemplateAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleMember(), rbac.RoleAdmin())
		org, err := other.CreateOrganization(context.Background(), codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		version := coderdtest.CreateTemplateVersion(t, other, org.ID, nil)
		template := coderdtest.CreateTemplate(t, other, org.ID, version.ID)

		_, err = client.CreateWorkspace(context.Background(), first.OrganizationID, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       workspace.Name,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	})

	t.Run("TemplateCustomTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		templateTTL := 24 * time.Hour.Milliseconds()
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MaxTTLMillis = ptr.Ref(templateTTL)
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = nil // ensure that no default TTL is set
		})
		// TTL should be set by the template
		require.Equal(t, template.MaxTTLMillis, templateTTL)
		require.Equal(t, template.MaxTTLMillis, template.MaxTTLMillis, workspace.TTLMillis)
	})

	t.Run("InvalidTTL", func(t *testing.T) {
		t.Parallel()
		t.Run("BelowMin", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			req := codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
				Name:       "testing",
				TTLMillis:  ptr.Ref((59 * time.Second).Milliseconds()),
			}
			_, err := client.CreateWorkspace(context.Background(), template.OrganizationID, req)
			require.Error(t, err)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
			require.Len(t, apiErr.Validations, 1)
			require.Equal(t, apiErr.Validations[0].Field, "ttl_ms")
			require.Equal(t, apiErr.Validations[0].Detail, "ttl must be at least one minute")
		})

		t.Run("AboveMax", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			req := codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
				Name:       "testing",
				TTLMillis:  ptr.Ref((24*7*time.Hour + time.Minute).Milliseconds()),
			}
			_, err := client.CreateWorkspace(context.Background(), template.OrganizationID, req)
			require.Error(t, err)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
			require.Len(t, apiErr.Validations, 1)
			require.Equal(t, apiErr.Validations[0].Field, "ttl_ms")
			require.Equal(t, apiErr.Validations[0].Detail, "ttl must be less than 7 days")
		})
	})

	t.Run("InvalidAutostart", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		req := codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "testing",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central * * * * *"),
		}
		_, err := client.CreateWorkspace(context.Background(), template.OrganizationID, req)
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Len(t, apiErr.Validations, 1)
		require.Equal(t, apiErr.Validations[0].Field, "schedule")
		require.Equal(t, apiErr.Validations[0].Detail, "Minimum autostart interval 1m0s below template minimum 1h0m0s")
	})
}

func TestWorkspaceByOwnerAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "something", codersdk.WorkspaceOptions{})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
	})
	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// Given:
		// We delete the workspace
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// Then:
		// When we call without includes_deleted, we don't expect to get the workspace back
		_, err = client.WorkspaceByOwnerAndName(context.Background(), workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{})
		require.ErrorContains(t, err, "404")

		// Then:
		// When we call with includes_deleted, we should get the workspace back
		workspaceNew, err := client.WorkspaceByOwnerAndName(context.Background(), workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{IncludeDeleted: true})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Given:
		// We recreate the workspace with the same name
		workspace, err = client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
			TemplateID:        workspace.TemplateID,
			Name:              workspace.Name,
			AutostartSchedule: workspace.AutostartSchedule,
			TTLMillis:         workspace.TTLMillis,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// Then:
		// We can fetch the most recent workspace
		workspaceNew, err = client.WorkspaceByOwnerAndName(context.Background(), workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)

		// Given:
		// We delete the workspace again
		build, err = client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err, "delete the workspace")
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		// Then:
		// When we fetch the deleted workspace, we get the most recently deleted one
		workspaceNew, err = client.WorkspaceByOwnerAndName(context.Background(), workspace.OwnerName, workspace.Name, codersdk.WorkspaceOptions{IncludeDeleted: true})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, workspaceNew.ID)
	})
}

// TestWorkspaceFilter creates a set of workspaces, users, and organizations
// to run various filters against for testing.
func TestWorkspaceFilter(t *testing.T) {
	t.Parallel()
	type coderUser struct {
		*codersdk.Client
		User codersdk.User
		Org  codersdk.Organization
	}

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	first := coderdtest.CreateFirstUser(t, client)

	users := make([]coderUser, 0)
	for i := 0; i < 10; i++ {
		userClient := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleAdmin())
		user, err := userClient.User(context.Background(), codersdk.Me)
		require.NoError(t, err, "fetch me")

		org, err := userClient.CreateOrganization(context.Background(), codersdk.CreateOrganizationRequest{
			Name: user.Username + "-org",
		})
		require.NoError(t, err, "create org")

		users = append(users, coderUser{
			Client: userClient,
			User:   user,
			Org:    org,
		})
	}

	type madeWorkspace struct {
		Owner     codersdk.User
		Workspace codersdk.Workspace
		Template  codersdk.Template
	}

	availTemplates := make([]codersdk.Template, 0)
	allWorkspaces := make([]madeWorkspace, 0)

	// Create some random workspaces
	for _, user := range users {
		version := coderdtest.CreateTemplateVersion(t, client, user.Org.ID, nil)

		// Create a template & workspace in the user's org
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.Org.ID, version.ID)
		availTemplates = append(availTemplates, template)
		workspace := coderdtest.CreateWorkspace(t, user.Client, template.OrganizationID, template.ID)
		allWorkspaces = append(allWorkspaces, madeWorkspace{
			Workspace: workspace,
			Template:  template,
			Owner:     user.User,
		})

		// Make a workspace with a random template
		idx, _ := cryptorand.Intn(len(availTemplates))
		randTemplate := availTemplates[idx]
		randWorkspace := coderdtest.CreateWorkspace(t, user.Client, randTemplate.OrganizationID, randTemplate.ID)
		allWorkspaces = append(allWorkspaces, madeWorkspace{
			Workspace: randWorkspace,
			Template:  randTemplate,
			Owner:     user.User,
		})
	}

	// Make sure all workspaces are done. Do it after all are made
	for i, w := range allWorkspaces {
		latest := coderdtest.AwaitWorkspaceBuildJob(t, client, w.Workspace.LatestBuild.ID)
		allWorkspaces[i].Workspace.LatestBuild = latest
	}

	// --- Setup done ---
	testCases := []struct {
		Name   string
		Filter codersdk.WorkspaceFilter
		// If FilterF is true, we include it in the expected results
		FilterF func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool
	}{
		{
			Name:   "All",
			Filter: codersdk.WorkspaceFilter{},
			FilterF: func(_ codersdk.WorkspaceFilter, _ madeWorkspace) bool {
				return true
			},
		},
		{
			Name: "Owner",
			Filter: codersdk.WorkspaceFilter{
				Owner: users[2].User.Username,
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return workspace.Owner.Username == f.Owner
			},
		},
		{
			Name: "TemplateName",
			Filter: codersdk.WorkspaceFilter{
				Template: allWorkspaces[5].Template.Name,
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return workspace.Template.Name == f.Template
			},
		},
		{
			Name: "Name",
			Filter: codersdk.WorkspaceFilter{
				// Use a common letter... one has to have this letter in it
				Name: "a",
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return strings.Contains(workspace.Workspace.Name, f.Name)
			},
		},
		{
			Name: "Q-Owner/Name",
			Filter: codersdk.WorkspaceFilter{
				FilterQuery: allWorkspaces[5].Owner.Username + "/" + allWorkspaces[5].Workspace.Name,
			},
			FilterF: func(_ codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return workspace.Workspace.ID == allWorkspaces[5].Workspace.ID
			},
		},
		{
			Name: "Many filters",
			Filter: codersdk.WorkspaceFilter{
				Owner:    allWorkspaces[3].Owner.Username,
				Template: allWorkspaces[3].Template.Name,
				Name:     allWorkspaces[3].Workspace.Name,
			},
			FilterF: func(f codersdk.WorkspaceFilter, workspace madeWorkspace) bool {
				return workspace.Workspace.ID == allWorkspaces[3].Workspace.ID
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			workspaces, err := client.Workspaces(context.Background(), c.Filter)
			require.NoError(t, err, "fetch workspaces")

			exp := make([]codersdk.Workspace, 0)
			for _, made := range allWorkspaces {
				if c.FilterF(c.Filter, made) {
					exp = append(exp, made.Workspace)
				}
			}
			require.ElementsMatch(t, exp, workspaces, "expected workspaces returned")
		})
	}
}

// TestWorkspaceFilterManual runs some specific setups with basic checks.
func TestWorkspaceFilterManual(t *testing.T) {
	t.Parallel()

	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		// full match
		ws, err := client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Name: workspace.Name,
		})
		require.NoError(t, err)
		require.Len(t, ws, 1, workspace.Name)
		require.Equal(t, workspace.ID, ws[0].ID)

		// partial match
		ws, err = client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Name: workspace.Name[1 : len(workspace.Name)-2],
		})
		require.NoError(t, err)
		require.Len(t, ws, 1)
		require.Equal(t, workspace.ID, ws[0].ID)

		// no match
		ws, err = client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Name: "$$$$",
		})
		require.NoError(t, err)
		require.Len(t, ws, 0)
	})
	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		template2 := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template2.ID)

		// empty
		ws, err := client.Workspaces(context.Background(), codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, ws, 2)

		// single template
		ws, err = client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Template: template.Name,
		})
		require.NoError(t, err)
		require.Len(t, ws, 1)
		require.Equal(t, workspace.ID, ws[0].ID)
	})
	t.Run("FilterQuery", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		template2 := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template2.ID)

		// single workspace
		ws, err := client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			FilterQuery: fmt.Sprintf("template:%s %s/%s", template.Name, workspace.OwnerName, workspace.Name),
		})
		require.NoError(t, err)
		require.Len(t, ws, 1)
		require.Equal(t, workspace.ID, ws[0].ID)
	})
}

func TestPostWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("NoTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: uuid.New(),
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("TemplateVersionFailedImport", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		_, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("AlreadyActive", func(t *testing.T) {
		t.Parallel()
		client, coderAPI := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, coderAPI)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		// Close here so workspace build doesn't process!
		closeDaemon.Close()
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("IncrementBuildNumber", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
	})

	t.Run("WithState", func(t *testing.T) {
		t.Parallel()
		client, coderAPI := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, coderAPI)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		_ = closeDaemon.Close()
		wantState := []byte("something")
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			TemplateVersionID: template.ActiveVersionID,
			Transition:        codersdk.WorkspaceTransitionStart,
			ProvisionerState:  wantState,
		})
		require.NoError(t, err)
		gotState, err := client.WorkspaceBuildState(context.Background(), build.ID)
		require.NoError(t, err)
		require.Equal(t, wantState, gotState)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.BuildNumber+1, build.BuildNumber)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		workspaces, err := client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Owner: user.UserID.String(),
		})
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})
}

func TestWorkspaceBuildByName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		_, err := client.WorkspaceBuildByName(context.Background(), workspace.ID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build, err := client.WorkspaceBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		_, err = client.WorkspaceBuildByName(context.Background(), workspace.ID, build.Name)
		require.NoError(t, err)
	})
}

func TestWorkspaceUpdateAutostart(t *testing.T) {
	t.Parallel()
	var dublinLoc = mustLocation(t, "Europe/Dublin")

	testCases := []struct {
		name             string
		schedule         *string
		expectedError    string
		at               time.Time
		expectedNext     time.Time
		expectedInterval time.Duration
	}{
		{
			name:          "disable autostart",
			schedule:      ptr.Ref(""),
			expectedError: "",
		},
		{
			name:             "friday to monday",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * 1-5"),
			expectedError:    "",
			at:               time.Date(2022, 5, 6, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 5, 9, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 71*time.Hour + 59*time.Minute,
		},
		{
			name:             "monday to tuesday",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * 1-5"),
			expectedError:    "",
			at:               time.Date(2022, 5, 9, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 5, 10, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 23*time.Hour + 59*time.Minute,
		},
		{
			// DST in Ireland began on Mar 27 in 2022 at 0100. Forward 1 hour.
			name:             "DST start",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * *"),
			expectedError:    "",
			at:               time.Date(2022, 3, 26, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 3, 27, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 22*time.Hour + 59*time.Minute,
		},
		{
			// DST in Ireland ends on Oct 30 in 2022 at 0200. Back 1 hour.
			name:             "DST end",
			schedule:         ptr.Ref("CRON_TZ=Europe/Dublin 30 9 * * *"),
			expectedError:    "",
			at:               time.Date(2022, 10, 29, 9, 31, 0, 0, dublinLoc),
			expectedNext:     time.Date(2022, 10, 30, 9, 30, 0, 0, dublinLoc),
			expectedInterval: 24*time.Hour + 59*time.Minute,
		},
		{
			name:          "invalid location",
			schedule:      ptr.Ref("CRON_TZ=Imaginary/Place 30 9 * * 1-5"),
			expectedError: "parse schedule: provided bad location Imaginary/Place: unknown time zone Imaginary/Place",
		},
		{
			name:          "invalid schedule",
			schedule:      ptr.Ref("asdf asdf asdf "),
			expectedError: `validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix`,
		},
		{
			name:          "only 3 values",
			schedule:      ptr.Ref("CRON_TZ=Europe/Dublin 30 9 *"),
			expectedError: `validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var (
				ctx       = context.Background()
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTLMillis = nil
				})
			)

			// ensure test invariant: new workspaces have no autostart schedule.
			require.Empty(t, workspace.AutostartSchedule, "expected newly-minted workspace to have no autostart schedule")

			err := client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: testCase.schedule,
			})

			if testCase.expectedError != "" {
				require.ErrorContains(t, err, testCase.expectedError, "Invalid autostart schedule")
				return
			}

			require.NoError(t, err, "expected no error setting workspace autostart schedule")

			updated, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err, "fetch updated workspace")

			if testCase.schedule == nil || *testCase.schedule == "" {
				require.Nil(t, updated.AutostartSchedule)
				return
			}

			require.EqualValues(t, *testCase.schedule, *updated.AutostartSchedule, "expected autostart schedule to equal requested")

			sched, err := schedule.Weekly(*updated.AutostartSchedule)
			require.NoError(t, err, "parse returned schedule")

			next := sched.Next(testCase.at)
			require.Equal(t, testCase.expectedNext, next, "unexpected next scheduled autostart time")
			interval := next.Sub(testCase.at)
			require.Equal(t, testCase.expectedInterval, interval, "unexpected interval")
		})
	}

	t.Run("NotFound", func(t *testing.T) {
		var (
			ctx    = context.Background()
			client = coderdtest.New(t, nil)
			_      = coderdtest.CreateFirstUser(t, client)
			wsid   = uuid.New()
			req    = codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: ptr.Ref("9 30 1-5"),
			}
		)

		err := client.UpdateWorkspaceAutostart(ctx, wsid, req)
		require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
		coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
		require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
		require.Contains(t, coderSDKErr.Message, "Resource not found", "unexpected response code")
	})
}

func TestWorkspaceUpdateTTL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		ttlMillis      *int64
		expectedError  string
		modifyTemplate func(*codersdk.CreateTemplateRequest)
	}{
		{
			name:          "disable ttl",
			ttlMillis:     nil,
			expectedError: "",
		},
		{
			name:          "update ttl",
			ttlMillis:     ptr.Ref(12 * time.Hour.Milliseconds()),
			expectedError: "",
		},
		{
			name:          "below minimum ttl",
			ttlMillis:     ptr.Ref((30 * time.Second).Milliseconds()),
			expectedError: "ttl must be at least one minute",
		},
		{
			name:          "minimum ttl",
			ttlMillis:     ptr.Ref(time.Minute.Milliseconds()),
			expectedError: "",
		},
		{
			name:          "maximum ttl",
			ttlMillis:     ptr.Ref((24 * 7 * time.Hour).Milliseconds()),
			expectedError: "",
		},
		{
			name:          "above maximum ttl",
			ttlMillis:     ptr.Ref((24*7*time.Hour + time.Minute).Milliseconds()),
			expectedError: "ttl must be less than 7 days",
		},
		{
			name:           "above template maximum ttl",
			ttlMillis:      ptr.Ref((12 * time.Hour).Milliseconds()),
			expectedError:  "ttl_ms: ttl must be below template maximum 8h0m0s",
			modifyTemplate: func(ctr *codersdk.CreateTemplateRequest) { ctr.MaxTTLMillis = ptr.Ref((8 * time.Hour).Milliseconds()) },
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mutators := make([]func(*codersdk.CreateTemplateRequest), 0)
			if testCase.modifyTemplate != nil {
				mutators = append(mutators, testCase.modifyTemplate)
			}
			var (
				ctx       = context.Background()
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, mutators...)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
					cwr.AutostartSchedule = nil
					cwr.TTLMillis = nil
				})
				_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			)

			err := client.UpdateWorkspaceTTL(ctx, workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: testCase.ttlMillis,
			})

			if testCase.expectedError != "" {
				require.ErrorContains(t, err, testCase.expectedError, "unexpected error when setting workspace autostop schedule")
				return
			}

			require.NoError(t, err, "expected no error setting workspace autostop schedule")

			updated, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err, "fetch updated workspace")

			require.Equal(t, testCase.ttlMillis, updated.TTLMillis, "expected autostop ttl to equal requested")
		})
	}

	t.Run("NotFound", func(t *testing.T) {
		var (
			ctx    = context.Background()
			client = coderdtest.New(t, nil)
			_      = coderdtest.CreateFirstUser(t, client)
			wsid   = uuid.New()
			req    = codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: ptr.Ref(time.Hour.Milliseconds()),
			}
		)

		err := client.UpdateWorkspaceTTL(ctx, wsid, req)
		require.IsType(t, err, &codersdk.Error{}, "expected codersdk.Error")
		coderSDKErr, _ := err.(*codersdk.Error) //nolint:errorlint
		require.Equal(t, coderSDKErr.StatusCode(), 404, "expected status code 404")
		require.Contains(t, coderSDKErr.Message, "Resource not found", "unexpected response code")
	})
}

func TestWorkspaceExtend(t *testing.T) {
	t.Parallel()
	var (
		ttl         = 8 * time.Hour
		newDeadline = time.Now().Add(ttl + time.Hour).UTC()
		ctx         = context.Background()
		client      = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user        = coderdtest.CreateFirstUser(t, client)
		version     = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_           = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template    = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace   = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
		})
		_ = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	)

	workspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "fetch provisioned workspace")
	oldDeadline := workspace.LatestBuild.Deadline

	// Updating the deadline should succeed
	req := codersdk.PutExtendWorkspaceRequest{
		Deadline: newDeadline,
	}
	err = client.PutExtendWorkspace(ctx, workspace.ID, req)
	require.NoError(t, err, "failed to extend workspace")

	// Ensure deadline set correctly
	updated, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "failed to fetch updated workspace")
	require.WithinDuration(t, newDeadline, updated.LatestBuild.Deadline, time.Minute)

	// Zero time should fail
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: time.Time{},
	})
	require.ErrorContains(t, err, "deadline: Validation failed for tag \"required\" with value: \"0001-01-01 00:00:00 +0000 UTC\"", "setting an empty deadline on a workspace should fail")

	// Updating with a deadline less than 30 minutes in the future should fail
	deadlineTooSoon := time.Now().Add(15 * time.Minute) // XXX: time.Now
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: deadlineTooSoon,
	})
	require.ErrorContains(t, err, "new deadline must be at least 30 minutes in the future", "setting a deadline less than 30 minutes in the future should fail")

	// And with a deadline greater than the template max_ttl should also fail
	deadlineExceedsMaxTTL := time.Now().Add(time.Duration(template.MaxTTLMillis) * time.Millisecond).Add(time.Minute)
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: deadlineExceedsMaxTTL,
	})
	require.ErrorContains(t, err, "new deadline is greater than template allows", "setting a deadline greater than that allowed by the template should fail")

	// Updating with a deadline 30 minutes in the future should succeed
	deadlineJustSoonEnough := time.Now().Add(30 * time.Minute)
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: deadlineJustSoonEnough,
	})
	require.NoError(t, err, "setting a deadline at least 30 minutes in the future should succeed")

	// Updating with a deadline an hour before the previous deadline should succeed
	err = client.PutExtendWorkspace(ctx, workspace.ID, codersdk.PutExtendWorkspaceRequest{
		Deadline: oldDeadline.Add(-time.Hour),
	})
	require.NoError(t, err, "setting an earlier deadline should not fail")

	// Ensure deadline still set correctly
	updated, err = client.Workspace(ctx, workspace.ID)
	require.NoError(t, err, "failed to fetch updated workspace")
	require.WithinDuration(t, oldDeadline.Add(-time.Hour), updated.LatestBuild.Deadline, time.Minute)
}

func TestWorkspaceWatcher(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	w, err := client.Workspace(context.Background(), workspace.ID)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wc, err := client.WatchWorkspace(ctx, w.ID)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, more := <-wc
		require.True(t, more)
	}
	cancel()
	require.EqualValues(t, codersdk.Workspace{}, <-wc)
}

func mustLocation(t *testing.T, location string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(location)
	if err != nil {
		t.Errorf("failed to load location %s: %s", location, err.Error())
	}

	return loc
}
