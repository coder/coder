package enidpsync_test

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/enidpsync"
	"github.com/coder/coder/v2/testutil"
)

func TestEnterpriseParseRoleClaims(t *testing.T) {
	t.Parallel()

	entitled := entitlements.New()
	entitled.Modify(func(en *codersdk.Entitlements) {
		en.Features[codersdk.FeatureUserRoleManagement] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
		}
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		mgr := runtimeconfig.NewManager()
		s := enidpsync.NewSync(testutil.Logger(t), mgr, entitlements.New(), idpsync.DeploymentSyncSettings{})

		params, err := s.ParseRoleClaims(context.Background(), jwt.MapClaims{})
		require.Nil(t, err)
		require.False(t, params.SyncEntitled)
		require.False(t, params.SyncSiteWide)
	})

	t.Run("NotEntitledButEnabled", func(t *testing.T) {
		t.Parallel()
		// Since it is not entitled, it should not be enabled

		mgr := runtimeconfig.NewManager()
		s := enidpsync.NewSync(testutil.Logger(t), mgr, entitlements.New(), idpsync.DeploymentSyncSettings{
			SiteRoleField: "roles",
		})

		params, err := s.ParseRoleClaims(context.Background(), jwt.MapClaims{})
		require.Nil(t, err)
		require.False(t, params.SyncEntitled)
		require.False(t, params.SyncSiteWide)
	})

	t.Run("SiteDisabled", func(t *testing.T) {
		t.Parallel()

		mgr := runtimeconfig.NewManager()
		s := enidpsync.NewSync(testutil.Logger(t), mgr, entitled, idpsync.DeploymentSyncSettings{})

		params, err := s.ParseRoleClaims(context.Background(), jwt.MapClaims{})
		require.Nil(t, err)
		require.True(t, params.SyncEntitled)
		require.False(t, params.SyncSiteWide)
	})

	t.Run("SiteEnabled", func(t *testing.T) {
		t.Parallel()

		mgr := runtimeconfig.NewManager()
		s := enidpsync.NewSync(testutil.Logger(t), mgr, entitled, idpsync.DeploymentSyncSettings{
			SiteRoleField:    "roles",
			SiteRoleMapping:  map[string][]string{},
			SiteDefaultRoles: []string{rbac.RoleTemplateAdmin().Name},
		})

		params, err := s.ParseRoleClaims(context.Background(), jwt.MapClaims{
			"roles": []string{rbac.RoleAuditor().Name},
		})
		require.Nil(t, err)
		require.True(t, params.SyncEntitled)
		require.True(t, params.SyncSiteWide)
		require.ElementsMatch(t, []string{
			rbac.RoleTemplateAdmin().Name,
			rbac.RoleAuditor().Name,
		}, params.SiteWideRoles)
	})

	t.Run("SiteMapping", func(t *testing.T) {
		t.Parallel()

		mgr := runtimeconfig.NewManager()
		s := enidpsync.NewSync(testutil.Logger(t), mgr, entitled, idpsync.DeploymentSyncSettings{
			SiteRoleField: "roles",
			SiteRoleMapping: map[string][]string{
				"foo": {rbac.RoleAuditor().Name, rbac.RoleUserAdmin().Name},
				"bar": {rbac.RoleOwner().Name},
			},
			SiteDefaultRoles: []string{rbac.RoleTemplateAdmin().Name},
		})

		params, err := s.ParseRoleClaims(context.Background(), jwt.MapClaims{
			"roles": []string{"foo", "bar", "random"},
		})
		require.Nil(t, err)
		require.True(t, params.SyncEntitled)
		require.True(t, params.SyncSiteWide)
		require.ElementsMatch(t, []string{
			rbac.RoleTemplateAdmin().Name,
			rbac.RoleAuditor().Name,
			rbac.RoleUserAdmin().Name,
			rbac.RoleOwner().Name,
			// Invalid claims are still passed at this point
			"random",
		}, params.SiteWideRoles)
	})

	t.Run("DuplicateRoles", func(t *testing.T) {
		t.Parallel()

		mgr := runtimeconfig.NewManager()
		s := enidpsync.NewSync(testutil.Logger(t), mgr, entitled, idpsync.DeploymentSyncSettings{
			SiteRoleField: "roles",
			SiteRoleMapping: map[string][]string{
				"foo": {rbac.RoleOwner().Name, rbac.RoleAuditor().Name},
				"bar": {rbac.RoleOwner().Name},
			},
			SiteDefaultRoles: []string{rbac.RoleAuditor().Name},
		})

		params, err := s.ParseRoleClaims(context.Background(), jwt.MapClaims{
			"roles": []string{"foo", "bar", rbac.RoleAuditor().Name, rbac.RoleOwner().Name},
		})
		require.Nil(t, err)
		require.True(t, params.SyncEntitled)
		require.True(t, params.SyncSiteWide)
		require.ElementsMatch(t, []string{
			rbac.RoleAuditor().Name,
			rbac.RoleOwner().Name,
		}, params.SiteWideRoles)
	})
}
