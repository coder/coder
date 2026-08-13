package rbac_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

// protectedWorkspaceActions are the workspace actions the designation boundary
// requires a matching designation for. Read and create are exempt.
var protectedWorkspaceActions = []policy.Action{
	policy.ActionUpdate,
	policy.ActionDelete,
	policy.ActionWorkspaceStart,
	policy.ActionWorkspaceStop,
	policy.ActionSSH,
	policy.ActionApplicationConnect,
}

// aiDesignationFixture builds a sponsor with owner-level roles plus two AI agent
// identities delegating for that sponsor. Owner roles are used deliberately: the
// sponsor ceiling must not be what denies the AI actor, otherwise the test would
// pass without the boundary doing any work.
type aiDesignationFixture struct {
	sponsorID uuid.UUID
	agentA    uuid.UUID
	agentB    uuid.UUID
	human     rbac.Subject
	actorA    rbac.Subject
	actorB    rbac.Subject
}

func newAIDesignationFixture(t *testing.T) aiDesignationFixture {
	t.Helper()

	sponsorID := uuid.New()
	agentA := uuid.New()
	agentB := uuid.New()

	human := rbac.Subject{
		Type:  rbac.SubjectTypeUser,
		ID:    sponsorID.String(),
		Roles: rbac.RoleIdentifiers{rbac.RoleOwner()},
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	return aiDesignationFixture{
		sponsorID: sponsorID,
		agentA:    agentA,
		agentB:    agentB,
		human:     human,
		actorA:    human.AsAIAgent(agentA, "agent-a"),
		actorB:    human.AsAIAgent(agentB, "agent-b"),
	}
}

// workspaceOwnedBySponsor returns an undesignated workspace object owned by the
// sponsor, which is the shape of an ordinary human workspace.
func (f aiDesignationFixture) workspaceOwnedBySponsor() rbac.Object {
	return rbac.ResourceWorkspace.
		WithID(uuid.New()).
		InOrg(uuid.New()).
		WithOwner(f.sponsorID.String())
}

func (f aiDesignationFixture) workspaceDesignatedTo(agentID uuid.UUID) rbac.Object {
	return f.workspaceOwnedBySponsor().WithAIAgentID(agentID.String())
}

func TestAIDesignationBoundary(t *testing.T) {
	t.Parallel()

	auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())

	t.Run("DeniesProtectedActionsOnUndesignatedWorkspace", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		ordinary := f.workspaceOwnedBySponsor()

		for _, action := range protectedWorkspaceActions {
			err := auth.Authorize(ctx, f.actorA, action, ordinary)
			require.Error(t, err, "AI actor must not %s an undesignated workspace", action)
		}
	})

	t.Run("AllowsProtectedActionsOnOwnWorkspace", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		own := f.workspaceDesignatedTo(f.agentA)

		for _, action := range protectedWorkspaceActions {
			err := auth.Authorize(ctx, f.actorA, action, own)
			require.NoError(t, err, "AI actor must %s a workspace designated to it", action)
		}
	})

	t.Run("DeniesAnotherAgentsWorkspace", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		// Same sponsor, different agent identity.
		theirs := f.workspaceDesignatedTo(f.agentB)

		for _, action := range protectedWorkspaceActions {
			err := auth.Authorize(ctx, f.actorA, action, theirs)
			require.Error(t, err, "AI actor must not %s a workspace designated to another agent", action)
		}
	})

	t.Run("ExemptActionsAreAllowedEverywhere", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Read supports workspace inventory for a chat.
		require.NoError(t, auth.Authorize(ctx, f.actorA, policy.ActionRead, f.workspaceOwnedBySponsor()))
		require.NoError(t, auth.Authorize(ctx, f.actorA, policy.ActionRead, f.workspaceDesignatedTo(f.agentB)))

		// Create must be authorized before the workspace has an ID to designate.
		require.NoError(t, auth.Authorize(ctx, f.actorA, policy.ActionCreate, f.workspaceOwnedBySponsor()))

		// Agent lifecycle actions update agent rows and daemon state, not the
		// human workspace's runtime or credentials. Bound agents in ordinary
		// workspaces need these to report startup, lifecycle, metadata, apps,
		// and sub-agent state. Exact workspace scope and API parent checks
		// remain responsible for constraining the target rows.
		for _, action := range []policy.Action{
			policy.ActionCreateAgent,
			policy.ActionUpdateAgent,
			policy.ActionDeleteAgent,
		} {
			require.NoError(t, auth.Authorize(ctx, f.actorA, action, f.workspaceOwnedBySponsor()),
				"AI actor must be able to perform agent lifecycle action %s", action)
		}
	})

	t.Run("HumanSubjectIsUnaffected", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		for _, obj := range []rbac.Object{
			f.workspaceOwnedBySponsor(),
			f.workspaceDesignatedTo(f.agentA),
			f.workspaceDesignatedTo(f.agentB),
		} {
			for _, action := range protectedWorkspaceActions {
				err := auth.Authorize(ctx, f.human, action, obj)
				require.NoError(t, err, "human must retain %s on every workspace", action)
			}
		}
	})

	t.Run("FailsClosedOnEmptyActingIdentity", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// An AI subject type whose acting identity was never populated must be
		// denied protected actions rather than treated as a human.
		broken := rbac.Subject{
			Type:  rbac.SubjectTypeAIAgent,
			ID:    f.sponsorID.String(),
			Roles: rbac.RoleIdentifiers{rbac.RoleOwner()},
			Scope: rbac.ScopeAll,
		}.WithCachedASTValue()

		for _, action := range protectedWorkspaceActions {
			err := auth.Authorize(ctx, broken, action, f.workspaceDesignatedTo(f.agentA))
			require.Error(t, err, "an AI subject with no acting identity must not %s any workspace", action)
		}
		// It also cannot reach an undesignated workspace, where both sides are
		// empty strings.
		require.Error(t, auth.Authorize(ctx, broken, policy.ActionSSH, f.workspaceOwnedBySponsor()))
	})

	t.Run("DeniesAggregateObject", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// All() clears the designation, so an aggregate object cannot satisfy an
		// exact match for a protected action.
		all := f.workspaceDesignatedTo(f.agentA).All()
		require.Empty(t, all.AIAgentID, "All must clear the designation")
		require.Error(t, auth.Authorize(ctx, f.actorA, policy.ActionSSH, all))
	})

	t.Run("DormantAndPrebuiltTypesAreProtected", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Each type is probed with an action its resource definition and the
		// owner role both permit, so the only variable under test is
		// designation. Dormant workspaces cannot be started (they are activated
		// with update first) and prebuilt workspaces support only update and
		// delete.
		cases := []struct {
			resource rbac.Object
			action   policy.Action
		}{
			{rbac.ResourceWorkspaceDormant, policy.ActionWorkspaceStop},
			{rbac.ResourcePrebuiltWorkspace, policy.ActionUpdate},
		}
		for _, tc := range cases {
			undesignated := tc.resource.
				WithID(uuid.New()).
				InOrg(uuid.New()).
				WithOwner(f.sponsorID.String())
			require.Error(t, auth.Authorize(ctx, f.actorA, tc.action, undesignated),
				"%s must be protected", tc.resource.Type)
			require.NoError(t, auth.Authorize(ctx, f.actorA, tc.action,
				undesignated.WithAIAgentID(f.agentA.String())),
				"%s designated to the actor must be permitted", tc.resource.Type)
		}
	})

	t.Run("NonWorkspaceResourcesAreUnaffected", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// The boundary only governs workspace-typed objects. A template update
		// is not a workspace action, so designation never applies to it.
		template := rbac.ResourceTemplate.WithID(uuid.New()).InOrg(uuid.New())
		require.NoError(t, auth.Authorize(ctx, f.actorA, policy.ActionRead, template))
		require.NoError(t, auth.Authorize(ctx, f.actorA, policy.ActionUpdate, template))
	})
}

// TestAIDesignationPartialEvaluation covers the SQL filter path. Workspace
// listing is the hot path for partial evaluation, so the read exemption must
// stay ground and emit no designation predicate; protected actions must still
// compile, which requires the regosql converter to be registered.
func TestAIDesignationPartialEvaluation(t *testing.T) {
	t.Parallel()

	auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())

	t.Run("ReadFilterIsUnchangedForAIActor", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		humanPrepared, err := auth.Prepare(ctx, f.human, policy.ActionRead, rbac.ResourceWorkspace.Type)
		require.NoError(t, err)
		humanSQL, err := humanPrepared.CompileToSQL(ctx, rbac.ConfigWorkspaces())
		require.NoError(t, err)

		aiPrepared, err := auth.Prepare(ctx, f.actorA, policy.ActionRead, rbac.ResourceWorkspace.Type)
		require.NoError(t, err)
		aiSQL, err := aiPrepared.CompileToSQL(ctx, rbac.ConfigWorkspaces())
		require.NoError(t, err)

		// The acting identity must not leak into the workspace list filter: the
		// read exemption resolves at partial evaluation time because the action
		// and object type are known.
		require.Equal(t, humanSQL, aiSQL,
			"an AI actor's workspace read filter must match a human's")
		require.NotContains(t, aiSQL, "ai_agent_id",
			"the read filter must not carry a designation predicate")
	})

	t.Run("ProtectedActionCompiles", func(t *testing.T) {
		t.Parallel()

		f := newAIDesignationFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// A protected action leaves the designation comparison in the residual,
		// so this fails if the regosql converter is missing a matcher.
		prepared, err := auth.Prepare(ctx, f.actorA, policy.ActionSSH, rbac.ResourceWorkspace.Type)
		require.NoError(t, err)
		sql, err := prepared.CompileToSQL(ctx, rbac.ConfigWorkspaces())
		require.NoError(t, err, "protected-action SQL must compile with the designation matcher")
		require.NotEmpty(t, sql)
	})
}
