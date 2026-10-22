package networkpolicy

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
)

type startupStore struct {
	database.Store
	t           *testing.T
	template    database.Template
	policies    []database.NetworkPolicy
	attachments map[uuid.UUID][]database.NetworkPolicyAttachment
	targets     map[uuid.UUID][]database.NetworkPolicyTarget
	runs        map[uuid.UUID][]database.NetworkPolicyRun
	jobs        map[uuid.UUID]database.ProvisionerJob
	readError   error
}

func (s *startupStore) InTx(fn func(database.Store) error, _ *database.TxOptions) error { return fn(s) }

func (s *startupStore) GetNetworkPoliciesByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]database.NetworkPolicy, error) {
	actor, ok := dbauthz.ActorFromContext(ctx)
	require.True(s.t, ok)
	require.Equal(s.t, rbac.SubjectTypeSystemRestricted, actor.Type, "ordinary workspace users must not silently filter out policies")
	require.Equal(s.t, s.template.OrganizationID, orgID)
	return slices.Clone(s.policies), s.readError
}

func (s *startupStore) GetNetworkPolicyAttachmentsByPolicyID(_ context.Context, id uuid.UUID) ([]database.NetworkPolicyAttachment, error) {
	return s.attachments[id], nil
}

func (s *startupStore) GetNetworkPolicyByIDForUpdate(_ context.Context, id uuid.UUID) (database.NetworkPolicy, error) {
	for _, policy := range s.policies {
		if policy.ID == id {
			return policy, nil
		}
	}
	return database.NetworkPolicy{}, sql.ErrNoRows
}

func (s *startupStore) GetNetworkPolicyTargetsByPolicyID(_ context.Context, id uuid.UUID) ([]database.NetworkPolicyTarget, error) {
	return slices.Clone(s.targets[id]), nil
}

func (s *startupStore) GetNetworkPolicyTargetByIDForUpdate(_ context.Context, id uuid.UUID) (database.NetworkPolicyTarget, error) {
	for _, targets := range s.targets {
		for _, target := range targets {
			if target.ID == id {
				return target, nil
			}
		}
	}
	return database.NetworkPolicyTarget{}, sql.ErrNoRows
}

func (s *startupStore) GetNetworkPolicyRunsByPolicyID(_ context.Context, id uuid.UUID) ([]database.NetworkPolicyRun, error) {
	return s.runs[id], nil
}

func (s *startupStore) GetProvisionerJobByID(_ context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	job, ok := s.jobs[id]
	if !ok {
		return database.ProvisionerJob{}, sql.ErrNoRows
	}
	return job, nil
}

func newStartupStore(t *testing.T) *startupStore {
	t.Helper()
	template := database.Template{ID: uuid.New(), OrganizationID: uuid.New()}
	policy := database.NetworkPolicy{ID: uuid.New(), OrganizationID: template.OrganizationID, PublishedRevisionID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	target := database.NetworkPolicyTarget{
		ID: uuid.New(), PolicyID: policy.ID, OrganizationID: policy.OrganizationID,
		Generation: 2, State: []byte("authoritative applied state"), AppliedRevisionID: policy.PublishedRevisionID,
		DesiredRevisionID: policy.PublishedRevisionID, Status: "succeeded",
	}
	job := database.ProvisionerJob{
		ID: uuid.New(), OrganizationID: policy.OrganizationID, Type: database.ProvisionerJobTypeNetworkPolicy,
		StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}
	target.LastJobID = uuid.NullUUID{UUID: job.ID, Valid: true}
	run := database.NetworkPolicyRun{
		ID: uuid.New(), PolicyID: policy.ID, OrganizationID: policy.OrganizationID, TargetID: target.ID,
		JobID: job.ID, RevisionID: policy.PublishedRevisionID.UUID, Generation: target.Generation, Operation: "apply", TemplateIDs: []uuid.UUID{template.ID},
	}
	return &startupStore{
		t: t, template: template, policies: []database.NetworkPolicy{policy},
		attachments: map[uuid.UUID][]database.NetworkPolicyAttachment{policy.ID: {{PolicyID: policy.ID, TemplateID: template.ID, OrganizationID: template.OrganizationID}}},
		targets:     map[uuid.UUID][]database.NetworkPolicyTarget{policy.ID: {target}}, runs: map[uuid.UUID][]database.NetworkPolicyRun{policy.ID: {run}},
		jobs: map[uuid.UUID]database.ProvisionerJob{job.ID: job},
	}
}

func (s *startupStore) target() *database.NetworkPolicyTarget { return &s.targets[s.policies[0].ID][0] }

func (s *startupStore) addRun(operation, errorCode string) {
	target := s.target()
	target.Generation++
	job := database.ProvisionerJob{
		ID: uuid.New(), OrganizationID: target.OrganizationID, Type: database.ProvisionerJobTypeNetworkPolicy,
		StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}
	if errorCode != "" {
		job.Error = sql.NullString{String: "job failed", Valid: true}
		job.ErrorCode = sql.NullString{String: errorCode, Valid: true}
		target.Status = "failed"
	}
	target.LastJobID = uuid.NullUUID{UUID: job.ID, Valid: true}
	s.jobs[job.ID] = job
	s.runs[target.PolicyID] = append(s.runs[target.PolicyID], database.NetworkPolicyRun{
		ID: uuid.New(), JobID: job.ID, TargetID: target.ID, PolicyID: target.PolicyID, OrganizationID: target.OrganizationID,
		RevisionID: target.DesiredRevisionID.UUID, Generation: target.Generation, Operation: operation, TemplateIDs: []uuid.UUID{s.template.ID},
	})
}

func (s *startupStore) newDesiredRevision() {
	s.policies[0].PublishedRevisionID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	s.target().DesiredRevisionID = s.policies[0].PublishedRevisionID
}

func TestCheckStartup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mutate  func(*startupStore)
		options StartupOptions
		blocked bool
	}{
		{name: "CurrentApplied"},
		{name: "Deleting", mutate: func(s *startupStore) { s.policies[0].DeletionRequestedAt.Valid = true }, blocked: true},
		{name: "DeletingCannotUseFallback", mutate: func(s *startupStore) { s.policies[0].DeletionRequestedAt.Valid = true }, options: StartupOptions{Mode: StartupAllowLastApplied, DriftAction: DriftAllow}, blocked: true},
		{name: "DestroyedTarget", mutate: func(s *startupStore) { s.target().DestroyedAt.Valid = true }, blocked: true},
		{name: "NoPolicies", mutate: func(s *startupStore) { s.policies = nil }},
		{name: "NoAttachment", mutate: func(s *startupStore) { clear(s.attachments) }},
		{name: "Unpublished", mutate: func(s *startupStore) { s.policies[0].PublishedRevisionID = uuid.NullUUID{} }, blocked: true},
		{name: "NoTargets", mutate: func(s *startupStore) { clear(s.targets) }, blocked: true},
		{name: "NeverApplied", mutate: func(s *startupStore) { s.target().AppliedRevisionID = uuid.NullUUID{} }, blocked: true},
		{name: "MissingState", mutate: func(s *startupStore) { s.target().State = nil }, blocked: true},
		{name: "Pending", mutate: func(s *startupStore) { s.target().ActiveJobID = uuid.NullUUID{UUID: uuid.New(), Valid: true} }, blocked: true},
		{name: "PendingStatusWithoutReservation", mutate: func(s *startupStore) { s.target().Status = "pending" }, blocked: true},
		{name: "UnknownStatus", mutate: func(s *startupStore) { s.target().Status = "unknown" }, blocked: true},
		{name: "NoProvenance", mutate: func(s *startupStore) { clear(s.runs) }, blocked: true},
		{name: "PlanIsNotApply", mutate: func(s *startupStore) { s.runs[s.policies[0].ID][0].Operation = "plan" }, blocked: true},
		{name: "PlanAfterApply", mutate: func(s *startupStore) { s.addRun("plan", "") }},
		{name: "FailedPlanAfterApply", mutate: func(s *startupStore) { s.addRun("plan", ErrorCodeStateUnchanged) }},
		{name: "TemplateNotInAppliedSnapshot", mutate: func(s *startupStore) { s.runs[s.policies[0].ID][0].TemplateIDs = []uuid.UUID{uuid.New()} }, blocked: true},
		{name: "NewDesiredRevision", mutate: func(s *startupStore) { s.newDesiredRevision() }, blocked: true},
		{name: "SafeLastApplied", mutate: func(s *startupStore) { s.newDesiredRevision(); s.addRun("apply", ErrorCodeStateUnchanged) }, options: StartupOptions{Mode: StartupAllowLastApplied}},
		{name: "LastAppliedMissingSnapshot", mutate: func(s *startupStore) {
			s.newDesiredRevision()
			s.addRun("apply", ErrorCodeStateUnchanged)
			s.runs[s.policies[0].ID][0].TemplateIDs = nil
		}, options: StartupOptions{Mode: StartupAllowLastApplied}, blocked: true},
		{name: "LastAppliedNeverApplied", mutate: func(s *startupStore) { s.target().AppliedRevisionID = uuid.NullUUID{} }, options: StartupOptions{Mode: StartupAllowLastApplied}, blocked: true},
		{name: "LastAppliedPending", mutate: func(s *startupStore) {
			s.newDesiredRevision()
			s.addRun("apply", "")
			s.target().ActiveJobID = s.target().LastJobID
		}, options: StartupOptions{Mode: StartupAllowLastApplied}, blocked: true},
		{name: "UnknownCheckpoint", mutate: func(s *startupStore) {
			s.addRun("apply", ErrorCodeStateUnknown)
			s.target().State = []byte("partial checkpoint")
		}, options: StartupOptions{Mode: StartupAllowLastApplied, DriftAction: DriftAllow}, blocked: true},
		{name: "KilledApplyOldState", mutate: func(s *startupStore) { s.addRun("apply", ErrorCodeStateUnknown) }, options: StartupOptions{Mode: StartupAllowLastApplied}, blocked: true},
		{name: "UnprovenFailedApply", mutate: func(s *startupStore) { s.addRun("apply", "GENERIC_ERROR") }, options: StartupOptions{Mode: StartupAllowLastApplied}, blocked: true},
		{name: "PlanCannotClearUnknownApply", mutate: func(s *startupStore) { s.addRun("apply", ErrorCodeStateUnknown); s.addRun("plan", "") }, options: StartupOptions{Mode: StartupAllowLastApplied}, blocked: true},
		{name: "SuccessfulApplyReestablishesState", mutate: func(s *startupStore) {
			s.addRun("apply", ErrorCodeStateUnknown)
			s.addRun("apply", "")
			s.target().Status = "succeeded"
		}},
		{name: "DefaultBlocksDrift", mutate: func(s *startupStore) { s.target().Drifted = true }, blocked: true},
		{name: "ExplicitCurrentRevisionDriftException", mutate: func(s *startupStore) { s.target().Drifted = true }, options: StartupOptions{DriftAction: DriftAllow}},
		{name: "DriftedOldRevisionNeverFallback", mutate: func(s *startupStore) { s.newDesiredRevision(); s.target().Drifted = true }, options: StartupOptions{Mode: StartupAllowLastApplied, DriftAction: DriftAllow}, blocked: true},
		{name: "EveryTargetChecked", mutate: func(s *startupStore) {
			other := *s.target()
			other.ID = uuid.New()
			other.State = nil
			s.targets[other.PolicyID] = append(s.targets[other.PolicyID], other)
		}, blocked: true},
		{name: "EveryAttachedPolicyChecked", mutate: func(s *startupStore) {
			p := database.NetworkPolicy{ID: uuid.New(), OrganizationID: s.template.OrganizationID}
			s.policies = append(s.policies, p)
			s.attachments[p.ID] = []database.NetworkPolicyAttachment{{PolicyID: p.ID, TemplateID: s.template.ID}}
		}, blocked: true},
		{name: "UnrelatedUnreadyPolicyIgnored", mutate: func(s *startupStore) {
			s.policies = append(s.policies, database.NetworkPolicy{ID: uuid.New(), OrganizationID: s.template.OrganizationID})
		}},
		{name: "MissingLatestJob", mutate: func(s *startupStore) { s.target().LastJobID = uuid.NullUUID{} }, blocked: true},
		{name: "GenerationMismatch", mutate: func(s *startupStore) { s.target().Generation++ }, blocked: true},
		{name: "InvalidMode", options: StartupOptions{Mode: "disabled"}, blocked: true},
		{name: "InvalidDriftAction", options: StartupOptions{DriftAction: "ignored"}, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newStartupStore(t)
			if tc.mutate != nil {
				tc.mutate(store)
			}
			err := CheckStartup(t.Context(), store, store.template, tc.options)
			if tc.blocked {
				require.ErrorIs(t, err, ErrStartupBlocked)
				for _, p := range store.policies {
					require.NotContains(t, err.Error(), p.ID.String())
				}
				require.NotContains(t, err.Error(), store.template.ID.String())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckStartupReadFailure(t *testing.T) {
	t.Parallel()
	store := newStartupStore(t)
	store.readError = xerrors.New("database unavailable")
	err := CheckStartup(t.Context(), store, store.template, StartupOptions{})
	require.ErrorIs(t, err, store.readError)
}
