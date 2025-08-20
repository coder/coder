package cli_test

import (
	"bytes"
	"database/sql"
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
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestPrebuildsPause(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		inv, conf := newCLI(t, "prebuilds", "pause")
		var buf bytes.Buffer
		inv.Stderr = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Verify the output message
		assert.Contains(t, buf.String(), "Prebuilds are now paused.")

		// Verify the settings were actually updated
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv.Context())
		require.NoError(t, err)
		assert.True(t, settings.ReconciliationPaused)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		t.Parallel()

		adminClient, admin := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Create a regular user without admin privileges
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

		inv, conf := newCLI(t, "prebuilds", "pause")
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
		})

		inv, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		// Should fail without license
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("AlreadyPaused", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// First pause
		inv1, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)
		err := inv1.Run()
		require.NoError(t, err)

		// Try to pause again
		inv2, conf2 := newCLI(t, "prebuilds", "pause")
		clitest.SetupConfig(t, client, conf2)
		err = inv2.Run()
		require.NoError(t, err) // Should succeed even if already paused

		// Verify still paused
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv2.Context())
		require.NoError(t, err)
		assert.True(t, settings.ReconciliationPaused)
	})
}

func TestPrebuildsResume(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// First pause prebuilds
		inv1, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)
		err := inv1.Run()
		require.NoError(t, err)

		// Then resume
		inv2, conf2 := newCLI(t, "prebuilds", "resume")
		var buf bytes.Buffer
		inv2.Stderr = &buf
		clitest.SetupConfig(t, client, conf2)

		err = inv2.Run()
		require.NoError(t, err)

		// Verify the output message
		assert.Contains(t, buf.String(), "Prebuilds are now resumed.")

		// Verify the settings were actually updated
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv2.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)
	})

	t.Run("ResumeWhenNotPaused", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Resume without first pausing
		inv, conf := newCLI(t, "prebuilds", "resume")
		var buf bytes.Buffer
		inv.Stderr = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Should succeed and show the message
		assert.Contains(t, buf.String(), "Prebuilds are now resumed.")

		// Verify still not paused
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		t.Parallel()

		adminClient, admin := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Create a regular user without admin privileges
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

		inv, conf := newCLI(t, "prebuilds", "resume")
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
		})

		inv, conf := newCLI(t, "prebuilds", "resume")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		// Should fail without license
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})
}

func TestPrebuildsCommand(t *testing.T) {
	t.Parallel()

	t.Run("Help", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		inv, conf := newCLI(t, "prebuilds", "--help")
		var buf bytes.Buffer
		inv.Stdout = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Verify help output contains expected information
		output := buf.String()
		assert.Contains(t, output, "Manage Coder prebuilds")
		assert.Contains(t, output, "pause")
		assert.Contains(t, output, "resume")
		assert.Contains(t, output, "Administrators can use these commands")
	})

	t.Run("NoSubcommand", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		inv, conf := newCLI(t, "prebuilds")
		var buf bytes.Buffer
		inv.Stdout = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Should show help when no subcommand is provided
		output := buf.String()
		assert.Contains(t, output, "Manage Coder prebuilds")
		assert.Contains(t, output, "pause")
		assert.Contains(t, output, "resume")
	})
}

func TestPrebuildsSettingsAPI(t *testing.T) {
	t.Parallel()

	t.Run("GetSettings", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Get initial settings
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(t.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)

		// Pause prebuilds
		inv1, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)
		err = inv1.Run()
		require.NoError(t, err)

		// Get settings again
		settings, err = client.GetPrebuildsSettings(t.Context())
		require.NoError(t, err)
		assert.True(t, settings.ReconciliationPaused)

		// Resume prebuilds
		inv2, conf2 := newCLI(t, "prebuilds", "resume")
		clitest.SetupConfig(t, client, conf2)
		err = inv2.Run()
		require.NoError(t, err)

		// Get settings one more time
		settings, err = client.GetPrebuildsSettings(t.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)
	})
}

// TestSchedulePrebuilds verifies the CLI schedule command when used with prebuilds.
// Running the command on an unclaimed prebuild fails, but after the prebuild is
// claimed (becoming a regular workspace) it succeeds as expected.
func TestSchedulePrebuilds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		cliErrorMsg string
		cmdArgs     func(string) []string
	}{
		{
			name:        "AutostartPrebuildError",
			cliErrorMsg: "autostart configuration is not supported for prebuilt workspaces",
			cmdArgs: func(workspaceName string) []string {
				return []string{"schedule", "start", workspaceName, "7:30AM", "Mon-Fri", "Europe/Lisbon"}
			},
		},
		{
			name:        "AutostopPrebuildError",
			cliErrorMsg: "autostop configuration is not supported for prebuilt workspaces",
			cmdArgs: func(workspaceName string) []string {
				return []string{"schedule", "stop", workspaceName, "8h30m"}
			},
		},
		{
			name:        "ExtendPrebuildError",
			cliErrorMsg: "extend configuration is not supported for prebuilt workspaces",
			cmdArgs: func(workspaceName string) []string {
				return []string{"schedule", "extend", workspaceName, "90m"}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			clock.Set(dbtime.Now())

			// Setup
			client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
					Clock:                    clock,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureWorkspacePrebuilds: 1,
					},
				},
			})

			// Given: a template and a template version with preset and a prebuilt workspace
			presetID := uuid.New()
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
			dbgen.Preset(t, db, database.InsertPresetParams{
				ID:                presetID,
				TemplateVersionID: version.ID,
				DesiredInstances:  sql.NullInt32{Int32: 1, Valid: true},
			})
			workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:    database.PrebuildsSystemUserID,
				TemplateID: template.ID,
			}).Seed(database.WorkspaceBuild{
				TemplateVersionID: version.ID,
				TemplateVersionPresetID: uuid.NullUUID{
					UUID:  presetID,
					Valid: true,
				},
			}).WithAgent(func(agent []*proto.Agent) []*proto.Agent {
				return agent
			}).Do()

			// Mark the prebuilt workspace's agent as ready so the prebuild can be claimed
			ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
			agent, err := db.GetWorkspaceAgentAndLatestBuildByAuthToken(ctx, uuid.MustParse(workspaceBuild.AgentToken))
			require.NoError(t, err)
			err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agent.WorkspaceAgent.ID,
				LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			})
			require.NoError(t, err)

			// Given: a prebuilt workspace
			prebuild := coderdtest.MustWorkspace(t, client, workspaceBuild.Workspace.ID)

			// When: running the schedule command over a prebuilt workspace
			inv, root := clitest.New(t, tc.cmdArgs(prebuild.OwnerName+"/"+prebuild.Name)...)
			clitest.SetupConfig(t, client, root)
			ptytest.New(t).Attach(inv)
			doneChan := make(chan struct{})
			var runErr error
			go func() {
				defer close(doneChan)
				runErr = inv.Run()
			}()
			<-doneChan

			// Then: an error should be returned, with an error message specific to the lifecycle parameter
			require.Error(t, runErr)
			require.Contains(t, runErr.Error(), tc.cliErrorMsg)

			// Given: the prebuilt workspace is claimed by a user
			user, err := client.User(ctx, "testUser")
			require.NoError(t, err)
			claimedWorkspace, err := client.CreateUserWorkspace(ctx, user.ID.String(), codersdk.CreateWorkspaceRequest{
				TemplateVersionID:       version.ID,
				TemplateVersionPresetID: presetID,
				Name:                    coderdtest.RandomUsername(t),
				// The 'extend' command requires the workspace to have an existing deadline.
				// To ensure this, we set the workspace's TTL to 1 hour.
				TTLMillis: ptr.Ref[int64](time.Hour.Milliseconds()),
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, claimedWorkspace.LatestBuild.ID)
			workspace := coderdtest.MustWorkspace(t, client, claimedWorkspace.ID)
			require.Equal(t, prebuild.ID, workspace.ID)

			// When: running the schedule command over the claimed workspace
			inv, root = clitest.New(t, tc.cmdArgs(workspace.OwnerName+"/"+workspace.Name)...)
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t).Attach(inv)
			require.NoError(t, inv.Run())

			// Then: the updated schedule should be shown
			pty.ExpectMatch(workspace.OwnerName + "/" + workspace.Name)
		})
	}
}
