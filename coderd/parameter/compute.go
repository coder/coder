package parameter

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
)

// ComputeOptions provides options to customize the response of a compute operation.
type ComputeOptions struct {
	Schemas []database.ParameterSchema

	OrganizationID string
	ProvisionJobID uuid.UUID
	ProjectID      uuid.NullUUID
	UserID         string
	WorkspaceID    uuid.NullUUID

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
func Compute(ctx context.Context, db database.Store, options ComputeOptions) ([]ComputedValue, error) {
	compute := &compute{
		options:                 options,
		db:                      db,
		computedParameterByName: map[string]ComputedValue{},
		parameterSchemasByName:  map[string]database.ParameterSchema{},
	}
	for _, parameterSchema := range options.Schemas {
		compute.parameterSchemasByName[parameterSchema.Name] = parameterSchema
	}

	// Organization parameters come first!
	err := compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeOrganization,
		ScopeID: options.OrganizationID,
	})
	if err != nil {
		return nil, err
	}

	// Job parameters!
	err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeProvisionerJob,
		ScopeID: options.ProvisionJobID.String(),
	})
	if err != nil {
		return nil, err
	}

	for _, parameterSchema := range options.Schemas {
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
				Scope:             database.ParameterScopeProvisionerJob,
				ScopeID:           options.ProvisionJobID.String(),
			}, true)
			if err != nil {
				return nil, xerrors.Errorf("insert default value: %w", err)
			}
		default:
			return nil, xerrors.Errorf("unsupported source scheme for project version parameter %q: %q", parameterSchema.Name, string(parameterSchema.DefaultSourceScheme))
		}
	}

	if options.ProjectID.Valid {
		err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeProject,
			ScopeID: options.ProjectID.UUID.String(),
		})
		if err != nil {
			return nil, err
		}
	}

	err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeUser,
		ScopeID: options.UserID,
	})
	if err != nil {
		return nil, err
	}

	if options.WorkspaceID.Valid {
		err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeWorkspace,
			ScopeID: options.WorkspaceID.UUID.String(),
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
	options                 ComputeOptions
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
	// Namespaced variables are stored inside a provision job scope.
	if scopedParameter.Scope != database.ParameterScopeProvisionerJob {
		if strings.HasPrefix(scopedParameter.Name, Namespace) {
			return xerrors.Errorf("parameter %q in %q starts with %q; this is a reserved namespace",
				scopedParameter.Name,
				string(scopedParameter.Scope),
				Namespace)
		}
	}

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
