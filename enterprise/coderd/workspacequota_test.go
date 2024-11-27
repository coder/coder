package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func verifyQuota(ctx context.Context, t *testing.T, client *codersdk.Client, organizationID string, consumed, total int) {
	verifyQuotaUser(ctx, t, client, organizationID, codersdk.Me, consumed, total)
}

func verifyQuotaUser(ctx context.Context, t *testing.T, client *codersdk.Client, organizationID string, user string, consumed, total int) {
	t.Helper()

	got, err := client.WorkspaceQuota(ctx, organizationID, user)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceQuota{
		Budget:          total,
		CreditsConsumed: consumed,
	}, got)

	// Remove this check when the deprecated endpoint is removed.
	// This just makes sure the deprecated endpoint is still working
	// as intended. It will only work for the default organization.
	deprecatedGot, err := deprecatedQuotaEndpoint(ctx, client, user)
	require.NoError(t, err, "deprecated endpoint")
	// Only continue to check if the values differ
	if deprecatedGot.Budget != got.Budget || deprecatedGot.CreditsConsumed != got.CreditsConsumed {
		org, err := client.OrganizationByName(ctx, organizationID)
		if err != nil {
			return
		}
		if org.IsDefault {
			require.Equal(t, got, deprecatedGot)
		}
	}
}

func TestWorkspaceQuota(t *testing.T) {
	t.Parallel()

	// This first test verifies the behavior of creating and deleting workspaces.
	// It also tests multi-group quota stacking and the everyone group.
	t.Run("CreateDelete", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 1
		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		coderdtest.NewProvisionerDaemon(t, api.AGPL)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 0)

		// Patch the 'Everyone' group to verify its quota allowance is being accounted for.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(1),
		})
		require.NoError(t, err)
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 1)

		// Add user to two groups, granting them a total budget of 4.
		group1, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "test-1",
			QuotaAllowance: 1,
		})
		require.NoError(t, err)

		group2, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "test-2",
			QuotaAllowance: 2,
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user.UserID.String()},
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group2.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user.UserID.String()},
		})
		require.NoError(t, err)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 4)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name:      "example",
							Type:      "aws_instance",
							DailyCost: 1,
							Agents: []*proto.Agent{{
								Id:   uuid.NewString(),
								Name: "example",
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up three workspaces fine
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				workspace := coderdtest.CreateWorkspace(t, client, template.ID)
				build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
				assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
			}()
		}
		wg.Wait()
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		require.Contains(t, build.Job.Error, "quota")

		// Delete one random workspace, then quota should recover.
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		for _, w := range workspaces.Workspaces {
			if w.LatestBuild.Status != codersdk.WorkspaceStatusRunning {
				continue
			}
			build, err := client.CreateWorkspaceBuild(ctx, w.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionDelete,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
			verifyQuota(ctx, t, client, user.OrganizationID.String(), 3, 4)
			break
		}

		// Next one should now succeed
		workspace = coderdtest.CreateWorkspace(t, client, template.ID)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})

	t.Run("StartStop", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 1
		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		coderdtest.NewProvisionerDaemon(t, api.AGPL)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 0)

		// Patch the 'Everyone' group to verify its quota allowance is being accounted for.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(4),
		})
		require.NoError(t, err)
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 4)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlanMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: planWithCost(2),
				proto.WorkspaceTransition_STOP:  planWithCost(1),
			},
			ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: applyWithCost(2),
				proto.WorkspaceTransition_STOP:  applyWithCost(1),
			},
		})

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up two workspaces.
		var wg sync.WaitGroup
		var workspaces []codersdk.Workspace
		for i := 0; i < 2; i++ {
			workspace := coderdtest.CreateWorkspace(t, client, template.ID)
			workspaces = append(workspaces, workspace)
			build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		}
		wg.Wait()
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		require.Contains(t, build.Job.Error, "quota")

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)

		build = coderdtest.CreateWorkspaceBuild(t, client, workspaces[0], database.WorkspaceTransitionStop)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Quota goes down one
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 3, 4)
		require.Equal(t, codersdk.WorkspaceStatusStopped, build.Status)

		build = coderdtest.CreateWorkspaceBuild(t, client, workspaces[0], database.WorkspaceTransitionStart)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Quota goes back up
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})

	// Ensures allowance from everyone groups only counts if you are an org member.
	// This was a bug where the group "Everyone" was being counted for all users,
	// regardless of membership.
	t.Run("AllowanceEveryone", func(t *testing.T) {
		t.Parallel()

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		member, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

		// Create a second organization
		second := coderdenttest.CreateOrganization(t, owner, coderdenttest.CreateOrganizationOptions{})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// update everyone quotas
		//nolint:gocritic // using owner for simplicity
		_, err := owner.PatchGroup(ctx, first.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(30),
		})
		require.NoError(t, err)

		_, err = owner.PatchGroup(ctx, second.ID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(15),
		})
		require.NoError(t, err)

		verifyQuota(ctx, t, member, first.OrganizationID.String(), 0, 30)

		// Verify org scoped quota limits
		verifyQuota(ctx, t, owner, first.OrganizationID.String(), 0, 30)
		verifyQuota(ctx, t, owner, second.ID.String(), 0, 15)
	})

	// ManyWorkspaces uses dbfake and dbgen to insert a scenario into the db.
	t.Run("ManyWorkspaces", func(t *testing.T) {
		t.Parallel()

		owner, db, first := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		client, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleOwner())

		// Prepopulate database. Use dbfake as it is quicker and
		// easier than the api.
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		noise := dbgen.User(t, db, database.User{})

		second := dbfake.Organization(t, db).
			Members(user, noise).
			EveryoneAllowance(10).
			Group(database.Group{
				QuotaAllowance: 25,
			}, user, noise).
			Group(database.Group{
				QuotaAllowance: 30,
			}, noise).
			Do()

		third := dbfake.Organization(t, db).
			Members(noise).
			EveryoneAllowance(7).
			Do()

		verifyQuotaUser(ctx, t, client, second.Org.ID.String(), user.ID.String(), 0, 35)
		verifyQuotaUser(ctx, t, client, second.Org.ID.String(), noise.ID.String(), 0, 65)

		// Workspaces owned by the user
		consumed := 0
		for i := 0; i < 2; i++ {
			const cost = 5
			dbfake.WorkspaceBuild(t, db,
				database.WorkspaceTable{
					OwnerID:        user.ID,
					OrganizationID: second.Org.ID,
				}).
				Seed(database.WorkspaceBuild{
					DailyCost: cost,
				}).Do()
			consumed += cost
		}

		// Add some noise
		// Workspace by the user in the third org
		dbfake.WorkspaceBuild(t, db,
			database.WorkspaceTable{
				OwnerID:        user.ID,
				OrganizationID: third.Org.ID,
			}).
			Seed(database.WorkspaceBuild{
				DailyCost: 10,
			}).Do()

		// Workspace by another user in third org
		dbfake.WorkspaceBuild(t, db,
			database.WorkspaceTable{
				OwnerID:        noise.ID,
				OrganizationID: third.Org.ID,
			}).
			Seed(database.WorkspaceBuild{
				DailyCost: 10,
			}).Do()

		// Workspace by another user in second org
		dbfake.WorkspaceBuild(t, db,
			database.WorkspaceTable{
				OwnerID:        noise.ID,
				OrganizationID: second.Org.ID,
			}).
			Seed(database.WorkspaceBuild{
				DailyCost: 10,
			}).Do()

		verifyQuotaUser(ctx, t, client, second.Org.ID.String(), user.ID.String(), consumed, 35)
	})
}

// nolint:paralleltest,tparallel // Tests must run serially
func TestWorkspaceSerialization(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("Serialization errors only occur in postgres")
	}

	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	otherUser := dbgen.User(t, db, database.User{})

	org := dbfake.Organization(t, db).
		EveryoneAllowance(20).
		Members(user, otherUser).
		Group(database.Group{
			QuotaAllowance: 10,
		}, user, otherUser).
		Group(database.Group{
			QuotaAllowance: 10,
		}, user).
		Do()

	otherOrg := dbfake.Organization(t, db).
		EveryoneAllowance(20).
		Members(user, otherUser).
		Group(database.Group{
			QuotaAllowance: 10,
		}, user, otherUser).
		Group(database.Group{
			QuotaAllowance: 10,
		}, user).
		Do()

	// TX mixing tests. **DO NOT** run these in parallel.
	// The goal here is to mess around with different ordering of
	// transactions and queries.

	// UpdateBuildDeadline bumps a workspace deadline while doing a quota
	// commit to the same workspace build.
	//
	// Note: This passes if the interrupt is run before 'GetQuota()'
	// Passing orders:
	//	- BeginTX -> Bump! -> GetQuota -> GetAllowance -> UpdateCost -> EndTx
	//  - BeginTX -> GetQuota -> GetAllowance -> UpdateCost -> Bump! -> EndTx
	t.Run("UpdateBuildDeadline", func(t *testing.T) {
		t.Log("Expected to fail. As long as quota & deadline are on the same " +
			" table and affect the same row, this will likely always fail.")

		//  +------------------------------+------------------+
		//  | Begin Tx                     |                  |
		//  +------------------------------+------------------+
		//  | GetQuota(user)               |                  |
		//  +------------------------------+------------------+
		//  |                              | BumpDeadline(w1) |
		//  +------------------------------+------------------+
		//  | GetAllowance(user)           |                  |
		//  +------------------------------+------------------+
		//  | UpdateWorkspaceBuildCost(w1) |                  |
		//  +------------------------------+------------------+
		//  | CommitTx()                   |                  |
		//  +------------------------------+------------------+
		// pq: could not serialize access due to concurrent update
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		bumpDeadline := func() {
			err := db.InTx(func(db database.Store) error {
				err := db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
					Deadline:    dbtime.Now(),
					MaxDeadline: dbtime.Now(),
					UpdatedAt:   dbtime.Now(),
					ID:          myWorkspace.Build.ID,
				})
				return err
			}, &database.TxOptions{
				Isolation: sql.LevelSerializable,
			})
			assert.NoError(t, err)
		}

		// Start TX
		// Run order

		quota := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)
		quota.GetQuota(ctx, t)     // Step 1
		bumpDeadline()             // Interrupt
		quota.GetAllowance(ctx, t) // Step 2

		err := quota.DBTx.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
			ID:        myWorkspace.Build.ID,
			DailyCost: 10,
		}) // Step 3
		require.ErrorContains(t, err, "could not serialize access due to concurrent update")
		// End commit
		require.ErrorContains(t, quota.Done(), "failed transaction")
	})

	// UpdateOtherBuildDeadline bumps a user's other workspace deadline
	// while doing a quota commit.
	t.Run("UpdateOtherBuildDeadline", func(t *testing.T) {
		//  +------------------------------+------------------+
		//  | Begin Tx                     |                  |
		//  +------------------------------+------------------+
		//  | GetQuota(user)               |                  |
		//  +------------------------------+------------------+
		//  |                              | BumpDeadline(w2) |
		//  +------------------------------+------------------+
		//  | GetAllowance(user)           |                  |
		//  +------------------------------+------------------+
		//  | UpdateWorkspaceBuildCost(w1) |                  |
		//  +------------------------------+------------------+
		//  | CommitTx()                   |                  |
		//  +------------------------------+------------------+
		// Works!
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		// Use the same template
		otherWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).
			Seed(database.WorkspaceBuild{
				TemplateVersionID: myWorkspace.TemplateVersion.ID,
			}).
			Do()

		bumpDeadline := func() {
			err := db.InTx(func(db database.Store) error {
				err := db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
					Deadline:    dbtime.Now(),
					MaxDeadline: dbtime.Now(),
					UpdatedAt:   dbtime.Now(),
					ID:          otherWorkspace.Build.ID,
				})
				return err
			}, &database.TxOptions{
				Isolation: sql.LevelSerializable,
			})
			assert.NoError(t, err)
		}

		// Start TX
		// Run order

		quota := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)
		quota.GetQuota(ctx, t)                         // Step 1
		bumpDeadline()                                 // Interrupt
		quota.GetAllowance(ctx, t)                     // Step 2
		quota.UpdateWorkspaceBuildCostByID(ctx, t, 10) // Step 3
		// End commit
		require.NoError(t, quota.Done())
	})

	t.Run("ActivityBump", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Even though this test is expected to 'likely always fail', it doesn't fail on Windows")
		}

		t.Log("Expected to fail. As long as quota & deadline are on the same " +
			" table and affect the same row, this will likely always fail.")
		//  +---------------------+----------------------------------+
		//  | W1 Quota Tx         |                                  |
		//  +---------------------+----------------------------------+
		//  | Begin Tx            |                                  |
		//  +---------------------+----------------------------------+
		//  | GetQuota(w1)        |                                  |
		//  +---------------------+----------------------------------+
		//  | GetAllowance(w1)    |                                  |
		//  +---------------------+----------------------------------+
		//  |                     | ActivityBump(w1)                 |
		//  +---------------------+----------------------------------+
		//  | UpdateBuildCost(w1) |                                  |
		//  +---------------------+----------------------------------+
		//  | CommitTx()          |                                  |
		//  +---------------------+----------------------------------+
		// pq: could not serialize access due to concurrent update
		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).
			Seed(database.WorkspaceBuild{
				// Make sure the bump does something
				Deadline: dbtime.Now().Add(time.Hour * -20),
			}).
			Do()

		one := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)

		// Run order
		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		err := db.ActivityBumpWorkspace(ctx, database.ActivityBumpWorkspaceParams{
			NextAutostart: time.Now(),
			WorkspaceID:   myWorkspace.Workspace.ID,
		})

		assert.NoError(t, err)

		err = one.DBTx.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
			ID:        myWorkspace.Build.ID,
			DailyCost: 10,
		})
		require.ErrorContains(t, err, "could not serialize access due to concurrent update")

		// End commit
		assert.ErrorContains(t, one.Done(), "failed transaction")
	})

	t.Run("BumpLastUsedAt", func(t *testing.T) {
		//  +---------------------+----------------------------------+
		//  | W1 Quota Tx         |                                  |
		//  +---------------------+----------------------------------+
		//  | Begin Tx            |                                  |
		//  +---------------------+----------------------------------+
		//  | GetQuota(w1)        |                                  |
		//  +---------------------+----------------------------------+
		//  | GetAllowance(w1)    |                                  |
		//  +---------------------+----------------------------------+
		//  |                     | UpdateWorkspaceLastUsedAt(w1)    |
		//  +---------------------+----------------------------------+
		//  | UpdateBuildCost(w1) |                                  |
		//  +---------------------+----------------------------------+
		//  | CommitTx()          |                                  |
		//  +---------------------+----------------------------------+
		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		one := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)

		// Run order
		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		err := db.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         myWorkspace.Workspace.ID,
			LastUsedAt: dbtime.Now(),
		})
		assert.NoError(t, err)

		one.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		// End commit
		assert.NoError(t, one.Done())
	})

	t.Run("UserMod", func(t *testing.T) {
		//  +---------------------+----------------------------------+
		//  | W1 Quota Tx         |                                  |
		//  +---------------------+----------------------------------+
		//  | Begin Tx            |                                  |
		//  +---------------------+----------------------------------+
		//  | GetQuota(w1)        |                                  |
		//  +---------------------+----------------------------------+
		//  | GetAllowance(w1)    |                                  |
		//  +---------------------+----------------------------------+
		//  |                     | RemoveUserFromOrg                |
		//  +---------------------+----------------------------------+
		//  | UpdateBuildCost(w1) |                                  |
		//  +---------------------+----------------------------------+
		//  | CommitTx()          |                                  |
		//  +---------------------+----------------------------------+
		// Works!
		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)
		var err error

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		one := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)

		// Run order

		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		err = db.DeleteOrganizationMember(ctx, database.DeleteOrganizationMemberParams{
			OrganizationID: myWorkspace.Workspace.OrganizationID,
			UserID:         user.ID,
		})
		assert.NoError(t, err)

		one.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		// End commit
		assert.NoError(t, one.Done())
	})

	// QuotaCommit 2 workspaces in different orgs.
	// Workspaces do not share templates, owners, or orgs
	t.Run("DoubleQuotaUnrelatedWorkspaces", func(t *testing.T) {
		//  +---------------------+---------------------+
		//  | W1 Quota Tx         | W2 Quota Tx         |
		//  +---------------------+---------------------+
		//  | Begin Tx            |                     |
		//  +---------------------+---------------------+
		//  |                     | Begin Tx            |
		//  +---------------------+---------------------+
		//  | GetQuota(w1)        |                     |
		//  +---------------------+---------------------+
		//  | GetAllowance(w1)    |                     |
		//  +---------------------+---------------------+
		//  | UpdateBuildCost(w1) |                     |
		//  +---------------------+---------------------+
		//  |                     | UpdateBuildCost(w2) |
		//  +---------------------+---------------------+
		//  |                     | GetQuota(w2)        |
		//  +---------------------+---------------------+
		//  |                     | GetAllowance(w2)    |
		//  +---------------------+---------------------+
		//  | CommitTx()          |                     |
		//  +---------------------+---------------------+
		//  |                     | CommitTx()          |
		//  +---------------------+---------------------+
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		myOtherWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: otherOrg.Org.ID, // Different org!
			OwnerID:        otherUser.ID,
		}).Do()

		one := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)
		two := newCommitter(t, db, myOtherWorkspace.Workspace, myOtherWorkspace.Build)

		// Run order
		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		one.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		two.GetQuota(ctx, t)
		two.GetAllowance(ctx, t)
		two.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		// End commit
		assert.NoError(t, one.Done())
		assert.NoError(t, two.Done())
	})

	// QuotaCommit 2 workspaces in different orgs.
	// Workspaces do not share templates or orgs
	t.Run("DoubleQuotaUserWorkspacesDiffOrgs", func(t *testing.T) {
		//  +---------------------+---------------------+
		//  | W1 Quota Tx         | W2 Quota Tx         |
		//  +---------------------+---------------------+
		//  | Begin Tx            |                     |
		//  +---------------------+---------------------+
		//  |                     | Begin Tx            |
		//  +---------------------+---------------------+
		//  | GetQuota(w1)        |                     |
		//  +---------------------+---------------------+
		//  | GetAllowance(w1)    |                     |
		//  +---------------------+---------------------+
		//  | UpdateBuildCost(w1) |                     |
		//  +---------------------+---------------------+
		//  |                     | UpdateBuildCost(w2) |
		//  +---------------------+---------------------+
		//  |                     | GetQuota(w2)        |
		//  +---------------------+---------------------+
		//  |                     | GetAllowance(w2)    |
		//  +---------------------+---------------------+
		//  | CommitTx()          |                     |
		//  +---------------------+---------------------+
		//  |                     | CommitTx()          |
		//  +---------------------+---------------------+
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		myOtherWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: otherOrg.Org.ID, // Different org!
			OwnerID:        user.ID,
		}).Do()

		one := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)
		two := newCommitter(t, db, myOtherWorkspace.Workspace, myOtherWorkspace.Build)

		// Run order
		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		one.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		two.GetQuota(ctx, t)
		two.GetAllowance(ctx, t)
		two.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		// End commit
		assert.NoError(t, one.Done())
		assert.NoError(t, two.Done())
	})

	// QuotaCommit 2 workspaces in the same org.
	// Workspaces do not share templates
	t.Run("DoubleQuotaUserWorkspaces", func(t *testing.T) {
		t.Log("Setting a new build cost to a workspace in a org affects other " +
			"workspaces in the same org. This is expected to fail.")
		//  +---------------------+---------------------+
		//  | W1 Quota Tx         | W2 Quota Tx         |
		//  +---------------------+---------------------+
		//  | Begin Tx            |                     |
		//  +---------------------+---------------------+
		//  |                     | Begin Tx            |
		//  +---------------------+---------------------+
		//  | GetQuota(w1)        |                     |
		//  +---------------------+---------------------+
		//  | GetAllowance(w1)    |                     |
		//  +---------------------+---------------------+
		//  | UpdateBuildCost(w1) |                     |
		//  +---------------------+---------------------+
		//  |                     | UpdateBuildCost(w2) |
		//  +---------------------+---------------------+
		//  |                     | GetQuota(w2)        |
		//  +---------------------+---------------------+
		//  |                     | GetAllowance(w2)    |
		//  +---------------------+---------------------+
		//  | CommitTx()          |                     |
		//  +---------------------+---------------------+
		//  |                     | CommitTx()          |
		//  +---------------------+---------------------+
		// pq: could not serialize access due to read/write dependencies among transactions
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // testing
		ctx = dbauthz.AsSystemRestricted(ctx)

		myWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		myOtherWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		one := newCommitter(t, db, myWorkspace.Workspace, myWorkspace.Build)
		two := newCommitter(t, db, myOtherWorkspace.Workspace, myOtherWorkspace.Build)

		// Run order
		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		one.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		two.GetQuota(ctx, t)
		two.GetAllowance(ctx, t)
		two.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		// End commit
		assert.NoError(t, one.Done())
		assert.ErrorContains(t, two.Done(), "could not serialize access due to read/write dependencies among transactions")
	})
}

func deprecatedQuotaEndpoint(ctx context.Context, client *codersdk.Client, userID string) (codersdk.WorkspaceQuota, error) {
	res, err := client.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspace-quota/%s", userID), nil)
	if err != nil {
		return codersdk.WorkspaceQuota{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.WorkspaceQuota{}, codersdk.ReadBodyAsError(res)
	}
	var quota codersdk.WorkspaceQuota
	return quota, json.NewDecoder(res.Body).Decode(&quota)
}

func planWithCost(cost int32) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Resources: []*proto.Resource{{
					Name:      "example",
					Type:      "aws_instance",
					DailyCost: cost,
				}},
			},
		},
	}}
}

func applyWithCost(cost int32) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
				Resources: []*proto.Resource{{
					Name:      "example",
					Type:      "aws_instance",
					DailyCost: cost,
				}},
			},
		},
	}}
}

// committer does what the CommitQuota does, but allows
// stepping through the actions in the tx and controlling the
// timing.
// This is a nice wrapper to make the tests more concise.
type committer struct {
	DBTx *dbtestutil.DBTx
	w    database.WorkspaceTable
	b    database.WorkspaceBuild
}

func newCommitter(t *testing.T, db database.Store, workspace database.WorkspaceTable, build database.WorkspaceBuild) *committer {
	quotaTX := dbtestutil.StartTx(t, db, &database.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	})
	return &committer{DBTx: quotaTX, w: workspace, b: build}
}

// GetQuota touches:
//   - workspace_builds
//   - workspaces
func (c *committer) GetQuota(ctx context.Context, t *testing.T) int64 {
	t.Helper()

	consumed, err := c.DBTx.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
		OwnerID:        c.w.OwnerID,
		OrganizationID: c.w.OrganizationID,
	})
	require.NoError(t, err)
	return consumed
}

// GetAllowance touches:
//   - group_members_expanded
//   - users
//   - groups
//   - org_members
func (c *committer) GetAllowance(ctx context.Context, t *testing.T) int64 {
	t.Helper()

	allowance, err := c.DBTx.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
		UserID:         c.w.OwnerID,
		OrganizationID: c.w.OrganizationID,
	})
	require.NoError(t, err)
	return allowance
}

func (c *committer) UpdateWorkspaceBuildCostByID(ctx context.Context, t *testing.T, cost int32) bool {
	t.Helper()

	err := c.DBTx.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
		ID:        c.b.ID,
		DailyCost: cost,
	})
	return assert.NoError(t, err)
}

func (c *committer) Done() error {
	return c.DBTx.Done()
}
