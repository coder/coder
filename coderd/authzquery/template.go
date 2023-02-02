package authzquery

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetPreviousTemplateVersion(ctx context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
	// An actor can read the previous template version if they can read the related template.
	fetchRelated := func(_ database.TemplateVersion, _ database.GetPreviousTemplateVersionParams) (rbac.Objecter, error) {
		if !arg.TemplateID.Valid {
			// If no linked template exists, check if the actor can read the template in the organization.
			return rbac.ResourceTemplate.InOrg(arg.OrganizationID), nil
		}
		return q.database.GetTemplateByID(ctx, arg.TemplateID.UUID)
	}
	return authorizedQueryWithRelated(q.logger, q.authorizer, rbac.ActionRead, fetchRelated, q.database.GetPreviousTemplateVersion)(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	// An actor can read the average build time if they can read the related template.
	fetchRelated := func(database.GetTemplateAverageBuildTimeRow, database.GetTemplateAverageBuildTimeParams) (rbac.Objecter, error) {
		if !arg.TemplateID.Valid {
			// If no linked template exists, check if the actor can read *a* template.
			// We don't know the organization ID.
			return rbac.ResourceTemplate, nil
		}
		return q.database.GetTemplateByID(ctx, arg.TemplateID.UUID)
	}
	return authorizedQueryWithRelated(q.logger, q.authorizer, rbac.ActionRead, fetchRelated, q.database.GetTemplateAverageBuildTime)(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetTemplateByID)(ctx, id)
}

func (q *AuthzQuerier) GetTemplateByOrganizationAndName(ctx context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetTemplateByOrganizationAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateDAUs(ctx context.Context, templateID uuid.UUID) ([]database.GetTemplateDAUsRow, error) {
	// An actor can read the DAUs if they can read the related template.
	fetchRelated := func(_ []database.GetTemplateDAUsRow, _ uuid.UUID) (rbac.Objecter, error) {
		return q.database.GetTemplateByID(ctx, templateID)
	}
	return authorizedQueryWithRelated(q.logger, q.authorizer, rbac.ActionRead, fetchRelated, q.database.GetTemplateDAUs)(ctx, templateID)
}

func (q *AuthzQuerier) GetTemplateVersionByID(ctx context.Context, tvid uuid.UUID) (database.TemplateVersion, error) {
	// An actor can read the template version if they can read the related template.
	fetchRelated := func(tv database.TemplateVersion, _ uuid.UUID) (rbac.Objecter, error) {
		if !tv.TemplateID.Valid {
			// If no linked template exists, check if the actor can read a template
			// in the organization.
			return rbac.ResourceTemplate.InOrg(tv.OrganizationID), nil
		}
		return q.database.GetTemplateByID(ctx, tv.TemplateID.UUID)
	}
	return authorizedQueryWithRelated(
		q.logger,
		q.authorizer,
		rbac.ActionRead,
		fetchRelated,
		q.database.GetTemplateVersionByID,
	)(ctx, tvid)
}

func (q *AuthzQuerier) GetTemplateVersionByJobID(ctx context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	// An actor can read the template version if they can read the related template.
	fetchRelated := func(tv database.TemplateVersion, _ uuid.UUID) (rbac.Objecter, error) {
		if !tv.TemplateID.Valid {
			// If no linked template exists, check if the actor can read a
			// template in the organization.
			return rbac.ResourceTemplate.InOrg(tv.OrganizationID), nil
		}
		return q.database.GetTemplateByID(ctx, tv.TemplateID.UUID)
	}
	return authorizedQueryWithRelated(
		q.logger,
		q.authorizer,
		rbac.ActionRead,
		fetchRelated,
		q.database.GetTemplateVersionByJobID,
	)(ctx, jobID)
}

func (q *AuthzQuerier) GetTemplateVersionByOrganizationAndName(ctx context.Context, arg database.GetTemplateVersionByOrganizationAndNameParams) (database.TemplateVersion, error) {
	// An actor can read the template version if they can read the related template in the organization.
	fetchRelated := func(tv database.TemplateVersion, p database.GetTemplateVersionByOrganizationAndNameParams) (rbac.Objecter, error) {
		if !tv.TemplateID.Valid {
			// If no linked template exists, check if the actor can read
			// any template in the organization.
			return rbac.ResourceTemplate.InOrg(p.OrganizationID), nil
		}
		return q.database.GetTemplateByID(ctx, tv.TemplateID.UUID)
	}

	return authorizedQueryWithRelated(
		q.logger,
		q.authorizer,
		rbac.ActionRead,
		fetchRelated,
		q.database.GetTemplateVersionByOrganizationAndName,
	)(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateVersionByTemplateIDAndName(ctx context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	// An actor can read the template version if they can read the related template.
	fetchRelated := func(tv database.TemplateVersion, p database.GetTemplateVersionByTemplateIDAndNameParams) (rbac.Objecter, error) {
		if !tv.TemplateID.Valid {
			// If no linked template exists, check if the actor can read *a* template.
			// We don't know the organization ID.
			return rbac.ResourceTemplate, nil
		}
		return q.database.GetTemplateByID(ctx, tv.TemplateID.UUID)
	}

	return authorizedQueryWithRelated(
		q.logger,
		q.authorizer,
		rbac.ActionRead,
		fetchRelated,
		q.database.GetTemplateVersionByTemplateIDAndName,
	)(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateVersionParameters(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	// An actor can read template version parameters if they can read the related template.
	tv, err := q.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	var object rbac.Objecter
	template, err := q.database.GetTemplateByID(ctx, tv.TemplateID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		object = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		object = tv.RBACObject(template)
	}

	if err := q.authorizeContext(ctx, rbac.ActionRead, object); err != nil {
		return nil, err
	}
	return q.database.GetTemplateVersionParameters(ctx, templateVersionID)
}

func (q *AuthzQuerier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	// TODO: This is so inefficient
	versions, err := q.database.GetTemplateVersionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	checked := make(map[uuid.UUID]bool)
	for _, v := range versions {
		if _, ok := checked[v.TemplateID.UUID]; ok {
			continue
		}

		obj := v.RBACObjectNoTemplate()
		template, err := q.database.GetTemplateByID(ctx, v.TemplateID.UUID)
		if err == nil {
			obj = v.RBACObject(template)
		}
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err := q.authorizeContext(ctx, rbac.ActionRead, obj); err != nil {
			return nil, err
		}
		checked[v.TemplateID.UUID] = true
	}

	return versions, nil
}

func (q *AuthzQuerier) GetTemplateVersionsByTemplateID(ctx context.Context, arg database.GetTemplateVersionsByTemplateIDParams) ([]database.TemplateVersion, error) {
	// An actor can read template versions if they can read the related template.
	template, err := q.database.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return nil, err
	}

	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}

	return q.database.GetTemplateVersionsByTemplateID(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateVersionsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.TemplateVersion, error) {
	// An actor can read execute this query if they can read all templates.
	fetchRelated := func(tvs []database.TemplateVersion, _ time.Time) (rbac.Objecter, error) {
		return rbac.ResourceTemplate.All(), nil
	}
	return authorizedQueryWithRelated(
		q.logger,
		q.authorizer,
		rbac.ActionRead,
		fetchRelated,
		q.database.GetTemplateVersionsCreatedAfter,
	)(ctx, createdAt)
}

func (q *AuthzQuerier) GetAuthorizedTemplates(ctx context.Context, _ database.GetTemplatesWithFilterParams, _ rbac.PreparedAuthorized) ([]database.Template, error) {
	// TODO Delete this function, all GetTemplates should be authorized. For now just call getTemplates on the authz querier.
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
	obj := rbac.ResourceTemplate.InOrg(arg.OrganizationID)
	return authorizedInsertWithReturn(q.logger, q.authorizer, rbac.ActionCreate, obj, q.database.InsertTemplate)(ctx, arg)
}

func (q *AuthzQuerier) InsertTemplateVersion(ctx context.Context, arg database.InsertTemplateVersionParams) (database.TemplateVersion, error) {
	if !arg.TemplateID.Valid {
		// Making a new template version is the same permission as creating a new template.
		err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(arg.OrganizationID))
		if err != nil {
			return database.TemplateVersion{}, err
		}
	} else {
		// Must do an authorized fetch to prevent leaking template ids this way.
		tpl, err := q.GetTemplateByID(ctx, arg.TemplateID.UUID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		// Check the create permission on the template.
		err = q.authorizeContext(ctx, rbac.ActionCreate, tpl)
		if err != nil {
			return database.TemplateVersion{}, err
		}
	}

	return q.database.InsertTemplateVersion(ctx, arg)
}

func (q *AuthzQuerier) UpdateTemplateACLByID(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
	// UpdateTemplateACL uses the ActionCreate action. Only users that can create the template
	// may update the ACL.
	fetch := func(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
		return q.database.GetTemplateByID(ctx, arg.ID)
	}
	return authorizedFetchAndQuery(q.logger, q.authorizer, rbac.ActionCreate, fetch, q.database.UpdateTemplateACLByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateTemplateActiveVersionByID(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) (database.Template, error) {
		return q.database.GetTemplateByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.logger, q.authorizer, fetch, q.database.UpdateTemplateActiveVersionByID)(ctx, arg)
}

func (q *AuthzQuerier) SoftDeleteTemplateByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.database.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:        id,
			Deleted:   true,
			UpdatedAt: database.Now(),
		})
	}
	return authorizedDelete(q.logger, q.authorizer, q.database.GetTemplateByID, deleteF)(ctx, id)
}

// Deprecated: use SoftDeleteTemplateByID instead.
func (q *AuthzQuerier) UpdateTemplateDeletedByID(ctx context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
	return q.SoftDeleteTemplateByID(ctx, arg.ID)
}

func (q *AuthzQuerier) UpdateTemplateMetaByID(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
	fetch := func(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
		return q.database.GetTemplateByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.logger, q.authorizer, fetch, q.database.UpdateTemplateMetaByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateTemplateVersionByID(ctx context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	template, err := q.database.GetTemplateByID(ctx, arg.TemplateID.UUID)
	if err != nil {
		return err
	}
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, template); err != nil {
		return err
	}
	return q.database.UpdateTemplateVersionByID(ctx, arg)
}

func (q *AuthzQuerier) UpdateTemplateVersionDescriptionByJobID(ctx context.Context, arg database.UpdateTemplateVersionDescriptionByJobIDParams) error {
	// An actor is allowed to update the template version description if they are authorized to update the template.
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTemplate.All()); err != nil {
		return err
	}
	return q.database.UpdateTemplateVersionDescriptionByJobID(ctx, arg)
}

func (q *AuthzQuerier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	// An actor is authorized to read template group roles if they are authorized to read the template.
	template, err := q.database.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}
	return q.database.GetTemplateGroupRoles(ctx, id)
}

func (q *AuthzQuerier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	// An actor is authorized to query template user roles if they are authorized to read the template.
	template, err := q.database.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}
	return q.database.GetTemplateUserRoles(ctx, id)
}
