package networkpolicy

import (
	"context"
	"slices"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// DestroyProvenance returns the exact successful state-producing apply. A nil
// run proves the target never had state or a potentially mutating operation.
// Callers hold the policy and target locks and must not release unknown jobs.
//
//nolint:nilnil // A nil run explicitly proves no destroy is necessary.
func DestroyProvenance(ctx context.Context, db database.Store, target database.NetworkPolicyTarget) (*database.NetworkPolicyRun, error) {
	if target.ActiveJobID.Valid || target.DestroyedAt.Valid {
		return nil, xerrors.New("target is reserved or already destroyed")
	}
	if target.StateRunID.Valid {
		run, err := db.GetNetworkPolicyRunByID(ctx, target.StateRunID.UUID)
		if err != nil {
			return nil, xerrors.Errorf("get state-producing run: %w", err)
		}
		if len(target.State) == 0 {
			return nil, xerrors.New("state-producing run has no retained state")
		}
		if err := validateStateRun(ctx, db, target, run); err != nil {
			return nil, err
		}
		return &run, nil
	}

	// Before explicit provenance was stored, only a contiguous history ending
	// in successful apply, followed by proven unchanged operations, is trusted.
	runs, err := db.GetNetworkPolicyRunsByPolicyID(ctx, target.PolicyID)
	if err != nil {
		return nil, xerrors.Errorf("get retained state history: %w", err)
	}
	runs = slices.DeleteFunc(runs, func(run database.NetworkPolicyRun) bool { return run.TargetID != target.ID })
	slices.SortFunc(runs, func(a, b database.NetworkPolicyRun) int {
		if a.Generation > b.Generation {
			return -1
		}
		if a.Generation < b.Generation {
			return 1
		}
		return 0
	})
	if len(runs) == 0 {
		if target.Generation == 1 && !target.LastJobID.Valid && !target.AppliedRevisionID.Valid && len(target.State) == 0 {
			return nil, nil
		}
		return nil, xerrors.New("target has no trusted state history")
	}
	if !target.LastJobID.Valid || runs[0].JobID != target.LastJobID.UUID {
		return nil, xerrors.New("latest target run is missing")
	}
	generation := target.Generation
	for i, run := range runs {
		if run.Generation != generation || run.PolicyID != target.PolicyID || run.OrganizationID != target.OrganizationID || (i+1 < len(runs) && runs[i+1].Generation == run.Generation) {
			return nil, xerrors.New("ambiguous target state history")
		}
		generation--
		job, err := db.GetProvisionerJobByID(ctx, run.JobID)
		if err != nil {
			return nil, xerrors.Errorf("get historical apply job: %w", err)
		}
		if job.Type != database.ProvisionerJobTypeNetworkPolicy || job.OrganizationID != target.OrganizationID || !job.CompletedAt.Valid || job.ErrorCode.String == ErrorCodeStateUnknown {
			return nil, xerrors.New("target history has an uncertain outcome")
		}
		if run.Operation != "plan" && run.Operation != "apply" && run.Operation != "destroy" {
			return nil, xerrors.New("target history contains an invalid operation")
		}
		if run.Operation == "plan" || job.ErrorCode.String == ErrorCodeStateUnchanged {
			continue
		}
		if len(target.State) == 0 {
			return nil, xerrors.New("potential apply has no retained state")
		}
		if err := validateStateRun(ctx, db, target, run); err != nil {
			return nil, err
		}
		return &run, nil
	}
	if generation == 1 && len(target.State) == 0 && !target.AppliedRevisionID.Valid {
		return nil, nil
	}
	return nil, xerrors.New("no successful apply establishes retained state")
}

func validateStateRun(ctx context.Context, db database.Store, target database.NetworkPolicyTarget, run database.NetworkPolicyRun) error {
	if run.TargetID != target.ID || run.PolicyID != target.PolicyID || run.OrganizationID != target.OrganizationID || run.Generation > target.Generation || run.Operation != "apply" {
		return xerrors.New("state-producing run does not match target")
	}
	job, err := db.GetProvisionerJobByID(ctx, run.JobID)
	if err != nil {
		return xerrors.Errorf("get state-producing job: %w", err)
	}
	if job.Type != database.ProvisionerJobTypeNetworkPolicy || job.OrganizationID != target.OrganizationID || !job.CompletedAt.Valid || !job.StartedAt.Valid || !job.WorkerID.Valid || job.Error.Valid || job.Error.String != "" || job.ErrorCode.String != "" {
		return xerrors.New("state-producing apply outcome is not established")
	}
	revision, err := db.GetNetworkPolicyRevisionByID(ctx, run.RevisionID)
	if err != nil {
		return xerrors.Errorf("get state-producing revision: %w", err)
	}
	if revision.PolicyID != target.PolicyID || revision.OrganizationID != target.OrganizationID || revision.FileID != job.FileID {
		return xerrors.New("state-producing revision does not match job archive")
	}
	return nil
}
