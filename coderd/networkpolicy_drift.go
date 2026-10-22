package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// startNetworkPolicyDrift waits one interval before checking already-applied
// policies. The returned function cancels and joins the worker before the API's
// database and pubsub dependencies close. Nonpositive intervals disable it.
func (api *API) startNetworkPolicyDrift(interval time.Duration) func() {
	if interval <= 0 {
		return func() {}
	}
	ctx, cancel := context.WithCancel(api.ctx)
	ticker := api.Clock.TickerFunc(ctx, interval, func() error {
		if err := api.queueNetworkPolicyDrift(ctx); err != nil && ctx.Err() == nil {
			api.Logger.Error(ctx, "check network policy drift", slog.Error(err))
		}
		return nil
	}, "network-policy-drift")
	return sync.OnceFunc(func() {
		cancel()
		_ = ticker.Wait()
	})
}

func (api *API) queueNetworkPolicyDrift(ctx context.Context) error {
	//nolint:gocritic // Internal report-only worker checks previously applied policies, never applies changes or releases uncertain reservations.
	ctx = dbauthz.AsSystemRestricted(ctx)
	organizations, err := api.Database.GetOrganizations(ctx, database.GetOrganizationsParams{})
	if err != nil {
		return xerrors.Errorf("list organizations: %w", err)
	}
	for _, organization := range organizations {
		policies, err := api.Database.GetNetworkPoliciesByOrganizationID(ctx, organization.ID)
		if err != nil {
			api.Logger.Error(ctx, "list policies for drift check", slog.F("organization_id", organization.ID), slog.Error(err))
			continue
		}
		for _, p := range policies {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			targets, err := api.Database.GetNetworkPolicyTargetsByPolicyID(ctx, p.ID)
			if err != nil {
				api.Logger.Error(ctx, "list targets for drift check", slog.F("policy_id", p.ID), slog.Error(err))
				continue
			}
			for _, target := range targets {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !networkPolicyDriftEligible(p, target) {
					continue
				}
				jobs, err := queueNetworkPolicyDriftTarget(ctx, api.Database, p.ID, target.ID)
				if err != nil {
					api.Logger.Error(ctx, "queue network policy drift check", slog.F("policy_id", p.ID), slog.F("target_id", target.ID), slog.Error(err))
					continue
				}
				api.notifyNetworkPolicyJobs(ctx, jobs)
			}
		}
	}
	return nil
}

// networkPolicyDriftEligible deliberately excludes failed and canceled targets,
// even if their revisions match. Drift detection must not act as a retry loop.
func networkPolicyDriftEligible(p database.NetworkPolicy, target database.NetworkPolicyTarget) bool {
	return !p.DeletionRequestedAt.Valid && !p.DeletedAt.Valid && !target.DestroyedAt.Valid &&
		p.CreatedBy != uuid.Nil && p.PublishedRevisionID.Valid &&
		target.PolicyID == p.ID && target.OrganizationID == p.OrganizationID &&
		!target.ActiveJobID.Valid && target.Status == "succeeded" && target.LastError == "" &&
		len(target.State) > 0 && target.AppliedRevisionID.Valid && target.DesiredRevisionID.Valid &&
		target.AppliedRevisionID.UUID == p.PublishedRevisionID.UUID &&
		target.DesiredRevisionID.UUID == target.AppliedRevisionID.UUID
}

func queueNetworkPolicyDriftTarget(ctx context.Context, db database.Store, policyID, targetID uuid.UUID) ([]database.ProvisionerJob, error) {
	var jobs []database.ProvisionerJob
	err := db.InTx(func(tx database.Store) error {
		// Publication and attachment updates use this same lock. Never queue
		// from the unlocked discovery snapshot above.
		p, err := tx.GetNetworkPolicyByIDForUpdate(ctx, policyID)
		if err != nil {
			return err
		}
		targets, err := lockNetworkPolicyTargets(ctx, tx, p, []uuid.UUID{targetID})
		if err != nil {
			return err
		}
		if !networkPolicyDriftEligible(p, targets[0]) {
			return nil
		}
		revision, err := tx.GetNetworkPolicyRevisionByID(ctx, p.PublishedRevisionID.UUID)
		if err != nil {
			return err
		}
		if revision.PolicyID != p.ID || revision.OrganizationID != p.OrganizationID {
			return xerrors.New("published network policy revision does not belong to policy")
		}
		jobs, _, err = queueNetworkPolicyJobs(ctx, tx, p, revision, targets, p.CreatedBy, codersdk.NetworkPolicyOperationPlan)
		return err
	}, nil)
	var conflict *networkPolicyHTTPError
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &conflict) && conflict.status == http.StatusConflict) {
		// Another instance or an API request reserved the target first.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return jobs, nil
}
