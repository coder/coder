package coderd

import (
	"net/http"
	"slices"
	"strings"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Delete network policy infrastructure
// @ID delete-network-policy
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param request body codersdk.DeleteNetworkPolicyRequest true "Deletion"
// @Success 202 {object} codersdk.NetworkPolicy
// @Success 200 {object} codersdk.NetworkPolicy
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteNetworkPolicy(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.DeleteNetworkPolicyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	var result codersdk.NetworkPolicy
	var jobs []database.ProvisionerJob
	err := api.Database.InTx(func(tx database.Store) error {
		// The lock authorizes current attachments and all historical snapshots.
		p, err := networkPolicyByRequest(ctx, r, tx.GetNetworkPolicyByIDForUpdate)
		if err != nil {
			return err
		}
		if p.Generation != req.ExpectedGeneration {
			return networkPolicyConflict("Network policy has changed. Reload before retrying.")
		}
		if p.DeletedAt.Valid {
			result, err = networkPolicyDetail(ctx, tx, p)
			return err
		}
		//nolint:gocritic // The policy lock above authorizes the entire retained history.
		systemCtx := dbauthz.AsSystemRestricted(ctx)
		targets, err := tx.GetNetworkPolicyTargetsByPolicyID(systemCtx, p.ID)
		if err != nil {
			return err
		}
		slices.SortFunc(targets, func(a, b database.NetworkPolicyTarget) int { return strings.Compare(a.ID.String(), b.ID.String()) })
		for _, candidate := range targets {
			target, err := tx.GetNetworkPolicyTargetByIDForUpdate(systemCtx, candidate.ID)
			if err != nil {
				return err
			}
			if target.DestroyedAt.Valid {
				continue
			}
			if target.ActiveJobID.Valid {
				// Never acquire an existing job lock while holding the policy.
				// Finalizers serialize job, policy, target in that order.
				run, err := tx.GetNetworkPolicyRunByJobID(systemCtx, target.ActiveJobID.UUID)
				if err != nil {
					return err
				}
				job, err := tx.GetProvisionerJobByID(systemCtx, target.ActiveJobID.UUID)
				if err != nil {
					return err
				}
				if p.DeletionRequestedAt.Valid && run.Operation == "destroy" && !job.CompletedAt.Valid {
					continue
				}
				return networkPolicyConflict("A target has an active or uncertain operation. Reconcile it before deleting.")
			}
			source, err := networkpolicy.DestroyProvenance(systemCtx, tx, target)
			if err != nil {
				return networkPolicyConflict("Target state provenance is not established. Reconciliation is required before deleting.")
			}
			if source == nil {
				_, err = tx.DestroyUnappliedNetworkPolicyTarget(systemCtx, database.DestroyUnappliedNetworkPolicyTargetParams{ID: target.ID, Generation: target.Generation, UpdatedAt: dbtime.Now()})
				if err != nil {
					return err
				}
				continue
			}
			revision, err := tx.GetNetworkPolicyRevisionByID(systemCtx, source.RevisionID)
			if err != nil {
				return err
			}
			queued, _, err := queueNetworkPolicyJobs(systemCtx, tx, p, revision, []database.NetworkPolicyTarget{target}, httpmw.APIKey(r).UserID, codersdk.NetworkPolicyOperationDestroy)
			if err != nil {
				return err
			}
			jobs = append(jobs, queued...)
		}
		if !p.DeletionRequestedAt.Valid || len(jobs) > 0 {
			p, err = tx.RequestNetworkPolicyDeletion(ctx, database.RequestNetworkPolicyDeletionParams{ID: p.ID, UpdatedAt: dbtime.Now()})
			if err != nil {
				return err
			}
		}
		_, err = tx.CompleteNetworkPolicyDeletion(systemCtx, database.CompleteNetworkPolicyDeletionParams{ID: p.ID, UpdatedAt: dbtime.Now()})
		if err != nil {
			return err
		}
		p, err = tx.GetNetworkPolicyByID(systemCtx, p.ID)
		if err != nil {
			return err
		}
		result, err = networkPolicyDetail(systemCtx, tx, p)
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	api.notifyNetworkPolicyJobs(ctx, jobs)
	status := http.StatusAccepted
	if result.DeletedAt != nil {
		status = http.StatusOK
	}
	httpapi.Write(ctx, rw, status, result)
}
