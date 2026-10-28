package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// Foundation source, outputs, and history have an independent organization permission.
func (q *querier) authorizeTemplateResources(ctx context.Context, foundationID uuid.UUID, action policy.Action) error {
	if actor, ok := ActorFromContext(ctx); ok && (actor.Type == rbac.SubjectTypeSystemRestricted || actor.Type == rbac.SubjectTypeProvisionerd || actor.Type == rbac.SubjectTypeJobReaper) {
		return q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem)
	}
	foundation, err := q.db.GetFoundationByID(ctx, foundationID)
	if err != nil {
		return err
	}
	return q.authorizeContext(ctx, action, foundation)
}

func (q *querier) LockTemplateResourceTemplate(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.LockTemplateResourceTemplate(ctx, id)
}

func (q *querier) GetTemplateResourceState(ctx context.Context, id uuid.UUID) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, id, policy.ActionRead); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.GetTemplateResourceState(ctx, id)
}

func (q *querier) GetTemplateResourceStateForUpdate(ctx context.Context, id uuid.UUID) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, id, policy.ActionRead); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.GetTemplateResourceStateForUpdate(ctx, id)
}

func (q *querier) InsertTemplateResourceState(ctx context.Context, arg database.InsertTemplateResourceStateParams) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, arg.FoundationID, policy.ActionUpdate); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.InsertTemplateResourceState(ctx, arg)
}

func (q *querier) InsertTemplateResourceRevision(ctx context.Context, arg database.InsertTemplateResourceRevisionParams) (database.TemplateResourceRevision, error) {
	if err := q.authorizeTemplateResources(ctx, arg.FoundationID, policy.ActionUpdate); err != nil {
		return database.TemplateResourceRevision{}, err
	}
	return q.db.InsertTemplateResourceRevision(ctx, arg)
}

func (q *querier) GetTemplateResourceRevisionByID(ctx context.Context, id uuid.UUID) (database.TemplateResourceRevision, error) {
	row, err := q.db.GetTemplateResourceRevisionByID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeTemplateResources(ctx, row.FoundationID, policy.ActionRead); err != nil {
		return database.TemplateResourceRevision{}, err
	}
	return row, nil
}

func (q *querier) GetTemplateResourceRevisionsByFoundationID(ctx context.Context, id uuid.UUID) ([]database.TemplateResourceRevision, error) {
	if err := q.authorizeTemplateResources(ctx, id, policy.ActionRead); err != nil {
		return nil, err
	}
	return q.db.GetTemplateResourceRevisionsByFoundationID(ctx, id)
}

func (q *querier) SetTemplateResourceCurrentRevision(ctx context.Context, arg database.SetTemplateResourceCurrentRevisionParams) (database.TemplateResourceState, error) {
	if err := q.authorizeTemplateResources(ctx, arg.FoundationID, policy.ActionUpdate); err != nil {
		return database.TemplateResourceState{}, err
	}
	return q.db.SetTemplateResourceCurrentRevision(ctx, arg)
}

func (q *querier) InsertTemplateResourceRun(ctx context.Context, arg database.InsertTemplateResourceRunParams) (database.TemplateResourceRun, error) {
	action := policy.ActionUse
	if arg.Action == "plan" {
		action = policy.ActionUpdate
	}
	if err := q.authorizeTemplateResources(ctx, arg.FoundationID, action); err != nil {
		return database.TemplateResourceRun{}, err
	}
	return q.db.InsertTemplateResourceRun(ctx, arg)
}

func (q *querier) GetTemplateResourceRunByID(ctx context.Context, id uuid.UUID) (database.TemplateResourceRun, error) {
	row, err := q.db.GetTemplateResourceRunByID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeTemplateResources(ctx, row.FoundationID, policy.ActionRead); err != nil {
		return database.TemplateResourceRun{}, err
	}
	return row, nil
}

func (q *querier) GetTemplateResourceRunByJobID(ctx context.Context, id uuid.UUID) (database.TemplateResourceRun, error) {
	row, err := q.db.GetTemplateResourceRunByJobID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeTemplateResources(ctx, row.FoundationID, policy.ActionRead); err != nil {
		return database.TemplateResourceRun{}, err
	}
	return row, nil
}

func (q *querier) GetTemplateResourceRunsByFoundationID(ctx context.Context, id uuid.UUID) ([]database.TemplateResourceRun, error) {
	if err := q.authorizeTemplateResources(ctx, id, policy.ActionRead); err != nil {
		return nil, err
	}
	return q.db.GetTemplateResourceRunsByFoundationID(ctx, id)
}

func (q *querier) StartTemplateResourceRun(ctx context.Context, arg database.StartTemplateResourceRunParams) (database.TemplateResourceState, error) {
	run, err := q.db.GetTemplateResourceRunByID(ctx, arg.RunID.UUID)
	if err != nil {
		return database.TemplateResourceState{}, err
	}
	action := policy.ActionUse
	if run.Action == "plan" {
		action = policy.ActionUpdate
	}
	if err := q.authorizeTemplateResources(ctx, arg.FoundationID, action); err != nil {
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
