package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (q *querier) InsertFoundation(ctx context.Context, arg database.InsertFoundationParams) (database.Foundation, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceFoundation.InOrg(arg.OrganizationID)); err != nil {
		return database.Foundation{}, err
	}
	return q.db.InsertFoundation(ctx, arg)
}

func (q *querier) GetFoundationByID(ctx context.Context, id uuid.UUID) (database.Foundation, error) {
	f, err := q.db.GetFoundationByID(ctx, id)
	if err != nil {
		return f, err
	}
	if err := q.authorizeTemplateResources(ctx, f.ID, policy.ActionRead); err != nil {
		return database.Foundation{}, err
	}
	return f, nil
}

func (q *querier) GetFoundationByTemplateID(ctx context.Context, id uuid.NullUUID) (database.Foundation, error) {
	f, err := q.db.GetFoundationByTemplateID(ctx, id)
	if err != nil {
		return f, err
	}
	if err := q.authorizeTemplateResources(ctx, f.ID, policy.ActionRead); err != nil {
		return database.Foundation{}, err
	}
	return f, nil
}

func (q *querier) GetFoundationsByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]database.Foundation, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetFoundationsByOrganizationID)(ctx, orgID)
}

func (q *querier) UpdateFoundation(ctx context.Context, arg database.UpdateFoundationParams) (database.Foundation, error) {
	if err := q.authorizeTemplateResources(ctx, arg.ID, policy.ActionUpdate); err != nil {
		return database.Foundation{}, err
	}
	return q.db.UpdateFoundation(ctx, arg)
}

func (q *querier) BindFoundation(ctx context.Context, arg database.BindFoundationParams) (database.Foundation, error) {
	if err := q.authorizeTemplateResources(ctx, arg.ID, policy.ActionUse); err != nil {
		return database.Foundation{}, err
	}
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID.UUID)
	if err != nil {
		return database.Foundation{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return database.Foundation{}, err
	}
	return q.db.BindFoundation(ctx, arg)
}

func (q *querier) DeleteFoundation(ctx context.Context, arg database.DeleteFoundationParams) (database.Foundation, error) {
	if err := q.authorizeTemplateResources(ctx, arg.ID, policy.ActionDelete); err != nil {
		return database.Foundation{}, err
	}
	return q.db.DeleteFoundation(ctx, arg)
}

func (q *querier) LockFoundation(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeTemplateResources(ctx, id, policy.ActionRead); err != nil {
		return err
	}
	return q.db.LockFoundation(ctx, id)
}

func (q *querier) GetFoundationConsumerCounts(ctx context.Context, id uuid.UUID) (database.GetFoundationConsumerCountsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetFoundationConsumerCountsRow{}, err
	}
	return q.db.GetFoundationConsumerCounts(ctx, id)
}

func (q *querier) GetFoundationWorkspaceCount(ctx context.Context, id uuid.UUID) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetFoundationWorkspaceCount(ctx, id)
}

func (q *querier) GetFileFoundations(ctx context.Context, id uuid.UUID) ([]database.Foundation, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetFileFoundations(ctx, id)
}
