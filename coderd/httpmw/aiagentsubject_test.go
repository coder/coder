package httpmw_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

// The first milestone of WP4 in poc_audit/work_breakdown.md, verifiable before
// anything on the authentication path calls it.
//
// Every agent here is created by entity.CreateAIAgent, which writes no users
// row. That is the point: the subject is assembled without one.
func TestAIAgentRBACSubject(t *testing.T) {
	t.Parallel()

	newAgent := func(t *testing.T, db database.Store, roles ...string) (entity.NewAIAgent, database.User) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		owner := dbgen.User(t, db, database.User{RBACRoles: roles})
		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner:        entity.Ref{Type: entity.TypeUser, ID: owner.ID},
			CreationSite: entity.CreationSite{Type: entity.CreationSiteTypeWorkspace, ID: uuid.New()},
		})
		require.NoError(t, err)
		return created, owner
	}

	t.Run("CarriesTheOwnersRolesAndItsOwnIdentity", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, db)

		subject, status, attribution, err := httpmw.AIAgentRBACSubject(ctx, db, agent.ID, rbac.ScopeAll)
		require.NoError(t, err)

		require.Equal(t, rbac.SubjectTypeAIAgent, subject.Type)
		require.Equal(t, agent.ID.String(), subject.AIAgentID,
			"the acting identity is the agent")
		require.Equal(t, owner.ID.String(), subject.ID,
			"the subject's own identity stays the owner's, an agent acting with the owner's roles")
		require.Equal(t, database.UserStatusActive, status)

		// The attribution channel, assembled from the same row. Capability and
		// attribution are used apart and there is no reason to read twice.
		require.Equal(t, agent.ID, attribution.AgentUserID)
		require.Equal(t, owner.ID, attribution.OwnerUserID)
		require.Equal(t, database.AIAgentOriginWorkspace, attribution.OriginType)

		// The name is computed, so it must match what the function computes
		// rather than anything stored.
		row, err := db.GetAIAgentLedgerRowByID(ctx, agent.ID)
		require.NoError(t, err)
		require.Equal(t,
			entity.DisplayName(entity.CreationSiteType(row.CreationSiteType), agent.ID),
			subject.FriendlyName)

		owners, _, err := httpmw.UserRBACSubject(ctx, db, owner.ID, rbac.ScopeAll)
		require.NoError(t, err)
		require.Equal(t, owners.Roles, subject.Roles, "the roles are the owner's, unchanged")
	})

	// The subject has to reach the policy as an AI agent, which is a stronger
	// claim than carrying the right fields: both are policy inputs read through
	// a cached value, so a subject decorated without rebuilding that value
	// authorizes as what it was before the decoration. Asserting it through the
	// authorizer is what distinguishes the two.
	t.Run("TheDesignationBoundaryEngages", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())

		// The owner holds the site owner role, so every denial below is
		// attributable to the boundary rather than to a subject that could do
		// nothing anyway. The human control proves that.
		agent, owner := newAgent(t, db, rbac.RoleOwner().Name)

		subject, _, _, err := httpmw.AIAgentRBACSubject(ctx, db, agent.ID, rbac.ScopeAll)
		require.NoError(t, err)
		human, _, err := httpmw.UserRBACSubject(ctx, db, owner.ID, rbac.ScopeAll)
		require.NoError(t, err)

		workspace := func(designatedTo string) rbac.Object {
			return rbac.ResourceWorkspace.
				WithID(uuid.New()).
				InOrg(uuid.New()).
				WithOwner(owner.ID.String()).
				WithAIAgentID(designatedTo)
		}

		undesignated := workspace("")
		require.NoError(t, auth.Authorize(ctx, human, policy.ActionUpdate, undesignated),
			"the owner can update their own workspace, so the roles are not what denies the agent")
		require.Error(t, auth.Authorize(ctx, subject, policy.ActionUpdate, undesignated),
			"an agent must not update a workspace designated to nobody")
		require.Error(t, auth.Authorize(ctx, subject, policy.ActionUpdate, workspace(uuid.NewString())),
			"an agent must not update a workspace designated to another agent")
		require.NoError(t, auth.Authorize(ctx, subject, policy.ActionUpdate, workspace(agent.ID.String())),
			"and must be allowed on the one designated to it")
	})

	t.Run("RefusesAnAgentThatIsNotActive", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name  string
			event entity.Event
		}{
			{"Finished", entity.EventAIAgentFinish},
			{"Killed", entity.EventAIAgentKill},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				agent, owner := newAgent(t, db)

				require.NoError(t, entity.RetireAIAgent(ctx, db, agent.ID, tc.event,
					entity.Ref{Type: entity.TypeUser, ID: owner.ID}, time.Time{}))

				_, _, _, err := httpmw.AIAgentRBACSubject(ctx, db, agent.ID, rbac.ScopeAll)
				require.ErrorContains(t, err, "is retired")
			})
		}
	})

	t.Run("RefusesAnAgentThatDoesNotExist", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, _, _, err := httpmw.AIAgentRBACSubject(ctx, db, uuid.New(), rbac.ScopeAll)
		require.Error(t, err, "no ledger row is no subject")
	})
}
