package license_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestCountWorkspaceCapableUsers verifies workspace-capable license seat
// counting: only users the RBAC engine authorizes to create workspaces
// consume seats, so members without workspace-create ("gateway accounts")
// are excluded.
func TestCountWorkspaceCapableUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	authorizer := rbac.NewCachingAuthorizer(prometheus.NewRegistry())

	activeUser := func(t *testing.T, db database.Store, seed database.User) database.User {
		seed.Status = database.UserStatusActive
		return dbgen.User(t, db, seed)
	}
	member := func(t *testing.T, db database.Store, orgID uuid.UUID, user database.User, roles ...string) {
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: orgID,
			UserID:         user.ID,
			Roles:          roles,
		})
	}
	emptyDefaultRoles := func(t *testing.T, db database.Store, org database.Organization) {
		_, err := db.UpdateOrganization(ctx, database.UpdateOrganizationParams{
			ID:                    org.ID,
			UpdatedAt:             dbtime.Now(),
			Name:                  org.Name,
			DisplayName:           org.DisplayName,
			Description:           org.Description,
			Icon:                  org.Icon,
			DefaultOrgMemberRoles: []string{},
		})
		require.NoError(t, err)
	}

	t.Run("DefaultRolesParity", func(t *testing.T) {
		t.Parallel()
		// Orgs keep the default default_org_member_roles, which include
		// organization-workspace-access, so every active org member counts
		// and the workspace-capable count matches the legacy count except
		// for zero-org plain members.
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		plainMember := activeUser(t, db, database.User{})
		member(t, db, org.ID, plainMember)

		orgAdmin := activeUser(t, db, database.User{})
		member(t, db, org.ID, orgAdmin, rbac.RoleOrgAdmin())

		owner := activeUser(t, db, database.User{RBACRoles: []string{rbac.RoleOwner().Name}})
		member(t, db, org.ID, owner)

		// Counts under legacy, not under workspace-capable counting: no org, no
		// workspace-create anywhere.
		activeUser(t, db, database.User{})

		// Counts under both: the owner site role grants workspace-create
		// in any organization, independent of membership.
		activeUser(t, db, database.User{RBACRoles: []string{rbac.RoleOwner().Name}})

		// Never counted: not active.
		suspended := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
		member(t, db, org.ID, suspended)
		dormant := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
		member(t, db, org.ID, dormant)

		// Never counted: service accounts are excluded from seat counts.
		sa := activeUser(t, db, database.User{IsServiceAccount: true})
		member(t, db, org.ID, sa)

		legacy, err := db.GetActiveUserCount(ctx, false)
		require.NoError(t, err)
		require.Equal(t, int64(5), legacy)

		count, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), db, authorizer)
		require.NoError(t, err)
		require.Equal(t, int64(4), count, "zero-org plain member must not count")
	})

	t.Run("EmptyDefaultRoles", func(t *testing.T) {
		t.Parallel()
		// organization-member carries no workspace permissions on its
		// own. With default_org_member_roles cleared, workspace-create
		// flows exclusively through an explicit
		// organization-workspace-access grant.
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, org)

		// Gateway account: floor only, no workspace-create. Not counted.
		gateway := activeUser(t, db, database.User{})
		member(t, db, org.ID, gateway)

		// Explicit organization-workspace-access grant. Counted.
		wsUser := activeUser(t, db, database.User{})
		member(t, db, org.ID, wsUser, rbac.RoleOrgWorkspaceAccess())

		// The creation ban negates workspace-create even when the
		// workspace-access role is present. Not counted.
		banned := activeUser(t, db, database.User{})
		member(t, db, org.ID, banned, rbac.RoleOrgWorkspaceAccess(), rbac.RoleOrgWorkspaceCreationBan())

		// Org admins retain workspace-create. Counted.
		orgAdmin := activeUser(t, db, database.User{})
		member(t, db, org.ID, orgAdmin, rbac.RoleOrgAdmin())

		// Owners retain workspace-create. Counted.
		owner := activeUser(t, db, database.User{RBACRoles: []string{rbac.RoleOwner().Name}})
		member(t, db, org.ID, owner)

		// Members of an org that keeps organization-workspace-access in
		// default_org_member_roles inherit workspace-create. Counted.
		defaultOrg := dbgen.Organization(t, db, database.Organization{})
		defaultMember := activeUser(t, db, database.User{})
		member(t, db, defaultOrg.ID, defaultMember)

		count, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), db, authorizer)
		require.NoError(t, err)
		require.Equal(t, int64(4), count)
	})

	t.Run("MultiOrgSplitCapability", func(t *testing.T) {
		t.Parallel()
		// Users whose capability differs between their organizations:
		// workspace-create in any one org is sufficient to be counted.
		db, _ := dbtestutil.NewDB(t)
		orgA := dbgen.Organization(t, db, database.Organization{})
		orgB := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, orgA)
		emptyDefaultRoles(t, db, orgB)

		// Gateway in org A, workspace-create in org B. Counted.
		split := activeUser(t, db, database.User{})
		member(t, db, orgA.ID, split)
		member(t, db, orgB.ID, split, rbac.RoleOrgWorkspaceAccess())

		// The creation ban is scoped to org A and must not negate the
		// org B grant. Counted.
		bannedSplit := activeUser(t, db, database.User{})
		member(t, db, orgA.ID, bannedSplit, rbac.RoleOrgWorkspaceAccess(), rbac.RoleOrgWorkspaceCreationBan())
		member(t, db, orgB.ID, bannedSplit, rbac.RoleOrgWorkspaceAccess())

		// Gateway in both orgs. Not counted.
		gateway := activeUser(t, db, database.User{})
		member(t, db, orgA.ID, gateway)
		member(t, db, orgB.ID, gateway)

		count, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), db, authorizer)
		require.NoError(t, err)
		require.Equal(t, int64(2), count)
	})

	t.Run("CustomOrgRole", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, org)

		creatorRole, err := db.InsertCustomRole(ctx, database.InsertCustomRoleParams{
			Name:           "workspace-creator",
			DisplayName:    "Workspace Creator",
			OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
			OrgPermissions: []database.CustomRolePermission{{
				ResourceType: rbac.ResourceWorkspace.Type,
				Action:       policy.ActionCreate,
			}},
		})
		require.NoError(t, err)

		auditRole, err := db.InsertCustomRole(ctx, database.InsertCustomRoleParams{
			Name:           "org-reader",
			DisplayName:    "Org Reader",
			OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
			OrgPermissions: []database.CustomRolePermission{{
				ResourceType: rbac.ResourceOrganization.Type,
				Action:       policy.ActionRead,
			}},
		})
		require.NoError(t, err)

		// Custom org role with workspace-create. Counted.
		creator := activeUser(t, db, database.User{})
		member(t, db, org.ID, creator, creatorRole.Name)

		// Custom org role without workspace-create. Not counted.
		reader := activeUser(t, db, database.User{})
		member(t, db, org.ID, reader, auditRole.Name)

		count, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), db, authorizer)
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})

	t.Run("MalformedRoleNotCounted", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		// Authorization fails closed on an unparsable stored role, so
		// this user is not workspace-capable even though their org
		// membership would otherwise qualify them.
		corrupt := activeUser(t, db, database.User{RBACRoles: []string{"bad:role:extra"}})
		member(t, db, org.ID, corrupt)

		// The bad row must not fail the count for everyone else.
		capable := activeUser(t, db, database.User{})
		member(t, db, org.ID, capable)

		count, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), db, authorizer)
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})

	t.Run("EntitlementsAddonGate", func(t *testing.T) {
		t.Parallel()
		// Permission-based counting is gated on both the experiment and a
		// valid license carrying the AI Governance addon. Without either,
		// the legacy active user count applies.
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, org)

		gateway := activeUser(t, db, database.User{})
		member(t, db, org.ID, gateway)
		wsUser := activeUser(t, db, database.User{})
		member(t, db, org.ID, wsUser, rbac.RoleOrgWorkspaceAccess())

		enablements := map[codersdk.FeatureName]bool{}
		experimentOn := codersdk.Experiments{codersdk.ExperimentWorkspaceCapableLicensing}

		// No license: legacy count, even with the experiment on.
		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, authorizer, experimentOn)
		require.NoError(t, err)
		require.Equal(t, int64(2), *entitlements.Features[codersdk.FeatureUserLimit].Actual)

		// License without the AI Governance addon: still the legacy count.
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureUserLimit: 100},
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		entitlements, err = license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, authorizer, experimentOn)
		require.NoError(t, err)
		require.Equal(t, int64(2), *entitlements.Features[codersdk.FeatureUserLimit].Actual)

		// License with the AI Governance addon: only the workspace-capable
		// user counts.
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, *(&coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureUserLimit: 100},
			}).AIGovernanceAddon(10)),
			Exp: dbtime.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		entitlements, err = license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, authorizer, experimentOn)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)
		require.Equal(t, int64(1), *entitlements.Features[codersdk.FeatureUserLimit].Actual)

		// Addon present but experiment off: legacy count.
		entitlements, err = license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, authorizer, nil)
		require.NoError(t, err)
		require.Equal(t, int64(2), *entitlements.Features[codersdk.FeatureUserLimit].Actual)

		// Addon present, experiment on, but no authorizer: fall back to the
		// legacy count instead of failing.
		entitlements, err = license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, nil, experimentOn)
		require.NoError(t, err)
		require.Equal(t, int64(2), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
	})

	t.Run("LicensesEntitlementsCountFn", func(t *testing.T) {
		t.Parallel()
		// Exercises LicensesEntitlements directly: the count function is
		// only invoked when a valid license carries the addon, grace
		// period licenses still gate the count, and count errors fall
		// back to the legacy count with a recorded error.
		now := time.Now()
		enablements := map[codersdk.FeatureName]bool{}

		dbLicense := func(t *testing.T, opts coderdenttest.LicenseOptions) database.License {
			t.Helper()
			return database.License{
				UUID: uuid.New(),
				JWT:  coderdenttest.GenerateLicense(t, opts),
				Exp:  now.Add(time.Hour * 24 * 60),
			}
		}
		addonLicense := func(t *testing.T) database.License {
			t.Helper()
			return dbLicense(t, *(&coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureUserLimit: 100},
			}).Valid(now).AIGovernanceAddon(10))
		}

		t.Run("NoAddonFnNotCalled", func(t *testing.T) {
			t.Parallel()
			licenses := []database.License{dbLicense(t, *(&coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureUserLimit: 100},
			}).Valid(now))}
			entitlements, err := license.LicensesEntitlements(ctx, now, licenses, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					t.Fatal("count fn must not be called without the addon")
					return 0, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, int64(7), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
		})

		t.Run("AddonMissingDependenciesIgnored", func(t *testing.T) {
			t.Parallel()
			// A license carrying the addon without its required features
			// records a validation error and the addon is skipped, so
			// workspace-capable counting must not activate.
			opts := (&coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureUserLimit: 100},
			}).Valid(now)
			opts.Addons = append(opts.Addons, codersdk.AddonAIGovernance)
			entitlements, err := license.LicensesEntitlements(ctx, now, []database.License{dbLicense(t, *opts)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					t.Fatal("count fn must not be called when addon dependencies are unmet")
					return 0, nil
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, entitlements.Errors)
			require.Equal(t, int64(7), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
		})

		t.Run("ActiveModeIgnoresFn", func(t *testing.T) {
			t.Parallel()
			// UserCountingMode is authoritative: with the mode left at its
			// active-users zero value, the counting function must not be
			// called even though it is set and the addon is present.
			entitlements, err := license.LicensesEntitlements(ctx, now, []database.License{addonLicense(t)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount: 7,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					t.Fatal("count fn must not be called in active counting mode")
					return 0, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, int64(7), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
			require.Equal(t, int64(100), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
		})

		t.Run("AddonUsesFn", func(t *testing.T) {
			t.Parallel()
			entitlements, err := license.LicensesEntitlements(ctx, now, []database.License{addonLicense(t)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:   7,
				ActiveAISeatCount: 5,
				UserCountingMode:  license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					return 3, nil
				},
			})
			require.NoError(t, err)
			require.Empty(t, entitlements.Errors)
			require.Equal(t, int64(3), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
			// Permission-based counting applies to workspace seats only:
			// AI Governance seats keep their own count and limit.
			aiSeats := entitlements.Features[codersdk.FeatureAIGovernanceUserLimit]
			require.Equal(t, int64(5), *aiSeats.Actual)
			require.Equal(t, int64(10), *aiSeats.Limit)
			// Under the limit: no user-limit warning, even though the
			// legacy active user count would also have been under it.
			for _, warning := range entitlements.Warnings {
				require.NotContains(t, warning, "users but")
			}
			// A fully valid addon license must not warn about the
			// counting-mode revert.
			for _, warning := range entitlements.Warnings {
				require.NotContains(t, warning, "fully expires")
			}
		})

		t.Run("BestPairSelection", func(t *testing.T) {
			t.Parallel()
			// A deployment holding both an addon license and a non-addon
			// license has two user_limit candidates, each evaluated with
			// its own counting mode. Limits and modes never mix.
			licenses := []database.License{
				dbLicense(t, *(&coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureUserLimit: 200},
				}).Valid(now)),
				addonLicense(t), // user_limit 100, AI Governance addon.
			}
			run := func(t *testing.T, activeUsers, capableUsers int64) codersdk.Entitlements {
				entitlements, err := license.LicensesEntitlements(ctx, now, licenses, enablements, coderdenttest.Keys, license.FeatureArguments{
					ActiveUserCount:  activeUsers,
					UserCountingMode: license.UserCountingModeWorkspaceCapable,
					WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
						return capableUsers, nil
					},
				})
				require.NoError(t, err)
				return entitlements
			}

			t.Run("LegacyPairCompliant", func(t *testing.T) {
				t.Parallel()
				// 180 active <= 200 wins over 150 capable > 100: the
				// non-addon license keeps the deployment compliant.
				entitlements := run(t, 180, 150)
				require.Equal(t, int64(180), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
				require.Equal(t, int64(200), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
				for _, warning := range entitlements.Warnings {
					require.NotContains(t, warning, "users but")
				}
			})

			t.Run("AddonPairCompliant", func(t *testing.T) {
				t.Parallel()
				// 90 capable <= 100 wins over 250 active > 200: the addon
				// license keeps the deployment compliant.
				entitlements := run(t, 250, 90)
				require.Equal(t, int64(90), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
				require.Equal(t, int64(100), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
				for _, warning := range entitlements.Warnings {
					require.NotContains(t, warning, "users but")
				}
			})

			t.Run("NeitherPairCompliant", func(t *testing.T) {
				t.Parallel()
				// Both pairs over: the higher limit is reported, with the
				// counting mode of its own license.
				entitlements := run(t, 250, 150)
				require.Equal(t, int64(250), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
				require.Equal(t, int64(200), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
				require.Contains(t, entitlements.Warnings,
					"Your deployment has 250 active users but is only licensed for 200.")
			})

			t.Run("GraceAddonCompliantBeatsEntitledOver", func(t *testing.T) {
				t.Parallel()
				// A grace-period addon pair that fits its count wins over an
				// entitled non-addon pair that does not, carrying its grace
				// entitlement and the revert warning with it.
				licenses := []database.License{
					dbLicense(t, *(&coderdenttest.LicenseOptions{
						Features: license.Features{codersdk.FeatureUserLimit: 200},
					}).Valid(now)),
					dbLicense(t, *(&coderdenttest.LicenseOptions{
						Features: license.Features{codersdk.FeatureUserLimit: 100},
					}).GracePeriod(now).AIGovernanceAddon(10)),
				}
				entitlements, err := license.LicensesEntitlements(ctx, now, licenses, enablements, coderdenttest.Keys, license.FeatureArguments{
					ActiveUserCount:  250,
					UserCountingMode: license.UserCountingModeWorkspaceCapable,
					WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
						return 90, nil
					},
				})
				require.NoError(t, err)
				require.Equal(t, int64(90), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
				require.Equal(t, int64(100), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
				require.Equal(t, codersdk.EntitlementGracePeriod, entitlements.Features[codersdk.FeatureUserLimit].Entitlement)
				require.Contains(t, entitlements.Warnings,
					"Your deployment has 90 workspace-capable users but the license with the limit 100 is expired.")
				require.Contains(t, entitlements.Warnings,
					"Your license with the AI Governance addon is expired. When it fully expires, all 250 active users will count toward the user limit instead of the 90 workspace-capable users.")
			})

			t.Run("EqualLimitsPreferAddon", func(t *testing.T) {
				t.Parallel()
				// Identical limit and entitlement on an addon and a
				// non-addon license: the addon pair wins the tie, so the
				// workspace-capable count is displayed.
				licenses := []database.License{
					dbLicense(t, *(&coderdenttest.LicenseOptions{
						Features: license.Features{codersdk.FeatureUserLimit: 100},
					}).Valid(now)),
					addonLicense(t),
				}
				entitlements, err := license.LicensesEntitlements(ctx, now, licenses, enablements, coderdenttest.Keys, license.FeatureArguments{
					ActiveUserCount:  80,
					UserCountingMode: license.UserCountingModeWorkspaceCapable,
					WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
						return 30, nil
					},
				})
				require.NoError(t, err)
				require.Equal(t, int64(30), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
				require.Equal(t, int64(100), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
			})

			t.Run("TwoAddonCandidates", func(t *testing.T) {
				t.Parallel()
				// Two addon licenses: the entitled higher-limit pair fits
				// the capable count and wins over the grace pair, and its
				// presence suppresses the revert warning.
				licenses := []database.License{
					dbLicense(t, *(&coderdenttest.LicenseOptions{
						Features: license.Features{codersdk.FeatureUserLimit: 100},
					}).GracePeriod(now).AIGovernanceAddon(10)),
					dbLicense(t, *(&coderdenttest.LicenseOptions{
						Features: license.Features{codersdk.FeatureUserLimit: 300},
					}).Valid(now).AIGovernanceAddon(10)),
				}
				entitlements, err := license.LicensesEntitlements(ctx, now, licenses, enablements, coderdenttest.Keys, license.FeatureArguments{
					ActiveUserCount:  500,
					UserCountingMode: license.UserCountingModeWorkspaceCapable,
					WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
						return 150, nil
					},
				})
				require.NoError(t, err)
				require.Equal(t, int64(150), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
				require.Equal(t, int64(300), *entitlements.Features[codersdk.FeatureUserLimit].Limit)
				require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureUserLimit].Entitlement)
				for _, warning := range entitlements.Warnings {
					require.NotContains(t, warning, "fully expires")
					require.NotContains(t, warning, "users but")
				}
			})
		})

		t.Run("ModeWithoutFnIsDevError", func(t *testing.T) {
			t.Parallel()
			_, err := license.LicensesEntitlements(ctx, now, []database.License{addonLicense(t)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
			})
			require.ErrorContains(t, err, "dev error")
		})

		t.Run("OverLimitWarnsWithCapableCount", func(t *testing.T) {
			t.Parallel()
			// The over-limit warning must report the workspace-capable
			// count it was compared against, and say so, rather than
			// claiming that many "active users" exist.
			entitlements, err := license.LicensesEntitlements(ctx, now, []database.License{addonLicense(t)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					return 150, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, int64(150), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
			require.Contains(t, entitlements.Warnings,
				"Your deployment has 150 workspace-capable users but is only licensed for 100.")
		})

		t.Run("GracePeriodAddonUsesFn", func(t *testing.T) {
			t.Parallel()
			// A license in its grace period still includes the addon, so
			// counting must not revert until the license hard-expires.
			licenses := []database.License{dbLicense(t, *(&coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureUserLimit: 100},
			}).GracePeriod(now).AIGovernanceAddon(10))}
			entitlements, err := license.LicensesEntitlements(ctx, now, licenses, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					return 3, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, int64(3), *entitlements.Features[codersdk.FeatureUserLimit].Actual)
			require.Contains(t, entitlements.Warnings,
				"Your deployment has 3 workspace-capable users but the license with the limit 100 is expired.")
			// The revert warning gives admins the legacy count they will be
			// measured by once the grace period ends.
			require.Contains(t, entitlements.Warnings,
				"Your license with the AI Governance addon is expired. When it fully expires, all 7 active users will count toward the user limit instead of the 3 workspace-capable users.")
		})

		t.Run("FnErrorPropagates", func(t *testing.T) {
			t.Parallel()
			// A failed capable count aborts the computation, matching the
			// legacy active-user-count error semantics; the caller keeps
			// the previous entitlements rather than seeing a silently
			// different count.
			_, err := license.LicensesEntitlements(ctx, now, []database.License{addonLicense(t)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					return 0, xerrors.New("boom")
				},
			})
			require.ErrorContains(t, err, "count workspace capable users")
			require.ErrorContains(t, err, "boom")
		})

		t.Run("ContextCanceledBails", func(t *testing.T) {
			t.Parallel()
			_, err := license.LicensesEntitlements(ctx, now, []database.License{addonLicense(t)}, enablements, coderdenttest.Keys, license.FeatureArguments{
				ActiveUserCount:  7,
				UserCountingMode: license.UserCountingModeWorkspaceCapable,
				WorkspaceCapableUserCountFn: func(context.Context) (int64, error) {
					return 0, context.Canceled
				},
			})
			require.ErrorIs(t, err, context.Canceled)
		})
	})
}

// TestCountWorkspaceCapableUsersErrors covers the count's database
// failure paths, which abort the count rather than skewing it.
func TestCountWorkspaceCapableUsersErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	authorizer := rbac.NewCachingAuthorizer(prometheus.NewRegistry())

	prefetchParams := database.CustomRolesParams{IncludeSystemRoles: true}

	t.Run("NilAuthorizer", func(t *testing.T) {
		t.Parallel()
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		_, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), mDB, nil)
		require.ErrorContains(t, err, "dev error")
	})

	t.Run("PrefetchError", func(t *testing.T) {
		t.Parallel()
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		mDB.EXPECT().CustomRoles(gomock.Any(), prefetchParams).Return(nil, xerrors.New("boom"))

		_, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), mDB, authorizer)
		require.ErrorContains(t, err, "prefetch custom roles")
	})

	t.Run("RolesQueryError", func(t *testing.T) {
		t.Parallel()
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		mDB.EXPECT().CustomRoles(gomock.Any(), prefetchParams).Return([]database.CustomRole{}, nil)
		mDB.EXPECT().GetActiveUsersAuthorizationRoles(gomock.Any()).Return(nil, xerrors.New("boom"))

		_, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), mDB, authorizer)
		require.ErrorContains(t, err, "get active users authorization roles")
	})

	t.Run("ExpandLookupError", func(t *testing.T) {
		t.Parallel()
		// A custom role that was not prefetched (deleted, or created
		// mid-count) is looked up individually; a database failure there
		// aborts the count.
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		userID := uuid.New()
		orgID := uuid.New()
		mDB.EXPECT().CustomRoles(gomock.Any(), prefetchParams).Return([]database.CustomRole{}, nil)
		mDB.EXPECT().GetActiveUsersAuthorizationRoles(gomock.Any()).Return([]database.GetActiveUsersAuthorizationRolesRow{{
			ID:    userID,
			Roles: []string{"member", "dangling-role:" + orgID.String()},
		}}, nil)
		mDB.EXPECT().CustomRoles(gomock.Any(), gomock.Not(prefetchParams)).Return(nil, xerrors.New("boom"))

		_, err := license.CountWorkspaceCapableUsers(ctx, testutil.Logger(t), mDB, authorizer)
		require.ErrorContains(t, err, "evaluate workspace-create for user "+userID.String())
		require.ErrorContains(t, err, "expand roles")
	})
}
