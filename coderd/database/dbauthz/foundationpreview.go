package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (q *querier) UpdateTemplateResourceRunPlanPreview(ctx context.Context, arg database.UpdateTemplateResourceRunPlanPreviewParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpdateTemplateResourceRunPlanPreview(ctx, arg)
}

func (q *querier) GetFoundationConsumerTemplateVersions(ctx context.Context, id uuid.UUID) ([]database.GetFoundationConsumerTemplateVersionsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetFoundationConsumerTemplateVersions(ctx, id)
}
