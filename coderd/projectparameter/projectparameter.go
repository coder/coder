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
	ProjectHistoryID   uuid.UUID
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
// These sources are iterated in a hierarchial fashion to determine
// the runtime parameter vaues for a project.
func Compute(ctx context.Context, db database.Store, scope Scope) ([]Value, error) {
	compute := &compute{
		parameterByName:        map[string]Value{},
		projectParameterByName: map[string]database.ProjectParameter{},
	}

	// All parameters for the project version!
	projectHistoryParameters, err := db.GetProjectParametersByHistoryID(ctx, scope.ProjectHistoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.New("no parameters found for history id")
	}
	if err != nil {
		return nil, xerrors.Errorf("get project parameters: %w", err)
	}
	for _, projectParameter := range projectHistoryParameters {
		compute.projectParameterByName[projectParameter.Name] = projectParameter
	}

	// Organization parameters come first!
	organizationParameters, err := db.GetParameterValuesByScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeOrganization,
		ScopeID: scope.OrganizationID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get organization parameters: %w", err)
	}
	err = compute.inject(organizationParameters)
	if err != nil {
		return nil, xerrors.Errorf("inject organization parameters: %w", err)
	}

	// Default project parameter values come second!
	for _, projectParameter := range projectHistoryParameters {
		if !projectParameter.DefaultSourceValue.Valid {
			continue
		}
		if !projectParameter.DefaultDestinationValue.Valid {
			continue
		}

		destinationScheme, err := convertDestinationScheme(projectParameter.DefaultDestinationScheme)
		if err != nil {
			return nil, xerrors.Errorf("convert default destination scheme for project parameter %q: %w", projectParameter.Name, err)
		}

		switch projectParameter.DefaultSourceScheme {
		case database.ParameterSourceSchemeData:
			compute.parameterByName[projectParameter.Name] = Value{
				Proto: &proto.ParameterValue{
					DestinationScheme: destinationScheme,
					Name:              projectParameter.DefaultDestinationValue.String,
					Value:             projectParameter.DefaultSourceValue.String,
				},
				DefaultValue: true,
				Scope:        database.ParameterScopeProject,
				ScopeID:      scope.ProjectID.String(),
			}
		default:
			return nil, xerrors.Errorf("unsupported source scheme for project parameter %q: %q", projectParameter.Name, string(projectParameter.DefaultSourceScheme))
		}
	}

	// Project parameters come third!
	projectParameters, err := db.GetParameterValuesByScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeProject,
		ScopeID: scope.ProjectID.String(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get project parameters: %w", err)
	}
	err = compute.inject(projectParameters)
	if err != nil {
		return nil, xerrors.Errorf("inject project parameters: %w", err)
	}

	// User parameters come fourth!
	userParameters, err := db.GetParameterValuesByScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeUser,
		ScopeID: scope.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get user parameters: %w", err)
	}
	err = compute.inject(userParameters)
	if err != nil {
		return nil, xerrors.Errorf("inject user parameters: %w", err)
	}

	// Workspace parameters come last!
	workspaceParameters, err := db.GetParameterValuesByScope(ctx, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeWorkspace,
		ScopeID: scope.WorkspaceID.String(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get workspace parameters: %w", err)
	}
	err = compute.inject(workspaceParameters)
	if err != nil {
		return nil, xerrors.Errorf("inject workspace parameters: %w", err)
	}

	for _, projectParameter := range compute.projectParameterByName {
		if _, ok := compute.parameterByName[projectParameter.Name]; ok {
			continue
		}
		return nil, NoValueError{
			ParameterID:   projectParameter.ID,
			ParameterName: projectParameter.Name,
		}
	}

	values := make([]Value, 0, len(compute.parameterByName))
	for _, value := range compute.parameterByName {
		values = append(values, value)
	}
	return values, nil
}

type compute struct {
	parameterByName        map[string]Value
	projectParameterByName map[string]database.ProjectParameter
}

// Validates and computes the value for parameters; setting the value on "parameterByName".
func (c *compute) inject(scopedParameters []database.ParameterValue) error {
	for _, scopedParameter := range scopedParameters {
		projectParameter, hasProjectParameter := c.projectParameterByName[scopedParameter.Name]
		if !hasProjectParameter {
			// Don't inject parameters that aren't defined by the project.
			continue
		}

		_, hasExistingParameter := c.parameterByName[scopedParameter.Name]
		if hasExistingParameter {
			// If a parameter already exists, check if this variable can override it.
			// Injection hierarchy is the responsibility of the caller. This check ensures
			// project parameters cannot be overridden if already set.
			if !projectParameter.AllowOverrideSource && scopedParameter.Scope != database.ParameterScopeProject {
				continue
			}
		}

		destinationScheme, err := convertDestinationScheme(scopedParameter.DestinationScheme)
		if err != nil {
			return xerrors.Errorf("convert destination scheme: %w", err)
		}

		switch scopedParameter.SourceScheme {
		case database.ParameterSourceSchemeData:
			c.parameterByName[projectParameter.Name] = Value{
				Proto: &proto.ParameterValue{
					DestinationScheme: destinationScheme,
					Name:              scopedParameter.SourceValue,
					Value:             scopedParameter.DestinationValue,
				},
			}
		default:
			return xerrors.Errorf("unsupported source scheme: %q", string(projectParameter.DefaultSourceScheme))
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
