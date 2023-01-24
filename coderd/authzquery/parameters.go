package authzquery

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) parameterRBACResource(ctx context.Context, scope database.ParameterScope, scopeID uuid.UUID) (rbac.Objecter, error) {
	var resource rbac.Objecter
	var err error
	switch scope {
	case database.ParameterScopeWorkspace:
		resource, err = q.database.GetWorkspaceByID(ctx, scopeID)
	case database.ParameterScopeImportJob:
		var version database.TemplateVersion
		version, err = q.database.GetTemplateVersionByJobID(ctx, scopeID)
		if err != nil {
			break
		}
		var template database.Template
		template, err = q.database.GetTemplateByID(ctx, version.TemplateID.UUID)
		if err != nil {
			break
		}
		resource = version.RBACObject(template)

	case database.ParameterScopeTemplate:
		resource, err = q.database.GetTemplateByID(ctx, scopeID)
	default:
		err = xerrors.Errorf("Parameter scope %q unsupported", scope)
	}

	if err != nil {
		return nil, err
	}
	return resource, nil
}

func (q *AuthzQuerier) InsertParameterValue(ctx context.Context, arg database.InsertParameterValueParams) (database.ParameterValue, error) {
	resource, err := q.parameterRBACResource(ctx, arg.Scope, arg.ScopeID)
	if err != nil {
		return database.ParameterValue{}, err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, resource)
	if err != nil {
		return database.ParameterValue{}, err
	}

	return q.InsertParameterValue(ctx, arg)
}

func (q *AuthzQuerier) ParameterValue(ctx context.Context, id uuid.UUID) (database.ParameterValue, error) {
	parameter, err := q.ParameterValue(ctx, id)
	if err != nil {
		return database.ParameterValue{}, err
	}

	resource, err := q.parameterRBACResource(ctx, parameter.Scope, parameter.ScopeID)
	if err != nil {
		return database.ParameterValue{}, err
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, resource)
	if err != nil {
		return database.ParameterValue{}, err
	}

	return parameter, nil
}

func (q *AuthzQuerier) ParameterValues(ctx context.Context, arg database.ParameterValuesParams) ([]database.ParameterValue, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterSchemasByJobID(ctx context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	version, err := q.database.GetTemplateVersionByJobID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	object := version.RBACObjectNoTemplate()
	if version.TemplateID.Valid {
		tpl, err := q.database.GetTemplateByID(ctx, version.TemplateID.UUID)
		if err != nil {
			return nil, err
		}
		object = version.RBACObject(tpl)
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, object)
	if err != nil {
		return nil, err
	}
	return q.GetParameterSchemasByJobID(ctx, jobID)
}

func (q *AuthzQuerier) GetParameterValueByScopeAndName(ctx context.Context, arg database.GetParameterValueByScopeAndNameParams) (database.ParameterValue, error) {
	resource, err := q.parameterRBACResource(ctx, arg.Scope, arg.ScopeID)
	if err != nil {
		return database.ParameterValue{}, err
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, resource)
	if err != nil {
		return database.ParameterValue{}, err
	}

	return q.GetParameterValueByScopeAndName(ctx, arg)
}

func (q *AuthzQuerier) DeleteParameterValueByID(ctx context.Context, id uuid.UUID) error {
	parameter, err := q.database.ParameterValue(ctx, id)
	if err != nil {
		return err
	}

	resource, err := q.parameterRBACResource(ctx, parameter.Scope, parameter.ScopeID)
	if err != nil {
		return err
	}

	// A deleted param is still updating the underlying resource for the scope.
	err = q.authorizeContext(ctx, rbac.ActionUpdate, resource)
	if err != nil {
		return err
	}

	return q.DeleteParameterValueByID(ctx, id)
}
