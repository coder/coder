package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplates(t *testing.T) {
	t.Parallel()

	// TODO(@dean): remove legacy max_ttl tests
	t.Run("CreateUpdateWorkspaceMaxTTL", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		exp := 24 * time.Hour.Milliseconds()
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = &exp
			ctr.MaxTTLMillis = &exp
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// No TTL provided should use template default
		req := codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "testing",
		}
		ws, err := anotherClient.CreateWorkspace(ctx, template.OrganizationID, codersdk.Me, req)
		require.NoError(t, err)
		require.NotNil(t, ws.TTLMillis)
		require.EqualValues(t, exp, *ws.TTLMillis)

		// Editing a workspace to have a higher TTL than the template's max
		// should error
		exp = exp + time.Minute.Milliseconds()
		err = anotherClient.UpdateWorkspaceTTL(ctx, ws.ID, codersdk.UpdateWorkspaceTTLRequest{
			TTLMillis: &exp,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Len(t, apiErr.Validations, 1)
		require.Equal(t, apiErr.Validations[0].Field, "ttl_ms")
		require.Contains(t, apiErr.Validations[0].Detail, "time until shutdown must be less than or equal to the template's maximum TTL")

		// Creating workspace with TTL higher than max should error
		req.Name = "testing2"
		req.TTLMillis = &exp
		ws, err = anotherClient.CreateWorkspace(ctx, template.OrganizationID, codersdk.Me, req)
		require.Error(t, err)
		apiErr = nil
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Len(t, apiErr.Validations, 1)
		require.Equal(t, apiErr.Validations[0].Field, "ttl_ms")
		require.Contains(t, apiErr.Validations[0].Detail, "time until shutdown must be less than or equal to the template's maximum TTL")
	})

	t.Run("Deprecated", func(t *testing.T) {
		t.Parallel()

		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAccessControl: 1,
				},
			},
		})
		client, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			DeprecationMessage: ptr.Ref("Stop using this template"),
		})
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		// AGPL cannot deprecate, expect no change
		assert.True(t, updated.Deprecated)
		assert.NotEmpty(t, updated.DeprecationMessage)

		_, err = client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "foobar",
		})
		require.ErrorContains(t, err, "deprecated")

		// Unset deprecated and try again
		updated, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{DeprecationMessage: ptr.Ref("")})
		require.NoError(t, err)
		assert.False(t, updated.Deprecated)
		assert.Empty(t, updated.DeprecationMessage)

		_, err = client.CreateWorkspace(ctx, user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "foobar",
		})
		require.NoError(t, err)
	})

	t.Run("MaxPortShareLevel", func(t *testing.T) {
		t.Parallel()

		cfg := coderdtest.DeploymentValues(t)
		cfg.Experiments = []string{"shared-ports"}
		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				DeploymentValues:         cfg,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureControlSharedPorts: 1,
				},
			},
		})
		client, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "example",
					},
				},
			}, {
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
							Agents: []*proto.Agent{{
								Id: "something",
								Auth: &proto.Agent_Token{
									Token: uuid.NewString(),
								},
								Name: "test",
							}},
						}, {
							Name: "another",
							Type: "example",
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws, err := client.Workspace(context.Background(), ws.ID)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// OK
		var level codersdk.WorkspaceAgentPortShareLevel = codersdk.WorkspaceAgentPortShareLevelPublic
		updated, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			MaxPortShareLevel: &level,
		})
		require.NoError(t, err)
		assert.Equal(t, level, updated.MaxPortShareLevel)

		// Invalid level
		level = "invalid"
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			MaxPortShareLevel: &level,
		})
		require.ErrorContains(t, err, "invalid max port sharing level")

		// Create public port share
		_, err = client.UpsertWorkspaceAgentPortShare(ctx, ws.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
			AgentName:  ws.LatestBuild.Resources[0].Agents[0].Name,
			Port:       8080,
			ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
		})
		require.NoError(t, err)

		// Reduce max level to authenticated
		level = codersdk.WorkspaceAgentPortShareLevelAuthenticated
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			MaxPortShareLevel: &level,
		})
		require.NoError(t, err)

		// Ensure previously public port is now authenticated
		wpsr, err := client.GetWorkspaceAgentPortShares(ctx, ws.ID)
		require.NoError(t, err)
		require.Len(t, wpsr.Shares, 1)
		assert.Equal(t, codersdk.WorkspaceAgentPortShareLevelAuthenticated, wpsr.Shares[0].ShareLevel)

		// reduce max level to owner
		level = codersdk.WorkspaceAgentPortShareLevelOwner
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			MaxPortShareLevel: &level,
		})
		require.NoError(t, err)

		// Ensure previously authenticated port is removed
		wpsr, err = client.GetWorkspaceAgentPortShares(ctx, ws.ID)
		require.NoError(t, err)
		require.Empty(t, wpsr.Shares)
	})

	t.Run("BlockDisablingAutoOffWithMaxTTL", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		exp := 24 * time.Hour.Milliseconds()
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MaxTTLMillis = &exp
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// No TTL provided should use template default
		req := codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "testing",
		}
		ws, err := anotherClient.CreateWorkspace(ctx, template.OrganizationID, codersdk.Me, req)
		require.NoError(t, err)
		require.NotNil(t, ws.TTLMillis)
		require.EqualValues(t, exp, *ws.TTLMillis)

		// Editing a workspace to disable the TTL should do nothing
		err = anotherClient.UpdateWorkspaceTTL(ctx, ws.ID, codersdk.UpdateWorkspaceTTLRequest{
			TTLMillis: nil,
		})
		require.NoError(t, err)
		ws, err = anotherClient.Workspace(ctx, ws.ID)
		require.NoError(t, err)
		require.EqualValues(t, exp, *ws.TTLMillis)

		// Editing a workspace to have a TTL of 0 should do nothing
		zero := int64(0)
		err = anotherClient.UpdateWorkspaceTTL(ctx, ws.ID, codersdk.UpdateWorkspaceTTLRequest{
			TTLMillis: &zero,
		})
		require.NoError(t, err)
		ws, err = anotherClient.Workspace(ctx, ws.ID)
		require.NoError(t, err)
		require.EqualValues(t, exp, *ws.TTLMillis)
	})

	t.Run("SetAutostartRequirement", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		require.Equal(t, []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}, template.AutostartRequirement.DaysOfWeek)

		ctx := testutil.Context(t, testutil.WaitLong)
		updated, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			Name:        template.Name,
			DisplayName: template.DisplayName,
			Description: template.Description,
			Icon:        template.Icon,
			AutostartRequirement: &codersdk.TemplateAutostartRequirement{
				DaysOfWeek: []string{"monday", "saturday"},
			},
		})
		require.NoError(t, err)
		require.Equal(t, []string{"monday", "saturday"}, updated.AutostartRequirement.DaysOfWeek)

		template, err = anotherClient.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, []string{"monday", "saturday"}, template.AutostartRequirement.DaysOfWeek)

		// Ensure a missing field is a noop
		updated, err = anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			Name:        template.Name,
			DisplayName: template.DisplayName,
			Description: template.Description,
			Icon:        template.Icon + "something",
		})
		require.NoError(t, err)
		require.Equal(t, []string{"monday", "saturday"}, updated.AutostartRequirement.DaysOfWeek)

		template, err = anotherClient.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, []string{"monday", "saturday"}, template.AutostartRequirement.DaysOfWeek)
		require.Empty(t, template.DeprecationMessage)
		require.False(t, template.Deprecated)
	})

	t.Run("SetInvalidAutostartRequirement", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		require.Equal(t, []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}, template.AutostartRequirement.DaysOfWeek)

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			Name:        template.Name,
			DisplayName: template.DisplayName,
			Description: template.Description,
			Icon:        template.Icon,
			AutostartRequirement: &codersdk.TemplateAutostartRequirement{
				DaysOfWeek: []string{"foobar", "saturday"},
			},
		})
		require.Error(t, err)
		require.Empty(t, template.DeprecationMessage)
		require.False(t, template.Deprecated)
	})

	t.Run("SetAutostopRequirement", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		require.Empty(t, 0, template.AutostopRequirement.DaysOfWeek)
		require.EqualValues(t, 1, template.AutostopRequirement.Weeks)

		ctx := context.Background()
		updated, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			Name:                         template.Name,
			DisplayName:                  template.DisplayName,
			Description:                  template.Description,
			Icon:                         template.Icon,
			AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
			DefaultTTLMillis:             time.Hour.Milliseconds(),
			AutostopRequirement: &codersdk.TemplateAutostopRequirement{
				DaysOfWeek: []string{"monday", "saturday"},
				Weeks:      3,
			},
		})
		require.NoError(t, err)
		require.Equal(t, []string{"monday", "saturday"}, updated.AutostopRequirement.DaysOfWeek)
		require.EqualValues(t, 3, updated.AutostopRequirement.Weeks)

		template, err = anotherClient.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, []string{"monday", "saturday"}, template.AutostopRequirement.DaysOfWeek)
		require.EqualValues(t, 3, template.AutostopRequirement.Weeks)
		require.Empty(t, template.DeprecationMessage)
		require.False(t, template.Deprecated)
	})

	t.Run("CleanupTTLs", func(t *testing.T) {
		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			client, user := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureAdvancedTemplateScheduling: 1,
					},
				},
			})
			anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			require.EqualValues(t, 0, template.TimeTilDormantMillis)
			require.EqualValues(t, 0, template.FailureTTLMillis)
			require.EqualValues(t, 0, template.TimeTilDormantAutoDeleteMillis)

			var (
				failureTTL    = 1 * time.Minute
				inactivityTTL = 2 * time.Minute
				dormantTTL    = 3 * time.Minute
			)

			updated, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                           template.Name,
				DisplayName:                    template.DisplayName,
				Description:                    template.Description,
				Icon:                           template.Icon,
				AllowUserCancelWorkspaceJobs:   template.AllowUserCancelWorkspaceJobs,
				TimeTilDormantMillis:           inactivityTTL.Milliseconds(),
				FailureTTLMillis:               failureTTL.Milliseconds(),
				TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
			})
			require.NoError(t, err)
			require.Equal(t, failureTTL.Milliseconds(), updated.FailureTTLMillis)
			require.Equal(t, inactivityTTL.Milliseconds(), updated.TimeTilDormantMillis)
			require.Equal(t, dormantTTL.Milliseconds(), updated.TimeTilDormantAutoDeleteMillis)

			// Validate fetching the template returns the same values as updating
			// the template.
			template, err = anotherClient.Template(ctx, template.ID)
			require.NoError(t, err)
			require.Equal(t, failureTTL.Milliseconds(), updated.FailureTTLMillis)
			require.Equal(t, inactivityTTL.Milliseconds(), updated.TimeTilDormantMillis)
			require.Equal(t, dormantTTL.Milliseconds(), updated.TimeTilDormantAutoDeleteMillis)
		})

		t.Run("BadRequest", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			client, user := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureAdvancedTemplateScheduling: 1,
					},
				},
			})
			anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

			type testcase struct {
				Name                string
				TimeTilDormantMS    int64
				FailureTTLMS        int64
				DormantAutoDeleteMS int64
			}

			cases := []testcase{
				{
					Name:                "NegativeValue",
					TimeTilDormantMS:    -1,
					FailureTTLMS:        -2,
					DormantAutoDeleteMS: -3,
				},
				{
					Name:                "ValueTooSmall",
					TimeTilDormantMS:    1,
					FailureTTLMS:        999,
					DormantAutoDeleteMS: 500,
				},
			}

			for _, c := range cases {
				c := c

				// nolint: paralleltest // context is from parent t.Run
				t.Run(c.Name, func(t *testing.T) {
					_, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
						Name:                           template.Name,
						DisplayName:                    template.DisplayName,
						Description:                    template.Description,
						Icon:                           template.Icon,
						AllowUserCancelWorkspaceJobs:   template.AllowUserCancelWorkspaceJobs,
						TimeTilDormantMillis:           c.TimeTilDormantMS,
						FailureTTLMillis:               c.FailureTTLMS,
						TimeTilDormantAutoDeleteMillis: c.DormantAutoDeleteMS,
					})
					require.Error(t, err)
					cerr, ok := codersdk.AsError(err)
					require.True(t, ok)
					require.Len(t, cerr.Validations, 3)
					require.Equal(t, "Value must be at least one minute.", cerr.Validations[0].Detail)
				})
			}
		})
	})

	t.Run("UpdateTimeTilDormantAutoDelete", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		activeWS := coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		dormantWS := coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		require.Nil(t, activeWS.DeletingAt)
		require.Nil(t, dormantWS.DeletingAt)

		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, activeWS.LatestBuild.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, dormantWS.LatestBuild.ID)

		err := anotherClient.UpdateWorkspaceDormancy(ctx, dormantWS.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		dormantWS = coderdtest.MustWorkspace(t, client, dormantWS.ID)
		require.NotNil(t, dormantWS.DormantAt)
		// The deleting_at field should be nil since there is no template time_til_dormant_autodelete set.
		require.Nil(t, dormantWS.DeletingAt)

		dormantTTL := time.Minute
		updated, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
		})
		require.NoError(t, err)
		require.Equal(t, dormantTTL.Milliseconds(), updated.TimeTilDormantAutoDeleteMillis)

		activeWS = coderdtest.MustWorkspace(t, client, activeWS.ID)
		require.Nil(t, activeWS.DormantAt)
		require.Nil(t, activeWS.DeletingAt)

		updatedDormantWorkspace := coderdtest.MustWorkspace(t, client, dormantWS.ID)
		require.NotNil(t, updatedDormantWorkspace.DormantAt)
		require.NotNil(t, updatedDormantWorkspace.DeletingAt)
		require.Equal(t, updatedDormantWorkspace.DormantAt.Add(dormantTTL), *updatedDormantWorkspace.DeletingAt)
		require.Equal(t, updatedDormantWorkspace.DormantAt, dormantWS.DormantAt)

		// Disable the time_til_dormant_auto_delete on the template, then we can assert that the workspaces
		// no longer have a deleting_at field.
		updated, err = anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: 0,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, updated.TimeTilDormantAutoDeleteMillis)

		// The active workspace should remain unchanged.
		activeWS = coderdtest.MustWorkspace(t, client, activeWS.ID)
		require.Nil(t, activeWS.DormantAt)
		require.Nil(t, activeWS.DeletingAt)

		// Fetch the dormant workspace. It should still be dormant, but it should no
		// longer be scheduled for deletion.
		dormantWS = coderdtest.MustWorkspace(t, client, dormantWS.ID)
		require.NotNil(t, dormantWS.DormantAt)
		require.Nil(t, dormantWS.DeletingAt)
	})

	t.Run("UpdateDormantAt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		activeWS := coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		dormantWS := coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		require.Nil(t, activeWS.DeletingAt)
		require.Nil(t, dormantWS.DeletingAt)

		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, activeWS.LatestBuild.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, dormantWS.LatestBuild.ID)

		err := anotherClient.UpdateWorkspaceDormancy(ctx, dormantWS.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		dormantWS = coderdtest.MustWorkspace(t, client, dormantWS.ID)
		require.NotNil(t, dormantWS.DormantAt)
		// The deleting_at field should be nil since there is no template time_til_dormant_autodelete set.
		require.Nil(t, dormantWS.DeletingAt)

		dormantTTL := time.Minute
		//nolint:gocritic // non-template-admin cannot update template meta
		updated, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
			UpdateWorkspaceDormantAt:       true,
		})
		require.NoError(t, err)
		require.Equal(t, dormantTTL.Milliseconds(), updated.TimeTilDormantAutoDeleteMillis)

		activeWS = coderdtest.MustWorkspace(t, client, activeWS.ID)
		require.Nil(t, activeWS.DormantAt)
		require.Nil(t, activeWS.DeletingAt)

		updatedDormantWorkspace := coderdtest.MustWorkspace(t, client, dormantWS.ID)
		require.NotNil(t, updatedDormantWorkspace.DormantAt)
		require.NotNil(t, updatedDormantWorkspace.DeletingAt)
		// Validate that the workspace dormant_at value is updated.
		require.True(t, updatedDormantWorkspace.DormantAt.After(*dormantWS.DormantAt))
		require.Equal(t, updatedDormantWorkspace.DormantAt.Add(dormantTTL), *updatedDormantWorkspace.DeletingAt)
	})

	t.Run("UpdateLastUsedAt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		activeWorkspace := coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		dormantWorkspace := coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		require.Nil(t, activeWorkspace.DeletingAt)
		require.Nil(t, dormantWorkspace.DeletingAt)

		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, activeWorkspace.LatestBuild.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, dormantWorkspace.LatestBuild.ID)

		err := anotherClient.UpdateWorkspaceDormancy(ctx, dormantWorkspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		dormantWorkspace = coderdtest.MustWorkspace(t, client, dormantWorkspace.ID)
		require.NotNil(t, dormantWorkspace.DormantAt)
		// The deleting_at field should be nil since there is no template time_til_dormant_autodelete set.
		require.Nil(t, dormantWorkspace.DeletingAt)

		inactivityTTL := time.Minute
		updated, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantMillis:      inactivityTTL.Milliseconds(),
			UpdateWorkspaceLastUsedAt: true,
		})
		require.NoError(t, err)
		require.Equal(t, inactivityTTL.Milliseconds(), updated.TimeTilDormantMillis)

		updatedActiveWS := coderdtest.MustWorkspace(t, client, activeWorkspace.ID)
		require.Nil(t, updatedActiveWS.DormantAt)
		require.Nil(t, updatedActiveWS.DeletingAt)
		require.True(t, updatedActiveWS.LastUsedAt.After(activeWorkspace.LastUsedAt))

		updatedDormantWS := coderdtest.MustWorkspace(t, client, dormantWorkspace.ID)
		require.NotNil(t, updatedDormantWS.DormantAt)
		require.Nil(t, updatedDormantWS.DeletingAt)
		// Validate that the workspace dormant_at value is updated.
		require.Equal(t, updatedDormantWS.DormantAt, dormantWorkspace.DormantAt)
		require.True(t, updatedDormantWS.LastUsedAt.After(dormantWorkspace.LastUsedAt))
	})

	t.Run("RequireActiveVersion", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAccessControl: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.RequireActiveVersion = true
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		require.True(t, template.RequireActiveVersion)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Update the field and assert it persists.
		updatedTemplate, err := anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			RequireActiveVersion: false,
		})
		require.NoError(t, err)
		require.False(t, updatedTemplate.RequireActiveVersion)

		// Flip it back to ensure we aren't hardcoding to a default value.
		updatedTemplate, err = anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			RequireActiveVersion: true,
		})
		require.NoError(t, err)
		require.True(t, updatedTemplate.RequireActiveVersion)

		// Assert that fetching a template is no different from the response
		// when updating.
		template, err = anotherClient.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, updatedTemplate, template)
		require.Empty(t, template.DeprecationMessage)
		require.False(t, template.Deprecated)
	})

	// Create a template, remove the group, see if an owner can
	// still fetch the template.
	t.Run("GetOnEveryoneRemove", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore()),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAccessControl: 1,
					codersdk.FeatureTemplateRBAC:  1,
				},
			},
		})

		client, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: nil,
			GroupPerms: map[string]codersdk.TemplateRole{
				// OrgID is the everyone ID
				first.OrganizationID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err = owner.Template(ctx, template.ID)
		require.NoError(t, err)
	})
}

func TestTemplateACL(t *testing.T) {
	t.Parallel()

	t.Run("UserRoles", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		err := anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		templateUser2 := codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleUse,
		}

		templateUser3 := codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		}

		require.Len(t, acl.Users, 2)
		require.Contains(t, acl.Users, templateUser2)
		require.Contains(t, acl.Users, templateUser3)
	})

	t.Run("everyoneGroup", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		// Create a user to assert they aren't returned in the response.
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Groups[0].Members, 2)
		require.Len(t, acl.Users, 0)
	})

	t.Run("NoGroups", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		client1, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		//nolint:gocritic // non-template-admin cannot update template acl
		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		// User should be able to read template due to allUsers group.
		_, err = client1.Template(ctx, template.ID)
		require.NoError(t, err)

		allUsers := acl.Groups[0]

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				allUsers.ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		//nolint:gocritic // non-template-admin cannot update template acl
		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 0)
		require.Len(t, acl.Users, 0)

		// User should not be able to read template due to allUsers group being deleted.
		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})

	t.Run("DisableEveryoneGroupAccess", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		//nolint:gocritic // non-template-admin cannot get template acl
		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, 1, len(acl.Groups))
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			Name:                         template.Name,
			DisplayName:                  template.DisplayName,
			Description:                  template.Description,
			Icon:                         template.Icon,
			AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
			DisableEveryoneGroupAccess:   true,
		})
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, 0, len(acl.Groups), acl.Groups)
	})

	// Test that we do not return deleted users.
	t.Run("FilterDeletedUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())

		_, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		err := anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user1,
			Role: codersdk.TemplateRoleUse,
		})

		err = anotherClient.DeleteUser(ctx, user1.ID)
		require.NoError(t, err)

		acl, err = anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 0, "deleted users should be filtered")
	})

	// Test that we do not return suspended users.
	t.Run("FilterSuspendedUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())

		_, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		err := anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user1,
			Role: codersdk.TemplateRoleUse,
		})

		_, err = anotherClient.UpdateUserStatus(ctx, user1.ID.String(), codersdk.UserStatusSuspended)
		require.NoError(t, err)

		acl, err = anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 0, "suspended users should be filtered")
	})

	// Test that we do not return deleted groups.
	t.Run("FilterDeletedGroups", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		group, err := anotherClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "test",
		})
		require.NoError(t, err)

		err = anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				group.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		// Length should be 2 for test group and the implicit allUsers group.
		require.Len(t, acl.Groups, 2)

		require.Contains(t, acl.Groups, codersdk.TemplateGroup{
			Group: group,
			Role:  codersdk.TemplateRoleUse,
		})

		err = anotherClient.DeleteGroup(ctx, group.ID)
		require.NoError(t, err)

		acl, err = anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		// Length should be 1 for the allUsers group.
		require.Len(t, acl.Groups, 1)
		require.NotContains(t, acl.Groups, codersdk.TemplateGroup{
			Group: group,
			Role:  codersdk.TemplateRoleUse,
		})
	})

	t.Run("AdminCanPushVersions", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		client1, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // test setup
		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		data, err := echo.Tar(nil)
		require.NoError(t, err)
		file, err := client1.Upload(context.Background(), codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)

		_, err = client1.CreateTemplateVersion(ctx, user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "testme",
			TemplateID:    template.ID,
			FileID:        file.ID,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.Error(t, err)

		//nolint:gocritic // test setup
		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		_, err = client1.CreateTemplateVersion(ctx, user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "testme",
			TemplateID:    template.ID,
			FileID:        file.ID,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
	})
}

func TestUpdateTemplateACL(t *testing.T) {
	t.Parallel()

	t.Run("UserPerms", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		templateUser2 := codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleUse,
		}

		templateUser3 := codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		}

		require.Len(t, acl.Users, 2)
		require.Contains(t, acl.Users, templateUser2)
		require.Contains(t, acl.Users, templateUser3)
	})

	t.Run("Audit", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Auditor:                  auditor,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
					codersdk.FeatureAuditLog:     1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		numLogs := len(auditor.AuditLogs())

		req := codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				user.OrganizationID.String(): codersdk.TemplateRoleDeleted,
			},
		}
		err := anotherClient.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)
		numLogs++

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionWrite,
			ResourceID: template.ID,
		}))
	})

	t.Run("DeleteUser", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := anotherClient.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleUse,
		})
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		})

		req = codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleAdmin,
				user3.ID.String(): codersdk.TemplateRoleDeleted,
			},
		}

		err = anotherClient.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err = anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleAdmin,
		})

		require.NotContains(t, acl.Users, codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		})
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				"hi": "admin",
			},
		}

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // we're testing invalid UUID so testing RBAC is not relevant here.
		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				uuid.NewString(): "admin",
			},
		}

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // we're testing invalid user so testing RBAC is not relevant here.
		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("InvalidRole", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): "updater",
			},
		}

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // we're testing invalid role so testing RBAC is not relevant here.
		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("RegularUserCannotUpdatePerms", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		client1, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		client2, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
			},
		}

		ctx := testutil.Context(t, testutil.WaitLong)

		err := client1.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		req = codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleAdmin,
			},
		}

		err = client2.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusInternalServerError, cerr.StatusCode())
	})

	t.Run("RegularUserWithAdminCanUpdate", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		client1, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		client2, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleAdmin,
			},
		}

		ctx := testutil.Context(t, testutil.WaitLong)

		err := client1.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		// Should be able to see user 3
		available, err := client2.TemplateACLAvailable(ctx, template.ID)
		require.NoError(t, err)
		userFound := false
		for _, avail := range available.Users {
			if avail.ID == user3.ID {
				userFound = true
			}
		}
		require.True(t, userFound, "user not found in acl available")

		req = codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user3.ID.String(): codersdk.TemplateRoleUse,
			},
		}

		err = client2.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err := client2.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		found := false
		for _, u := range acl.Users {
			if u.ID == user3.ID {
				found = true
			}
		}
		require.True(t, found, "user not found in acl")
	})

	t.Run("allUsersGroup", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)
	})

	t.Run("CustomGroupHasAccess", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())

		client1, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// Create a group to add to the template.
		group, err := anotherClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "test",
		})
		require.NoError(t, err)

		// Check that the only current group is the allUsers group.
		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Len(t, acl.Groups, 1)

		// Update the template to only allow access to the 'test' group.
		err = anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				// The allUsers group shares the same ID as the organization.
				user.OrganizationID.String(): codersdk.TemplateRoleDeleted,
				group.ID.String():            codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		// Get the ACL list for the template and assert the test group is
		// present.
		acl, err = anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)
		require.Equal(t, group.ID, acl.Groups[0].ID)

		// Try to get the template as the regular user. This should
		// fail since we haven't been added to the template yet.
		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		// Patch the group to add the regular user.
		group, err = anotherClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String()},
		})
		require.NoError(t, err)
		require.Len(t, group.Members, 1)
		require.Equal(t, user1.ID, group.Members[0].ID)

		// Fetching the template should succeed since our group has view access.
		_, err = client1.Template(ctx, template.ID)
		require.NoError(t, err)
	})

	t.Run("NoAccess", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		client1, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		// User should be able to read template due to allUsers group.
		_, err = client1.Template(ctx, template.ID)
		require.NoError(t, err)

		allUsers := acl.Groups[0]

		err = anotherClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				allUsers.ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		acl, err = anotherClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 0)
		require.Len(t, acl.Users, 0)

		// User should not be able to read template due to allUsers group being deleted.
		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})
}

func TestReadFileWithTemplateUpdate(t *testing.T) {
	t.Parallel()
	t.Run("HasTemplateUpdate", func(t *testing.T) {
		t.Parallel()

		// Upload a file
		client, first := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // regular user cannot create file
		resp, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(make([]byte, 1024)))
		require.NoError(t, err)

		// Make a new user
		member, memberData := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		// Try to download file, this should fail
		_, _, err = member.Download(ctx, resp.ID)
		require.Error(t, err, "no template yet")

		// Make a new template version with the file
		version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil, func(request *codersdk.CreateTemplateVersionRequest) {
			request.FileID = resp.ID
		})
		template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)

		// Not in acl yet
		_, _, err = member.Download(ctx, resp.ID)
		require.Error(t, err, "not in acl yet")

		//nolint:gocritic // regular user cannot update template acl
		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				memberData.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		_, _, err = member.Download(ctx, resp.ID)
		require.NoError(t, err)
	})
}

// TestTemplateAccess tests the rego -> sql conversion. We need to implement
// this test on at least 1 table type to ensure that the conversion is correct.
// The rbac tests only assert against static SQL queries.
// This is a full rbac test of many of the common role combinations.
//
//nolint:tparallel
func TestTemplateAccess(t *testing.T) {
	t.Parallel()
	// TODO: This context is for all the subtests. Each subtest should have its
	// own context.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*3)
	t.Cleanup(cancel)

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureTemplateRBAC: 1,
		},
	}})

	type coderUser struct {
		*codersdk.Client
		User codersdk.User
	}

	type orgSetup struct {
		Admin         coderUser
		MemberInGroup coderUser
		MemberNoGroup coderUser

		DefaultTemplate codersdk.Template
		AllRead         codersdk.Template
		UserACL         codersdk.Template
		GroupACL        codersdk.Template

		Group codersdk.Group
		Org   codersdk.Organization
	}

	// Create the following users
	// - owner: Site wide owner
	// - template-admin
	// - org-admin (org 1)
	// - org-admin (org 2)
	// - org-member (org 1)
	// - org-member (org 2)

	// Create the following templates in each org
	// - template 1, default acls
	// - template 2, all_user read
	// - template 3, user_acl read for member
	// - template 4, group_acl read for groupMember

	templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

	makeTemplate := func(t *testing.T, client *codersdk.Client, orgID uuid.UUID, acl codersdk.UpdateTemplateACL) codersdk.Template {
		version := coderdtest.CreateTemplateVersion(t, client, orgID, nil)
		template := coderdtest.CreateTemplate(t, client, orgID, version.ID)

		err := client.UpdateTemplateACL(ctx, template.ID, acl)
		require.NoError(t, err, "failed to update template acl")

		return template
	}

	makeOrg := func(t *testing.T) orgSetup {
		// Make org
		orgName, err := cryptorand.String(5)
		require.NoError(t, err, "org name")

		// Make users
		newOrg, err := ownerClient.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{Name: orgName})
		require.NoError(t, err, "failed to create org")

		adminCli, adminUsr := coderdtest.CreateAnotherUser(t, ownerClient, newOrg.ID, rbac.RoleOrgAdmin(newOrg.ID))
		groupMemCli, groupMemUsr := coderdtest.CreateAnotherUser(t, ownerClient, newOrg.ID, rbac.RoleOrgMember(newOrg.ID))
		memberCli, memberUsr := coderdtest.CreateAnotherUser(t, ownerClient, newOrg.ID, rbac.RoleOrgMember(newOrg.ID))

		// Make group
		group, err := adminCli.CreateGroup(ctx, newOrg.ID, codersdk.CreateGroupRequest{
			Name: "SingleUser",
		})
		require.NoError(t, err, "failed to create group")

		group, err = adminCli.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{groupMemUsr.ID.String()},
		})
		require.NoError(t, err, "failed to add user to group")

		// Make templates

		return orgSetup{
			Admin:         coderUser{Client: adminCli, User: adminUsr},
			MemberInGroup: coderUser{Client: groupMemCli, User: groupMemUsr},
			MemberNoGroup: coderUser{Client: memberCli, User: memberUsr},
			Org:           newOrg,
			Group:         group,

			DefaultTemplate: makeTemplate(t, adminCli, newOrg.ID, codersdk.UpdateTemplateACL{
				GroupPerms: map[string]codersdk.TemplateRole{
					newOrg.ID.String(): codersdk.TemplateRoleDeleted,
				},
			}),
			AllRead: makeTemplate(t, adminCli, newOrg.ID, codersdk.UpdateTemplateACL{
				GroupPerms: map[string]codersdk.TemplateRole{
					newOrg.ID.String(): codersdk.TemplateRoleUse,
				},
			}),
			UserACL: makeTemplate(t, adminCli, newOrg.ID, codersdk.UpdateTemplateACL{
				GroupPerms: map[string]codersdk.TemplateRole{
					newOrg.ID.String(): codersdk.TemplateRoleDeleted,
				},
				UserPerms: map[string]codersdk.TemplateRole{
					memberUsr.ID.String(): codersdk.TemplateRoleUse,
				},
			}),
			GroupACL: makeTemplate(t, adminCli, newOrg.ID, codersdk.UpdateTemplateACL{
				GroupPerms: map[string]codersdk.TemplateRole{
					group.ID.String():  codersdk.TemplateRoleUse,
					newOrg.ID.String(): codersdk.TemplateRoleDeleted,
				},
			}),
		}
	}

	// Make 2 organizations
	orgs := []orgSetup{
		makeOrg(t),
		makeOrg(t),
	}

	testTemplateRead := func(t *testing.T, org orgSetup, usr *codersdk.Client, read []codersdk.Template) {
		found, err := usr.TemplatesByOrganization(ctx, org.Org.ID)
		if len(read) == 0 && err != nil {
			require.ErrorContains(t, err, "Resource not found")
			return
		}
		require.NoError(t, err, "failed to get templates")

		exp := make(map[uuid.UUID]codersdk.Template)
		for _, tmpl := range read {
			exp[tmpl.ID] = tmpl
		}

		for _, f := range found {
			if _, ok := exp[f.ID]; !ok {
				t.Errorf("found unexpected template %q", f.Name)
			}
			delete(exp, f.ID)
		}
		require.Len(t, exp, 0, "expected templates not found")
	}

	// nolint:paralleltest
	t.Run("OwnerReadAll", func(t *testing.T) {
		for _, o := range orgs {
			// Owners can read all templates in all orgs
			exp := []codersdk.Template{o.DefaultTemplate, o.AllRead, o.UserACL, o.GroupACL}
			testTemplateRead(t, o, ownerClient, exp)
		}
	})

	// nolint:paralleltest
	t.Run("TemplateAdminReadAll", func(t *testing.T) {
		for _, o := range orgs {
			// Template Admins can read all templates in all orgs
			exp := []codersdk.Template{o.DefaultTemplate, o.AllRead, o.UserACL, o.GroupACL}
			testTemplateRead(t, o, templateAdmin, exp)
		}
	})

	// nolint:paralleltest
	t.Run("OrgAdminReadAllTheirs", func(t *testing.T) {
		for i, o := range orgs {
			cli := o.Admin.Client
			// Only read their own org
			exp := []codersdk.Template{o.DefaultTemplate, o.AllRead, o.UserACL, o.GroupACL}
			testTemplateRead(t, o, cli, exp)

			other := orgs[(i+1)%len(orgs)]
			require.NotEqual(t, other.Org.ID, o.Org.ID, "this test needs at least 2 orgs")
			testTemplateRead(t, other, cli, []codersdk.Template{})
		}
	})

	// nolint:paralleltest
	t.Run("TestMemberNoGroup", func(t *testing.T) {
		for i, o := range orgs {
			cli := o.MemberNoGroup.Client
			// Only read their own org
			exp := []codersdk.Template{o.AllRead, o.UserACL}
			testTemplateRead(t, o, cli, exp)

			other := orgs[(i+1)%len(orgs)]
			require.NotEqual(t, other.Org.ID, o.Org.ID, "this test needs at least 2 orgs")
			testTemplateRead(t, other, cli, []codersdk.Template{})
		}
	})

	// nolint:paralleltest
	t.Run("TestMemberInGroup", func(t *testing.T) {
		for i, o := range orgs {
			cli := o.MemberInGroup.Client
			// Only read their own org
			exp := []codersdk.Template{o.AllRead, o.GroupACL}
			testTemplateRead(t, o, cli, exp)

			other := orgs[(i+1)%len(orgs)]
			require.NotEqual(t, other.Org.ID, o.Org.ID, "this test needs at least 2 orgs")
			testTemplateRead(t, other, cli, []codersdk.Template{})
		}
	})
}
