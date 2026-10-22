package networkpolicy

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

type deletionStore struct{ *startupStore }

func (s deletionStore) GetNetworkPolicyRunByID(_ context.Context, id uuid.UUID) (database.NetworkPolicyRun, error) {
	for _, runs := range s.runs {
		for _, run := range runs {
			if run.ID == id {
				return run, nil
			}
		}
	}
	return database.NetworkPolicyRun{}, sql.ErrNoRows
}

func (s deletionStore) GetNetworkPolicyRevisionByID(_ context.Context, id uuid.UUID) (database.NetworkPolicyRevision, error) {
	return database.NetworkPolicyRevision{ID: id, PolicyID: s.policies[0].ID, OrganizationID: s.template.OrganizationID}, nil
}

func TestDestroyProvenance(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		mutate         func(*startupStore)
		blocked, never bool
	}{
		{name: "LegacySuccess"},
		{name: "Explicit", mutate: func(s *startupStore) {
			s.target().StateRunID = uuid.NullUUID{UUID: s.runs[s.target().PolicyID][0].ID, Valid: true}
		}},
		{name: "NewerPublishedAndDesired", mutate: func(s *startupStore) { s.newDesiredRevision() }},
		{name: "CanceledSuccessfulApply", mutate: func(s *startupStore) {
			job := s.jobs[s.target().LastJobID.UUID]
			job.CanceledAt.Valid = true
			s.jobs[job.ID] = job
			s.target().AppliedRevisionID.Valid = false
		}},
		{name: "PlanAfterApply", mutate: func(s *startupStore) { s.addRun("plan", "") }},
		{name: "UnchangedApplyAfterApply", mutate: func(s *startupStore) { s.newDesiredRevision(); s.addRun("apply", ErrorCodeStateUnchanged) }},
		{name: "UnchangedDestroyAfterApply", mutate: func(s *startupStore) { s.addRun("destroy", ErrorCodeStateUnchanged) }},
		{name: "UnknownApply", mutate: func(s *startupStore) { s.addRun("apply", ErrorCodeStateUnknown) }, blocked: true},
		{name: "UnknownDestroy", mutate: func(s *startupStore) { s.addRun("destroy", ErrorCodeStateUnknown) }, blocked: true},
		{name: "FailedWithoutProof", mutate: func(s *startupStore) { s.addRun("apply", "OTHER_ERROR") }, blocked: true},
		{name: "Active", mutate: func(s *startupStore) { s.target().ActiveJobID = s.target().LastJobID }, blocked: true},
		{name: "Destroyed", mutate: func(s *startupStore) { s.target().DestroyedAt.Valid = true }, blocked: true},
		{name: "NoStateAfterApply", mutate: func(s *startupStore) { s.target().State = nil }, blocked: true},
		{name: "NoHistory", mutate: func(s *startupStore) { clear(s.runs) }, blocked: true},
		{name: "Gap", mutate: func(s *startupStore) { s.target().Generation++ }, blocked: true},
		{name: "DuplicateGeneration", mutate: func(s *startupStore) { id := s.target().PolicyID; s.runs[id] = append(s.runs[id], s.runs[id][0]) }, blocked: true},
		{name: "NoWorker", mutate: func(s *startupStore) {
			job := s.jobs[s.target().LastJobID.UUID]
			job.WorkerID.Valid = false
			s.jobs[job.ID] = job
		}, blocked: true},
		{name: "NeverApplied", mutate: func(s *startupStore) {
			target := s.target()
			target.Generation = 1
			target.LastJobID.Valid = false
			target.AppliedRevisionID.Valid = false
			target.State = nil
			clear(s.runs)
		}, never: true},
		{name: "OnlyUnchangedApply", mutate: func(s *startupStore) {
			target := s.target()
			job := s.jobs[target.LastJobID.UUID]
			job.ErrorCode.String = ErrorCodeStateUnchanged
			s.jobs[job.ID] = job
			target.AppliedRevisionID.Valid = false
			target.State = nil
		}, never: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newStartupStore(t)
			original := s.runs[s.target().PolicyID][0]
			if tc.mutate != nil {
				tc.mutate(s)
			}
			// Real query results are owned by the caller, as in the SQL store.
			s.runs[s.target().PolicyID] = slices.Clone(s.runs[s.target().PolicyID])
			run, err := DestroyProvenance(t.Context(), deletionStore{s}, *s.target())
			if tc.blocked {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.never {
				require.Nil(t, run)
				return
			}
			require.NotNil(t, run)
			require.Equal(t, original.ID, run.ID)
			require.Equal(t, original.RevisionID, run.RevisionID)
			require.Equal(t, original.TemplateIDs, run.TemplateIDs)
		})
	}
}
