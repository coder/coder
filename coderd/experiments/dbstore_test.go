package experiments_test

import (
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// newAuthzDBStore returns a DB store behind dbauthz, as in production. The
// test contexts carry no actor, so reads succeed only if the store reads
// as the system.
func newAuthzDBStore(t *testing.T, db database.Store) experiments.Store {
	t.Helper()
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())
	return experiments.NewDBStore(authzDB)
}

func TestDBStore(t *testing.T) {
	t.Parallel()

	t.Run("RulesByPrefixOnly", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		rules := map[string]string{
			string(scoped): `{"mode":"on","revision":1}`,
			// Unknown experiments and undecodable values are returned so
			// the Evaluator can ignore or fail closed on them.
			"not-a-real-experiment": `{"mode":"on","revision":1}`,
			"broken":                `{not json`,
		}
		for experiment, value := range rules {
			require.NoError(t, db.UpsertExperimentRule(ctx, database.UpsertExperimentRuleParams{Experiment: experiment, Value: value}))
		}
		// Keys that resemble the prefix. '_' is a LIKE wildcard, so
		// experimentXrule: would match a LIKE-based filter.
		for _, key := range []string{"experiment_rule", "experimentXrule:example", "other_experiment_rule:example"} {
			require.NoError(t, db.UpsertRuntimeConfig(ctx, database.UpsertRuntimeConfigParams{Key: key, Value: `{"mode":"on","revision":1}`}))
		}

		got, err := newAuthzDBStore(t, db).Rules(ctx)
		require.NoError(t, err)
		want := make(map[codersdk.Experiment]experiments.StoredRule, len(rules))
		for experiment, value := range rules {
			want[codersdk.Experiment(experiment)] = experiments.StoredRule{Value: []byte(value)}
		}
		require.Equal(t, want, got)
	})

	t.Run("UserAttributes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{RBACRoles: []string{rbac.RoleAuditor().String()}})
		orgA := dbgen.Organization(t, db, database.Organization{})
		orgB := dbgen.Organization(t, db, database.Organization{})
		// The user is not a member of orgC or its group.
		orgC := dbgen.Organization(t, db, database.Organization{})
		// Organization creation adds the Everyone group in coderd, not in
		// dbgen.
		for _, org := range []database.Organization{orgA, orgB, orgC} {
			_, err := db.InsertAllUsersGroup(ctx, org.ID)
			require.NoError(t, err)
		}
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: orgA.ID})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: orgB.ID})
		devs := dbgen.Group(t, db, database.Group{OrganizationID: orgB.ID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: devs.ID})
		dbgen.Group(t, db, database.Group{OrganizationID: orgC.ID})
		// A soft-deleted organization keeps its last member and its
		// Everyone group. Its name can be reused, so neither may reach
		// conditions.
		orgGone := dbgen.Organization(t, db, database.Organization{})
		_, err := db.InsertAllUsersGroup(ctx, orgGone.ID)
		require.NoError(t, err)
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: orgGone.ID})
		require.NoError(t, db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			ID:        orgGone.ID,
			UpdatedAt: dbtime.Now(),
		}))

		got, err := newAuthzDBStore(t, db).UserAttributes(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, user.ID.String(), got.ID)
		require.Equal(t, user.Username, got.Username)
		require.Equal(t, user.Email, got.Email)
		require.Equal(t, []string{rbac.RoleAuditor().String()}, got.Roles)
		require.ElementsMatch(t, []string{orgA.Name, orgB.Name}, got.Organizations)
		require.ElementsMatch(t, []string{
			orgA.Name + "/Everyone",
			orgB.Name + "/Everyone",
			orgB.Name + "/" + devs.Name,
		}, got.Groups)
		// Lists are sorted so conditions that index or compare whole lists
		// do not depend on query row order.
		require.True(t, slices.IsSorted(got.Organizations), got.Organizations)
		require.True(t, slices.IsSorted(got.Groups), got.Groups)
	})

	t.Run("SortedRoles", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{RBACRoles: []string{rbac.RoleTemplateAdmin().String(), rbac.RoleAuditor().String()}})

		got, err := newAuthzDBStore(t, db).UserAttributes(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, []string{rbac.RoleAuditor().String(), rbac.RoleTemplateAdmin().String()}, got.Roles)
	})
}
