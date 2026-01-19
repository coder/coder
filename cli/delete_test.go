package cli_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDelete(t *testing.T) {
	t.Parallel()
	t.Run("WithParameter", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, member, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		inv, root := clitest.New(t, "delete", workspace.Name, "-y")
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()
		pty.ExpectMatch("has been deleted")
		<-doneChan
	})

	t.Run("Orphan", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, templateAdmin, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, templateAdmin, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, workspace.LatestBuild.ID)

		ctx := testutil.Context(t, testutil.WaitShort)
		inv, root := clitest.New(t, "delete", workspace.Name, "-y", "--orphan")
		clitest.SetupConfig(t, templateAdmin, root)

		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()
		go func() {
			defer close(doneChan)
			err := inv.WithContext(ctx).Run()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()
		pty.ExpectMatch("has been deleted")
		testutil.TryReceive(ctx, t, doneChan)

		_, err := client.Workspace(ctx, workspace.ID)
		require.Error(t, err)
		cerr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusGone, cerr.StatusCode())
	})

	// Super orphaned, as the workspace doesn't even have a user.
	// This is not a scenario we should ever get into, as we do not allow users
	// to be deleted if they have workspaces. However issue #7872 shows that
	// it is possible to get into this state. An admin should be able to still
	// force a delete action on the workspace.
	t.Run("OrphanDeletedUser", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		deleteMeClient, deleteMeUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, deleteMeClient, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, deleteMeClient, workspace.LatestBuild.ID)

		// The API checks if the user has any workspaces, so we cannot delete a user
		// this way.
		ctx := testutil.Context(t, testutil.WaitShort)
		err := api.Database.UpdateUserDeletedByID(dbauthz.AsSystemRestricted(ctx), deleteMeUser.ID)
		require.NoError(t, err)

		inv, root := clitest.New(t, "delete", fmt.Sprintf("%s/%s", deleteMeUser.ID, workspace.Name), "-y", "--orphan")

		//nolint:gocritic // Deleting orphaned workspaces requires an admin.
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()
		go func() {
			defer close(doneChan)
			err := inv.Run()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()
		pty.ExpectMatch("has been deleted")
		<-doneChan
	})

	t.Run("DifferentUser", func(t *testing.T) {
		t.Parallel()
		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		adminUser := coderdtest.CreateFirstUser(t, adminClient)
		orgID := adminUser.OrganizationID
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, orgID)
		user, err := client.User(context.Background(), codersdk.Me)
		require.NoError(t, err)

		version := coderdtest.CreateTemplateVersion(t, adminClient, orgID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
		template := coderdtest.CreateTemplate(t, adminClient, orgID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		inv, root := clitest.New(t, "delete", user.Username+"/"+workspace.Name, "-y")
		//nolint:gocritic // This requires an admin.
		clitest.SetupConfig(t, adminClient, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			// When running with the race detector on, we sometimes get an EOF.
			if err != nil {
				assert.ErrorIs(t, err, io.EOF)
			}
		}()

		pty.ExpectMatch("has been deleted")
		<-doneChan

		workspace, err = client.Workspace(context.Background(), workspace.ID)
		require.ErrorContains(t, err, "was deleted")
	})

	t.Run("InvalidWorkspaceIdentifier", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		inv, root := clitest.New(t, "delete", "a/b/c", "-y")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.ErrorContains(t, err, "invalid workspace name: \"a/b/c\"")
		}()
		<-doneChan
	})

	t.Run("WarnNoProvisioners", func(t *testing.T) {
		t.Parallel()

		store, ps, db := dbtestutil.NewDBWithSQLDB(t)
		client, closeDaemon := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			Database:                 store,
			Pubsub:                   ps,
			IncludeProvisionerDaemon: true,
		})

		// Given: a user, template, and workspace
		user := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, templateAdmin, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdmin, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, templateAdmin, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, workspace.LatestBuild.ID)

		// When: all provisioner daemons disappear
		require.NoError(t, closeDaemon.Close())
		_, err := db.Exec("DELETE FROM provisioner_daemons;")
		require.NoError(t, err)

		// Then: the workspace deletion should warn about no provisioners
		inv, root := clitest.New(t, "delete", workspace.Name, "-y")
		pty := ptytest.New(t).Attach(inv)
		clitest.SetupConfig(t, templateAdmin, root)
		doneChan := make(chan struct{})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		go func() {
			defer close(doneChan)
			_ = inv.WithContext(ctx).Run()
		}()
		pty.ExpectMatch("there are no provisioners that accept the required tags")
		cancel()
		<-doneChan
	})

	t.Run("Prebuilt workspace delete permissions", func(t *testing.T) {
		t.Parallel()

		// Setup
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		client, _ := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{
			Database:                 db,
			Pubsub:                   pb,
			IncludeProvisionerDaemon: true,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		orgID := owner.OrganizationID

		// Given a template version with a preset and a template
		version := coderdtest.CreateTemplateVersion(t, client, orgID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		preset := setupTestDBPreset(t, db, version.ID)
		template := coderdtest.CreateTemplate(t, client, orgID, version.ID)

		cases := []struct {
			name                          string
			client                        *codersdk.Client
			expectedPrebuiltDeleteErrMsg  string
			expectedWorkspaceDeleteErrMsg string
		}{
			// Users with the OrgAdmin role should be able to delete both normal and prebuilt workspaces
			{
				name: "OrgAdmin",
				client: func() *codersdk.Client {
					client, _ := coderdtest.CreateAnotherUser(t, client, orgID, rbac.ScopedRoleOrgAdmin(orgID))
					return client
				}(),
			},
			// Users with the TemplateAdmin role should be able to delete prebuilt workspaces, but not normal workspaces
			{
				name: "TemplateAdmin",
				client: func() *codersdk.Client {
					client, _ := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleTemplateAdmin())
					return client
				}(),
				expectedWorkspaceDeleteErrMsg: "unexpected status code 403: You do not have permission to delete this workspace.",
			},
			// Users with the OrgTemplateAdmin role should be able to delete prebuilt workspaces, but not normal workspaces
			{
				name: "OrgTemplateAdmin",
				client: func() *codersdk.Client {
					client, _ := coderdtest.CreateAnotherUser(t, client, orgID, rbac.ScopedRoleOrgTemplateAdmin(orgID))
					return client
				}(),
				expectedWorkspaceDeleteErrMsg: "unexpected status code 403: You do not have permission to delete this workspace.",
			},
			// Users with the Member role should not be able to delete prebuilt or normal workspaces
			{
				name: "Member",
				client: func() *codersdk.Client {
					client, _ := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleMember())
					return client
				}(),
				expectedPrebuiltDeleteErrMsg:  "unexpected status code 404: Resource not found or you do not have access to this resource",
				expectedWorkspaceDeleteErrMsg: "unexpected status code 404: Resource not found or you do not have access to this resource",
			},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				clock := quartz.NewMock(t)
				ctx := testutil.Context(t, testutil.WaitSuperLong)

				// Create one prebuilt workspace (owned by system user) and one normal workspace (owned by a user)
				// Each workspace is persisted in the DB along with associated workspace jobs and builds.
				dbPrebuiltWorkspace := setupTestDBWorkspace(t, clock, db, pb, orgID, database.PrebuildsSystemUserID, template.ID, version.ID, preset.ID)
				userWorkspaceOwner, err := client.User(context.Background(), "testUser")
				require.NoError(t, err)
				dbUserWorkspace := setupTestDBWorkspace(t, clock, db, pb, orgID, userWorkspaceOwner.ID, template.ID, version.ID, preset.ID)

				assertWorkspaceDelete := func(
					runClient *codersdk.Client,
					workspace database.Workspace,
					workspaceOwner string,
					expectedErr string,
				) {
					t.Helper()

					// Attempt to delete the workspace as the test client
					inv, root := clitest.New(t, "delete", workspaceOwner+"/"+workspace.Name, "-y")
					clitest.SetupConfig(t, runClient, root)
					doneChan := make(chan struct{})
					pty := ptytest.New(t).Attach(inv)
					var runErr error
					go func() {
						defer close(doneChan)
						runErr = inv.Run()
					}()

					// Validate the result based on the expected error message
					if expectedErr != "" {
						<-doneChan
						require.Error(t, runErr)
						require.Contains(t, runErr.Error(), expectedErr)
					} else {
						pty.ExpectMatch("has been deleted")
						<-doneChan

						// When running with the race detector on, we sometimes get an EOF.
						if runErr != nil {
							assert.ErrorIs(t, runErr, io.EOF)
						}

						// Verify that the workspace is now marked as deleted
						_, err := client.Workspace(context.Background(), workspace.ID)
						require.ErrorContains(t, err, "was deleted")
					}
				}

				// Ensure at least one prebuilt workspace is reported as running in the database
				testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
					running, err := db.GetRunningPrebuiltWorkspaces(ctx)
					if !assert.NoError(t, err) || !assert.GreaterOrEqual(t, len(running), 1) {
						return false
					}
					return true
				}, testutil.IntervalMedium, "running prebuilt workspaces timeout")

				runningWorkspaces, err := db.GetRunningPrebuiltWorkspaces(ctx)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(runningWorkspaces), 1)

				// Get the full prebuilt workspace object from the DB
				prebuiltWorkspace, err := db.GetWorkspaceByID(ctx, dbPrebuiltWorkspace.ID)
				require.NoError(t, err)

				// Assert the prebuilt workspace deletion
				assertWorkspaceDelete(tc.client, prebuiltWorkspace, "prebuilds", tc.expectedPrebuiltDeleteErrMsg)

				// Get the full user workspace object from the DB
				userWorkspace, err := db.GetWorkspaceByID(ctx, dbUserWorkspace.ID)
				require.NoError(t, err)

				// Assert the user workspace deletion
				assertWorkspaceDelete(tc.client, userWorkspace, userWorkspaceOwner.Username, tc.expectedWorkspaceDeleteErrMsg)
			})
		}
	})
}

func setupTestDBPreset(
	t *testing.T,
	db database.Store,
	templateVersionID uuid.UUID,
) database.TemplateVersionPreset {
	t.Helper()

	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: templateVersionID,
		Name:              "preset-test",
		DesiredInstances: sql.NullInt32{
			Valid: true,
			Int32: 1,
		},
	})
	dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})

	return preset
}

func setupTestDBWorkspace(
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
	presetID uuid.UUID,
) database.WorkspaceTable {
	t.Helper()

	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     templateID,
		OrganizationID: orgID,
		OwnerID:        ownerID,
		Deleted:        false,
		CreatedAt:      time.Now().Add(-time.Hour * 2),
	})
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		InitiatorID:    ownerID,
		CreatedAt:      time.Now().Add(-time.Hour * 2),
		StartedAt:      sql.NullTime{Time: clock.Now().Add(-time.Hour * 2), Valid: true},
		CompletedAt:    sql.NullTime{Time: clock.Now().Add(-time.Hour), Valid: true},
		OrganizationID: orgID,
	})
	workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		InitiatorID:             ownerID,
		TemplateVersionID:       templateVersionID,
		JobID:                   job.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: presetID, Valid: true},
		Transition:              database.WorkspaceTransitionStart,
		CreatedAt:               clock.Now(),
	})
	dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
		{
			WorkspaceBuildID: workspaceBuild.ID,
			Name:             "test",
			Value:            "test",
		},
	})

	return workspace
}
