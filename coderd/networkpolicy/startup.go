package networkpolicy

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// ErrorCodeStateUnknown marks a policy job whose apply may have mutated
	// infrastructure without returning authoritative final state.
	ErrorCodeStateUnknown = "NETWORK_POLICY_STATE_UNKNOWN"
	// ErrorCodeStateUnchanged proves that a failed policy job did not start
	// applying. Nonempty state or the absence of an error is not this proof.
	ErrorCodeStateUnchanged = "NETWORK_POLICY_STATE_UNCHANGED"
)

// StartupMode determines whether a previously applied revision may be used.
type StartupMode string

const (
	StartupRequireCurrent   StartupMode = "require-current"
	StartupAllowLastApplied StartupMode = "allow-last-applied"
)

// DriftAction determines whether known drift of the current revision blocks
// startup. Allowing drift is an operator exception, not proof of live enforcement.
type DriftAction string

const (
	DriftBlock DriftAction = "block"
	DriftAllow DriftAction = "allow"
)

// StartupOptions configures admission of workspace starts. Zero values enforce
// the current published revision and block known drift. There is no bypass for
// unknown state, pending jobs, missing targets, or never-applied policies.
type StartupOptions struct {
	Mode        StartupMode
	DriftAction DriftAction
}

// StartupOptionsFromDeployment copies the deployment's startup settings.
// Missing settings retain the fail-closed defaults of CheckStartup.
func StartupOptionsFromDeployment(values *codersdk.DeploymentValues) StartupOptions {
	if values == nil {
		return StartupOptions{}
	}
	return StartupOptions{
		Mode:        StartupMode(values.NetworkPolicyStartupMode),
		DriftAction: DriftAction(values.NetworkPolicyDriftAction),
	}
}

// ErrStartupBlocked means an attached policy cannot admit a workspace start.
// Its wrapped reasons never contain policy state, target names, or template IDs.
var ErrStartupBlocked = xerrors.New("attached network policies are not ready for workspace startup")

// CheckStartup checks every target of every policy attached to the template.
// Callers must authorize the workspace operation before using this internal
// check. Policy administration is not required to start a workspace, so the
// reads use a restricted system actor rather than silently filtering policies
// that the workspace owner cannot administer.
//
// The check uses the caller's transaction when present, otherwise it creates a
// repeatable-read transaction. It is a point-in-time admission check, not a live
// cluster probe or a lease preventing policy changes after dispatch. Coder does
// not currently have authoritative workspace-to-cluster/namespace mapping, so
// no policy target is excluded based on guessed workspace metadata or tags.
func CheckStartup(ctx context.Context, store database.Store, template database.Template, options StartupOptions) error {
	return database.ReadModifyUpdate(store, func(tx database.Store) error {
		return CheckStartupInTx(ctx, tx, template, options)
	})
}

// CheckStartupInTx performs CheckStartup's readiness checks on an existing
// repeatable-read transaction. Callers must retain that transaction until the
// workspace build is inserted, rather than releasing the policy locks early.
func CheckStartupInTx(ctx context.Context, store database.Store, template database.Template, options StartupOptions) error {
	if options.Mode == "" {
		options.Mode = StartupRequireCurrent
	}
	if options.DriftAction == "" {
		options.DriftAction = DriftBlock
	}
	if options.Mode != StartupRequireCurrent && options.Mode != StartupAllowLastApplied {
		return xerrors.Errorf("%w: invalid startup mode", ErrStartupBlocked)
	}
	if options.DriftAction != DriftBlock && options.DriftAction != DriftAllow {
		return xerrors.Errorf("%w: invalid drift action", ErrStartupBlocked)
	}
	//nolint:gocritic // Read all attached policies, including those the workspace owner cannot administer.
	ctx = dbauthz.AsSystemRestricted(ctx)
	tx := store
	policies, err := tx.GetNetworkPoliciesByOrganizationID(ctx, template.OrganizationID)
	if err != nil {
		return xerrors.Errorf("get network policies for startup: %w", err)
	}
	// Multiple policy checks acquire their locks in a stable order.
	slices.SortFunc(policies, func(a, b database.NetworkPolicy) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	for _, candidate := range policies {
		attachments, err := tx.GetNetworkPolicyAttachmentsByPolicyID(ctx, candidate.ID)
		if err != nil {
			return xerrors.Errorf("get network policy attachments for startup: %w", err)
		}
		attached := slices.ContainsFunc(attachments, func(a database.NetworkPolicyAttachment) bool { return a.TemplateID == template.ID })
		if !attached {
			continue
		}
		policy, err := tx.GetNetworkPolicyByIDForUpdate(ctx, candidate.ID)
		if err != nil {
			return xerrors.Errorf("lock network policy for startup: %w", err)
		}
		if policy.DeletionRequestedAt.Valid || policy.DeletedAt.Valid {
			return xerrors.Errorf("%w: an attached policy is being deleted", ErrStartupBlocked)
		}
		if !policy.PublishedRevisionID.Valid {
			return xerrors.Errorf("%w: an attached policy has no published revision", ErrStartupBlocked)
		}
		targets, err := tx.GetNetworkPolicyTargetsByPolicyID(ctx, policy.ID)
		if err != nil {
			return xerrors.Errorf("get network policy targets for startup: %w", err)
		}
		if len(targets) == 0 {
			return xerrors.Errorf("%w: an attached policy has no configured targets", ErrStartupBlocked)
		}
		runs, err := tx.GetNetworkPolicyRunsByPolicyID(ctx, policy.ID)
		if err != nil {
			return xerrors.Errorf("get network policy runs for startup: %w", err)
		}
		slices.SortFunc(targets, func(a, b database.NetworkPolicyTarget) int { return bytes.Compare(a.ID[:], b.ID[:]) })
		for _, candidate := range targets {
			target, err := tx.GetNetworkPolicyTargetByIDForUpdate(ctx, candidate.ID)
			if err != nil {
				return xerrors.Errorf("lock network policy target for startup: %w", err)
			}
			if err := checkStartupTarget(ctx, tx, template.ID, policy, target, runs, options); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkStartupTarget(ctx context.Context, store database.Store, templateID uuid.UUID, policy database.NetworkPolicy, target database.NetworkPolicyTarget, runs []database.NetworkPolicyRun, options StartupOptions) error {
	if policy.DeletionRequestedAt.Valid || policy.DeletedAt.Valid || target.DestroyedAt.Valid {
		return xerrors.Errorf("%w: a policy or target is being deleted", ErrStartupBlocked)
	}
	if target.PolicyID != policy.ID || target.OrganizationID != policy.OrganizationID {
		return xerrors.Errorf("%w: inconsistent target ownership", ErrStartupBlocked)
	}
	if target.ActiveJobID.Valid {
		return xerrors.Errorf("%w: a target has a pending, running, or quarantined job", ErrStartupBlocked)
	}
	if !target.AppliedRevisionID.Valid || len(target.State) == 0 {
		return xerrors.Errorf("%w: a target has never been successfully applied", ErrStartupBlocked)
	}
	switch target.Status {
	case "succeeded", "failed", "canceled":
	default:
		return xerrors.Errorf("%w: a target is not in a completed state", ErrStartupBlocked)
	}
	current := target.DesiredRevisionID.Valid && target.DesiredRevisionID.UUID == target.AppliedRevisionID.UUID && policy.PublishedRevisionID.Valid && policy.PublishedRevisionID.UUID == target.AppliedRevisionID.UUID
	if options.Mode == StartupRequireCurrent && !current {
		return xerrors.Errorf("%w: a target has not applied the current revision", ErrStartupBlocked)
	}
	if target.Drifted && (options.DriftAction != DriftAllow || !current) {
		return xerrors.Errorf("%w: a target has known drift", ErrStartupBlocked)
	}

	// A succeeded plan is not an apply. Likewise, a retained applied revision
	// and nonempty checkpoint do not prove that a later apply was harmless.
	var targetRuns []database.NetworkPolicyRun
	for _, run := range runs {
		if run.TargetID == target.ID {
			targetRuns = append(targetRuns, run)
		}
	}
	slices.SortFunc(targetRuns, func(a, b database.NetworkPolicyRun) int {
		switch {
		case a.Generation > b.Generation:
			return -1
		case a.Generation < b.Generation:
			return 1
		default:
			return bytes.Compare(a.ID[:], b.ID[:])
		}
	})
	if len(targetRuns) == 0 || !target.LastJobID.Valid || targetRuns[0].JobID != target.LastJobID.UUID || targetRuns[0].Generation != target.Generation {
		return xerrors.Errorf("%w: the latest target outcome is missing", ErrStartupBlocked)
	}
	for _, run := range targetRuns {
		if run.PolicyID != policy.ID || run.OrganizationID != policy.OrganizationID || run.Generation > target.Generation {
			return xerrors.Errorf("%w: inconsistent apply provenance", ErrStartupBlocked)
		}
		job, err := store.GetProvisionerJobByID(ctx, run.JobID)
		if err != nil {
			return xerrors.Errorf("get network policy apply outcome for startup: %w", err)
		}
		if job.ID != run.JobID || job.Type != database.ProvisionerJobTypeNetworkPolicy || job.OrganizationID != policy.OrganizationID || !job.CompletedAt.Valid {
			return xerrors.Errorf("%w: an apply outcome is not established", ErrStartupBlocked)
		}
		if job.ErrorCode.String == ErrorCodeStateUnknown {
			return xerrors.Errorf("%w: a target requires reconciliation after an uncertain outcome", ErrStartupBlocked)
		}
		if run.Operation == "plan" {
			continue
		}
		if run.Operation != "apply" {
			return xerrors.Errorf("%w: invalid run operation", ErrStartupBlocked)
		}
		if job.Error.Valid || job.Error.String != "" || job.CanceledAt.Valid {
			if job.ErrorCode.String == ErrorCodeStateUnchanged {
				continue
			}
			return xerrors.Errorf("%w: a failed apply has no proof of unchanged state", ErrStartupBlocked)
		}
		if !job.StartedAt.Valid || !job.WorkerID.Valid {
			return xerrors.Errorf("%w: no provisioner established the apply outcome", ErrStartupBlocked)
		}
		if run.RevisionID != target.AppliedRevisionID.UUID || !slices.Contains(run.TemplateIDs, templateID) {
			return xerrors.Errorf("%w: the last applied policy does not cover this template", ErrStartupBlocked)
		}
		return nil
	}
	return xerrors.Errorf("%w: no successful apply establishes the target state", ErrStartupBlocked)
}
