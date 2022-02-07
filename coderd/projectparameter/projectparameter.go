package projectparameter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionersdk/proto"
)

// Scope targets identifiers to pull parameters from.
type Scope struct {
	OrganizationID     string
	ProjectID          uuid.UUID
	ProjectVersionID   uuid.UUID
	UserID             string
	WorkspaceID        uuid.UUID
	WorkspaceHistoryID uuid.UUID
}

// Value represents a computed parameter.
type Value struct {
	Proto *proto.ParameterValue
	// DefaultValue is whether a default value for the scope
	// was consumed. This can only be true for projects.
	DefaultValue bool
	Scope        database.ParameterScope
	ScopeID      string
}

// Compute accepts a scope in which parameter values are sourced.
// These sources are iterated in a hierarchical fashion to determine
// the runtime parameter values for a project.
func Compute(ctx context.Context, db database.Store, scope Scope) ([]Value, error) {
	compute := &compute{
		db:                             db,
		computedParameterByName:        map[string]Value{},
		projectVersionParametersByName: map[string]database.ProjectVersionParameter{},
	}

	// All parameters for the project version!
	projectVersionParameters, err := db.GetProjectVersionParametersByVersionID(ctx, scope.ProjectVersionID)
	if errors.Is(err, sql.ErrNoRows) {
		// This occurs when the project version has defined
		// no parameters, so we have nothing to compute!
		return []Value{}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get project parameters: %w", err)
	}
	for _, projectVersionParameter := range projectVersionParameters {
		compute.projectVersionParametersByName[projectVersionParameter.Name] = projectVersionParameter
	}

	// Organization parameters come first!
	err = compute.inject(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeOrganization,
		ScopeID: scope.OrganizationID,
	})
	if err != nil {
		return nil, err
	}

	// Default project parameter values come second!
	for _, projectVersionParameter := range projectVersionParameters {
		if !projectVersionParameter.DefaultSourceValue.Valid {
			continue
		}
		if !projectVersionParameter.DefaultDestinationValue.Valid {
			continue
		}

		destinationScheme, err := convertDestinationScheme(projectVersionParameter.DefaultDestinationScheme)
		if err != nil {
			return nil, xerrors.Errorf("convert default destination scheme for project version parameter %q: %w", projectVersionParameter.Name, err)
		}

		switch projectVersionParameter.DefaultSourceScheme {
		case database.ParameterSourceSchemeData:
			compute.computedParameterByName[projectVersionParameter.Name] = Value{
				Proto: &proto.ParameterValue{
					DestinationScheme: destinationScheme,
					Name:              projectVersionParameter.DefaultDestinationValue.String,
					Value:             projectVersionParameter.DefaultSourceValue.String,
				},
				DefaultValue: true,
				Scope:        database.ParameterScopeProject,
				ScopeID:      scope.ProjectID.String(),
			}
		default:
			return nil, xerrors.Errorf("unsupported source scheme for project version parameter %q: %q", projectVersionParameter.Name, string(projectVersionParameter.DefaultSourceScheme))
		}
	}

	// Project parameters come third!
	err = compute.inject(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeProject,
		ScopeID: scope.ProjectID.String(),
	})
	if err != nil {
		return nil, err
	}

	// User parameters come fourth!
	err = compute.inject(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeUser,
		ScopeID: scope.UserID,
	})
	if err != nil {
		return nil, err
	}

	// Workspace parameters come last!
	err = compute.inject(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeWorkspace,
		ScopeID: scope.WorkspaceID.String(),
	})
	if err != nil {
		return nil, err
	}

	for _, projectVersionParameter := range compute.projectVersionParametersByName {
		if _, ok := compute.computedParameterByName[projectVersionParameter.Name]; ok {
			continue
		}
		return nil, NoValueError{
			ParameterID:   projectVersionParameter.ID,
			ParameterName: projectVersionParameter.Name,
		}
	}

	values := make([]Value, 0, len(compute.computedParameterByName))
	for _, value := range compute.computedParameterByName {
		values = append(values, value)
	}
	return values, nil
}

type compute struct {
	db                             database.Store
	computedParameterByName        map[string]Value
	projectVersionParametersByName map[string]database.ProjectVersionParameter
}

// Validates and computes the value for parameters; setting the value on "parameterByName".
func (c *compute) inject(ctx context.Context, scopeParams database.GetParameterValuesByScopeParams) error {
	scopedParameters, err := c.db.GetParameterValuesByScope(ctx, scopeParams)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return xerrors.Errorf("get %s parameters: %w", scopeParams.Scope, err)
	}

	for _, scopedParameter := range scopedParameters {
		projectVersionParameter, hasProjectVersionParameter := c.projectVersionParametersByName[scopedParameter.Name]
		if !hasProjectVersionParameter {
			// Don't inject parameters that aren't defined by the project.
			continue
		}

		_, hasExistingParameter := c.computedParameterByName[scopedParameter.Name]
		if hasExistingParameter {
			// If a parameter already exists, check if this variable can override it.
			// Injection hierarchy is the responsibility of the caller. This check ensures
			// project parameters cannot be overridden if already set.
			if !projectVersionParameter.AllowOverrideSource && scopedParameter.Scope != database.ParameterScopeProject {
				continue
			}
		}

		destinationScheme, err := convertDestinationScheme(scopedParameter.DestinationScheme)
		if err != nil {
			return xerrors.Errorf("convert destination scheme: %w", err)
		}

		switch scopedParameter.SourceScheme {
		case database.ParameterSourceSchemeData:
			c.computedParameterByName[projectVersionParameter.Name] = Value{
				Proto: &proto.ParameterValue{
					DestinationScheme: destinationScheme,
					Name:              scopedParameter.SourceValue,
					Value:             scopedParameter.DestinationValue,
				},
			}
		default:
			return xerrors.Errorf("unsupported source scheme: %q", string(projectVersionParameter.DefaultSourceScheme))
		}
	}
	return nil
}

// Converts the database destination scheme to the protobuf version.
func convertDestinationScheme(scheme database.ParameterDestinationScheme) (proto.ParameterDestination_Scheme, error) {
	switch scheme {
	case database.ParameterDestinationSchemeEnvironmentVariable:
		return proto.ParameterDestination_ENVIRONMENT_VARIABLE, nil
	case database.ParameterDestinationSchemeProvisionerVariable:
		return proto.ParameterDestination_PROVISIONER_VARIABLE, nil
	default:
		return 0, xerrors.Errorf("unsupported destination scheme: %q", scheme)
	}
}

type NoValueError struct {
	ParameterID   uuid.UUID
	ParameterName string
}

func (e NoValueError) Error() string {
	return fmt.Sprintf("no value for parameter %q found", e.ParameterName)
}
