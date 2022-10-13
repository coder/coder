package parameter

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

// ComputeScope targets identifiers to pull parameters from.
type ComputeScope struct {
	TemplateImportJobID       uuid.UUID
	TemplateID                uuid.NullUUID
	WorkspaceID               uuid.NullUUID
	AdditionalParameterValues []database.ParameterValue
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
	index              int32     // Track parameter schema index for sorting.
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
	parameterSchemas, err := db.GetParameterSchemasByJobID(ctx, scope.TemplateImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get template parameters: %w", err)
	}
	for _, parameterSchema := range parameterSchemas {
		compute.parameterSchemasByName[parameterSchema.Name] = parameterSchema
	}

	// Job parameters come second!
	err = compute.injectScope(ctx, database.ParameterValuesParams{
		Scopes:   []database.ParameterScope{database.ParameterScopeImportJob},
		ScopeIds: []uuid.UUID{scope.TemplateImportJobID},
	})
	if err != nil {
		return nil, err
	}

	// Default template parameter values come second!
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
				ScopeID:           scope.TemplateImportJobID,
			}, true)
			if err != nil {
				return nil, xerrors.Errorf("insert default value: %w", err)
			}
		default:
			return nil, xerrors.Errorf("unsupported source scheme for template version parameter %q: %q", parameterSchema.Name, string(parameterSchema.DefaultSourceScheme))
		}
	}

	if scope.TemplateID.Valid {
		// Template parameters come third!
		err = compute.injectScope(ctx, database.ParameterValuesParams{
			Scopes:   []database.ParameterScope{database.ParameterScopeTemplate},
			ScopeIds: []uuid.UUID{scope.TemplateID.UUID},
		})
		if err != nil {
			return nil, err
		}
	}

	if scope.WorkspaceID.Valid {
		// Workspace parameters come last!
		err = compute.injectScope(ctx, database.ParameterValuesParams{
			Scopes:   []database.ParameterScope{database.ParameterScopeWorkspace},
			ScopeIds: []uuid.UUID{scope.WorkspaceID.UUID},
		})
		if err != nil {
			return nil, err
		}
	}

	// Finally, any additional parameter values declared in the input
	for _, v := range scope.AdditionalParameterValues {
		err = compute.injectSingle(v, false)
		if err != nil {
			return nil, xerrors.Errorf("inject single parameter value: %w", err)
		}
	}

	values := make([]ComputedValue, 0, len(compute.computedParameterByName))
	for _, value := range compute.computedParameterByName {
		values = append(values, value)
	}
	slices.SortFunc(values, func(a, b ComputedValue) bool {
		return a.index < b.index
	})
	return values, nil
}

type compute struct {
	options                 *ComputeOptions
	db                      database.Store
	computedParameterByName map[string]ComputedValue
	parameterSchemasByName  map[string]database.ParameterSchema
}

// Validates and computes the value for parameters; setting the value on "parameterByName".
func (c *compute) injectScope(ctx context.Context, scopeParams database.ParameterValuesParams) error {
	scopedParameters, err := c.db.ParameterValues(ctx, scopeParams)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return xerrors.Errorf("get %s parameters: %w", scopeParams.Scopes, err)
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
		// Don't inject parameters that aren't defined by the template.
		return nil
	}

	_, hasParameterValue := c.computedParameterByName[scopedParameter.Name]
	if hasParameterValue {
		if !parameterSchema.AllowOverrideSource &&
			// Workspaces cannot override anything on a template!
			scopedParameter.Scope == database.ParameterScopeWorkspace {
			return nil
		}
	}

	switch scopedParameter.SourceScheme {
	case database.ParameterSourceSchemeData:
		value := ComputedValue{
			ParameterValue:     scopedParameter,
			SchemaID:           parameterSchema.ID,
			DefaultSourceValue: defaultValue,
			index:              parameterSchema.Index,
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
