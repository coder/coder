package parameter

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
)

// ComputeScope targets identifiers to pull parameters from.
type ComputeScope struct {
	ProjectImportJobID uuid.UUID
	OrganizationID     string
	UserID             string
	ProjectID          uuid.NullUUID
	WorkspaceID        uuid.NullUUID
}

type ComputeOptions struct {
	// HideRedisplayValues removes the value from parameters that
	// come from schemas with RedisplayValue set to false.
	HideRedisplayValues bool
}

// ComputedValue represents a computed parameter value.
type ComputedValue struct {
	database.ParameterValue
	SchemaID           uuid.UUID `json:"schema_id"`
	DefaultSourceValue bool      `json:"default_source_value"`
}

// Compute accepts a scope in which parameter values are sourced.
// These sources are iterated in a hierarchical fashion to determine
// the runtime parameter values for schemas provided.
func Compute(ctx context.Context, db database.Store, scope ComputeScope, options *ComputeOptions) ([]ComputedValue, error) {
	if options == nil {
		options = &ComputeOptions{}
	}
	compute := &compute{
		options:                 options,
		db:                      db,
		computedParameterByName: map[string]ComputedValue{},
		parameterSchemasByName:  map[string]database.ParameterSchema{},
	}

	// All parameters for the import job ID!
	parameterSchemas, err := db.GetParameterSchemasByJobID(ctx, scope.ProjectImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get project parameters: %w", err)
	}
	for _, parameterSchema := range parameterSchemas {
		compute.parameterSchemasByName[parameterSchema.Name] = parameterSchema
	}

	// Organization parameters come first!
	err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeOrganization,
		ScopeID: scope.OrganizationID,
	})
	if err != nil {
		return nil, err
	}

	// Job parameters come second!
	err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeImportJob,
		ScopeID: scope.ProjectImportJobID.String(),
	})
	if err != nil {
		return nil, err
	}

	// Default project parameter values come second!
	for _, parameterSchema := range parameterSchemas {
		if parameterSchema.DefaultSourceScheme == database.ParameterSourceSchemeNone {
			continue
		}
		if _, ok := compute.computedParameterByName[parameterSchema.Name]; ok {
			// We already have a value! No need to use the default.
			continue
		}

		switch parameterSchema.DefaultSourceScheme {
		case database.ParameterSourceSchemeData:
			// Inject a default value scoped to the import job ID.
			// This doesn't need to be inserted into the database,
			// because it's a dynamic value associated with the schema.
			err = compute.injectSingle(database.ParameterValue{
				ID:                uuid.New(),
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				SourceScheme:      database.ParameterSourceSchemeData,
				Name:              parameterSchema.Name,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
				SourceValue:       parameterSchema.DefaultSourceValue,
				Scope:             database.ParameterScopeImportJob,
				ScopeID:           scope.ProjectImportJobID.String(),
			}, true)
			if err != nil {
				return nil, xerrors.Errorf("insert default value: %w", err)
			}
		default:
			return nil, xerrors.Errorf("unsupported source scheme for project version parameter %q: %q", parameterSchema.Name, string(parameterSchema.DefaultSourceScheme))
		}
	}

	if scope.ProjectID.Valid {
		// Project parameters come third!
		err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeProject,
			ScopeID: scope.ProjectID.UUID.String(),
		})
		if err != nil {
			return nil, err
		}
	}

	// User parameters come fourth!
	err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeUser,
		ScopeID: scope.UserID,
	})
	if err != nil {
		return nil, err
	}

	if scope.WorkspaceID.Valid {
		// Workspace parameters come last!
		err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeWorkspace,
			ScopeID: scope.WorkspaceID.UUID.String(),
		})
		if err != nil {
			return nil, err
		}
	}

	values := make([]ComputedValue, 0, len(compute.computedParameterByName))
	for _, value := range compute.computedParameterByName {
		values = append(values, value)
	}
	return values, nil
}

type compute struct {
	options                 *ComputeOptions
	db                      database.Store
	computedParameterByName map[string]ComputedValue
	parameterSchemasByName  map[string]database.ParameterSchema
}

// Validates and computes the value for parameters; setting the value on "parameterByName".
func (c *compute) injectScope(ctx context.Context, scopeParams database.GetParameterValuesByScopeParams) error {
	scopedParameters, err := c.db.GetParameterValuesByScope(ctx, scopeParams)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return xerrors.Errorf("get %s parameters: %w", scopeParams.Scope, err)
	}

	for _, scopedParameter := range scopedParameters {
		err = c.injectSingle(scopedParameter, false)
		if err != nil {
			return xerrors.Errorf("inject single %q: %w", scopedParameter.Name, err)
		}
	}
	return nil
}

func (c *compute) injectSingle(scopedParameter database.ParameterValue, defaultValue bool) error {
	parameterSchema, hasParameterSchema := c.parameterSchemasByName[scopedParameter.Name]
	if !hasParameterSchema {
		// Don't inject parameters that aren't defined by the project.
		return nil
	}

	_, hasParameterValue := c.computedParameterByName[scopedParameter.Name]
	if hasParameterValue {
		if !parameterSchema.AllowOverrideSource &&
			// Users and workspaces cannot override anything on a project!
			(scopedParameter.Scope == database.ParameterScopeUser ||
				scopedParameter.Scope == database.ParameterScopeWorkspace) {
			return nil
		}
	}

	switch scopedParameter.SourceScheme {
	case database.ParameterSourceSchemeData:
		value := ComputedValue{
			ParameterValue:     scopedParameter,
			SchemaID:           parameterSchema.ID,
			DefaultSourceValue: defaultValue,
		}
		if c.options.HideRedisplayValues && !parameterSchema.RedisplayValue {
			value.SourceValue = ""
		}
		c.computedParameterByName[scopedParameter.Name] = value
	default:
		return xerrors.Errorf("unsupported source scheme: %q", string(parameterSchema.DefaultSourceScheme))
	}
	return nil
}
