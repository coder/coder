package capabilities_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/capabilities"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDBCheckerCapabilities(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	newChecker := func(t *testing.T, db database.Store, clock quartz.Clock) *capabilities.DBChecker {
		checker, err := capabilities.NewDBChecker(capabilities.Options{
			DB:         db,
			Authorizer: rbac.NewCachingAuthorizer(prometheus.NewRegistry()),
			Logger:     testutil.Logger(t),
			Clock:      clock,
		})
		require.NoError(t, err)
		return checker
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

	t.Run("WorkspaceCapability", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, org)
		checker := newChecker(t, db, quartz.NewMock(t))

		// Explicit organization-workspace-access grant.
		wsUser := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		member(t, db, org.ID, wsUser, rbac.RoleOrgWorkspaceAccess())

		// Floor permissions only.
		gateway := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		member(t, db, org.ID, gateway)

		// The creation ban negates workspace-create.
		banned := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		member(t, db, org.ID, banned, rbac.RoleOrgWorkspaceAccess(), rbac.RoleOrgWorkspaceCreationBan())

		// Site-wide owner, no org membership.
		owner := dbgen.User(t, db, database.User{
			Status:    database.UserStatusActive,
			RBACRoles: []string{rbac.RoleOwner().Name},
		})

		// Org admin.
		orgAdmin := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		member(t, db, org.ID, orgAdmin, rbac.RoleOrgAdmin())

		// Belongs to no organization.
		orphan := dbgen.User(t, db, database.User{Status: database.UserStatusActive})

		// Roles are ignored for users who are not active.
		suspended := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
		member(t, db, org.ID, suspended, rbac.RoleOrgAdmin())

		// Service accounts are not excluded: the RBAC engine authorizes them
		// to create workspaces even though they do not consume license seats.
		serviceAccount := dbgen.User(t, db, database.User{
			Status:           database.UserStatusActive,
			IsServiceAccount: true,
		})
		member(t, db, org.ID, serviceAccount, rbac.RoleOrgWorkspaceAccess())

		for _, tc := range []struct {
			name    string
			userID  uuid.UUID
			capable bool
		}{
			{"workspace-access", wsUser.ID, true},
			{"gateway", gateway.ID, false},
			{"creation-ban", banned.ID, false},
			{"owner", owner.ID, true},
			{"org-admin", orgAdmin.ID, true},
			{"no-organization", orphan.ID, false},
			{"suspended", suspended.ID, false},
			{"service-account", serviceAccount.ID, true},
		} {
			caps, err := checker.Capabilities(ctx, tc.userID)
			require.NoError(t, err, tc.name)
			if tc.capable {
				require.Equal(t, []capabilities.Capability{capabilities.Workspace}, caps, tc.name)
			} else {
				require.Empty(t, caps, tc.name)
			}
		}
	})

	t.Run("CustomOrgRole", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, org)
		checker := newChecker(t, db, quartz.NewMock(t))

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

		creator := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		member(t, db, org.ID, creator, creatorRole.Name)

		caps, err := checker.Capabilities(ctx, creator.ID)
		require.NoError(t, err)
		require.Equal(t, []capabilities.Capability{capabilities.Workspace}, caps)
	})

	t.Run("CachesUntilTTLExpires", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		emptyDefaultRoles(t, db, org)
		clock := quartz.NewMock(t)
		checker := newChecker(t, db, clock)

		user := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
		member(t, db, org.ID, user)

		caps, err := checker.Capabilities(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, caps)

		// Granting the role does not invalidate the cached verdict.
		_, err = db.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
			GrantedRoles: []string{rbac.RoleOrgWorkspaceAccess()},
			UserID:       user.ID,
			OrgID:        org.ID,
		})
		require.NoError(t, err)

		caps, err = checker.Capabilities(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, caps, "cached verdict must be reused within the TTL")

		clock.Advance(capabilities.DefaultCacheTTL + time.Second)

		caps, err = checker.Capabilities(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, []capabilities.Capability{capabilities.Workspace}, caps)
	})
}

func TestNoop(t *testing.T) {
	t.Parallel()
	caps, err := capabilities.Noop{}.Capabilities(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Empty(t, caps)
}

func TestStrings(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{}, capabilities.Strings(nil))
	require.Equal(t, []string{"workspace"}, capabilities.Strings([]capabilities.Capability{
		capabilities.Workspace, capabilities.Workspace,
	}))
}

// erroringAuthorizer fails every authorization with a non-denial error.
type erroringAuthorizer struct {
	calls atomic.Int64
	err   error
}

func (a *erroringAuthorizer) Authorize(context.Context, rbac.Subject, policy.Action, rbac.Object) error {
	a.calls.Add(1)
	return a.err
}

func (*erroringAuthorizer) Prepare(context.Context, rbac.Subject, policy.Action, string) (rbac.PreparedAuthorized, error) {
	return nil, xerrors.New("not implemented")
}

// TestDBCheckerAuthorizerError covers the distinction between a denial and an
// evaluation failure: a failure must surface as an error and must not be cached
// as "holds no capabilities".
func TestDBCheckerAuthorizerError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{Status: database.UserStatusActive})

	authorizer := &erroringAuthorizer{err: xerrors.New("evaluation exploded")}
	checker, err := capabilities.NewDBChecker(capabilities.Options{
		DB:         db,
		Authorizer: authorizer,
		Logger:     testutil.Logger(t),
		Clock:      quartz.NewMock(t),
	})
	require.NoError(t, err)

	_, err = checker.Capabilities(ctx, user.ID)
	require.ErrorContains(t, err, "evaluation exploded")

	// The failure is not cached, so the next call re-evaluates.
	_, err = checker.Capabilities(ctx, user.ID)
	require.Error(t, err)
	require.Equal(t, int64(2), authorizer.calls.Load())
}
