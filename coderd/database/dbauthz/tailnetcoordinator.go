package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *querier) UpsertTailnetClient(ctx context.Context, arg database.UpsertTailnetClientParams) (database.TailnetClient, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetClient{}, err
	}
	return q.db.UpsertTailnetClient(ctx, arg)
}

func (q *querier) UpsertTailnetAgent(ctx context.Context, arg database.UpsertTailnetAgentParams) (database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetAgent{}, err
	}
	return q.db.UpsertTailnetAgent(ctx, arg)
}

func (q *querier) UpsertTailnetCoordinator(ctx context.Context, id uuid.UUID) (database.TailnetCoordinator, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetCoordinator{}, err
	}
	return q.db.UpsertTailnetCoordinator(ctx, id)
}

func (q *querier) DeleteTailnetClient(ctx context.Context, arg database.DeleteTailnetClientParams) (database.DeleteTailnetClientRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetClientRow{}, err
	}
	return q.db.DeleteTailnetClient(ctx, arg)
}

func (q *querier) DeleteTailnetAgent(ctx context.Context, arg database.DeleteTailnetAgentParams) (database.DeleteTailnetAgentRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetAgentRow{}, err
	}
	return q.db.DeleteTailnetAgent(ctx, arg)
}

func (q *querier) DeleteCoordinator(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.DeleteCoordinator(ctx, id)
}

func (q *querier) GetTailnetAgents(ctx context.Context, id uuid.UUID) ([]database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetAgents(ctx, id)
}

func (q *querier) GetTailnetClientsForAgent(ctx context.Context, agentID uuid.UUID) ([]database.TailnetClient, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetClientsForAgent(ctx, agentID)
}
