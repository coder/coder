package authzquery

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) GetPreviousTemplateVersion(ctx context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	return authorizedFetch(q.authorizer, q.database.GetTemplateByID)(ctx, id)
}

func (q *AuthzQuerier) GetTemplateByOrganizationAndName(ctx context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	return authorizedFetch(q.authorizer, q.database.GetTemplateByOrganizationAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateDAUs(ctx context.Context, templateID uuid.UUID) ([]database.GetTemplateDAUsRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionByID(ctx context.Context, id uuid.UUID) (database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionByJobID(ctx context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionByOrganizationAndName(ctx context.Context, arg database.GetTemplateVersionByOrganizationAndNameParams) (database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionByTemplateIDAndName(ctx context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionParameters(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionsByTemplateID(ctx context.Context, arg database.GetTemplateVersionsByTemplateIDParams) ([]database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateVersionsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAuthorizedTemplates(ctx context.Context, arg database.GetTemplatesWithFilterParams, _ rbac.PreparedAuthorized) ([]database.Template, error) {
	//TODO Delete this function, all GetTemplates should be authorized. For now just call getTemplates on the authz querier.
	return q.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{})
}

func (q *AuthzQuerier) GetTemplates(ctx context.Context) ([]database.Template, error) {
	// TODO: We should remove this and only expose the GetTemplatesWithFilter
	// This might be required as a system function.
	return q.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{})
}

func (q *AuthzQuerier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	prep, err := prepareSQLFilter(ctx, q.authorizer, rbac.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.database.GetAuthorizedTemplates(ctx, arg, prep)
}

func (q *AuthzQuerier) InsertTemplate(ctx context.Context, arg database.InsertTemplateParams) (database.Template, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertTemplateVersion(ctx context.Context, arg database.InsertTemplateVersionParams) (database.TemplateVersion, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertTemplateVersionParameter(ctx context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateTemplateACLByID(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
	// TODO: Allow preloading template in ctx cache
	tpl, err := q.database.GetTemplateByID(ctx, arg.ID)
	if err != nil {
		return database.Template{}, err
	}

	// UpdateTemplateACL uses the ActionCreate action. Only users that can create the template
	// may update the ACL.
	err = q.authorizeContext(ctx, rbac.ActionCreate, tpl.RBACObject())
	if err != nil {
		return database.Template{}, err
	}

	return q.database.UpdateTemplateACLByID(ctx, arg)
}

func (q *AuthzQuerier) UpdateTemplateActiveVersionByID(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	return authorizedUpdate(q.authorizer, func(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) (database.Template, error) {
		return q.database.GetTemplateByID(ctx, arg.ID)
	}, q.database.UpdateTemplateActiveVersionByID)(ctx, arg)
}

func (q *AuthzQuerier) SoftDeleteTemplateByID(ctx context.Context, id uuid.UUID) error {
	return authorizedDelete(q.authorizer, q.database.GetTemplateByID, func(ctx context.Context, id uuid.UUID) error {
		return q.database.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:        id,
			Deleted:   true,
			UpdatedAt: database.Now(),
		})
	})(ctx, id)
}

// Deprecated: use SoftDeleteTemplateByID instead.
func (q *AuthzQuerier) UpdateTemplateDeletedByID(ctx context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
	//TODO delete me. This function is a placeholder for database.Store.
	panic("implement me")
}

func (q *AuthzQuerier) UpdateTemplateMetaByID(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateTemplateVersionByID(ctx context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateTemplateVersionDescriptionByJobID(ctx context.Context, arg database.UpdateTemplateVersionDescriptionByJobIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	// Authorized fetch on the template first.
	// TODO: @emyrk this implementation feels like it could be better?
	_, err := authorizedFetch(q.authorizer, q.database.GetTemplateByID)(ctx, id)
	if err != nil {
		return nil, err
	}
	return q.database.GetTemplateGroupRoles(ctx, id)
}

func (q *AuthzQuerier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	// Authorized fetch on the template first.
	// TODO: @emyrk this implementation feels like it could be better?
	_, err := authorizedFetch(q.authorizer, q.database.GetTemplateByID)(ctx, id)
	if err != nil {
		return nil, err
	}
	return q.database.GetTemplateUserRoles(ctx, id)
}
