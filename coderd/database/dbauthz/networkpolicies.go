package dbauthz

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// networkPolicyJobVisible adds composite authorization without changing the
// existing authorization rules for other provisioner job types. Orphaned jobs
// are hidden from users, but internal workers can still recover or reap them.
func (q *querier) networkPolicyJobVisible(ctx context.Context, job database.ProvisionerJob) (bool, error) {
	if job.Type != database.ProvisionerJobTypeNetworkPolicy {
		return true, nil
	}
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return false, ErrNoActor
	}
	if actor.Type == rbac.SubjectTypeSystemRestricted || actor.Type == rbac.SubjectTypeProvisionerd {
		err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem)
		return err == nil, err
	}
	if err := q.authorizeProvisionerJob(ctx, job); err != nil {
		if IsNotAuthorizedError(err) || errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func filterNetworkPolicyJobRows[T any](ctx context.Context, q *querier, rows []T, job func(T) database.ProvisionerJob) ([]T, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	visible := make([]T, 0, len(rows))
	for _, row := range rows {
		ok, err := q.networkPolicyJobVisible(ctx, job(row))
		if err != nil {
			return nil, err
		}
		if ok {
			visible = append(visible, row)
		}
	}
	return visible, nil
}

// hiddenNetworkPolicyJobs is used by daemon queries that expose job IDs but
// do not embed the job type needed to select the authorization rule.
func (q *querier) hiddenNetworkPolicyJobs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	hidden := make(map[uuid.UUID]bool, len(ids))
	if len(ids) == 0 {
		return hidden, nil
	}
	jobs, err := q.db.GetProvisionerJobsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		hidden[id] = true
	}
	for _, job := range jobs {
		visible, err := q.networkPolicyJobVisible(ctx, job)
		if err != nil {
			return nil, err
		}
		if visible {
			delete(hidden, job.ID)
		}
	}
	return hidden, nil
}

// authorizeNetworkPolicy includes historical run snapshots so detaching a
// template cannot expose its state, logs, or frozen configuration to new users.
// Callers serialize mutations by locking the policy before changing attachments.
func (q *querier) authorizeNetworkPolicy(ctx context.Context, p database.NetworkPolicy) error {
	// Internal reconcilers must read policies even when there are no templates.
	if actor, ok := ActorFromContext(ctx); ok && (actor.Type == rbac.SubjectTypeSystemRestricted || actor.Type == rbac.SubjectTypeProvisionerd) {
		return q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem)
	}
	attachments, err := q.db.GetNetworkPolicyAttachmentsByPolicyID(ctx, p.ID)
	if err != nil {
		return xerrors.Errorf("get network policy attachments: %w", err)
	}
	runs, err := q.db.GetNetworkPolicyRunsByPolicyID(ctx, p.ID)
	if err != nil {
		return xerrors.Errorf("get network policy run snapshots: %w", err)
	}
	ids := make(map[uuid.UUID]struct{})
	for _, attachment := range attachments {
		ids[attachment.TemplateID] = struct{}{}
	}
	for _, run := range runs {
		for _, id := range run.TemplateIDs {
			ids[id] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceTemplate.InOrg(p.OrganizationID))
	}
	for id := range ids {
		template, err := q.db.GetTemplateByID(ctx, id)
		if err != nil {
			return xerrors.Errorf("get network policy template: %w", err)
		}
		if template.OrganizationID != p.OrganizationID {
			return NotAuthorizedError{Err: xerrors.New("network policy template organization mismatch")}
		}
		if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
			return err
		}
	}
	return nil
}

func (q *querier) GetNetworkPolicyByID(ctx context.Context, id uuid.UUID) (database.NetworkPolicy, error) {
	p, err := q.db.GetNetworkPolicyByID(ctx, id)
	if err != nil {
		return database.NetworkPolicy{}, err
	}
	if err := q.authorizeNetworkPolicy(ctx, p); err != nil {
		return database.NetworkPolicy{}, err
	}
	return p, nil
}

func (q *querier) GetNetworkPolicyByIDForUpdate(ctx context.Context, id uuid.UUID) (database.NetworkPolicy, error) {
	p, err := q.db.GetNetworkPolicyByIDForUpdate(ctx, id)
	if err != nil {
		return database.NetworkPolicy{}, err
	}
	if err := q.authorizeNetworkPolicy(ctx, p); err != nil {
		return database.NetworkPolicy{}, err
	}
	return p, nil
}

func (q *querier) GetNetworkPoliciesByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]database.NetworkPolicy, error) {
	if _, ok := ActorFromContext(ctx); !ok {
		return nil, ErrNoActor
	}
	rows, err := q.db.GetNetworkPoliciesByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	visible := make([]database.NetworkPolicy, 0, len(rows))
	for _, row := range rows {
		if err := q.authorizeNetworkPolicy(ctx, row); err != nil {
			if IsNotAuthorizedError(err) {
				continue
			}
			return nil, err
		}
		visible = append(visible, row)
	}
	return visible, nil
}

func (q *querier) InsertNetworkPolicy(ctx context.Context, arg database.InsertNetworkPolicyParams) (database.NetworkPolicy, error) {
	// The API may preauthorize a nonempty proposed template set before creating
	// the policy and its attachments together in a transaction.
	if actor, ok := ActorFromContext(ctx); ok && actor.Type == rbac.SubjectTypeSystemRestricted {
		if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
			return database.NetworkPolicy{}, err
		}
		return q.db.InsertNetworkPolicy(ctx, arg)
	}
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceTemplate.InOrg(arg.OrganizationID)); err != nil {
		return database.NetworkPolicy{}, err
	}
	return q.db.InsertNetworkPolicy(ctx, arg)
}

func (q *querier) UpdateNetworkPolicy(ctx context.Context, arg database.UpdateNetworkPolicyParams) (database.NetworkPolicy, error) {
	if _, err := q.GetNetworkPolicyByIDForUpdate(ctx, arg.ID); err != nil {
		return database.NetworkPolicy{}, err
	}
	return q.db.UpdateNetworkPolicy(ctx, arg)
}

func (q *querier) PublishNetworkPolicyRevision(ctx context.Context, arg database.PublishNetworkPolicyRevisionParams) (database.NetworkPolicy, error) {
	if _, err := q.GetNetworkPolicyByIDForUpdate(ctx, arg.ID); err != nil {
		return database.NetworkPolicy{}, err
	}
	return q.db.PublishNetworkPolicyRevision(ctx, arg)
}

func (q *querier) InsertNetworkPolicyRevision(ctx context.Context, arg database.InsertNetworkPolicyRevisionParams) (database.NetworkPolicyRevision, error) {
	if _, err := q.GetNetworkPolicyByIDForUpdate(ctx, arg.PolicyID); err != nil {
		return database.NetworkPolicyRevision{}, err
	}
	return q.db.InsertNetworkPolicyRevision(ctx, arg)
}

func (q *querier) GetNetworkPolicyRevisionByID(ctx context.Context, id uuid.UUID) (database.NetworkPolicyRevision, error) {
	row, err := q.db.GetNetworkPolicyRevisionByID(ctx, id)
	if err != nil {
		return database.NetworkPolicyRevision{}, err
	}
	if _, err := q.GetNetworkPolicyByID(ctx, row.PolicyID); err != nil {
		return database.NetworkPolicyRevision{}, err
	}
	return row, nil
}

func (q *querier) GetNetworkPolicyRevisionsByPolicyID(ctx context.Context, policyID uuid.UUID) ([]database.NetworkPolicyRevision, error) {
	if _, err := q.GetNetworkPolicyByID(ctx, policyID); err != nil {
		return nil, err
	}
	return q.db.GetNetworkPolicyRevisionsByPolicyID(ctx, policyID)
}

func (q *querier) InsertNetworkPolicyAttachment(ctx context.Context, arg database.InsertNetworkPolicyAttachmentParams) (database.NetworkPolicyAttachment, error) {
	if actor, ok := ActorFromContext(ctx); ok && actor.Type == rbac.SubjectTypeSystemRestricted {
		if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
			return database.NetworkPolicyAttachment{}, err
		}
		return q.db.InsertNetworkPolicyAttachment(ctx, arg)
	}
	if _, err := q.GetNetworkPolicyByIDForUpdate(ctx, arg.PolicyID); err != nil {
		return database.NetworkPolicyAttachment{}, err
	}
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return database.NetworkPolicyAttachment{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return database.NetworkPolicyAttachment{}, err
	}
	return q.db.InsertNetworkPolicyAttachment(ctx, arg)
}

func (q *querier) DeleteNetworkPolicyAttachment(ctx context.Context, arg database.DeleteNetworkPolicyAttachmentParams) (int64, error) {
	if _, err := q.GetNetworkPolicyByIDForUpdate(ctx, arg.PolicyID); err != nil {
		return 0, err
	}
	return q.db.DeleteNetworkPolicyAttachment(ctx, arg)
}

func (q *querier) GetNetworkPolicyAttachmentsByPolicyID(ctx context.Context, policyID uuid.UUID) ([]database.NetworkPolicyAttachment, error) {
	if _, err := q.GetNetworkPolicyByID(ctx, policyID); err != nil {
		return nil, err
	}
	return q.db.GetNetworkPolicyAttachmentsByPolicyID(ctx, policyID)
}

func (q *querier) InsertNetworkPolicyTarget(ctx context.Context, arg database.InsertNetworkPolicyTargetParams) (database.NetworkPolicyTarget, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return q.db.InsertNetworkPolicyTarget(ctx, arg)
}

func (q *querier) GetNetworkPolicyTargetByID(ctx context.Context, id uuid.UUID) (database.NetworkPolicyTarget, error) {
	row, err := q.db.GetNetworkPolicyTargetByID(ctx, id)
	if err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	if _, err := q.GetNetworkPolicyByID(ctx, row.PolicyID); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return row, nil
}

func (q *querier) GetNetworkPolicyTargetByIDForUpdate(ctx context.Context, id uuid.UUID) (database.NetworkPolicyTarget, error) {
	row, err := q.db.GetNetworkPolicyTargetByIDForUpdate(ctx, id)
	if err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	if _, err := q.GetNetworkPolicyByID(ctx, row.PolicyID); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return row, nil
}

func (q *querier) GetNetworkPolicyTargetsByPolicyID(ctx context.Context, policyID uuid.UUID) ([]database.NetworkPolicyTarget, error) {
	if _, err := q.GetNetworkPolicyByID(ctx, policyID); err != nil {
		return nil, err
	}
	return q.db.GetNetworkPolicyTargetsByPolicyID(ctx, policyID)
}

func (q *querier) StartNetworkPolicyTargetJob(ctx context.Context, arg database.StartNetworkPolicyTargetJobParams) (database.NetworkPolicyTarget, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return q.db.StartNetworkPolicyTargetJob(ctx, arg)
}

func (q *querier) UpdateNetworkPolicyTargetState(ctx context.Context, arg database.UpdateNetworkPolicyTargetStateParams) (database.NetworkPolicyTarget, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return q.db.UpdateNetworkPolicyTargetState(ctx, arg)
}

func (q *querier) CompleteNetworkPolicyTargetJob(ctx context.Context, arg database.CompleteNetworkPolicyTargetJobParams) (database.NetworkPolicyTarget, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return q.db.CompleteNetworkPolicyTargetJob(ctx, arg)
}

func (q *querier) InsertNetworkPolicyRun(ctx context.Context, arg database.InsertNetworkPolicyRunParams) (database.NetworkPolicyRun, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.NetworkPolicyRun{}, err
	}
	return q.db.InsertNetworkPolicyRun(ctx, arg)
}

func (q *querier) GetNetworkPolicyRunByID(ctx context.Context, id uuid.UUID) (database.NetworkPolicyRun, error) {
	row, err := q.db.GetNetworkPolicyRunByID(ctx, id)
	if err != nil {
		return database.NetworkPolicyRun{}, err
	}
	if _, err := q.GetNetworkPolicyByID(ctx, row.PolicyID); err != nil {
		return database.NetworkPolicyRun{}, err
	}
	return row, nil
}

func (q *querier) GetNetworkPolicyRunByJobID(ctx context.Context, jobID uuid.UUID) (database.NetworkPolicyRun, error) {
	row, err := q.db.GetNetworkPolicyRunByJobID(ctx, jobID)
	if err != nil {
		return database.NetworkPolicyRun{}, err
	}
	if _, err := q.GetNetworkPolicyByID(ctx, row.PolicyID); err != nil {
		return database.NetworkPolicyRun{}, err
	}
	return row, nil
}

func (q *querier) RequestNetworkPolicyDeletion(ctx context.Context, arg database.RequestNetworkPolicyDeletionParams) (database.NetworkPolicy, error) {
	if _, err := q.GetNetworkPolicyByIDForUpdate(ctx, arg.ID); err != nil {
		return database.NetworkPolicy{}, err
	}
	return q.db.RequestNetworkPolicyDeletion(ctx, arg)
}

func (q *querier) CompleteNetworkPolicyDeletion(ctx context.Context, arg database.CompleteNetworkPolicyDeletionParams) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.CompleteNetworkPolicyDeletion(ctx, arg)
}

func (q *querier) DestroyUnappliedNetworkPolicyTarget(ctx context.Context, arg database.DestroyUnappliedNetworkPolicyTargetParams) (database.NetworkPolicyTarget, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.NetworkPolicyTarget{}, err
	}
	return q.db.DestroyUnappliedNetworkPolicyTarget(ctx, arg)
}

func (q *querier) GetNetworkPolicyRunsByPolicyID(ctx context.Context, policyID uuid.UUID) ([]database.NetworkPolicyRun, error) {
	if _, err := q.GetNetworkPolicyByID(ctx, policyID); err != nil {
		return nil, err
	}
	return q.db.GetNetworkPolicyRunsByPolicyID(ctx, policyID)
}
