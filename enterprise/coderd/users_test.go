package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

const TimeFormatHHMM = coderd.TimeFormatHHMM

func TestUserQuietHours(t *testing.T) {
	t.Parallel()

	t.Run("DefaultToUTC", func(t *testing.T) {
		t.Parallel()

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		client, user := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.UserQuietHoursSchedule(ctx, user.ID.String())
		require.NoError(t, err)
		require.Equal(t, "UTC", res.Timezone)
		require.Equal(t, "00:00", res.Time)
		require.Equal(t, "CRON_TZ=UTC 0 0 * * *", res.RawSchedule)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		// Using 10 for minutes lets us test a format bug in which values greater
		// than 5 were causing the API to explode because the time was returned
		// incorrectly
		defaultQuietHoursSchedule := "CRON_TZ=America/Chicago 10 1 * * *"
		defaultScheduleParsed, err := cron.Daily(defaultQuietHoursSchedule)
		require.NoError(t, err)
		nextTime := defaultScheduleParsed.Next(time.Now().In(defaultScheduleParsed.Location()))
		if time.Until(nextTime) < time.Hour {
			// Use a different default schedule instead, because we want to avoid
			// the schedule "ticking over" during this test run.
			defaultQuietHoursSchedule = "CRON_TZ=America/Chicago 10 13 * * *"
			defaultScheduleParsed, err = cron.Daily(defaultQuietHoursSchedule)
			require.NoError(t, err)
		}

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set(defaultQuietHoursSchedule)

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		// Do it with another user to make sure that we're not hitting RBAC
		// errors.
		client, user := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

		// Get quiet hours for a user that doesn't have them set.
		ctx := testutil.Context(t, testutil.WaitLong)
		sched1, err := client.UserQuietHoursSchedule(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, defaultScheduleParsed.String(), sched1.RawSchedule)
		require.False(t, sched1.UserSet)
		require.Equal(t, defaultScheduleParsed.TimeParsed().Format(TimeFormatHHMM), sched1.Time)
		require.Equal(t, defaultScheduleParsed.Location().String(), sched1.Timezone)
		require.WithinDuration(t, defaultScheduleParsed.Next(time.Now()), sched1.Next, 15*time.Second)

		// Set their quiet hours.
		customQuietHoursSchedule := "CRON_TZ=Australia/Sydney 0 0 * * *"
		customScheduleParsed, err := cron.Daily(customQuietHoursSchedule)
		require.NoError(t, err)
		nextTime = customScheduleParsed.Next(time.Now().In(customScheduleParsed.Location()))
		if time.Until(nextTime) < time.Hour {
			// Use a different default schedule instead, because we want to avoid
			// the schedule "ticking over" during this test run.
			customQuietHoursSchedule = "CRON_TZ=Australia/Sydney 0 12 * * *"
			customScheduleParsed, err = cron.Daily(customQuietHoursSchedule)
			require.NoError(t, err)
		}

		sched2, err := client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: customQuietHoursSchedule,
		})
		require.NoError(t, err)
		require.Equal(t, customScheduleParsed.String(), sched2.RawSchedule)
		require.True(t, sched2.UserSet)
		require.Equal(t, customScheduleParsed.TimeParsed().Format(TimeFormatHHMM), sched2.Time)
		require.Equal(t, customScheduleParsed.Location().String(), sched2.Timezone)
		require.WithinDuration(t, customScheduleParsed.Next(time.Now()), sched2.Next, 15*time.Second)

		// Get quiet hours for a user that has them set.
		sched3, err := client.UserQuietHoursSchedule(ctx, user.ID.String())
		require.NoError(t, err)
		require.Equal(t, customScheduleParsed.String(), sched3.RawSchedule)
		require.True(t, sched3.UserSet)
		require.Equal(t, customScheduleParsed.TimeParsed().Format(TimeFormatHHMM), sched3.Time)
		require.Equal(t, customScheduleParsed.Location().String(), sched3.Timezone)
		require.WithinDuration(t, customScheduleParsed.Next(time.Now()), sched3.Next, 15*time.Second)

		// Try setting a garbage schedule.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "garbage",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse daily schedule")

		// Try setting a non-daily schedule.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 0 0 * * 1",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse daily schedule")

		// Try setting a schedule with a timezone that doesn't exist.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=Deans/House 0 0 * * *",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse daily schedule")

		// Try setting a schedule with more than one time.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 0 0,12 * * *",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "more than one time")
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 0-30 0 * * *",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "more than one time")

		// We don't allow unsetting the custom schedule so we don't need to worry
		// about it in this test.
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set("CRON_TZ=America/Chicago 0 0 * * *")

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					// Not entitled.
					// codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // We want to test the lack of entitlement, not RBAC.
		_, err := client.UserQuietHoursSchedule(ctx, user.UserID.String())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("UserCannotSet", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set("CRON_TZ=America/Chicago 0 0 * * *")
		dv.UserQuietHoursSchedule.AllowUserCustom.Set("false")

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		// Do it with another user to make sure that we're not hitting RBAC
		// errors.
		client, user := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

		// Get the schedule
		ctx := testutil.Context(t, testutil.WaitLong)
		sched, err := client.UserQuietHoursSchedule(ctx, user.ID.String())
		require.NoError(t, err)
		require.Equal(t, "CRON_TZ=America/Chicago 0 0 * * *", sched.RawSchedule)
		require.False(t, sched.UserSet)
		require.False(t, sched.UserCanSet)

		// Try to set
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 30 2 * * *",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "cannot set custom quiet hours schedule")
	})
}

func TestCreateFirstUser_Entitlements_Trial(t *testing.T) {
	t.Parallel()

	adminClient, _ := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Trial: true,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	//nolint:gocritic // we need the first user so admin
	entitlements, err := adminClient.Entitlements(ctx)
	require.NoError(t, err)
	require.True(t, entitlements.Trial, "Trial license should be immediately active.")
}

// TestAssignCustomOrgRoles verifies an organization admin (not just an owner) can create
// a custom role and assign it to an organization user.
func TestAssignCustomOrgRoles(t *testing.T) {
	t.Parallel()

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles: 1,
			},
		},
	})

	client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	tv := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, tv.ID)

	ctx := testutil.Context(t, testutil.WaitShort)
	// Create a custom role as an organization admin that allows making templates.
	auditorRole, err := client.CreateOrganizationRole(ctx, codersdk.Role{
		Name:            "org-template-admin",
		OrganizationID:  owner.OrganizationID.String(),
		DisplayName:     "Template Admin",
		SitePermissions: nil,
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceTemplate: codersdk.RBACResourceActions[codersdk.ResourceTemplate], // All template perms
		}),
		UserPermissions: nil,
	})
	require.NoError(t, err)

	createTemplateReq := codersdk.CreateTemplateRequest{
		Name:        "name",
		DisplayName: "Template",
		VersionID:   tv.ID,
	}
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	// Check the member cannot create a template
	_, err = memberClient.CreateTemplate(ctx, owner.OrganizationID, createTemplateReq)
	require.Error(t, err)

	// Assign new role to the member as the org admin
	_, err = client.UpdateOrganizationMemberRoles(ctx, owner.OrganizationID, member.ID.String(), codersdk.UpdateRoles{
		Roles: []string{auditorRole.Name},
	})
	require.NoError(t, err)

	// Now the member can create the template
	_, err = memberClient.CreateTemplate(ctx, owner.OrganizationID, createTemplateReq)
	require.NoError(t, err)
}

func TestGrantSiteRoles(t *testing.T) {
	t.Parallel()

	requireStatusCode := func(t *testing.T, err error, statusCode int) {
		t.Helper()
		var e *codersdk.Error
		require.ErrorAs(t, err, &e, "error is codersdk error")
		require.Equal(t, statusCode, e.StatusCode(), "correct status code")
	}

	dv := coderdtest.DeploymentValues(t)
	admin, first := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations:      1,
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})

	member, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
	randOrg := coderdenttest.CreateOrganization(t, admin, coderdenttest.CreateOrganizationOptions{})

	_, randOrgUser := coderdtest.CreateAnotherUser(t, admin, randOrg.ID, rbac.ScopedRoleOrgAdmin(randOrg.ID))
	userAdmin, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID, rbac.RoleUserAdmin())

	const newUser = "newUser"

	testCases := []struct {
		Name          string
		Client        *codersdk.Client
		OrgID         uuid.UUID
		AssignToUser  string
		Roles         []string
		ExpectedRoles []string
		Error         bool
		StatusCode    int
	}{
		{
			Name:         "OrgRoleInSite",
			Client:       admin,
			AssignToUser: codersdk.Me,
			Roles:        []string{rbac.RoleOrgAdmin()},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "UserNotExists",
			Client:       admin,
			AssignToUser: uuid.NewString(),
			Roles:        []string{codersdk.RoleOwner},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "MemberCannotUpdateRoles",
			Client:       member,
			AssignToUser: first.UserID.String(),
			Roles:        []string{},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			// Cannot update your own roles
			Name:         "AdminOnSelf",
			Client:       admin,
			AssignToUser: first.UserID.String(),
			Roles:        []string{},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "SiteRoleInOrg",
			Client:       admin,
			OrgID:        first.OrganizationID,
			AssignToUser: codersdk.Me,
			Roles:        []string{codersdk.RoleOwner},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "RoleInNotMemberOrg",
			Client:       orgAdmin,
			OrgID:        randOrg.ID,
			AssignToUser: randOrgUser.ID.String(),
			Roles:        []string{rbac.RoleOrgMember()},
			Error:        true,
			StatusCode:   http.StatusNotFound,
		},
		{
			Name:         "AdminUpdateOrgSelf",
			Client:       admin,
			OrgID:        first.OrganizationID,
			AssignToUser: first.UserID.String(),
			Roles:        []string{},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "OrgAdminPromote",
			Client:       orgAdmin,
			OrgID:        first.OrganizationID,
			AssignToUser: newUser,
			Roles:        []string{rbac.RoleOrgAdmin()},
			ExpectedRoles: []string{
				rbac.RoleOrgAdmin(),
			},
			Error: false,
		},
		{
			Name:         "UserAdminMakeMember",
			Client:       userAdmin,
			AssignToUser: newUser,
			Roles:        []string{codersdk.RoleMember},
			ExpectedRoles: []string{
				codersdk.RoleMember,
			},
			Error: false,
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			var err error
			if c.AssignToUser == newUser {
				orgID := first.OrganizationID
				if c.OrgID != uuid.Nil {
					orgID = c.OrgID
				}
				_, newUser := coderdtest.CreateAnotherUser(t, admin, orgID)
				c.AssignToUser = newUser.ID.String()
			}

			var newRoles []codersdk.SlimRole
			if c.OrgID != uuid.Nil {
				// Org assign
				var mem codersdk.OrganizationMember
				mem, err = c.Client.UpdateOrganizationMemberRoles(ctx, c.OrgID, c.AssignToUser, codersdk.UpdateRoles{
					Roles: c.Roles,
				})
				newRoles = mem.Roles
			} else {
				// Site assign
				var user codersdk.User
				user, err = c.Client.UpdateUserRoles(ctx, c.AssignToUser, codersdk.UpdateRoles{
					Roles: c.Roles,
				})
				newRoles = user.Roles
			}

			if c.Error {
				require.Error(t, err)
				requireStatusCode(t, err, c.StatusCode)
			} else {
				require.NoError(t, err)
				roles := make([]string, 0, len(newRoles))
				for _, r := range newRoles {
					roles = append(roles, r.Name)
				}
				require.ElementsMatch(t, roles, c.ExpectedRoles)
			}
		})
	}
}

func TestEnterprisePostUser(t *testing.T) {
	t.Parallel()

	t.Run("OrganizationNoAccess", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		notInOrg, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner(), rbac.RoleMember())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		org := coderdenttest.CreateOrganization(t, other, coderdenttest.CreateOrganizationOptions{}, func(request *codersdk.CreateOrganizationRequest) {
			request.Name = "another"
		})

		_, err := notInOrg.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "some@domain.com",
			Username:        "anotheruser",
			Password:        "SomeSecurePassword!",
			OrganizationIDs: []uuid.UUID{org.ID},
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("OrganizationNoAccess", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		notInOrg, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner(), rbac.RoleMember())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		org := coderdenttest.CreateOrganization(t, other, coderdenttest.CreateOrganizationOptions{})

		_, err := notInOrg.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "some@domain.com",
			Username:        "anotheruser",
			Password:        "SomeSecurePassword!",
			OrganizationIDs: []uuid.UUID{org.ID},
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("CreateWithoutOrg", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Add an extra org to try and confuse user creation
		coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		// nolint:gocritic // intentional using the owner.
		// Manually making a user with the request instead of the coderdtest util
		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:    "another@user.org",
			Username: "someone-else",
			Password: "SomeSecurePassword!",
		})
		require.ErrorContains(t, err, "No organization specified")
	})

	t.Run("MultipleOrganizations", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		// Add an extra org to assign member into
		second := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
		third := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// nolint:gocritic // intentional using the owner.
		// Manually making a user with the request instead of the coderdtest util
		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:    "another@user.org",
			Username: "someone-else",
			Password: "SomeSecurePassword!",
			OrganizationIDs: []uuid.UUID{
				second.ID,
				third.ID,
			},
		})
		require.NoError(t, err)

		memberedOrgs, err := client.OrganizationsByUser(ctx, user.ID.String())
		require.NoError(t, err)
		require.Len(t, memberedOrgs, 2)
		require.ElementsMatch(t, []uuid.UUID{second.ID, third.ID}, []uuid.UUID{memberedOrgs[0].ID, memberedOrgs[1].ID})
	})
}
