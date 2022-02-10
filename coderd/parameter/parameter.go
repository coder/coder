package parameter

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
)

// Scope targets identifiers to pull parameters from.
type Scope struct {
	ImportJobID    uuid.UUID
	OrganizationID string
	UserID         string
	ProjectID      uuid.NullUUID
	WorkspaceID    uuid.NullUUID
}

type Value struct {
	DestinationScheme database.ParameterDestinationScheme
	Name              string
	Value             string
	// Default is whether the default value from the schema
	// was consumed.
	Default bool
	Scope   database.ParameterScope
	ScopeID string
}

type Computed struct {
	Schema database.ParameterSchema
	// Value is nil when no value was computed for the schema.
	Value *Value
}

// Compute accepts a scope in which parameter values are sourced.
// These sources are iterated in a hierarchical fashion to determine
// the runtime parameter values for a project.
func Compute(ctx context.Context, db database.Store, scope Scope, additional ...database.ParameterValue) ([]Computed, error) {
	compute := &compute{
		db:               db,
		parametersByName: map[string]Computed{},
	}

	// All parameters for the import job ID!
	parameterSchemas, err := db.GetParameterSchemasByJobID(ctx, scope.ImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get project parameters: %w", err)
	}
	for _, parameterSchema := range parameterSchemas {
		compute.parametersByName[parameterSchema.Name] = Computed{
			Schema: parameterSchema,
		}
	}

	// Organization parameters come first!
	err = compute.injectScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeOrganization,
		ScopeID: scope.OrganizationID,
	})
	if err != nil {
		return nil, err
	}

	// Default project parameter values come second!
	for _, parameterSchema := range parameterSchemas {
		if !parameterSchema.DefaultSourceValue.Valid {
			continue
		}
		if !parameterSchema.DefaultDestinationValue.Valid {
			continue
		}

		switch parameterSchema.DefaultSourceScheme {
		case database.ParameterSourceSchemeData:
			compute.parametersByName[parameterSchema.Name] = Computed{
				Value: &Value{
					DestinationScheme: parameterSchema.DefaultDestinationScheme,
					Name:              parameterSchema.DefaultDestinationValue.String,
					Value:             parameterSchema.DefaultSourceValue.String,
					Default:           true,
					Scope:             database.ParameterScopeProject,
					ScopeID:           scope.ProjectID.UUID.String(),
				},
				Schema: parameterSchema,
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

	for _, parameterValue := range additional {
		err = compute.injectSingle(parameterValue)
		if err != nil {
			return nil, xerrors.Errorf("inject %q: %w", parameterValue.Name, err)
		}
	}

	values := make([]Computed, 0, len(compute.parametersByName))
	for _, value := range compute.parametersByName {
		values = append(values, value)
	}
	return values, nil
}

type compute struct {
	db               database.Store
	parametersByName map[string]Computed
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
		err = c.injectSingle(scopedParameter)
		if err != nil {
			return xerrors.Errorf("inject single %q: %w", scopedParameter.Name, err)
		}
	}
	return nil
}

func (c *compute) injectSingle(scopedParameter database.ParameterValue) error {
	computed, hasComputed := c.parametersByName[scopedParameter.Name]
	if !hasComputed {
		// Don't inject parameters that aren't defined by the project.
		return nil
	}

	if computed.Value != nil {
		// If a parameter already exists, check if this variable can override it.
		// Injection hierarchy is the responsibility of the caller. This check ensures
		// project parameters cannot be overridden if already set.
		if !computed.Schema.AllowOverrideSource && scopedParameter.Scope != database.ParameterScopeProject {
			return nil
		}
	}

	switch scopedParameter.SourceScheme {
	case database.ParameterSourceSchemeData:
		computed.Value = &Value{
			DestinationScheme: scopedParameter.DestinationScheme,
			Name:              scopedParameter.SourceValue,
			Value:             scopedParameter.DestinationValue,
			Scope:             scopedParameter.Scope,
			ScopeID:           scopedParameter.ScopeID,
			Default:           false,
		}
		c.parametersByName[scopedParameter.Name] = computed
	default:
		return xerrors.Errorf("unsupported source scheme: %q", string(computed.Schema.DefaultSourceScheme))
	}
	return nil
}
