package rolestore_test

import (
	"context"
	"database/sql"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/testutil"
)

type agentsAccessBackfillFixture struct {
	db           database.Store
	orgs         []uuid.UUID
	emptyOrg     uuid.UUID
	extraOrg     uuid.UUID
	customOrg    uuid.UUID
	member       database.User
	customMember database.User
	serviceAcct  database.User
}

// newAgentsAccessBackfillFixture seeds the state a release without the
// agents-access default role leaves behind.
func newAgentsAccessBackfillFixture(t *testing.T) agentsAccessBackfillFixture {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	defaultOrg, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)
	require.NotContains(t, defaultOrg.DefaultOrgMemberRoles, rbac.RoleAgentsAccess())

	legacyDefaults := []string{rbac.RoleOrgWorkspaceAccess()}
	extraOrg := dbgen.Organization(t, db, database.Organization{
		DefaultOrgMemberRoles: []string{rbac.RoleOrgWorkspaceAccess(), rbac.RoleOrgAuditor()},
	})
	emptyOrg := dbgen.Organization(t, db, database.Organization{DefaultOrgMemberRoles: legacyDefaults})
	_, err = db.UpdateOrganization(ctx, database.UpdateOrganizationParams{
		ID:                    emptyOrg.ID,
		UpdatedAt:             dbtime.Now(),
		Name:                  emptyOrg.Name,
		DisplayName:           emptyOrg.DisplayName,
		Description:           emptyOrg.Description,
		Icon:                  emptyOrg.Icon,
		DefaultOrgMemberRoles: []string{},
	})
	require.NoError(t, err)
	customOrg := dbgen.Organization(t, db, database.Organization{DefaultOrgMemberRoles: legacyDefaults})
	dbgen.CustomRole(t, db, database.CustomRole{
		Name:           rbac.RoleAgentsAccess(),
		OrganizationID: uuid.NullUUID{UUID: customOrg.ID, Valid: true},
	})

	member := dbgen.User(t, db, database.User{})
	customMember := dbgen.User(t, db, database.User{})
	for _, m := range []database.OrganizationMember{
		{OrganizationID: extraOrg.ID, UserID: member.ID},
		{OrganizationID: customOrg.ID, UserID: customMember.ID},
	} {
		m.Roles = []string{rbac.RoleAgentsAccess()}
		dbgen.OrganizationMember(t, db, m)
	}
	serviceAcct := dbgen.User(t, db, database.User{IsServiceAccount: true})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: extraOrg.ID,
		UserID:         serviceAcct.ID,
	})

	return agentsAccessBackfillFixture{
		db:           db,
		orgs:         []uuid.UUID{defaultOrg.ID, extraOrg.ID, emptyOrg.ID, customOrg.ID},
		emptyOrg:     emptyOrg.ID,
		extraOrg:     extraOrg.ID,
		customOrg:    customOrg.ID,
		member:       member,
		customMember: customMember,
		serviceAcct:  serviceAcct,
	}
}

func agentsAccessCount(t *testing.T, db database.Store, orgID uuid.UUID) int {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	org, err := db.GetOrganizationByID(ctx, orgID)
	require.NoError(t, err)
	count := 0
	for _, role := range org.DefaultOrgMemberRoles {
		if role == rbac.RoleAgentsAccess() {
			count++
		}
	}
	return count
}

func getBackfillMarker(t *testing.T, db database.Store) error {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := db.GetRuntimeConfig(ctx, rolestore.AgentsAccessDefaultRoleBackfilledKey)
	return err
}

func TestBackfillAgentsAccessDefaultRole(t *testing.T) {
	t.Parallel()

	t.Run("AppliesOnce", func(t *testing.T) {
		t.Parallel()

		f := newAgentsAccessBackfillFixture(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		sink := testutil.NewFakeSink(t)

		require.NoError(t, rolestore.BackfillAgentsAccessDefaultRole(ctx, sink.Logger(), f.db))
		require.NoError(t, getBackfillMarker(t, f.db))
		require.Len(t, sink.Entries(), 1)
		for _, orgID := range f.orgs {
			require.Equal(t, 1, agentsAccessCount(t, f.db, orgID))
		}

		extra, err := f.db.GetOrganizationByID(ctx, f.extraOrg)
		require.NoError(t, err)
		require.Equal(t, []string{rbac.RoleOrgWorkspaceAccess(), rbac.RoleOrgAuditor(), rbac.RoleAgentsAccess()}, extra.DefaultOrgMemberRoles)
		empty, err := f.db.GetOrganizationByID(ctx, f.emptyOrg)
		require.NoError(t, err)
		require.Equal(t, []string{rbac.RoleAgentsAccess()}, empty.DefaultOrgMemberRoles)

		roles, err := f.db.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles: []database.NameOrganizationPair{{Name: rbac.RoleAgentsAccess(), OrganizationID: f.customOrg}},
		})
		require.NoError(t, err)
		require.Empty(t, roles)

		mem, err := f.db.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: f.extraOrg,
			UserID:         f.member.ID,
		})
		require.NoError(t, err)
		require.Len(t, mem, 1)
		require.Equal(t, []string{rbac.RoleAgentsAccess()}, mem[0].OrganizationMember.Roles)

		// The custom role delete trigger strips grants of the deleted role in
		// its organization; those members keep access through the defaults.
		mem, err = f.db.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: f.customOrg,
			UserID:         f.customMember.ID,
		})
		require.NoError(t, err)
		require.Len(t, mem, 1)
		require.Empty(t, mem[0].OrganizationMember.Roles)
		customRoles, err := f.db.GetAuthorizationUserRoles(ctx, f.customMember.ID)
		require.NoError(t, err)
		require.Contains(t, customRoles.Roles, rbac.ScopedRoleAgentsAccess(f.customOrg).String())

		saRoles, err := f.db.GetAuthorizationUserRoles(ctx, f.serviceAcct.ID)
		require.NoError(t, err)
		require.NotContains(t, saRoles.Roles, rbac.ScopedRoleAgentsAccess(f.extraOrg).String())

		// A second run changes nothing.
		require.NoError(t, rolestore.BackfillAgentsAccessDefaultRole(ctx, sink.Logger(), f.db))
		require.Len(t, sink.Entries(), 1)
		for _, orgID := range f.orgs {
			require.Equal(t, 1, agentsAccessCount(t, f.db, orgID))
		}
	})

	t.Run("RemovalSurvivesRerun", func(t *testing.T) {
		t.Parallel()

		f := newAgentsAccessBackfillFixture(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		require.NoError(t, rolestore.BackfillAgentsAccessDefaultRole(ctx, testutil.Logger(t), f.db))

		org, err := f.db.GetOrganizationByID(ctx, f.extraOrg)
		require.NoError(t, err)
		_, err = f.db.UpdateOrganization(ctx, database.UpdateOrganizationParams{
			ID:          org.ID,
			UpdatedAt:   dbtime.Now(),
			Name:        org.Name,
			DisplayName: org.DisplayName,
			Description: org.Description,
			Icon:        org.Icon,
			DefaultOrgMemberRoles: slices.DeleteFunc(slices.Clone(org.DefaultOrgMemberRoles), func(role string) bool {
				return role == rbac.RoleAgentsAccess()
			}),
		})
		require.NoError(t, err)

		require.NoError(t, rolestore.BackfillAgentsAccessDefaultRole(ctx, testutil.Logger(t), f.db))
		require.Equal(t, 0, agentsAccessCount(t, f.db, f.extraOrg))
	})

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()

		f := newAgentsAccessBackfillFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		sink := testutil.NewFakeSink(t)

		const runs = 8
		var wg sync.WaitGroup
		errs := make(chan error, runs)
		for range runs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- rolestore.BackfillAgentsAccessDefaultRole(ctx, sink.Logger(), f.db)
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		require.NoError(t, getBackfillMarker(t, f.db))
		require.Len(t, sink.Entries(), 1)
		for _, orgID := range f.orgs {
			require.Equal(t, 1, agentsAccessCount(t, f.db, orgID))
		}
	})

	t.Run("FailureRollsBack", func(t *testing.T) {
		t.Parallel()

		f := newAgentsAccessBackfillFixture(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		sink := testutil.NewFakeSink(t)

		err := rolestore.BackfillAgentsAccessDefaultRole(ctx, sink.Logger(), failingMarkerStore{Store: f.db})
		require.Error(t, err)
		require.ErrorIs(t, getBackfillMarker(t, f.db), sql.ErrNoRows)
		require.Empty(t, sink.Entries())
		for _, orgID := range f.orgs {
			require.Equal(t, 0, agentsAccessCount(t, f.db, orgID))
		}
		roles, err := f.db.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles: []database.NameOrganizationPair{{Name: rbac.RoleAgentsAccess(), OrganizationID: f.customOrg}},
		})
		require.NoError(t, err)
		require.Len(t, roles, 1)

		require.NoError(t, rolestore.BackfillAgentsAccessDefaultRole(ctx, sink.Logger(), f.db))
		require.NoError(t, getBackfillMarker(t, f.db))
		require.Len(t, sink.Entries(), 1)
		for _, orgID := range f.orgs {
			require.Equal(t, 1, agentsAccessCount(t, f.db, orgID))
		}
	})
}

// failingMarkerStore fails the marker write inside the transaction, after
// the backfill statement has run.
type failingMarkerStore struct {
	database.Store
}

func (s failingMarkerStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(failingMarkerStore{Store: tx})
	}, opts)
}

func (failingMarkerStore) UpsertRuntimeConfig(context.Context, database.UpsertRuntimeConfigParams) error {
	return xerrors.New("marker write failed")
}
