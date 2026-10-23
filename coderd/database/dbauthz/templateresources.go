package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// Template resource source, logs and state require the existing template update ACL.
func (q *querier) authorizeTemplateResources(ctx context.Context, templateID uuid.UUID) error {
	if actor, ok := ActorFromContext(ctx); ok && (actor.Type == rbac.SubjectTypeSystemRestricted || actor.Type == rbac.SubjectTypeProvisionerd) {
		return q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem)
	}
	template, err := q.db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return err
	}
	return q.authorizeContext(ctx, policy.ActionUpdate, template)
}

func (q *querier) LockTemplateResourceTemplate(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeTemplateResources(ctx, id); err != nil {
		return err
	}
	return q.db.LockTemplateResourceTemplate(ctx, id)
}

func (q *querier) GetTemplateResourceState(ctx context.Context, id uuid.UUID) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, id); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.GetTemplateResourceState(ctx, id)
}

func (q *querier) GetTemplateResourceStateForUpdate(ctx context.Context, id uuid.UUID) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, id); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.GetTemplateResourceStateForUpdate(ctx, id)
}

func (q *querier) InsertTemplateResourceState(ctx context.Context, arg database.InsertTemplateResourceStateParams) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, arg.TemplateID); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.InsertTemplateResourceState(ctx, arg)
}

func (q *querier) InsertTemplateResourceRevision(ctx context.Context, arg database.InsertTemplateResourceRevisionParams) (database.TemplateResourceRevision, error) {
	if err := q.authorizeTemplateResources(ctx, arg.TemplateID); err != nil {
		return database.TemplateResourceRevision{}, err
	}
	return q.db.InsertTemplateResourceRevision(ctx, arg)
}

func (q *querier) GetTemplateResourceRevisionByID(ctx context.Context, id uuid.UUID) (database.TemplateResourceRevision, error) {
	row, err := q.db.GetTemplateResourceRevisionByID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeTemplateResources(ctx, row.TemplateID); err != nil {
		return database.TemplateResourceRevision{}, err
	}
	return row, nil
}

func (q *querier) GetTemplateResourceRevisionsByTemplateID(ctx context.Context, id uuid.UUID) ([]database.TemplateResourceRevision, error) {
	if err := q.authorizeTemplateResources(ctx, id); err != nil {
		return nil, err
	}
	return q.db.GetTemplateResourceRevisionsByTemplateID(ctx, id)
}

func (q *querier) SetTemplateResourceCurrentRevision(ctx context.Context, arg database.SetTemplateResourceCurrentRevisionParams) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, arg.TemplateID); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.SetTemplateResourceCurrentRevision(ctx, arg)
}

func (q *querier) InsertTemplateResourceRun(ctx context.Context, arg database.InsertTemplateResourceRunParams) (database.TemplateResourceRun, error) {
	if err := q.authorizeTemplateResources(ctx, arg.TemplateID); err != nil {
		return database.TemplateResourceRun{}, err
	}
	return q.db.InsertTemplateResourceRun(ctx, arg)
}

func (q *querier) GetTemplateResourceRunByID(ctx context.Context, id uuid.UUID) (database.TemplateResourceRun, error) {
	row, err := q.db.GetTemplateResourceRunByID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeTemplateResources(ctx, row.TemplateID); err != nil {
		return database.TemplateResourceRun{}, err
	}
	return row, nil
}

func (q *querier) GetTemplateResourceRunByJobID(ctx context.Context, id uuid.UUID) (database.TemplateResourceRun, error) {
	row, err := q.db.GetTemplateResourceRunByJobID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeTemplateResources(ctx, row.TemplateID); err != nil {
		return database.TemplateResourceRun{}, err
	}
	return row, nil
}

func (q *querier) GetTemplateResourceRunsByTemplateID(ctx context.Context, id uuid.UUID) ([]database.TemplateResourceRun, error) {
	if err := q.authorizeTemplateResources(ctx, id); err != nil {
		return nil, err
	}
	return q.db.GetTemplateResourceRunsByTemplateID(ctx, id)
}

func (q *querier) StartTemplateResourceRun(ctx context.Context, arg database.StartTemplateResourceRunParams) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, arg.TemplateID); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.StartTemplateResourceRun(ctx, arg)
}

func (q *querier) UpdateTemplateResourceState(ctx context.Context, arg database.UpdateTemplateResourceStateParams) (database.TemplateResourceState, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.UpdateTemplateResourceState(ctx, arg)
}

func (q *querier) CompleteTemplateResourceRun(ctx context.Context, arg database.CompleteTemplateResourceRunParams) (database.TemplateResourceState, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.CompleteTemplateResourceRun(ctx, arg)
}

func (q *querier) PinWorkspaceBuildTemplateResources(ctx context.Context, arg database.PinWorkspaceBuildTemplateResourcesParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.PinWorkspaceBuildTemplateResources(ctx, arg)
}
